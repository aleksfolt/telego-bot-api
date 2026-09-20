package metrics

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRegistry_MetricsCollection(t *testing.T) {
	r := NewRegistry()

	r.IncRequests("sendMessage", 200)
	r.IncRequests("sendMessage", 200)
	r.IncRequests("sendMessage", 429)
	r.IncRequests("getUpdates", 200)

	r.ObserveDuration("sendMessage", 15*time.Millisecond)
	r.ObserveDuration("sendMessage", 25*time.Millisecond)

	r.IncUpdatesReceived(5)
	r.IncWebhookDelivery("200")
	r.IncWebhookDelivery("error")

	var buf bytes.Buffer
	r.WritePrometheus(&buf, 3, 1, 0)

	output := buf.String()

	require.Contains(t, output, `telego_requests_total{method="sendMessage",status="200"} 2`)
	require.Contains(t, output, `telego_requests_total{method="sendMessage",status="429"} 1`)
	require.Contains(t, output, `telego_requests_total{method="getUpdates",status="200"} 1`)
	require.Contains(t, output, `telego_request_duration_seconds_count{method="sendMessage"} 2`)
	require.Contains(t, output, `telego_updates_received_total 5`)
	require.Contains(t, output, `telego_webhook_deliveries_total{status="200"} 1`)
	require.Contains(t, output, `telego_webhook_deliveries_total{status="error"} 1`)
	require.Contains(t, output, `telego_bots_active 3`)
	require.Contains(t, output, `telego_bots_hibernated 1`)
	require.Contains(t, output, `telego_bots_total 4`)
	require.True(t, strings.Contains(output, "go_goroutines"))
	require.True(t, strings.Contains(output, "go_memstats_alloc_bytes"))
}

func TestRegistry_ObserveDurationConcurrentFirstUse(t *testing.T) {
	r := NewRegistry()
	const workers = 64

	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			r.ObserveDuration("getMe", time.Millisecond)
		}()
	}
	wg.Wait()

	var buf bytes.Buffer
	r.WritePrometheus(&buf, 0, 0, 0)
	require.Contains(t, buf.String(), `telego_request_duration_seconds_count{method="getMe"} 64`)
}
