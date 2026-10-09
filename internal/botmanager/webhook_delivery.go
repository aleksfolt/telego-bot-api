package botmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"telego-bot-api/internal/converter"
	"telego-bot-api/internal/webhook"
)

const (
	// defaultWebhookConnections and maxWebhookConnections mirror setWebhook max_connections.
	defaultWebhookConnections = 40
	maxWebhookConnections     = 100

	// webhookMaxLoaded bounds the pending updates one bot keeps in memory for delivery.
	// Further stream entries are loaded as loaded ones are delivered.
	webhookMaxLoaded = 1000

	// webhookUpdateTTL matches the official Bot API: an update that could not be
	// delivered for a day is dropped, so a broken handler does not block its chat forever.
	webhookUpdateTTL = 24 * time.Hour

	webhookMaxRetryDelay = 30 * time.Second
)

// normalizeWebhookConnections applies the setWebhook max_connections default and bounds.
func normalizeWebhookConnections(n int) int {
	if n <= 0 {
		return defaultWebhookConnections
	}
	return min(n, maxWebhookConnections)
}

func (b *BotInstance) restoreWebhookConfig(ctx context.Context) {
	// Restore webhook configuration from Redis if present
	if wh, err := b.redisStore.GetWebhook(ctx, b.token); err == nil && wh != nil {
		b.mu.Lock()
		b.webhookURL = wh.URL
		b.secretToken = wh.SecretToken
		b.webhookMaxConns = wh.MaxConnections
		b.mu.Unlock()
		b.logger.Info("Restored webhook from Redis", zap.String("url", wh.URL))
	}

}

// webhookMu serializes configuration changes and the replay loop's lifetime.
func (b *BotInstance) restartWebhookDelivery() {
	b.webhookMu.Lock()
	defer b.webhookMu.Unlock()
	b.stopWebhookDeliveryLocked()
	b.startWebhookDeliveryLocked()
}

func (b *BotInstance) startWebhookDeliveryLocked() {
	b.mu.RLock()
	url, secret, botID, maxConns := b.webhookURL, b.secretToken, b.botID, b.webhookMaxConns
	b.mu.RUnlock()
	if b.webhookStopped || url == "" || botID == 0 || b.dispatcher == nil || b.redisStore == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	b.webhookCancel = cancel
	b.webhookDone = make(chan struct{})
	b.webhookWake = make(chan struct{}, 1)
	d := &webhookDelivery{
		bot: b, botID: botID, url: url, secret: secret,
		maxConns: normalizeWebhookConnections(maxConns),
		queues:   make(map[string][]*pendingWebhook),
		byID:     make(map[string]*pendingWebhook),
		tail:     "0-0",
	}
	go d.run(ctx, b.webhookWake, b.webhookDone)
}

func (b *BotInstance) stopWebhookDeliveryLocked() {
	if b.webhookCancel != nil {
		b.webhookCancel()
		<-b.webhookDone
		b.webhookCancel = nil
		b.webhookWake = nil
	}
}

func (b *BotInstance) notifyWebhookDelivery() {
	b.webhookMu.Lock()
	defer b.webhookMu.Unlock()
	if b.webhookWake != nil {
		select {
		case b.webhookWake <- struct{}{}:
		default:
		}
	}
}

type webhookResult struct {
	streamID string
	err      error
}

// pendingWebhook is a stream entry loaded for delivery.
type pendingWebhook struct {
	streamID  string
	queue     string
	update    *converter.Update
	createdAt time.Time

	inFlight  bool
	delivered bool // sent successfully, but removing it from the stream failed
	failures  int
	retryAt   time.Time
}

func (p *pendingWebhook) retry() {
	p.inFlight = false
	p.failures++
	delay := time.Second * time.Duration(1<<min(p.failures-1, 5))
	if delay > webhookMaxRetryDelay {
		delay = webhookMaxRetryDelay
	}
	p.retryAt = time.Now().Add(delay)
}

