package botmanager

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
	"telego-bot-api/internal/metrics"
)

const (
	watchdogInterval = 30 * time.Second
	// watchdogSilence is how long a busy bot may receive no updates before the
	// watchdog considers its MTProto client hung (given evidence of a stall).
	watchdogSilence = 4 * time.Minute
	// watchdogTrafficWindow and watchdogMinTraffic define a busy bot: at least that many
	// updates in the window before the silence. Quiet bots are never restarted.
	watchdogTrafficWindow = 10 * time.Minute
	watchdogMinTraffic    = 60
	// watchdogIntakeStuck is how long the gap manager may refuse queued updates.
	watchdogIntakeStuck = 2 * time.Minute
	// watchdogRestartInterval bounds restarts of one bot.
	watchdogRestartInterval = 10 * time.Minute

	// A steadily busy bot that suddenly receives nothing is restarted even without evidence
	// of a stalled sync request: after a reconnect Telegram may keep the updates for an old
	// session, so requests succeed while the update stream is silent (see HANDOFF).
	//
	// "Steady" means updates in at least watchdogQuietActiveMinutes minutes of the traffic
	// window, so a bot with a single burst of activity is not mistaken for a hung one. The
	// silence that counts as a hang depends on the bot's usual rate: long enough that at
	// least watchdogQuietExpected updates would normally have arrived, within
	// [watchdogQuietMinSilence, watchdogSilence].
	watchdogQuietActiveMinutes = 5
	watchdogQuietExpected      = 30
	watchdogQuietMinSilence    = time.Minute
	// A bot silent for longer than this is taken to be legitimately quiet (night, owner
	// gone) rather than hung, so it is not restarted over and over.
	watchdogQuietGiveUp = 30 * time.Minute
	// watchdogQuietRestartInterval bounds restarts of one bot for the silence above; short,
	// because the hang is cured by a new connection and a failed attempt is cheap.
	watchdogQuietRestartInterval = 3 * time.Minute
)

// clientWatchdog restarts the MTProto client of a busy bot that stopped receiving
// updates while its update synchronization is stuck. It is the last resort when a
// connection hangs in a way timeouts do not resolve.
type clientWatchdog struct {
	mu          sync.Mutex
	minutes     map[int64]int // updates per Unix minute
	lastUpdate  time.Time
	lastStall   time.Time
	lastRestart time.Time
}

// noteUpdates records n updates received from Telegram at now.
func (w *clientWatchdog) noteUpdates(now time.Time, n int) {
	if n <= 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.minutes == nil {
		w.minutes = make(map[int64]int)
	}
	minute := now.Unix() / 60
	w.minutes[minute] += n
	w.lastUpdate = now
	// Keep only the traffic window.
	for m := range w.minutes {
		if m <= minute-int64(watchdogTrafficWindow/time.Minute) {
			delete(w.minutes, m)
		}
	}
}

// noteSyncStall records an updates.getDifference/getChannelDifference/getState request
// that is stuck.
func (w *clientWatchdog) noteSyncStall() {
	w.mu.Lock()
	w.lastStall = time.Now()
	w.mu.Unlock()
}

// quietSilence is how long a bot with perMinute updates per active minute may be silent
// before it counts as hung.
func quietSilence(perMinute float64) time.Duration {
	silence := time.Duration(60*watchdogQuietExpected/perMinute) * time.Second
	if silence < watchdogQuietMinSilence {
		return watchdogQuietMinSilence
	}
	if silence > watchdogSilence {
		return watchdogSilence
	}
	return silence
}

// check decides whether the client must be restarted now. intakeStuck is how long the
// gap manager has been refusing queued updates.
func (w *clientWatchdog) check(now time.Time, intakeStuck time.Duration) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.lastUpdate.IsZero() {
		return "", false
	}
	silence := now.Sub(w.lastUpdate)
	if silence < watchdogQuietMinSilence {
		return "", false
	}
	traffic, active := 0, 0
	for _, n := range w.minutes {
		traffic += n
		if n > 0 {
			active++
		}
	}
	if traffic < watchdogMinTraffic {
		return "", false
	}
	var reason string
	interval := watchdogRestartInterval
	switch {
	case silence >= watchdogSilence && w.lastStall.After(w.lastUpdate):
		reason = "updates sync request stalled"
	case silence >= watchdogSilence && intakeStuck >= watchdogIntakeStuck:
		reason = "gap manager does not accept updates"
	case active >= watchdogQuietActiveMinutes && silence <= watchdogQuietGiveUp &&
		silence >= quietSilence(float64(traffic)/float64(active)):
		reason = "no updates from a steadily busy bot"
		interval = watchdogQuietRestartInterval
	default:
		return "", false
	}
	if !w.lastRestart.IsZero() && now.Sub(w.lastRestart) < interval {
		return "", false
	}
	w.lastRestart = now
	return reason, true
}

// runWatchdog periodically checks the bot and reconnects a hung MTProto client.
func (b *BotInstance) runWatchdog(ctx context.Context) {
	ticker := time.NewTicker(watchdogInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if reason, restart := b.watchdog.check(now, b.intake.stuckFor(now)); restart {
				b.logger.Error("MTProto client watchdog: busy bot stopped receiving updates, recreating the client",
					zap.Int64("bot_id", b.BotID()), zap.String("reason", reason))
				metrics.DefaultRegistry.IncWatchdogRestarts(b.BotID())
				b.restartClient()
			}
		}
	}
}

// restartClient stops the current MTProto client run; the client loop then creates a
// new client and connects again.
func (b *BotInstance) restartClient() {
	b.runCancelMu.Lock()
	cancel := b.runCancel
	b.runCancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
}
