package botmanager

import (
	"context"
	"errors"
	"sync"
	"time"

	"telego-bot-api/internal/diag"
	"telego-bot-api/internal/metrics"

	"github.com/gotd/log/logzap"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"go.uber.org/zap"
)

// channelDifferenceConcurrency bounds updates.getChannelDifference calls of one bot, so
// catching up many channels after a restart does not hit Telegram rate limits.
const channelDifferenceConcurrency = 4

// newGapManager creates the updates manager that tracks pts/qts/seq of the bot: it
// orders updates, recovers gaps and, after reconnects and restarts, fetches updates
// sent meanwhile via updates.getDifference. Without it these updates were lost.
func newGapManager(b *BotInstance) *updates.Manager {
	cfg := updates.Config{
		Handler:                         b,
		AccessHasher:                    gapAccessHasher{b},
		UserAccessHasher:                gapAccessHasher{b},
		Logger:                          logzap.New(b.logger.Named("updates")),
		MaxChannelDifferenceConcurrency: channelDifferenceConcurrency,
		OnTooLong: func() {
			b.logger.Warn("Too many updates were missed, Telegram returned differenceTooLong; some are lost")
		},
		OnChannelTooLong: func(channelID int64) {
			b.logger.Warn("Too many channel updates were missed; some are lost", zap.Int64("channel_id", channelID))
		},
	}
	if b.redisStore != nil {
		cfg.Storage = b.redisStore.UpdatesStateStorage()
	}
	return updates.New(cfg)
}

