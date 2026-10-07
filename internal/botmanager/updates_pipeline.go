package botmanager

import (
	"context"
	"fmt"
	"time"

	"github.com/gotd/td/tg"
	"go.uber.org/zap"
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
	defer func() {
		if r := recover(); r != nil {
			b.logger.Error("PANIC recovered in update worker", zap.Any("panic", r))
		}
	}()
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
func (b *BotInstance) processQueued(item queuedUpdates, backlog int) {
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
