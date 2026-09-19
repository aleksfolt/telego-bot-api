package botmanager

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/zap"
	"telego-bot-api/internal/converter"
	"telego-bot-api/internal/webhook"
)

const webhookInFlightLimit = 40

func (b *BotInstance) restoreWebhookConfig(ctx context.Context) {
	// Restore webhook configuration from Redis if present
	if wh, err := b.redisStore.GetWebhook(ctx, b.token); err == nil && wh != nil {
		b.mu.Lock()
		b.webhookURL = wh.URL
		b.secretToken = wh.SecretToken
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
	url, secret, botID := b.webhookURL, b.secretToken, b.botID
	b.mu.RUnlock()
	if b.webhookStopped || url == "" || botID == 0 || b.dispatcher == nil || b.redisStore == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	b.webhookCancel = cancel
	b.webhookDone = make(chan struct{})
	b.webhookWake = make(chan struct{}, 1)
	go b.deliverPendingWebhooks(ctx, botID, url, secret, b.webhookWake, b.webhookDone)
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

type webhookAttempt struct {
	inFlight  bool
	delivered bool
	failures  int
	retryAt   time.Time
}

func (a *webhookAttempt) retry() {
	a.inFlight = false
	a.failures++
	delay := time.Second * time.Duration(1<<min(a.failures-1, 5))
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	a.retryAt = time.Now().Add(delay)
}

// Redis is the source of pending work. Dispatcher tasks are only transient
// attempts; failures and restarts leave the original stream entry available.
func (b *BotInstance) deliverPendingWebhooks(ctx context.Context, botID int64, url, secret string, wake <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	results := make(chan webhookResult, webhookInFlightLimit)
	attempts := make(map[string]*webhookAttempt)
	inFlight := 0
	ack := func(id string) error {
		ackCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		return b.redisStore.AckUpdate(ackCtx, botID, id)
	}
	complete := func(result webhookResult) {
		a := attempts[result.streamID]
		if a == nil || !a.inFlight {
			return
		}
		inFlight--
		a.inFlight = false
		if result.err == nil {
			a.delivered = true
			if err := ack(result.streamID); err == nil {
				delete(attempts, result.streamID)
				return
			}
		} else {
			errCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			_ = b.redisStore.UpdateWebhookDeliveryError(errCtx, b.token, result.err.Error())
			cancel()
		}
		a.retry()
	}
	for ctx.Err() == nil {
		// Scan beyond failed entries so one unavailable event cannot hide later work.
		cursor := "0-0"
		seen := make(map[string]bool)
		for inFlight < webhookInFlightLimit && ctx.Err() == nil {
			readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			entries, err := b.redisStore.ReadUpdatesAfter(readCtx, botID, cursor, 100, 0)
			cancel()
			if err != nil {
				b.logger.Warn("Failed to read pending webhooks", zap.Error(err))
				break
			}
			if len(entries) == 0 {
				// Forget retry metadata for entries removed by queue trimming/polling.
				for id, a := range attempts {
					if !seen[id] && !a.inFlight {
						delete(attempts, id)
					}
				}
				break
			}
			for _, entry := range entries {
				cursor = entry.StreamID
				seen[cursor] = true
				a := attempts[cursor]
				if a != nil && (a.inFlight || time.Now().Before(a.retryAt)) {
					continue
				}
				if a != nil && a.delivered {
					// An acknowledgement failure must not immediately resend a delivered event.
					if err := ack(cursor); err == nil {
						delete(attempts, cursor)
					} else {
						a.retry()
					}
					continue
				}
				var update converter.Update
				if err := json.Unmarshal(entry.Payload, &update); err != nil {
					b.logger.Error("Invalid persisted webhook update", zap.String("stream_id", cursor), zap.Error(err))
					continue
				}
				if a == nil {
					a = &webhookAttempt{}
					attempts[cursor] = a
				}
				a.inFlight = true
				inFlight++
				id := entry.StreamID
				report := func(err error) {
					select {
					case results <- webhookResult{streamID: id, err: err}:
					case <-ctx.Done():
					}
				}
				b.dispatcher.EnqueueTask(&webhook.Task{
					Context: ctx, BotID: botID, URL: url, SecretToken: secret, Update: &update,
					OnSuccess: func(int64, int) { report(nil) },
					OnError:   func(_ int64, err error) { report(err) },
				})
				if inFlight >= webhookInFlightLimit {
					break
				}
			}
		}
		// New persisted updates and completions wake the loop immediately. The
		// timer also recovers from missed notifications and retries Redis failures.
		delay := time.Second
		if len(attempts) == 0 {
			delay = 30 * time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
		case <-wake:
		case result := <-results:
			complete(result)
		case <-timer.C:
		}
		timer.Stop()
		// Batch completions before querying Redis again.
		for len(results) > 0 {
			complete(<-results)
		}
	}
}
