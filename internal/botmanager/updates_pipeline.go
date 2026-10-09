package botmanager

import (
	"context"
	"fmt"
	"time"

	"github.com/gotd/td/tg"
	"go.uber.org/zap"
	"telego-bot-api/internal/converter"
	"telego-bot-api/internal/diag"
)

// queuedUpdates is an MTProto batch waiting for the update worker.
type queuedUpdates struct {
	updates  tg.UpdatesClass
	queuedAt time.Time
}

// updateQueueSize bounds the MTProto update batches buffered per bot before Handle
// applies backpressure to the connection.
const updateQueueSize = 4096

// Handle receives MTProto updates from Telegram.
//
// gotd calls it synchronously from the connection read loop, so any latency here
// (Redis round trips) also delays RPC responses of the bot and makes API calls of
// busy bots appear to hang. Handle therefore only enqueues the batch; a per-bot
// worker persists batches in arrival order.
func (b *BotInstance) Handle(ctx context.Context, u tg.UpdatesClass) error {
	b.touch()
	b.startUpdateWorker()
	item := queuedUpdates{updates: u, queuedAt: time.Now()}
	select {
	case b.updatesCh <- item:
		return nil
	case <-b.updatesQuit:
		return nil
	default:
	}
	// The worker is behind (e.g. Redis is slow): block rather than drop updates.
	b.logger.Warn("Update queue is full, delaying MTProto reads", zap.Int("queue_size", updateQueueSize))
	select {
	case b.updatesCh <- item:
	case <-b.updatesQuit:
	case <-ctx.Done():
	}
	return nil
}

// startUpdateWorker lazily starts the per-bot update worker.
func (b *BotInstance) startUpdateWorker() {
	b.updatesOnce.Do(func() {
		b.updatesCh = make(chan queuedUpdates, updateQueueSize)
		b.updatesQuit = make(chan struct{})
		go b.runUpdateWorker(b.updatesCh, b.updatesQuit)
	})
}

// stopUpdateWorker stops the worker after it persists the batches already queued.
func (b *BotInstance) stopUpdateWorker() {
	b.startUpdateWorker()
	b.updatesStopOnce.Do(func() { close(b.updatesQuit) })
}

func (b *BotInstance) runUpdateWorker(updates <-chan queuedUpdates, quit <-chan struct{}) {
	for {
		select {
		case item := <-updates:
			b.processQueued(item, len(updates))
		case <-quit:
			for {
				select {
				case item := <-updates:
					b.processQueued(item, len(updates))
				default:
					return
				}
			}
		}
	}
}

// processQueued persists one queued batch, reporting batches that waited or ran too long.
// A panic loses only this batch: if it stopped the worker, the queue would fill up and
// Handle would block the MTProto read loop, hanging every request of the bot.
func (b *BotInstance) processQueued(item queuedUpdates, backlog int) {
	defer func() {
		if r := recover(); r != nil {
			b.logger.Error("PANIC recovered in update worker, batch lost", zap.Int64("bot_id", b.BotID()),
				zap.String("type", fmt.Sprintf("%T", item.updates)), zap.Any("panic", r),
				zap.Stack("stack"))
		}
	}()
	if wait := time.Since(item.queuedAt); wait >= diag.SlowThreshold {
		b.logger.Warn("Update queue stalled", zap.Int64("bot_id", b.BotID()),
			zap.Duration("waited", wait), zap.Int("backlog", backlog))
		if wait >= diag.StallThreshold {
			diag.Dump("update queue", zap.Int64("bot_id", b.BotID()))
		}
	}
	done := diag.Watch("update_processing", fmt.Sprintf("%T", item.updates), zap.Int64("bot_id", b.BotID()))
	defer done()
	b.processUpdates(item.updates)
}

// retryRedis runs op until it succeeds, retrying transient Redis failures with backoff
// for roughly 15 seconds, so that a short Redis hiccup does not lose updates.
func (b *BotInstance) retryRedis(what string, op func(ctx context.Context) error) error {
	delay := 100 * time.Millisecond
	var err error
	for attempt := 0; attempt < 8; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err = op(ctx)
		cancel()
		if err == nil {
			return nil
		}
		b.logger.Warn("Redis operation failed, retrying", zap.String("operation", what),
			zap.Int("attempt", attempt+1), zap.Error(err))
		time.Sleep(delay)
		if delay < 2*time.Second {
			delay *= 2
		}
	}
	return err
}

// nextUpdateID allocates the next persistent update_id for the bot.
func (b *BotInstance) nextUpdateID(botID int64) (int, error) {
	var id int
	err := b.retryRedis("next update_id", func(ctx context.Context) error {
		var err error
		id, err = b.redisStore.NextUpdateID(ctx, botID)
		return err
	})
	return id, err
}

// appendUpdate persists an update to the bot's Redis Stream.
func (b *BotInstance) appendUpdate(botID int64, updateID int, payload []byte) error {
	return b.retryRedis("append update", func(ctx context.Context) error {
		return b.redisStore.AppendUpdate(ctx, botID, updateID, payload)
	})
}

// newUpdatesSignal returns a channel that is closed when new updates are persisted.
// Take it before reading the stream, so updates written in between are not missed.
func (b *BotInstance) newUpdatesSignal() <-chan struct{} {
	b.newUpdatesMu.Lock()
	defer b.newUpdatesMu.Unlock()
	if b.newUpdates == nil {
		b.newUpdates = make(chan struct{})
	}
	return b.newUpdates
}

// broadcastNewUpdates wakes all getUpdates calls waiting for new updates.
func (b *BotInstance) broadcastNewUpdates() {
	b.newUpdatesMu.Lock()
	defer b.newUpdatesMu.Unlock()
	if b.newUpdates != nil {
		close(b.newUpdates)
		b.newUpdates = nil
	}
}

// messageUpdateMaxAge matches the official Bot API: messages sent or edited more than
// a day ago are not delivered, e.g. when getDifference catches up after a long downtime.
const messageUpdateMaxAge = 24 * time.Hour

func staleMessageUpdate(upd *converter.Update) bool {
	var msg *converter.Message
	switch {
	case upd.Message != nil:
		msg = upd.Message
	case upd.EditedMessage != nil:
		msg = upd.EditedMessage
	case upd.ChannelPost != nil:
		msg = upd.ChannelPost
	case upd.EditedChannelPost != nil:
		msg = upd.EditedChannelPost
	default:
		return false
	}
	date := msg.Date
	if msg.EditDate != 0 {
		date = msg.EditDate
	}
	return date != 0 && time.Since(time.Unix(int64(date), 0)) > messageUpdateMaxAge
}

// replayedQtsUpdate reports a qts-sequenced update without a qts position. Telegram puts
// bot business updates into every updates.difference with qts 0 and the gap manager
// dispatches them each time (on reconnects, gaps and its 15-minute idle check), so the
// bot would receive business messages it already got. TDLib drops such updates too.
func replayedQtsUpdate(u tg.UpdateClass) bool {
	q, ok := u.(interface{ GetQts() int })
	return ok && q.GetQts() == 0
}
