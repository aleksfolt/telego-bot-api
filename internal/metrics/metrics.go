package metrics

import (
	"fmt"
	"io"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Registry holds all gateway operational metrics.
type Registry struct {
	requestsTotalMu sync.RWMutex
	requestsTotal   map[string]*atomic.Uint64 // key: "method:status"

	durationsMu sync.RWMutex
	durationSum map[string]*atomic.Uint64 // key: method (in microseconds)
	durationCnt map[string]*atomic.Uint64 // key: method

	updatesReceived   atomic.Uint64
	webhookDeliveries sync.RWMutex
	webhookStatus     map[string]*atomic.Uint64 // key: status

	startTime time.Time
}

var DefaultRegistry = NewRegistry()

func NewRegistry() *Registry {
	return &Registry{
		requestsTotal: make(map[string]*atomic.Uint64),
		durationSum:   make(map[string]*atomic.Uint64),
		durationCnt:   make(map[string]*atomic.Uint64),
		webhookStatus: make(map[string]*atomic.Uint64),
		startTime:     time.Now(),
	}
}

// IncRequests increments the request counter for the given method and HTTP status code.
func (r *Registry) IncRequests(method string, statusCode int) {
	key := fmt.Sprintf("%s:%d", method, statusCode)
	r.requestsTotalMu.RLock()
	counter, ok := r.requestsTotal[key]
	r.requestsTotalMu.RUnlock()

	if !ok {
		r.requestsTotalMu.Lock()
		counter, ok = r.requestsTotal[key]
		if !ok {
			counter = &atomic.Uint64{}
			r.requestsTotal[key] = counter
		}
		r.requestsTotalMu.Unlock()
	}

	counter.Add(1)
}

// ObserveDuration records the execution latency of an API method.
func (r *Registry) ObserveDuration(method string, d time.Duration) {
	micros := uint64(d.Microseconds())

	r.durationsMu.RLock()
	sumCounter, ok := r.durationSum[method]
	cntCounter := r.durationCnt[method]
	r.durationsMu.RUnlock()

	if !ok {
		r.durationsMu.Lock()
		sumCounter, ok = r.durationSum[method]
		cntCounter = r.durationCnt[method]
		if !ok || cntCounter == nil {
			if !ok {
				sumCounter = &atomic.Uint64{}
				r.durationSum[method] = sumCounter
			}
			if cntCounter == nil {
				cntCounter = &atomic.Uint64{}
				r.durationCnt[method] = cntCounter
			}
		}
		r.durationsMu.Unlock()
	}

	sumCounter.Add(micros)
	cntCounter.Add(1)
}

// IncUpdatesReceived increments the incoming Telegram updates counter.
func (r *Registry) IncUpdatesReceived(count int) {
	if count > 0 {
		r.updatesReceived.Add(uint64(count))
	}
}

// IncWebhookDelivery records a webhook delivery outcome.
func (r *Registry) IncWebhookDelivery(status string) {
	r.webhookDeliveries.RLock()
	counter, ok := r.webhookStatus[status]
	r.webhookDeliveries.RUnlock()

	if !ok {
		r.webhookDeliveries.Lock()
		counter, ok = r.webhookStatus[status]
		if !ok {
			counter = &atomic.Uint64{}
			r.webhookStatus[status] = counter
		}
		r.webhookDeliveries.Unlock()
	}

	counter.Add(1)
}

// WritePrometheus serializes all recorded metrics in the standard Prometheus text exposition format.
func (r *Registry) WritePrometheus(w io.Writer, activeBots, hibernatedBots, webhookQueueSize int) {
	now := time.Now()
	uptime := now.Sub(r.startTime).Seconds()

	// 1. Process & Uptime
	fmt.Fprintf(w, "# HELP telego_uptime_seconds Total time since telego-bot-api started in seconds\n")
	fmt.Fprintf(w, "# TYPE telego_uptime_seconds gauge\n")
	fmt.Fprintf(w, "telego_uptime_seconds %.2f\n\n", uptime)

	// 2. Bot instances
	fmt.Fprintf(w, "# HELP telego_bots_active Current number of active (awake) bots\n")
	fmt.Fprintf(w, "# TYPE telego_bots_active gauge\n")
	fmt.Fprintf(w, "telego_bots_active %d\n\n", activeBots)

	fmt.Fprintf(w, "# HELP telego_bots_hibernated Current number of idle hibernated bots\n")
	fmt.Fprintf(w, "# TYPE telego_bots_hibernated gauge\n")
	fmt.Fprintf(w, "telego_bots_hibernated %d\n\n", hibernatedBots)

	fmt.Fprintf(w, "# HELP telego_bots_total Total registered bot sessions\n")
	fmt.Fprintf(w, "# TYPE telego_bots_total gauge\n")
	fmt.Fprintf(w, "telego_bots_total %d\n\n", activeBots+hibernatedBots)

	// 3. API Requests
	fmt.Fprintf(w, "# HELP telego_requests_total Total number of HTTP requests served\n")
	fmt.Fprintf(w, "# TYPE telego_requests_total counter\n")
	r.requestsTotalMu.RLock()
	keys := make([]string, 0, len(r.requestsTotal))
	for k := range r.requestsTotal {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		val := r.requestsTotal[k].Load()
		var method string
		var status int
		_, _ = fmt.Sscanf(k, "%s:%d", &method, &status)
		// If method is empty or format mismatch, split by colon
		if method == "" {
			parts := sort.StringSlice(keys)
			_ = parts
		}
		// Clean split
		for i := len(k) - 1; i >= 0; i-- {
			if k[i] == ':' {
				method = k[:i]
				fmt.Sscanf(k[i+1:], "%d", &status) //nolint:errcheck
				break
			}
		}
		fmt.Fprintf(w, "telego_requests_total{method=\"%s\",status=\"%d\"} %d\n", method, status, val)
	}
	r.requestsTotalMu.RUnlock()
	fmt.Fprintln(w)

	// 4. Latency
	fmt.Fprintf(w, "# HELP telego_request_duration_seconds Total execution time of API methods in seconds\n")
	fmt.Fprintf(w, "# TYPE telego_request_duration_seconds summary\n")
	r.durationsMu.RLock()
	durKeys := make([]string, 0, len(r.durationSum))
	for k := range r.durationSum {
		durKeys = append(durKeys, k)
	}
	sort.Strings(durKeys)
	for _, m := range durKeys {
		sumMicros := r.durationSum[m].Load()
		cnt := r.durationCnt[m].Load()
		sumSeconds := float64(sumMicros) / 1_000_000.0
		fmt.Fprintf(w, "telego_request_duration_seconds_sum{method=\"%s\"} %.6f\n", m, sumSeconds)
		fmt.Fprintf(w, "telego_request_duration_seconds_count{method=\"%s\"} %d\n", m, cnt)
	}
	r.durationsMu.RUnlock()
	fmt.Fprintln(w)

	// 5. Updates
	fmt.Fprintf(w, "# HELP telego_updates_received_total Total updates received from MTProto\n")
	fmt.Fprintf(w, "# TYPE telego_updates_received_total counter\n")
	fmt.Fprintf(w, "telego_updates_received_total %d\n\n", r.updatesReceived.Load())

	// 6. Webhooks
	fmt.Fprintf(w, "# HELP telego_webhook_deliveries_total Total webhook delivery attempts\n")
	fmt.Fprintf(w, "# TYPE telego_webhook_deliveries_total counter\n")
	r.webhookDeliveries.RLock()
	whKeys := make([]string, 0, len(r.webhookStatus))
	for k := range r.webhookStatus {
		whKeys = append(whKeys, k)
	}
	sort.Strings(whKeys)
	for _, status := range whKeys {
		cnt := r.webhookStatus[status].Load()
		fmt.Fprintf(w, "telego_webhook_deliveries_total{status=\"%s\"} %d\n", status, cnt)
	}
	r.webhookDeliveries.RUnlock()
	fmt.Fprintln(w)

	fmt.Fprintf(w, "# HELP telego_webhook_queue_size Current size of the webhook dispatch buffer\n")
	fmt.Fprintf(w, "# TYPE telego_webhook_queue_size gauge\n")
	fmt.Fprintf(w, "telego_webhook_queue_size %d\n\n", webhookQueueSize)

	// 7. Go Runtime Stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	fmt.Fprintf(w, "# HELP go_goroutines Number of goroutines currently existing\n")
	fmt.Fprintf(w, "# TYPE go_goroutines gauge\n")
	fmt.Fprintf(w, "go_goroutines %d\n\n", runtime.NumGoroutine())

	fmt.Fprintf(w, "# HELP go_memstats_alloc_bytes Bytes allocated and still in use\n")
	fmt.Fprintf(w, "# TYPE go_memstats_alloc_bytes gauge\n")
	fmt.Fprintf(w, "go_memstats_alloc_bytes %d\n\n", m.Alloc)

	fmt.Fprintf(w, "# HELP go_memstats_sys_bytes Total bytes of memory obtained from the OS\n")
	fmt.Fprintf(w, "# TYPE go_memstats_sys_bytes gauge\n")
	fmt.Fprintf(w, "go_memstats_sys_bytes %d\n\n", m.Sys)
}
