package webhook

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"telego-bot-api/internal/converter"
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
	BotID       int64
	URL         string
	SecretToken string
	Update      *converter.Update
	OnSuccess   func(botID int64, updateID int)
	OnError     func(botID int64, err error)
}

// NewDispatcher creates a webhook dispatcher with a dedicated worker pool.
func NewDispatcher(workers int, queueSize int, logger *zap.Logger) *Dispatcher {
	d := &Dispatcher{
		client: &fasthttp.Client{
			MaxConnsPerHost: 10000,
			// Node.js closes idle connections after 5s by default.
			// Keeping MaxIdleConnDuration at 4s ensures fasthttp cleans up
			// before Node.js sends FIN, preventing stale connection resets.
			MaxIdleConnDuration:           4 * time.Second,
			ReadTimeout:                   5 * time.Second,
			WriteTimeout:                  5 * time.Second,
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
	if err := d.sendWebhook(req, resp, task); err != nil {
		d.logger.Error("Failed to deliver webhook",
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
		req.Reset()
		resp.Reset()

		req.SetRequestURI(task.URL)
		req.Header.SetMethod(fasthttp.MethodPost)
		req.Header.SetContentType("application/json")
		if task.SecretToken != "" {
			req.Header.Set("X-Telegram-Bot-Api-Secret-Token", task.SecretToken)
		}
		req.SetBody(body)

		err := d.client.DoTimeout(req, resp, 5*time.Second)
		if err == nil {
			statusCode := resp.StatusCode()
			if statusCode >= 200 && statusCode < 300 {
				d.logger.Info("Webhook delivered successfully",
					zap.String("url", task.URL),
					zap.Int("update_id", task.Update.UpdateID),
					zap.Int("status", statusCode),
				)
				return nil
			}
			lastErr = fmt.Errorf("unexpected status code: %d", statusCode)
			// Don't retry client 4xx errors
			if statusCode >= 400 && statusCode < 500 {
				break
			}
		} else {
			lastErr = err
		}

		// If the connection was closed prematurely, retry with short backoff
		if errors.Is(err, fasthttp.ErrConnectionClosed) || (err != nil && strings.Contains(err.Error(), "closed connection")) {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("do request: %w", lastErr)
}

// Stop gracefully stops the dispatcher worker pool, draining remaining queued tasks.
func (d *Dispatcher) Stop() {
	close(d.quit)
	d.wg.Wait()
}