// runGapManager runs the gap manager for one MTProto connection until runCtx is done.
// Temporary failures (Redis, getState) are retried; authorization errors are returned so
// the client loop can reset the session.
func (b *BotInstance) runGapManager(runCtx context.Context, botID int64) error {
	backoff := time.Second
	for {
		err := b.gaps.Run(runCtx, b.raw(), botID, updates.AuthOptions{IsBot: true})
		b.gaps.Reset()
		if runCtx.Err() != nil {
			return runCtx.Err()
		}
		if auth.IsUnauthorized(err) || tgerr.Is(err, "AUTH_KEY_UNREGISTERED", "SESSION_EXPIRED") {
			return err
		}
		b.logger.Warn("Updates gap manager stopped, restarting", zap.Error(err), zap.Duration("retry_after", backoff))
		select {
		case <-runCtx.Done():
			return runCtx.Err()
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

// gapUpdatesMiddleware passes updates returned by RPC calls (sent messages, edits) and
// pts changes of affected-messages results to the gap manager, so the bot's own actions
// do not look like gaps and trigger needless getDifference calls.
type gapUpdatesMiddleware struct {
	bot *BotInstance
}

func (m gapUpdatesMiddleware) Handle(next tg.Invoker) telegram.InvokeFunc {
	return func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		if err := next.Invoke(ctx, input, output); err != nil {
			return err
		}
		// Business requests act in the account of the business user; their results do
		// not belong to the bot's update sequences.
		if _, ok := input.(*tg.InvokeWithBusinessConnectionRequest); ok {
			return nil
		}
		switch out := output.(type) {
		case *tg.UpdatesBox:
			// Through the intake: a stuck gap manager must not hang API requests.
			m.bot.intake.push(intakeItem{updates: out.Updates})
		case *tg.MessagesAffectedMessages:
			m.handleAffected(input, out.Pts, out.PtsCount)
		case *tg.MessagesAffectedHistory:
			m.handleAffected(input, out.Pts, out.PtsCount)
		}
		return nil
	}
}

// handleAffected routes the pts of an affected-messages result to its channel, or to the
// common sequence, the same way as gotd's hook.AffectedHook.
func (m gapUpdatesMiddleware) handleAffected(input bin.Encoder, pts, ptsCount int) {
	type hasChannelID interface{ GetChannelID() int64 }
	var channelID int64
	if req, ok := input.(interface{ GetChannel() tg.InputChannelClass }); ok {
		channel, ok := req.GetChannel().(hasChannelID)
		if !ok {
			return
		}
		channelID = channel.GetChannelID()
	} else if req, ok := input.(interface{ GetPeer() tg.InputPeerClass }); ok {
		if channel, ok := req.GetPeer().(hasChannelID); ok {
			channelID = channel.GetChannelID()
		}
	}
	m.bot.intake.push(intakeItem{affected: &affectedPts{channelID: channelID, pts: pts, ptsCount: ptsCount}})
}

// gapAccessHasher gives the gap manager access hashes known to the bot: in memory first,
// then the peers persisted in Redis, so they survive restarts and hibernation.
type gapAccessHasher struct {
	b *BotInstance
}

func (h gapAccessHasher) SetChannelAccessHash(ctx context.Context, _, channelID, accessHash int64) error {
	h.b.peers.SaveChannel(channelID, accessHash)
	if h.b.redisStore == nil {
		return nil
	}
	return h.b.redisStore.SavePeer(ctx, h.b.token, -1000000000000-channelID, accessHash, "channel")
}

func (h gapAccessHasher) GetChannelAccessHash(ctx context.Context, _, channelID int64) (int64, bool, error) {
	if hash, ok := h.b.peers.ChannelHash(channelID); ok {
		return hash, true, nil
	}
	hash, ok, err := h.storedPeer(ctx, -1000000000000-channelID, "channel")
	if ok {
		h.b.peers.SaveChannel(channelID, hash)
	}
	return hash, ok, err
}

func (h gapAccessHasher) SetUserAccessHash(_ context.Context, _, userID, accessHash int64) error {
	h.b.peers.SaveUser(userID, accessHash)
	return nil
}

func (h gapAccessHasher) GetUserAccessHash(ctx context.Context, _, userID int64) (int64, bool, error) {
	if hash, ok := h.b.peers.UserHash(userID); ok {
		return hash, true, nil
	}
	hash, ok, err := h.storedPeer(ctx, userID, "user")
	if ok {
		h.b.peers.SaveUser(userID, hash)
	}
	return hash, ok, err
}

func (h gapAccessHasher) storedPeer(ctx context.Context, chatID int64, peerType string) (int64, bool, error) {
	if h.b.redisStore == nil {
		return 0, false, nil
	}
	hash, storedType, ok, err := h.b.redisStore.GetPeer(ctx, h.b.token, chatID)
	if err != nil || !ok || storedType != peerType {
		return 0, false, err
	}
	return hash, true, nil
}

// updatesSyncTimeout bounds updates.getDifference, getChannelDifference and getState.
// They have no deadline of their own: on a dead connection the gap manager waited for
// the answer forever and stopped processing updates. Overridden in tests.
var updatesSyncTimeout = 60 * time.Second

// syncStallAfter is when a running sync request is reported to the watchdog as stalled.
var syncStallAfter = diag.StallThreshold

func isUpdatesSyncRequest(input bin.Encoder) bool {
	switch input.(type) {
	case *tg.UpdatesGetDifferenceRequest, *tg.UpdatesGetChannelDifferenceRequest, *tg.UpdatesGetStateRequest:
		return true
	}
	return false
}

// syncDeadlineMiddleware applies updatesSyncTimeout to update synchronization requests
// and reports the ones that stalled to the watchdog. Other requests are untouched:
// upload.getFile and uploads may legitimately run long.
type syncDeadlineMiddleware struct {
	bot *BotInstance
}

func (m syncDeadlineMiddleware) Handle(next tg.Invoker) telegram.InvokeFunc {
	return func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		if !isUpdatesSyncRequest(input) {
			return next.Invoke(ctx, input, output)
		}
		ctx, cancel := context.WithTimeout(ctx, updatesSyncTimeout)
		defer cancel()
		// Report the stall while the request is still stuck, not only when it ends.
		stalled := time.AfterFunc(syncStallAfter, m.bot.watchdog.noteSyncStall)
		defer stalled.Stop()
		err := next.Invoke(ctx, input, output)
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			m.bot.logger.Warn("Updates sync request timed out", zap.String("method", mtprotoMethod(input)),
				zap.Duration("timeout", updatesSyncTimeout), zap.Error(err))
		}
		return err
	}
}

// affectedPts is a pts change from a messages.affectedMessages/affectedHistory result.
type affectedPts struct {
	channelID     int64
	pts, ptsCount int
}

type intakeItem struct {
	updates  tg.UpdatesClass
	affected *affectedPts
}

