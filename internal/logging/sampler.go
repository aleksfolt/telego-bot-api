package logging

import (
	"sync"
	"sync/atomic"
	"time"
)

// Per-event log lines (every update, webhook delivery and API request) are sampled per
// bot: the first SampleFirst lines of a bot in a second are written, then every
// SampleThereafter-th. Quiet bots are logged in full, so their events can be looked up
// by user_id, while a busy bot no longer floods the journal (and does not use up the
// global zap sampling budget of other bots). Warnings and errors are never sampled.
var (
	sampleFirst      atomic.Int64
	sampleThereafter atomic.Int64
)

func init() {
	ConfigureSampling(5, 100)
}

// ConfigureSampling sets per-bot sampling of per-event log lines. thereafter <= 1
// disables sampling.
func ConfigureSampling(first, thereafter int) {
	if first < 0 {
		first = 0
	}
	sampleFirst.Store(int64(first))
	sampleThereafter.Store(int64(thereafter))
}

// Event kinds sampled independently of each other.
const (
	EventUpdate          = "update"
	EventWebhookDelivery = "webhook_delivery"
	EventAPIRequest      = "api_request"
)

type sampleWindow struct {
	mu      sync.Mutex
	second  int64
	count   int64
	skipped int
}

var samplers sync.Map // kind + bot_id -> *sampleWindow

type sampleKey struct {
	kind  string
	botID int64
}

// Sample reports whether a per-event line of the bot should be written now and how
// many lines of this kind were skipped since the last written one (log it as
// "sampled_out" so counts stay visible).
func Sample(kind string, botID int64) (bool, int) {
	return sampleAt(kind, botID, time.Now())
}

func sampleAt(kind string, botID int64, now time.Time) (bool, int) {
	thereafter := sampleThereafter.Load()
	if thereafter <= 1 {
		return true, 0
	}
	key := sampleKey{kind: kind, botID: botID}
	v, ok := samplers.Load(key)
	if !ok {
		v, _ = samplers.LoadOrStore(key, &sampleWindow{})
	}
	w := v.(*sampleWindow)

	w.mu.Lock()
	defer w.mu.Unlock()
	if sec := now.Unix(); sec != w.second {
		w.second, w.count = sec, 0
	}
	w.count++
	if w.count <= sampleFirst.Load() || (w.count-sampleFirst.Load())%thereafter == 0 {
		skipped := w.skipped
		w.skipped = 0
		return true, skipped
	}
	w.skipped++
	return false, 0
}