// webhookDelivery delivers a bot's persisted updates to its webhook. Redis is the
// source of truth: an update leaves the stream only after the application accepted it,
// so failures and restarts never lose it.
//
// Like the official Bot API, updates of one chat form a queue delivered strictly in
// order: the next one is sent only after the previous one was accepted, and a failing
// update delays only its own chat. Updates without a chat are delivered independently.
// Entries are loaded from the stream once and kept in memory, so new updates and
// retries do not re-read the stream.
type webhookDelivery struct {
	bot         *BotInstance
	botID       int64
	url, secret string
	maxConns    int

	queues   map[string][]*pendingWebhook
	byID     map[string]*pendingWebhook
	tail     string // last stream ID loaded
	inFlight int
}

func (d *webhookDelivery) run(ctx context.Context, wake <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	// Every in-flight task reports exactly once, so this buffer never blocks a report,
	// including the synchronous error report of a full dispatcher queue.
	results := make(chan webhookResult, d.maxConns)
	for ctx.Err() == nil {
		d.load(ctx)
		d.dispatch(ctx, results)

		timer := time.NewTimer(d.nextRetryDelay())
		select {
		case <-ctx.Done():
		case <-wake:
		case result := <-results:
			d.complete(ctx, result)
		case <-timer.C:
		}
		timer.Stop()
		// Batch completions before touching Redis again.
		for len(results) > 0 {
			d.complete(ctx, <-results)
		}
	}
}

// load reads stream entries after the last loaded one, up to webhookMaxLoaded in memory.
func (d *webhookDelivery) load(ctx context.Context) {
	for len(d.byID) < webhookMaxLoaded && ctx.Err() == nil {
		readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		entries, err := d.bot.redisStore.ReadUpdatesAfter(readCtx, d.botID, d.tail, min(100, webhookMaxLoaded-len(d.byID)), 0)
		cancel()
		if err != nil {
			d.bot.logger.Warn("Failed to read pending webhooks", zap.Error(err))
			return
		}
		for _, entry := range entries {
			d.tail = entry.StreamID
			var update converter.Update
			if err := json.Unmarshal(entry.Payload, &update); err != nil {
				d.bot.logger.Error("Invalid persisted webhook update", zap.String("stream_id", entry.StreamID), zap.Error(err))
				continue
			}
			p := &pendingWebhook{
				streamID:  entry.StreamID,
				queue:     webhookQueueKey(&update, entry.StreamID),
				update:    &update,
				createdAt: streamEntryTime(entry.StreamID),
			}
			d.queues[p.queue] = append(d.queues[p.queue], p)
			d.byID[p.streamID] = p
		}
		if len(entries) < 100 {
			return
		}
	}
}

// dispatch sends the head of every chat queue that is ready, within max_connections.
func (d *webhookDelivery) dispatch(ctx context.Context, results chan<- webhookResult) {
	now := time.Now()
	for key, queue := range d.queues {
		for len(queue) > 0 && d.inFlight < d.maxConns && ctx.Err() == nil {
			head := queue[0]
			if head.inFlight || now.Before(head.retryAt) {
				break
			}
			if head.delivered || now.Sub(head.createdAt) > webhookUpdateTTL {
				if !head.delivered {
					d.bot.logger.Warn("Dropping webhook update that could not be delivered for a day",
						zap.Int("update_id", head.update.UpdateID), zap.Int("failures", head.failures))
				}
				// An acknowledgement failure must not resend a delivered update.
				if !d.remove(ctx, head) {
					break
				}
				queue = d.queues[key]
				continue
			}
			d.send(ctx, head, results)
			break
		}
		if d.inFlight >= d.maxConns {
			return
		}
	}
}