// intakeMaxItems bounds the intake. When the gap manager is stuck this long, further
// updates are dropped: those with pts/qts are refetched by getDifference later.
const intakeMaxItems = 100_000

// updateIntake buffers updates between the MTProto connection and the gap manager.
//
// gotd calls the update handler from per-message goroutines of a connection, and the
// connection read loop waits for them before it can close. The gap manager's Push blocks
// while the manager waits for getDifference on that same connection, so after a network
// drop a direct call deadlocked the bot: the read loop could not finish, the connection
// could not reconnect and getDifference never got an answer. push never blocks; a
// per-bot goroutine feeds the gap manager.
type updateIntake struct {
	mu       sync.Mutex
	items    []intakeItem
	wake     chan struct{}
	dropped  int
	progress time.Time // when the feeder last handed an item to the gap manager
	busy     bool      // the feeder is inside the gap manager
}

func (q *updateIntake) wakeChan() chan struct{} {
	if q.wake == nil {
		q.wake = make(chan struct{}, 1)
	}
	return q.wake
}

// push adds an item without blocking. It returns false if the item was dropped.
func (q *updateIntake) push(item intakeItem) bool {
	q.mu.Lock()
	if len(q.items) >= intakeMaxItems {
		q.dropped++
		q.mu.Unlock()
		return false
	}
	q.items = append(q.items, item)
	wake := q.wakeChan()
	q.mu.Unlock()
	select {
	case wake <- struct{}{}:
	default:
	}
	return true
}

// pop takes all queued items.
func (q *updateIntake) pop() ([]intakeItem, int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	items, dropped := q.items, q.dropped
	q.items, q.dropped = nil, 0
	return items, dropped
}

func (q *updateIntake) setBusy(busy bool) {
	q.mu.Lock()
	q.busy = busy
	q.progress = time.Now()
	q.mu.Unlock()
}

// stuckFor reports how long the feeder has been unable to hand pending items to the
// gap manager (0 when it keeps up).
func (q *updateIntake) stuckFor(now time.Time) time.Duration {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.busy || len(q.items) == 0 || q.progress.IsZero() {
		return 0
	}
	return now.Sub(q.progress)
}

func (q *updateIntake) waitChan() <-chan struct{} {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.wakeChan()
}

// runIntake feeds the gap manager from the intake until ctx is done.
func (b *BotInstance) runIntake(ctx context.Context) {
	wake := b.intake.waitChan()
	for {
		items, dropped := b.intake.pop()
		if len(items) > 0 {
			b.intakeOverflowLogged.Store(false)
		}
		if dropped > 0 {
			b.logger.Error("Update intake overflowed while the gap manager was stuck; dropped updates",
				zap.Int("dropped", dropped))
		}
		for _, item := range items {
			b.intake.setBusy(true)
			var err error
			if item.affected != nil {
				err = b.gaps.HandleAffected(ctx, item.affected.channelID, item.affected.pts, item.affected.ptsCount)
			} else {
				err = b.gaps.Handle(ctx, item.updates)
			}
			b.intake.setBusy(false)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				b.logger.Warn("Gap manager rejected updates", zap.Error(err))
			}
		}
		if len(items) > 0 {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-wake:
		}
	}
}

// intakeHandler is the update handler of the MTProto client: it only queues updates.
type intakeHandler struct {
	bot *BotInstance
}

func (h intakeHandler) Handle(_ context.Context, u tg.UpdatesClass) error {
	h.bot.touch()
	h.bot.watchdog.noteUpdates(time.Now(), countUpdates(u))
	metrics.DefaultRegistry.RecordBotUpdates(h.bot.BotID(), countUpdates(u))
	if !h.bot.intake.push(intakeItem{updates: u}) && h.bot.intakeOverflowLogged.CompareAndSwap(false, true) {
		h.bot.logger.Error("Update intake is full, dropping updates until the gap manager catches up",
			zap.Int("limit", intakeMaxItems))
	}
	return nil
}

// countUpdates returns how many updates an MTProto batch carries.
func countUpdates(u tg.UpdatesClass) int {
	switch u := u.(type) {
	case *tg.Updates:
		return len(u.Updates)
	case *tg.UpdatesCombined:
		return len(u.Updates)
	case *tg.UpdatesTooLong:
		return 0
	}
	return 1
}
