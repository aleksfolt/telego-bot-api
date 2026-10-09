package botmanager

import (
	"context"
	"time"

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
			if err := m.bot.gaps.Handle(ctx, out.Updates); err != nil {
				m.bot.logger.Warn("Failed to pass RPC result updates to the gap manager", zap.Error(err))
			}
		case *tg.MessagesAffectedMessages:
			m.handleAffected(ctx, input, out.Pts, out.PtsCount)
		case *tg.MessagesAffectedHistory:
			m.handleAffected(ctx, input, out.Pts, out.PtsCount)
		}
		return nil
	}
}

// handleAffected routes the pts of an affected-messages result to its channel, or to the
// common sequence, the same way as gotd's hook.AffectedHook.
func (m gapUpdatesMiddleware) handleAffected(ctx context.Context, input bin.Encoder, pts, ptsCount int) {
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
	if err := m.bot.gaps.HandleAffected(ctx, channelID, pts, ptsCount); err != nil {
		m.bot.logger.Warn("Failed to pass affected pts to the gap manager", zap.Error(err))
	}
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
