package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"telego-bot-api/internal/converter"
	"telego-bot-api/internal/logging"
	"telego-bot-api/internal/metrics"

	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

// Dispatcher manages the delivery of Bot API updates to target webhooks.
// Uses an in-memory pooled HTTP pipeline with sub-millisecond dispatch times.
type Dispatcher struct {
	client *fasthttp.Client
	logger *zap.Logger
	queue  chan *Task
	wg     sync.WaitGroup
	quit   chan struct{}
}

// Task represents a pending webhook delivery task.
type Task struct {
	// Context cancels queued attempts when the webhook changes or the bot stops.
	Context     context.Context
	BotID       int64
	URL         string
	SecretToken string
	Update      *converter.Update
	OnSuccess   func(botID int64, updateID int)
	OnError     func(botID int64, err error)
}

// webhookTimeout bounds one webhook request. A response slower than this is retried
// later by the delivery loop, so it must be generous: a short timeout makes a busy
// application receive the same update several times.
const webhookTimeout = 30 * time.Second

// NewDispatcher creates a webhook dispatcher with a dedicated worker pool.
func NewDispatcher(workers int, queueSize int, logger *zap.Logger) *Dispatcher {
	d := &Dispatcher{
		client: &fasthttp.Client{
			MaxConnsPerHost: 10000,
			// Node.js closes idle connections after 5s by default.
			// Keeping MaxIdleConnDuration at 4s ensures fasthttp cleans up
			// before Node.js sends FIN, preventing stale connection resets.
			MaxIdleConnDuration:           4 * time.Second,
			ReadTimeout:                   webhookTimeout,
			WriteTimeout:                  webhookTimeout,
			NoDefaultUserAgentHeader:      true,
			DisableHeaderNamesNormalizing: true,
		},
		logger: logger,
		queue:  make(chan *Task, queueSize),
		quit:   make(chan struct{}),
	}

	for i := 0; i < workers; i++ {
		d.wg.Add(1)
		go d.worker()
	}

	return d
}

// Enqueue adds an update to the delivery queue.
func (d *Dispatcher) Enqueue(url, secretToken string, update *converter.Update) {
	d.EnqueueTask(&Task{
		URL:         url,
		SecretToken: secretToken,
		Update:      update,
	})
}

// EnqueueTask adds a complete delivery task with callbacks to the delivery queue.
func (d *Dispatcher) EnqueueTask(task *Task) {
	select {
	case d.queue <- task:
	default:
		d.logger.Warn("Webhook queue full, dropping update",
			zap.Int64("bot_id", task.BotID),
			zap.Int("update_id", task.Update.UpdateID),
			zap.String("url", task.URL),
		)
		metrics.DefaultRegistry.IncWebhookDelivery("dropped_queue_full")
		if task.OnError != nil {
			task.OnError(task.BotID, errors.New("webhook queue full"))
		}
	}
}

// QueueSize returns the current number of pending items in the delivery queue.
func (d *Dispatcher) QueueSize() int {
	return len(d.queue)
}

// worker runs an HTTP POST loop against target webhooks.
func (d *Dispatcher) worker() {
	defer d.wg.Done()

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	for {
		select {
		case <-d.quit:
			// Drain remaining tasks before shutting down worker
			for {
				select {
				case task := <-d.queue:
					d.processTask(req, resp, task)
				default:
					return
				}
			}
		case task := <-d.queue:
			d.processTask(req, resp, task)
		}
	}
}

func (d *Dispatcher) processTask(req *fasthttp.Request, resp *fasthttp.Response, task *Task) {
	if task.Context != nil && task.Context.Err() != nil {
		return
	}
	if err := d.sendWebhook(req, resp, task); err != nil {
		d.logger.Error("Failed to deliver webhook",
			zap.Int64("bot_id", task.BotID),
			zap.String("url", task.URL),
			zap.Int("update_id", task.Update.UpdateID),
			zap.Error(err),
		)
		metrics.DefaultRegistry.IncWebhookDelivery("error")
		if task.OnError != nil {
			task.OnError(task.BotID, err)
		}
	} else {
		metrics.DefaultRegistry.IncWebhookDelivery("200")
		if task.OnSuccess != nil {
			task.OnSuccess(task.BotID, task.Update.UpdateID)
		}
	}
}

func (d *Dispatcher) sendWebhook(req *fasthttp.Request, resp *fasthttp.Response, task *Task) error {
	body, err := json.Marshal(task.Update)
	if err != nil {
		return fmt.Errorf("marshal update: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if task.Context != nil && task.Context.Err() != nil {
			return task.Context.Err()
		}
		req.Reset()
		resp.Reset()

		req.SetRequestURI(task.URL)
		req.Header.SetMethod(fasthttp.MethodPost)
		req.Header.SetContentType("application/json")
		if task.SecretToken != "" {
			req.Header.Set("X-Telegram-Bot-Api-Secret-Token", task.SecretToken)
		}
		req.SetBody(body)

		err := d.client.DoTimeout(req, resp, webhookTimeout)
		if err == nil {
			statusCode := resp.StatusCode()
			if statusCode >= 200 && statusCode < 300 {
				if ok, skipped := logging.Sample(logging.EventWebhookDelivery, task.BotID); ok {
					fields := []zap.Field{
						zap.Int64("bot_id", task.BotID),
						zap.String("url", task.URL),
						zap.Int("update_id", task.Update.UpdateID),
						zap.Int("status", statusCode),
					}
					if skipped > 0 {
						fields = append(fields, zap.Int("sampled_out", skipped))
					}
					d.logger.Info("Webhook delivered successfully", fields...)
				}
				return nil
			}
			// The application received the update; the delivery loop retries it with backoff.
			lastErr = fmt.Errorf("unexpected status code: %d", statusCode)
			break
		}
		lastErr = err
		// Retry immediately only when the request surely did not reach the application
		// (stale keep-alive connection, connection refused). After a timeout the
		// application may still be processing it, so an immediate resend would duplicate it.
		if !isNotDeliveredError(err) {
			break
		}
		time.Sleep(time.Duration(50*(attempt+1)) * time.Millisecond)
	}

	return fmt.Errorf("do request: %w", lastErr)
}

// isNotDeliveredError reports whether a request failed before the application could
// have received it.
func isNotDeliveredError(err error) bool {
	if errors.Is(err, fasthttp.ErrConnectionClosed) || errors.Is(err, fasthttp.ErrNoFreeConns) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "closed connection") || strings.Contains(msg, "error when dialing") ||
		strings.Contains(msg, "connection refused")
}

// Stop gracefully stops the dispatcher worker pool, draining remaining queued tasks.
func (d *Dispatcher) Stop() {
	close(d.quit)
	d.wg.Wait()
}