func (d *webhookDelivery) send(ctx context.Context, p *pendingWebhook, results chan<- webhookResult) {
	p.inFlight = true
	d.inFlight++
	id := p.streamID
	report := func(err error) {
		select {
		case results <- webhookResult{streamID: id, err: err}:
		case <-ctx.Done():
		}
	}
	d.bot.dispatcher.EnqueueTask(&webhook.Task{
		Context: ctx, BotID: d.botID, URL: d.url, SecretToken: d.secret, Update: p.update,
		OnSuccess: func(int64, int) { report(nil) },
		OnError:   func(_ int64, err error) { report(err) },
	})
}

func (d *webhookDelivery) complete(ctx context.Context, result webhookResult) {
	p := d.byID[result.streamID]
	if p == nil || !p.inFlight {
		return
	}
	d.inFlight--
	p.inFlight = false
	if result.err != nil {
		errCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_ = d.bot.redisStore.UpdateWebhookDeliveryError(errCtx, d.bot.token, result.err.Error())
		cancel()
		p.retry()
		return
	}
	p.delivered = true
	d.remove(ctx, p)
}

// remove deletes a delivered or expired head from the stream and from its queue.
func (d *webhookDelivery) remove(ctx context.Context, p *pendingWebhook) bool {
	ackCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	err := d.bot.redisStore.AckUpdate(ackCtx, d.botID, p.streamID)
	cancel()
	if err != nil {
		d.bot.logger.Warn("Failed to remove delivered webhook update", zap.String("stream_id", p.streamID), zap.Error(err))
		p.retry()
		return false
	}
	delete(d.byID, p.streamID)
	queue := d.queues[p.queue]
	if len(queue) > 0 && queue[0] == p {
		queue = queue[1:]
	}
	if len(queue) == 0 {
		delete(d.queues, p.queue)
	} else {
		d.queues[p.queue] = queue
	}
	return true
}

// nextRetryDelay sleeps until the earliest scheduled retry. New updates and completions
// wake the loop earlier; the timer also recovers from Redis failures.
func (d *webhookDelivery) nextRetryDelay() time.Duration {
	delay := webhookMaxRetryDelay
	now := time.Now()
	for _, queue := range d.queues {
		if head := queue[0]; !head.inFlight && head.retryAt.Sub(now) < delay {
			delay = head.retryAt.Sub(now)
		}
	}
	if delay < 10*time.Millisecond {
		delay = 10 * time.Millisecond
	}
	return delay
}

// webhookQueueKey groups updates that must be delivered in order, like webhook_queue_id
// of the official Bot API: messages (including edits and business messages) per chat,
// and chat member, boost and reaction updates per chat. Other updates are independent.
func webhookQueueKey(upd *converter.Update, streamID string) string {
	chatID := updateChatID(upd)
	if chatID != 0 {
		switch {
		case upd.Message != nil, upd.EditedMessage != nil, upd.ChannelPost != nil, upd.EditedChannelPost != nil,
			upd.BusinessMessage != nil, upd.EditedBusinessMessage != nil, upd.DeletedBusinessMessages != nil:
			return fmt.Sprintf("message:%d", chatID)
		case upd.MyChatMember != nil, upd.ChatMember != nil, upd.ChatJoinRequest != nil:
			return fmt.Sprintf("member:%d", chatID)
		case upd.ChatBoost != nil, upd.RemovedChatBoost != nil:
			return fmt.Sprintf("boost:%d", chatID)
		case upd.MessageReaction != nil, upd.MessageReactionCount != nil:
			return fmt.Sprintf("reaction:%d", chatID)
		}
	}
	return "update:" + streamID
}

// streamEntryTime returns when a Redis stream entry was added (its ID starts with
// the Unix time in milliseconds).
func streamEntryTime(streamID string) time.Time {
	ms, _, _ := strings.Cut(streamID, "-")
	n, err := strconv.ParseInt(ms, 10, 64)
	if err != nil {
		return time.Now()
	}
	return time.UnixMilli(n)
}
