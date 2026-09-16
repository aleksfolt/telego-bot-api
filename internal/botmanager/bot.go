package botmanager

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"telego-bot-api/internal/converter"
	"telego-bot-api/internal/logging"
	"telego-bot-api/internal/netpool"
	"telego-bot-api/internal/peer"
	"telego-bot-api/internal/storage"
	"telego-bot-api/internal/webhook"

	"github.com/gotd/log/logzap"
	"github.com/gotd/td/fileid"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"go.uber.org/zap"
)

// BotInstance represents an active MTProto bot session.
type BotInstance struct {
	token      string
	appID      int
	appHash    string
	client     *telegram.Client
	raw        *tg.Client
	peers      *peer.Storage
	converter  *converter.MTProtoConverter
	dispatcher *webhook.Dispatcher
	redisStore *storage.RedisStore
	logger     *zap.Logger

	mu          sync.RWMutex
	webhookURL  string
	secretToken string
	self        *converter.User
	// botID is the Telegram user ID of the bot, used as the Redis Stream key.
	// Set to 0 until authentication completes.
	botID        int64
	updateNotify chan struct{}

	lastActive atomic.Int64 // UnixNano timestamp of last incoming or outgoing event
	hibernated atomic.Bool  // true when RAM caches are freed and bot is in idle/hibernation

	busConnsMu sync.RWMutex
	busConns   map[string]*converter.BusinessConnection

	mediaCacheMu sync.RWMutex
	mediaCache   map[string]*tg.InputPhoto
	docCache     map[string]*tg.InputDocument

	cancel context.CancelFunc
	ready  chan struct{}
}

// NewBotInstance initializes a new bot instance backed by Redis storage.
func NewBotInstance(
	token string,
	appID int,
	appHash string,
	dispatcher *webhook.Dispatcher,
	redisStore *storage.RedisStore,
	logger *zap.Logger,
	dialer *netpool.PoolDialer,
	mtprotoDebug bool,
) *BotInstance {
	var botID int64
	if parts := strings.Split(token, ":"); len(parts) > 0 {
		botID, _ = strconv.ParseInt(parts[0], 10, 64)
	}

	bot := &BotInstance{
		token:        token,
		appID:        appID,
		appHash:      appHash,
		peers:        peer.NewStorage(),
		converter:    converter.NewMTProtoConverter(),
		dispatcher:   dispatcher,
		redisStore:   redisStore,
		logger:       logger.With(zap.String("token_prefix", token[:min(10, len(token))])),
		botID:        botID,
		updateNotify: make(chan struct{}, 1),
		busConns:     make(map[string]*converter.BusinessConnection),
		mediaCache:   make(map[string]*tg.InputPhoto),
		docCache:     make(map[string]*tg.InputDocument),
		ready:        make(chan struct{}),
	}
	bot.lastActive.Store(time.Now().UnixNano())

	opts := telegram.Options{
		SessionStorage: redisStore.SessionStorage(token),
		UpdateHandler:  bot,
		Logger:         logzap.New(logging.MTProtoLogger(logger, mtprotoDebug)),
	}

	if dialer != nil {
		opts.Resolver = dcs.Plain(dcs.PlainOptions{
			Dial: dialer.DialContext,
		})
	}

	bot.client = telegram.NewClient(appID, appHash, opts)
	return bot
}

// Start launches the MTProto background client loop and logs in with the bot token.
func (b *BotInstance) Start(ctx context.Context) error {
	ctx, b.cancel = context.WithCancel(ctx)

	// Restore webhook configuration from Redis if present
	if wh, err := b.redisStore.GetWebhook(ctx, b.token); err == nil && wh != nil {
		b.mu.Lock()
		b.webhookURL = wh.URL
		b.secretToken = wh.SecretToken
		b.mu.Unlock()
		b.logger.Info("Restored webhook from Redis", zap.String("url", wh.URL))
	}

	errChan := make(chan error, 1)

	go func() {
		backoff := 1 * time.Second
		maxBackoff := 30 * time.Second
		firstRun := true

		for {
			if ctx.Err() != nil {
				return
			}

			runErr := b.client.Run(ctx, func(runCtx context.Context) error {
				// Connected successfully: reset backoff
				backoff = 1 * time.Second
				b.raw = b.client.API()

				// Authenticate bot using token
				auth, err := b.client.Auth().Bot(runCtx, b.token)
				if err != nil {
					return fmt.Errorf("bot auth failed: %w", err)
				}

				user, ok := auth.User.AsNotEmpty()
				if ok {
					canConnectBusiness := user.GetBotBusiness() || os.Getenv("TELEGO_FORCE_BUSINESS_MODE") == "true"
					b.mu.Lock()
					b.botID = user.ID
					b.self = &converter.User{
						ID:                      user.ID,
						IsBot:                   true,
						FirstName:               user.FirstName,
						LastName:                user.LastName,
						Username:                user.Username,
						CanJoinGroups:           !user.GetBotNochats(),
						CanReadAllGroupMessages: user.GetBotChatHistory(),
						SupportsInlineQueries:   user.GetBotInlineGeo() || user.BotInlinePlaceholder != "",
						CanConnectToBusiness:    canConnectBusiness,
						HasMainWebApp:           user.GetBotAttachMenu(),
					}
					b.converter.SetSelfUserID(user.ID)
					b.mu.Unlock()
					b.logger.Info("Bot connected and authorized",
						zap.String("username", user.Username),
						zap.Int64("id", user.ID),
						zap.Bool("can_connect_to_business", canConnectBusiness),
					)
				}

				if firstRun {
					firstRun = false
					close(b.ready)
				}

				// Keep alive until context is cancelled (persistent 24/7 connection)
				<-runCtx.Done()
				return runCtx.Err()
			})

			if ctx.Err() != nil {
				// Stopped intentionally (server shutdown or bot closed)
				return
			}

			if firstRun {
				// Failed on initial connect / auth before ready
				select {
				case errChan <- runErr:
				default:
				}
				return
			}

			b.logger.Warn("MTProto client disconnected, attempting reconnection...",
				zap.Error(runErr),
				zap.Duration("retry_after", backoff),
			)

			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
		}
	}()

	select {
	case err := <-errChan:
		return err
	case <-b.ready:
		return nil
	case <-time.After(20 * time.Second):
		return fmt.Errorf("timeout waiting for bot connection to Telegram DC")
	}
}

// Handle handles incoming MTProto updates from Telegram.
func (b *BotInstance) Handle(ctx context.Context, u tg.UpdatesClass) error {
	b.touch()

	b.mu.RLock()
	url := b.webhookURL
	secret := b.secretToken
	botID := b.botID
	b.mu.RUnlock()

	var entities *converter.EntityContext
	var updatesList []tg.UpdateClass
	var extraUpdates []*converter.Update

	switch upds := u.(type) {
	case *tg.Updates:
		b.peers.IngestPeers(upds.Users, upds.Chats)
		b.savePeersToRedis(upds.Users, upds.Chats)
		entities = converter.NewEntityContext(upds.Users, upds.Chats)
		updatesList = upds.Updates
	case *tg.UpdatesCombined:
		b.peers.IngestPeers(upds.Users, upds.Chats)
		b.savePeersToRedis(upds.Users, upds.Chats)
		entities = converter.NewEntityContext(upds.Users, upds.Chats)
		updatesList = upds.Updates
	case *tg.UpdateShort:
		updatesList = []tg.UpdateClass{upds.Update}
	case *tg.UpdateShortMessage:
		if botID == 0 {
			b.logger.Warn("Skipping update: botID not yet set")
			return nil
		}
		uid, err := b.redisStore.NextUpdateID(ctx, botID)
		if err != nil {
			b.logger.Warn("Failed to get next update_id", zap.Error(err))
			return nil
		}
		extraUpdates = append(extraUpdates, b.converter.ConvertShortMessage(uid, upds))
	case *tg.UpdateShortChatMessage:
		if botID == 0 {
			b.logger.Warn("Skipping update: botID not yet set")
			return nil
		}
		uid, err := b.redisStore.NextUpdateID(ctx, botID)
		if err != nil {
			b.logger.Warn("Failed to get next update_id", zap.Error(err))
			return nil
		}
		extraUpdates = append(extraUpdates, b.converter.ConvertShortChatMessage(uid, upds))
	default:
		b.logger.Debug("Unhandled MTProto update type", zap.String("type", fmt.Sprintf("%T", u)))
	}

	if botID == 0 {
		b.logger.Warn("Skipping updates: botID not yet set")
		return nil
	}

	for _, rawUpd := range updatesList {
		uid, err := b.redisStore.NextUpdateID(ctx, botID)
		if err != nil {
			b.logger.Warn("Failed to get next update_id", zap.Error(err))
			continue
		}
		converted, err := b.converter.ConvertUpdate(uid, rawUpd, entities)
		if err != nil {
			b.logger.Warn("Failed to convert update", zap.Error(err))
			continue
		}
		if converted != nil {
			if converted.BusinessConnection != nil {
				b.busConnsMu.Lock()
				b.busConns[converted.BusinessConnection.ID] = converted.BusinessConnection
				b.busConnsMu.Unlock()
			}
			extraUpdates = append(extraUpdates, converted)
		}
	}

	if len(extraUpdates) == 0 {
		return nil
	}

	// Persist each update to Redis Stream and optionally enqueue webhook delivery.
	for _, upd := range extraUpdates {
		payload, err := json.Marshal(upd)
		if err != nil {
			b.logger.Warn("Failed to marshal update for Redis", zap.Error(err))
			continue
		}

		appendCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		appendErr := b.redisStore.AppendUpdate(appendCtx, botID, upd.UpdateID, payload)
		cancel()
		if appendErr != nil {
			b.logger.Error("Failed to append update to Redis Stream", zap.Error(appendErr), zap.Int("update_id", upd.UpdateID))
		}

		kind := updateKind(upd)
		sender := describeUpdateSender(upd)
		if url != "" {
			b.dispatcher.Enqueue(url, secret, upd)
			b.logger.Info("Update received -> Webhook",
				zap.Int("update_id", upd.UpdateID),
				zap.String("type", kind),
				zap.String("from", sender),
			)
		} else {
			b.logger.Info("Update received -> Polling stream",
				zap.Int("update_id", upd.UpdateID),
				zap.String("type", kind),
				zap.String("from", sender),
			)
		}
	}

	// Notify long-polling listeners
	select {
	case b.updateNotify <- struct{}{}:
	default:
	}

	return nil
}

// GetUpdates retrieves buffered updates from Redis Streams for long-polling.
func (b *BotInstance) GetUpdates(ctx context.Context, offset, limit, timeout int, allowedUpdates []string) ([]*converter.Update, error) {
	b.touch()

	if limit <= 0 || limit > 100 {
		limit = 100
	}
	if timeout < 0 {
		timeout = 0
	}

	b.mu.RLock()
	botID := b.botID
	b.mu.RUnlock()

	if botID == 0 {
		// Bot not yet authorized; return empty
		return []*converter.Update{}, nil
	}

	// Positive offset means the client acknowledges all updates with id < offset.
	if offset > 0 {
		ackCtx, ackCancel := context.WithTimeout(context.Background(), 3*time.Second)
		if err := b.redisStore.AckUpdates(ackCtx, botID, offset); err != nil {
			b.logger.Warn("Failed to ack updates in Redis", zap.Error(err))
		}
		ackCancel()
	}

	// Negative offset: keep last |offset| updates — treat as offset=0 for reading
	// (the client rarely uses this; we just ignore trimming in that case).
	readOffset := offset
	if readOffset < 0 {
		readOffset = 0
	}

	deadline := time.Now().Add(time.Duration(timeout) * time.Second)

	for {
		remaining := time.Until(deadline)
		if remaining < 0 {
			remaining = 0
		}

		// Use blocking read only when we still have time budget left and there are
		// no results yet. Cap block at remaining to respect the HTTP timeout.
		blockDuration := time.Duration(0)
		if timeout > 0 && remaining > 0 {
			blockDuration = remaining
		}

		readCtx, readCancel := context.WithTimeout(ctx, remaining+2*time.Second)
		entries, err := b.redisStore.ReadUpdates(readCtx, botID, readOffset, limit, blockDuration)
		readCancel()

		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			b.logger.Warn("Failed to read updates from Redis", zap.Error(err))
			return []*converter.Update{}, nil
		}

		// Deserialise and apply allowed_updates filter
		var res []*converter.Update
		for _, entry := range entries {
			var upd converter.Update
			if err := json.Unmarshal(entry.Payload, &upd); err != nil {
				b.logger.Warn("Failed to unmarshal update from Redis Stream",
					zap.Error(err), zap.String("stream_id", entry.StreamID))
				continue
			}
			if updateAllowed(&upd, allowedUpdates) {
				res = append(res, &upd)
			}
		}

		if len(res) > 0 || timeout == 0 || time.Now().After(deadline) {
			if res == nil {
				return []*converter.Update{}, nil
			}
			return res, nil
		}

		// ReadUpdates already blocked in Redis; loop back to check deadline.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
}

func updateAllowed(update *converter.Update, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	typeName := ""
	switch {
	case update.Message != nil:
		typeName = "message"
	case update.EditedMessage != nil:
		typeName = "edited_message"
	case update.ChannelPost != nil:
		typeName = "channel_post"
	case update.EditedChannelPost != nil:
		typeName = "edited_channel_post"
	case update.BusinessConnection != nil:
		typeName = "business_connection"
	case update.BusinessMessage != nil:
		typeName = "business_message"
	case update.EditedBusinessMessage != nil:
		typeName = "edited_business_message"
	case update.DeletedBusinessMessages != nil:
		typeName = "deleted_business_messages"
	case update.CallbackQuery != nil:
		typeName = "callback_query"
	case update.InlineQuery != nil:
		typeName = "inline_query"
	case update.ChosenInlineResult != nil:
		typeName = "chosen_inline_result"
	case update.ShippingQuery != nil:
		typeName = "shipping_query"
	case update.PreCheckoutQuery != nil:
		typeName = "pre_checkout_query"
	case update.Poll != nil:
		typeName = "poll"
	case update.PollAnswer != nil:
		typeName = "poll_answer"
	case update.MyChatMember != nil:
		typeName = "my_chat_member"
	case update.ChatMember != nil:
		typeName = "chat_member"
	case update.ChatJoinRequest != nil:
		typeName = "chat_join_request"
	case update.MessageReaction != nil:
		typeName = "message_reaction"
	case update.MessageReactionCount != nil:
		typeName = "message_reaction_count"
	case update.ChatBoost != nil:
		typeName = "chat_boost"
	case update.PurchasedPaidMedia != nil:
		typeName = "purchased_paid_media"
	}
	for _, value := range allowed {
		if value == typeName {
			return true
		}
	}
	return false
}

func (b *BotInstance) savePeersToRedis(users []tg.UserClass, chats []tg.ChatClass) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		for _, u := range users {
			if user, ok := u.(*tg.User); ok && user.AccessHash != 0 {
				_ = b.redisStore.SavePeer(ctx, b.token, user.ID, user.AccessHash, "user")
			}
		}
		for _, c := range chats {
			if channel, ok := c.(*tg.Channel); ok && channel.AccessHash != 0 {
				_ = b.redisStore.SavePeer(ctx, b.token, -1000000000000-channel.ID, channel.AccessHash, "channel")
			}
		}
	}()
}



// DropPendingUpdates removes all pending updates for this bot from Redis Streams.
func (b *BotInstance) DropPendingUpdates(ctx context.Context) error {
	b.mu.RLock()
	botID := b.botID
	b.mu.RUnlock()
	if botID == 0 {
		return nil
	}
	if err := b.redisStore.DropPendingUpdates(ctx, botID); err != nil {
		b.logger.Error("Failed to drop pending updates from Redis", zap.Error(err))
		return err
	}
	b.logger.Info("Dropped all pending updates from Redis Stream")
	return nil
}

// SetWebhook sets the webhook URL for the bot and persists it to Redis.
// If dropPending is true, all pending updates are discarded.
func (b *BotInstance) SetWebhook(ctx context.Context, url, secretToken string, dropPending bool) error {
	b.mu.Lock()
	b.webhookURL = url
	b.secretToken = secretToken
	b.mu.Unlock()

	data := &storage.WebhookData{
		URL:         url,
		SecretToken: secretToken,
		UpdatedAt:   time.Now(),
	}
	if err := b.redisStore.SaveWebhook(ctx, b.token, data); err != nil {
		b.logger.Error("Failed to save webhook to Redis", zap.Error(err))
		return err
	}

	if dropPending {
		if err := b.DropPendingUpdates(ctx); err != nil {
			b.logger.Warn("Failed to drop pending updates during SetWebhook", zap.Error(err))
		}
	}

	b.logger.Info("Webhook set successfully", zap.String("url", url))
	return nil
}

// DeleteWebhook removes the webhook from memory and Redis.
// If dropPending is true, all pending updates are discarded.
func (b *BotInstance) DeleteWebhook(ctx context.Context, dropPending bool) error {
	b.mu.Lock()
	b.webhookURL = ""
	b.secretToken = ""
	b.mu.Unlock()

	if err := b.redisStore.DeleteWebhook(ctx, b.token); err != nil {
		b.logger.Error("Failed to delete webhook from Redis", zap.Error(err))
		return err
	}

	if dropPending {
		if err := b.DropPendingUpdates(ctx); err != nil {
			b.logger.Warn("Failed to drop pending updates during DeleteWebhook", zap.Error(err))
		}
	}

	b.logger.Info("Webhook deleted")
	return nil
}

// GetWebhookInfo returns current webhook info.
func (b *BotInstance) GetWebhookInfo(ctx context.Context) (*converter.WebhookInfo, error) {
	b.mu.RLock()
	url := b.webhookURL
	b.mu.RUnlock()

	return &converter.WebhookInfo{
		URL:                  url,
		HasCustomCertificate: false,
		PendingUpdateCount:   0,
	}, nil
}

// HasWebhook returns true if a webhook URL is currently configured for the bot.
func (b *BotInstance) HasWebhook() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.webhookURL != ""
}

// touch updates the last active timestamp and wakes up the bot if hibernated.
func (b *BotInstance) touch() {
	b.lastActive.Store(time.Now().UnixNano())
	if b.hibernated.Load() {
		b.WakeUp()
	}
}

// Touch notifies the bot of an activity event to prevent/exit hibernation.
func (b *BotInstance) Touch() {
	b.touch()
}

// LastActive returns the timestamp of the last incoming or outgoing event.
func (b *BotInstance) LastActive() time.Time {
	val := b.lastActive.Load()
	if val == 0 {
		return time.Now()
	}
	return time.Unix(0, val)
}

// IsHibernated reports whether the bot's heavy caches are currently evicted.
func (b *BotInstance) IsHibernated() bool {
	return b.hibernated.Load()
}

// Hibernate evicts in-memory peer, media and doc caches to minimize RAM usage.
// The underlying MTProto TCP connection remains open in idle ping-pong mode (~10-20KB).
// The bot wakes up automatically (0ms delay) upon the next incoming update or outgoing call.
func (b *BotInstance) Hibernate() {
	if !b.hibernated.CompareAndSwap(false, true) {
		return
	}

	b.mediaCacheMu.Lock()
	b.mediaCache = make(map[string]*tg.InputPhoto)
	b.docCache = make(map[string]*tg.InputDocument)
	b.mediaCacheMu.Unlock()

	b.busConnsMu.Lock()
	b.busConns = make(map[string]*converter.BusinessConnection)
	b.busConnsMu.Unlock()

	b.peers.Reset()

	b.logger.Info("Bot entered hibernation mode; RAM caches evicted", zap.Int64("bot_id", b.botID))
}

// WakeUp restores the bot to active mode.
func (b *BotInstance) WakeUp() {
	if !b.hibernated.CompareAndSwap(true, false) {
		return
	}

	b.logger.Info("Bot woke up from hibernation", zap.Int64("bot_id", b.botID))
}

// GetMe returns bot profile information.
func (b *BotInstance) GetMe() *converter.User {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.self
}

// RefreshMe fetches the freshest bot profile from Telegram MTProto to reflect any recent BotFather changes.
func (b *BotInstance) RefreshMe(ctx context.Context) *converter.User {
	if b.raw != nil {
		users, err := b.raw.UsersGetUsers(ctx, []tg.InputUserClass{&tg.InputUserSelf{}})
		if err == nil && len(users) > 0 {
			if u, ok := users[0].AsNotEmpty(); ok {
				canConnectBusiness := u.GetBotBusiness() || os.Getenv("TELEGO_FORCE_BUSINESS_MODE") == "true"
				b.mu.Lock()
				b.self = &converter.User{
					ID:                      u.ID,
					IsBot:                   true,
					FirstName:               u.FirstName,
					LastName:                u.LastName,
					Username:                u.Username,
					CanJoinGroups:           !u.GetBotNochats(),
					CanReadAllGroupMessages: u.GetBotChatHistory(),
					SupportsInlineQueries:   u.GetBotInlineGeo() || u.BotInlinePlaceholder != "",
					CanConnectToBusiness:    canConnectBusiness,
					HasMainWebApp:           u.GetBotAttachMenu(),
				}
				b.converter.SetSelfUserID(u.ID)
				b.mu.Unlock()
				b.logger.Info("Refreshed bot profile",
					zap.String("username", u.Username),
					zap.Bool("can_connect_to_business", canConnectBusiness),
				)
			}
		}
	}
	return b.GetMe()
}

// GetBusinessConnection retrieves a business connection by its ID.
func (b *BotInstance) GetBusinessConnection(ctx context.Context, connectionID string) (*converter.BusinessConnection, error) {
	if b.raw != nil {
		updates, err := b.raw.AccountGetBotBusinessConnection(ctx, connectionID)
		if err == nil {
			var conn *tg.BotBusinessConnection
			var users []tg.UserClass

			switch u := updates.(type) {
			case *tg.Updates:
				users = u.Users
				for _, upd := range u.Updates {
					if bbc, ok := upd.(*tg.UpdateBotBusinessConnect); ok {
						conn = &bbc.Connection
						break
					}
				}
			case *tg.UpdatesCombined:
				users = u.Users
				for _, upd := range u.Updates {
					if bbc, ok := upd.(*tg.UpdateBotBusinessConnect); ok {
						conn = &bbc.Connection
						break
					}
				}
			}

			if conn != nil {
				var user converter.User
				if len(users) > 0 {
					entCtx := converter.NewEntityContext(users, nil)
					if u := entCtx.GetUser(conn.UserID); u != nil {
						user = *u
					}
				}
				if user.ID == 0 {
					user = converter.User{
						ID:        conn.UserID,
						IsBot:     false,
						FirstName: "User",
					}
				}

				res := &converter.BusinessConnection{
					ID:         conn.ConnectionID,
					User:       user,
					UserChatID: conn.UserID,
					Date:       conn.Date,
					CanReply:   conn.Rights.Reply,
					IsEnabled:  !conn.Disabled,
				}

				b.busConnsMu.Lock()
				b.busConns[connectionID] = res
				b.busConnsMu.Unlock()

				return res, nil
			}
		}
	}

	// Fallback to in-memory cached connection if MTProto query fails or not found
	b.busConnsMu.RLock()
	cached, ok := b.busConns[connectionID]
	b.busConnsMu.RUnlock()
	if ok && cached != nil {
		return cached, nil
	}

	return nil, fmt.Errorf("business connection not found")
}

// resolvePeer tries in-memory cache first, falling back to Redis if needed.
func (b *BotInstance) resolvePeer(chatID int64) (tg.InputPeerClass, error) {
	peer, err := b.peers.ResolvePeer(chatID)
	if err == nil {
		return peer, nil
	}

	// Try Redis
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	hash, pType, ok, _ := b.redisStore.GetPeer(ctx, b.token, chatID)
	if ok {
		if pType == "channel" {
			channelID := -chatID - 1000000000000
			b.peers.SaveChannel(channelID, hash)
			return &tg.InputPeerChannel{ChannelID: channelID, AccessHash: hash}, nil
		}
		if pType == "user" {
			b.peers.SaveUser(chatID, hash)
			return &tg.InputPeerUser{UserID: chatID, AccessHash: hash}, nil
		}
	}

	return nil, err
}

// ------------------------------------------------------------------------------------------------
// Messages & Actions Methods
// ------------------------------------------------------------------------------------------------

// SendMessage sends a text message to a chat.
func (b *BotInstance) SendMessage(ctx context.Context, req *converter.SendMessageRequest) (*converter.Message, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}

	text := req.Text
	var entities []tg.MessageEntityClass

	if len(req.Entities) > 0 {
		entities = converter.ConvertEntities(req.Entities)
	} else if req.ParseMode != "" {
		cleanText, parsedEntities, err := converter.ParseTextFormatting(req.Text, req.ParseMode)
		if err == nil {
			text = cleanText
			entities = parsedEntities
		}
	}

	randomID, _ := rand.Int(rand.Reader, big.NewInt(1<<62))

	sendReq := &tg.MessagesSendMessageRequest{
		Peer:       peer,
		Message:    text,
		RandomID:   randomID.Int64(),
		Noforwards: req.ProtectContent,
		Entities:   entities,
	}

	if (req.LinkPreviewOptions != nil && req.LinkPreviewOptions.IsDisabled) || req.DisableWebPagePreview {
		sendReq.NoWebpage = true
	}

	if len(req.ReplyMarkup) > 0 {
		markup, _ := converter.ParseReplyMarkup(req.ReplyMarkup)
		sendReq.ReplyMarkup = markup
	}

	if req.ReplyParameters != nil && req.ReplyParameters.MessageID > 0 {
		sendReq.ReplyTo = &tg.InputReplyToMessage{
			ReplyToMsgID: int(req.ReplyParameters.MessageID),
			TopMsgID:     req.MessageThreadID,
		}
	} else if req.ReplyToMessageID > 0 {
		sendReq.ReplyTo = &tg.InputReplyToMessage{
			ReplyToMsgID: int(req.ReplyToMessageID),
			TopMsgID:     req.MessageThreadID,
		}
	} else if req.MessageThreadID > 0 {
		sendReq.ReplyTo = &tg.InputReplyToMessage{
			ReplyToMsgID: req.MessageThreadID,
			TopMsgID:     req.MessageThreadID,
		}
	}

	var updates tg.UpdatesClass
	if req.BusinessConnectionID != "" {
		var box tg.UpdatesBox
		err = b.client.Invoke(ctx, &tg.InvokeWithBusinessConnectionRequest{
			ConnectionID: req.BusinessConnectionID,
			Query:        sendReq,
		}, &box)
		if err != nil {
			return nil, fmt.Errorf("mtproto business send message: %w", err)
		}
		updates = box.Updates
	} else {
		updates, err = b.raw.MessagesSendMessage(ctx, sendReq)
		if err != nil {
			return nil, fmt.Errorf("mtproto send message: %w", err)
		}
	}

	msgID := extractSentMessageID(updates)

	return &converter.Message{
		MessageID: msgID,
		From:      b.GetMe(),
		Chat:      converter.Chat{ID: req.ChatID},
		Date:      int(time.Now().Unix()),
		Text:      text,
		Entities:  req.Entities,
	}, nil
}

// EditMessageText edits text of an existing message.
func (b *BotInstance) EditMessageText(ctx context.Context, req *converter.EditMessageTextRequest) (*converter.Message, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}

	text := req.Text
	var entities []tg.MessageEntityClass

	if len(req.Entities) > 0 {
		entities = converter.ConvertEntities(req.Entities)
	} else if req.ParseMode != "" {
		cleanText, parsedEntities, err := converter.ParseTextFormatting(req.Text, req.ParseMode)
		if err == nil {
			text = cleanText
			entities = parsedEntities
		}
	}

	editReq := &tg.MessagesEditMessageRequest{
		Peer:     peer,
		ID:       int(req.MessageID),
		Message:  text,
		Entities: entities,
	}

	if req.LinkPreviewOptions != nil && req.LinkPreviewOptions.IsDisabled {
		editReq.NoWebpage = true
	}

	if len(req.ReplyMarkup) > 0 {
		markup, _ := converter.ParseReplyMarkup(req.ReplyMarkup)
		editReq.ReplyMarkup = markup
	}

	if req.BusinessConnectionID != "" {
		var box tg.UpdatesBox
		err = b.client.Invoke(ctx, &tg.InvokeWithBusinessConnectionRequest{
			ConnectionID: req.BusinessConnectionID,
			Query:        editReq,
		}, &box)
		if err != nil {
			return nil, fmt.Errorf("mtproto business edit message text: %w", err)
		}
	} else {
		_, err = b.raw.MessagesEditMessage(ctx, editReq)
		if err != nil {
			return nil, fmt.Errorf("mtproto edit message text: %w", err)
		}
	}

	return &converter.Message{
		MessageID: req.MessageID,
		From:      b.GetMe(),
		Chat:      converter.Chat{ID: req.ChatID},
		Date:      int(time.Now().Unix()),
		Text:      text,
		Entities:  req.Entities,
	}, nil
}

// DeleteMessage deletes a message from a chat.
func (b *BotInstance) DeleteMessage(ctx context.Context, req *converter.DeleteMessageRequest) (bool, error) {
	if req.BusinessConnectionID != "" {
		var box tg.UpdatesBox
		deleteReq := &tg.MessagesDeleteMessagesRequest{
			Revoke: true,
			ID:     []int{int(req.MessageID)},
		}
		err := b.client.Invoke(ctx, &tg.InvokeWithBusinessConnectionRequest{
			ConnectionID: req.BusinessConnectionID,
			Query:        deleteReq,
		}, &box)
		if err != nil {
			return false, fmt.Errorf("mtproto business delete message: %w", err)
		}
		return true, nil
	}

	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}

	if channel, ok := peer.(*tg.InputPeerChannel); ok {
		_, err = b.raw.ChannelsDeleteMessages(ctx, &tg.ChannelsDeleteMessagesRequest{
			Channel: &tg.InputChannel{
				ChannelID:  channel.ChannelID,
				AccessHash: channel.AccessHash,
			},
			ID: []int{int(req.MessageID)},
		})
	} else {
		_, err = b.raw.MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{
			Revoke: true,
			ID:     []int{int(req.MessageID)},
		})
	}

	if err != nil {
		return false, fmt.Errorf("mtproto delete message: %w", err)
	}
	return true, nil
}

// SendChatAction sends a chat action (typing, upload_photo, etc.).
func (b *BotInstance) SendChatAction(ctx context.Context, req *converter.SendChatActionRequest) (bool, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}

	var action tg.SendMessageActionClass
	switch req.Action {
	case "typing":
		action = &tg.SendMessageTypingAction{}
	case "upload_photo":
		action = &tg.SendMessageUploadPhotoAction{}
	case "record_video":
		action = &tg.SendMessageRecordVideoAction{}
	case "upload_video":
		action = &tg.SendMessageUploadVideoAction{}
	case "record_voice", "record_audio":
		action = &tg.SendMessageRecordAudioAction{}
	case "upload_voice", "upload_audio":
		action = &tg.SendMessageUploadAudioAction{}
	case "upload_document":
		action = &tg.SendMessageUploadDocumentAction{}
	case "choose_sticker":
		action = &tg.SendMessageChooseStickerAction{}
	case "find_location":
		action = &tg.SendMessageGeoLocationAction{}
	case "record_video_note":
		action = &tg.SendMessageRecordRoundAction{}
	case "upload_video_note":
		action = &tg.SendMessageUploadRoundAction{}
	default:
		action = &tg.SendMessageTypingAction{}
	}

	_, err = b.raw.MessagesSetTyping(ctx, &tg.MessagesSetTypingRequest{
		Peer:     peer,
		Action:   action,
		TopMsgID: req.MessageThreadID,
	})
	if err != nil {
		return false, fmt.Errorf("mtproto set typing: %w", err)
	}
	return true, nil
}

// CopyMessage copies a message from one chat to another without forward attribution.
func (b *BotInstance) CopyMessage(ctx context.Context, req *converter.CopyMessageRequest) (*converter.MessageID, error) {
	toPeer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve to_peer: %w", err)
	}
	fromPeer, err := b.resolvePeer(req.FromChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve from_peer: %w", err)
	}

	randomID, _ := rand.Int(rand.Reader, big.NewInt(1<<62))

	updates, err := b.raw.MessagesForwardMessages(ctx, &tg.MessagesForwardMessagesRequest{
		DropAuthor: true,
		Noforwards: req.ProtectContent,
		FromPeer:   fromPeer,
		ToPeer:     toPeer,
		ID:         []int{int(req.MessageID)},
		RandomID:   []int64{randomID.Int64()},
	})
	if err != nil {
		return nil, fmt.Errorf("mtproto forward messages (copy): %w", err)
	}

	var newMsgID int64
	switch u := updates.(type) {
	case *tg.Updates:
		for _, upd := range u.Updates {
			if num, ok := upd.(*tg.UpdateMessageID); ok {
				newMsgID = int64(num.ID)
			}
		}
	case *tg.UpdateShortSentMessage:
		newMsgID = int64(u.ID)
	}

	// If custom caption or reply_markup was requested, edit the copied message
	if (req.Caption != "" || len(req.ReplyMarkup) > 0) && newMsgID > 0 {
		editReq := &tg.MessagesEditMessageRequest{
			Peer: toPeer,
			ID:   int(newMsgID),
		}
		if req.Caption != "" {
			editReq.Message = req.Caption
			if len(req.CaptionEntities) > 0 {
				editReq.Entities = converter.ConvertEntities(req.CaptionEntities)
			}
		}
		if len(req.ReplyMarkup) > 0 {
			markup, err := converter.ParseReplyMarkup(req.ReplyMarkup)
			if err != nil {
				return nil, fmt.Errorf("parse reply markup: %w", err)
			}
			editReq.ReplyMarkup = markup
		}
		if _, err := b.raw.MessagesEditMessage(ctx, editReq); err != nil {
			return nil, fmt.Errorf("edit copied message: %w", err)
		}
	}
	if newMsgID == 0 {
		return nil, fmt.Errorf("copy succeeded without a message id")
	}

	return &converter.MessageID{MessageID: newMsgID}, nil
}

// EditMessageCaption edits caption of a media message.
func (b *BotInstance) EditMessageCaption(ctx context.Context, req *converter.EditMessageCaptionRequest) (*converter.Message, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}

	caption := req.Caption
	var entities []tg.MessageEntityClass

	if len(req.CaptionEntities) > 0 {
		entities = converter.ConvertEntities(req.CaptionEntities)
	} else if req.ParseMode != "" {
		cleanCaption, parsedEntities, err := converter.ParseTextFormatting(req.Caption, req.ParseMode)
		if err == nil {
			caption = cleanCaption
			entities = parsedEntities
		}
	}

	editReq := &tg.MessagesEditMessageRequest{
		Peer:     peer,
		ID:       int(req.MessageID),
		Message:  caption,
		Entities: entities,
	}

	if len(req.ReplyMarkup) > 0 {
		markup, _ := converter.ParseReplyMarkup(req.ReplyMarkup)
		editReq.ReplyMarkup = markup
	}

	if req.BusinessConnectionID != "" {
		var box tg.UpdatesBox
		err = b.client.Invoke(ctx, &tg.InvokeWithBusinessConnectionRequest{
			ConnectionID: req.BusinessConnectionID,
			Query:        editReq,
		}, &box)
		if err != nil {
			return nil, fmt.Errorf("mtproto business edit message caption: %w", err)
		}
	} else {
		_, err = b.raw.MessagesEditMessage(ctx, editReq)
		if err != nil {
			return nil, fmt.Errorf("mtproto edit message caption: %w", err)
		}
	}

	return &converter.Message{
		MessageID: req.MessageID,
		From:      b.GetMe(),
		Chat:      converter.Chat{ID: req.ChatID},
		Date:      int(time.Now().Unix()),
		Caption:   caption,
	}, nil
}

// EditMessageReplyMarkup edits only the reply markup of a message.
func (b *BotInstance) EditMessageReplyMarkup(ctx context.Context, req *converter.EditMessageReplyMarkupRequest) (*converter.Message, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}

	markup, _ := converter.ParseReplyMarkup(req.ReplyMarkup)

	editReq := &tg.MessagesEditMessageRequest{
		Peer:        peer,
		ID:          int(req.MessageID),
		ReplyMarkup: markup,
	}

	if req.BusinessConnectionID != "" {
		var box tg.UpdatesBox
		err = b.client.Invoke(ctx, &tg.InvokeWithBusinessConnectionRequest{
			ConnectionID: req.BusinessConnectionID,
			Query:        editReq,
		}, &box)
		if err != nil {
			return nil, fmt.Errorf("mtproto business edit reply markup: %w", err)
		}
	} else {
		_, err = b.raw.MessagesEditMessage(ctx, editReq)
		if err != nil {
			return nil, fmt.Errorf("mtproto edit reply markup: %w", err)
		}
	}

	return &converter.Message{
		MessageID: req.MessageID,
		From:      b.GetMe(),
		Chat:      converter.Chat{ID: req.ChatID},
		Date:      int(time.Now().Unix()),
	}, nil
}

// EditMessageMedia edits the media of a message.
func (b *BotInstance) EditMessageMedia(ctx context.Context, req *converter.EditMessageMediaRequest) (*converter.Message, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}

	editReq := &tg.MessagesEditMessageRequest{
		Peer: peer,
		ID:   int(req.MessageID),
	}

	if len(req.ReplyMarkup) > 0 {
		markup, _ := converter.ParseReplyMarkup(req.ReplyMarkup)
		editReq.ReplyMarkup = markup
	}

	var mediaMap map[string]interface{}
	if err := json.Unmarshal(req.Media, &mediaMap); err == nil {
		if caption, ok := mediaMap["caption"].(string); ok && caption != "" {
			editReq.Message = caption
		}
	}

	_, err = b.raw.MessagesEditMessage(ctx, editReq)
	if err != nil {
		return nil, fmt.Errorf("mtproto edit media: %w", err)
	}

	return &converter.Message{
		MessageID: req.MessageID,
		From:      b.GetMe(),
		Chat:      converter.Chat{ID: req.ChatID},
		Date:      int(time.Now().Unix()),
	}, nil
}

// SendPhoto sends a photo to a chat with optional caption and markup.
func (b *BotInstance) SendPhoto(ctx context.Context, req *converter.SendPhotoRequest) (*converter.Message, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}

	caption := req.Caption
	var entities []tg.MessageEntityClass

	if len(req.CaptionEntities) > 0 {
		entities = converter.ConvertEntities(req.CaptionEntities)
	} else if req.ParseMode != "" {
		cleanCaption, parsedEntities, err := converter.ParseTextFormatting(req.Caption, req.ParseMode)
		if err == nil {
			caption = cleanCaption
			entities = parsedEntities
		}
	}

	var media tg.InputMediaClass

	if len(req.PhotoData) > 0 {
		fileName := req.PhotoFileName
		if fileName == "" {
			fileName = "photo.jpg"
		}
		u := uploader.NewUploader(b.raw)
		inputFile, err := u.FromBytes(ctx, fileName, req.PhotoData)
		if err != nil {
			return nil, fmt.Errorf("upload photo bytes: %w", err)
		}
		media = &tg.InputMediaUploadedPhoto{
			File:    inputFile,
			Spoiler: req.HasSpoiler,
		}
	} else if isMediaURL(req.Photo) {
		cachedPhoto := b.getPhotoCache(ctx, req.Photo)
		if cachedPhoto != nil {
			media = &tg.InputMediaPhoto{
				ID:      cachedPhoto,
				Spoiler: req.HasSpoiler,
			}
		} else {
			// Stream directly from HTTP into Telegram — no intermediate buffer.
			inputFile, mimeType, _, err := b.uploadFromURL(ctx, req.Photo)
			if err == nil {
				if isImage(mimeType) {
					media = &tg.InputMediaUploadedPhoto{
						File:    inputFile,
						Spoiler: req.HasSpoiler,
					}
				} else {
					// Non-image URL sent as sendPhoto → treat as document
					media = &tg.InputMediaUploadedDocument{
						File:     inputFile,
						MimeType: mimeType,
						Spoiler:  req.HasSpoiler,
						Attributes: []tg.DocumentAttributeClass{
							&tg.DocumentAttributeFilename{FileName: "photo"},
						},
					}
				}
			} else {
				b.logger.Warn("Streaming upload failed, falling back to Telegram external fetch",
					zap.String("url", req.Photo), zap.Error(err))
				media = &tg.InputMediaPhotoExternal{
					URL:     req.Photo,
					Spoiler: req.HasSpoiler,
				}
			}
		}
	} else {
		fid, err := fileid.DecodeFileID(req.Photo)
		if err == nil && (fid.Type == fileid.Photo || fid.Type == fileid.Thumbnail) {
			media = &tg.InputMediaPhoto{
				ID: &tg.InputPhoto{
					ID:            fid.ID,
					AccessHash:    fid.AccessHash,
					FileReference: fid.FileReference,
				},
				Spoiler: req.HasSpoiler,
			}
		} else if err == nil && fid.Type == fileid.Document {
			media = &tg.InputMediaDocument{
				ID: &tg.InputDocument{
					ID:            fid.ID,
					AccessHash:    fid.AccessHash,
					FileReference: fid.FileReference,
				},
				Spoiler: req.HasSpoiler,
			}
		} else {
			media = &tg.InputMediaPhotoExternal{
				URL:     req.Photo,
				Spoiler: req.HasSpoiler,
			}
		}
	}

	randomID, _ := rand.Int(rand.Reader, big.NewInt(1<<62))

	sendReq := &tg.MessagesSendMediaRequest{
		Peer:       peer,
		Media:      media,
		Message:    caption,
		Entities:   entities,
		RandomID:   randomID.Int64(),
		Noforwards: req.ProtectContent,
	}

	if len(req.ReplyMarkup) > 0 {
		markup, _ := converter.ParseReplyMarkup(req.ReplyMarkup)
		sendReq.ReplyMarkup = markup
	}

	if req.ReplyParameters != nil && req.ReplyParameters.MessageID > 0 {
		sendReq.ReplyTo = &tg.InputReplyToMessage{
			ReplyToMsgID: int(req.ReplyParameters.MessageID),
			TopMsgID:     req.MessageThreadID,
		}
	} else if req.ReplyToMessageID > 0 {
		sendReq.ReplyTo = &tg.InputReplyToMessage{
			ReplyToMsgID: int(req.ReplyToMessageID),
			TopMsgID:     req.MessageThreadID,
		}
	} else if req.MessageThreadID > 0 {
		sendReq.ReplyTo = &tg.InputReplyToMessage{
			ReplyToMsgID: req.MessageThreadID,
			TopMsgID:     req.MessageThreadID,
		}
	}

	var updates tg.UpdatesClass
	if req.BusinessConnectionID != "" {
		var box tg.UpdatesBox
		err = b.client.Invoke(ctx, &tg.InvokeWithBusinessConnectionRequest{
			ConnectionID: req.BusinessConnectionID,
			Query:        sendReq,
		}, &box)
		if err != nil {
			return nil, fmt.Errorf("mtproto business send photo: %w", err)
		}
		updates = box.Updates
	} else {
		updates, err = b.raw.MessagesSendMedia(ctx, sendReq)
		if err != nil {
			return nil, fmt.Errorf("mtproto send photo: %w", err)
		}
	}

	msgID := extractSentMessageID(updates)

	if req.Photo != "" {
		if u, ok := updates.(*tg.Updates); ok {
			for _, upd := range u.Updates {
				if msgUpd, ok := upd.(*tg.UpdateNewMessage); ok {
					if m, ok := msgUpd.Message.(*tg.Message); ok {
						if pMedia, ok := m.Media.(*tg.MessageMediaPhoto); ok {
							if photo, ok := pMedia.Photo.(*tg.Photo); ok {
								b.setPhotoCache(ctx, req.Photo, &tg.InputPhoto{
									ID:            photo.ID,
									AccessHash:    photo.AccessHash,
									FileReference: photo.FileReference,
								})
							}
						}
					}
				}
			}
		}
	}

	return &converter.Message{
		MessageID: msgID,
		From:      b.GetMe(),
		Chat:      converter.Chat{ID: req.ChatID},
		Date:      int(time.Now().Unix()),
		Caption:   caption,
		Photo: []converter.PhotoSize{
			{
				FileID:       req.Photo,
				FileUniqueID: "photo",
				Width:        800,
				Height:       600,
			},
		},
	}, nil
}

// SendVideo sends a video to a chat.
func (b *BotInstance) SendVideo(ctx context.Context, req *converter.SendVideoRequest) (*converter.Message, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}

	caption := req.Caption
	var entities []tg.MessageEntityClass

	if len(req.CaptionEntities) > 0 {
		entities = converter.ConvertEntities(req.CaptionEntities)
	} else if req.ParseMode != "" {
		cleanCaption, parsedEntities, err := converter.ParseTextFormatting(req.Caption, req.ParseMode)
		if err == nil {
			caption = cleanCaption
			entities = parsedEntities
		}
	}

	var media tg.InputMediaClass

	if len(req.VideoData) > 0 {
		fileName := req.VideoFileName
		if fileName == "" {
			fileName = "video.mp4"
		}
		u := uploader.NewUploader(b.raw)
		inputFile, err := u.FromBytes(ctx, fileName, req.VideoData)
		if err != nil {
			return nil, fmt.Errorf("upload video bytes: %w", err)
		}
		media = &tg.InputMediaUploadedDocument{
			File:     inputFile,
			MimeType: "video/mp4",
			Spoiler:  req.HasSpoiler,
			Attributes: []tg.DocumentAttributeClass{
				&tg.DocumentAttributeVideo{
					Duration:          float64(req.Duration),
					W:                 req.Width,
					H:                 req.Height,
					SupportsStreaming: req.SupportsStreaming,
				},
			},
		}
	} else if isMediaURL(req.Video) {
		if cachedDoc := b.getDocCache(ctx, req.Video); cachedDoc != nil {
			media = &tg.InputMediaDocument{
				ID:      cachedDoc,
				Spoiler: req.HasSpoiler,
			}
		} else {
			inputFile, mimeType, fileName, err := b.uploadFromURL(ctx, req.Video)
			if err == nil {
				media = &tg.InputMediaUploadedDocument{
					File:         inputFile,
					MimeType:     mimeType,
					Spoiler:      req.HasSpoiler,
					NosoundVideo: false,
					Attributes: []tg.DocumentAttributeClass{
						&tg.DocumentAttributeVideo{
							Duration:          float64(req.Duration),
							W:                 req.Width,
							H:                 req.Height,
							SupportsStreaming: req.SupportsStreaming,
						},
						&tg.DocumentAttributeFilename{FileName: fileName},
					},
				}
			} else {
				media = &tg.InputMediaDocumentExternal{
					URL:     req.Video,
					Spoiler: req.HasSpoiler,
				}
			}
		}
	} else {
		fid, err := fileid.DecodeFileID(req.Video)
		if err == nil {
			media = &tg.InputMediaDocument{
				ID: &tg.InputDocument{
					ID:            fid.ID,
					AccessHash:    fid.AccessHash,
					FileReference: fid.FileReference,
				},
				Spoiler: req.HasSpoiler,
			}
		} else {
			media = &tg.InputMediaDocumentExternal{
				URL:     req.Video,
				Spoiler: req.HasSpoiler,
			}
		}
	}

	randomID, _ := rand.Int(rand.Reader, big.NewInt(1<<62))

	sendReq := &tg.MessagesSendMediaRequest{
		Peer:       peer,
		Media:      media,
		Message:    caption,
		Entities:   entities,
		RandomID:   randomID.Int64(),
		Noforwards: req.ProtectContent,
	}

	if len(req.ReplyMarkup) > 0 {
		markup, _ := converter.ParseReplyMarkup(req.ReplyMarkup)
		sendReq.ReplyMarkup = markup
	}

	if req.ReplyParameters != nil && req.ReplyParameters.MessageID > 0 {
		sendReq.ReplyTo = &tg.InputReplyToMessage{
			ReplyToMsgID: int(req.ReplyParameters.MessageID),
			TopMsgID:     req.MessageThreadID,
		}
	} else if req.ReplyToMessageID > 0 {
		sendReq.ReplyTo = &tg.InputReplyToMessage{
			ReplyToMsgID: int(req.ReplyToMessageID),
			TopMsgID:     req.MessageThreadID,
		}
	}

	updates, err := b.raw.MessagesSendMedia(ctx, sendReq)
	if err != nil {
		return nil, fmt.Errorf("mtproto send video: %w", err)
	}

	msgID := extractSentMessageID(updates)

	if req.Video != "" {
		if u, ok := updates.(*tg.Updates); ok {
			for _, upd := range u.Updates {
				if msgUpd, ok := upd.(*tg.UpdateNewMessage); ok {
					if m, ok := msgUpd.Message.(*tg.Message); ok {
						if dMedia, ok := m.Media.(*tg.MessageMediaDocument); ok {
							if doc, ok := dMedia.Document.(*tg.Document); ok {
								b.setDocCache(ctx, req.Video, &tg.InputDocument{
									ID:            doc.ID,
									AccessHash:    doc.AccessHash,
									FileReference: doc.FileReference,
								})
							}
						}
					}
				}
			}
		}
	}

	return &converter.Message{
		MessageID: msgID,
		From:      b.GetMe(),
		Chat:      converter.Chat{ID: req.ChatID},
		Date:      int(time.Now().Unix()),
		Caption:   caption,
	}, nil
}

// SendDocument sends a general file/document to a chat.
func (b *BotInstance) SendDocument(ctx context.Context, req *converter.SendDocumentRequest) (*converter.Message, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}

	caption := req.Caption
	var entities []tg.MessageEntityClass

	if len(req.CaptionEntities) > 0 {
		entities = converter.ConvertEntities(req.CaptionEntities)
	} else if req.ParseMode != "" {
		cleanCaption, parsedEntities, err := converter.ParseTextFormatting(req.Caption, req.ParseMode)
		if err == nil {
			caption = cleanCaption
			entities = parsedEntities
		}
	}

	var media tg.InputMediaClass

	if len(req.DocumentData) > 0 {
		fileName := req.DocumentFileName
		if fileName == "" {
			fileName = "document.bin"
		}
		u := uploader.NewUploader(b.raw)
		inputFile, err := u.FromBytes(ctx, fileName, req.DocumentData)
		if err != nil {
			return nil, fmt.Errorf("upload document bytes: %w", err)
		}
		media = &tg.InputMediaUploadedDocument{
			File:     inputFile,
			MimeType: "application/octet-stream",
			Attributes: []tg.DocumentAttributeClass{
				&tg.DocumentAttributeFilename{FileName: fileName},
			},
		}
	} else if strings.HasPrefix(req.Document, "http://") || strings.HasPrefix(req.Document, "https://") {
		media = &tg.InputMediaDocumentExternal{
			URL: req.Document,
		}
	} else {
		fid, err := fileid.DecodeFileID(req.Document)
		if err == nil {
			media = &tg.InputMediaDocument{
				ID: &tg.InputDocument{
					ID:            fid.ID,
					AccessHash:    fid.AccessHash,
					FileReference: fid.FileReference,
				},
			}
		} else {
			media = &tg.InputMediaDocumentExternal{
				URL: req.Document,
			}
		}
	}

	randomID, _ := rand.Int(rand.Reader, big.NewInt(1<<62))

	sendReq := &tg.MessagesSendMediaRequest{
		Peer:       peer,
		Media:      media,
		Message:    caption,
		Entities:   entities,
		RandomID:   randomID.Int64(),
		Noforwards: req.ProtectContent,
	}

	if len(req.ReplyMarkup) > 0 {
		markup, _ := converter.ParseReplyMarkup(req.ReplyMarkup)
		sendReq.ReplyMarkup = markup
	}

	if req.ReplyParameters != nil && req.ReplyParameters.MessageID > 0 {
		sendReq.ReplyTo = &tg.InputReplyToMessage{
			ReplyToMsgID: int(req.ReplyParameters.MessageID),
			TopMsgID:     req.MessageThreadID,
		}
	} else if req.ReplyToMessageID > 0 {
		sendReq.ReplyTo = &tg.InputReplyToMessage{
			ReplyToMsgID: int(req.ReplyToMessageID),
			TopMsgID:     req.MessageThreadID,
		}
	}

	updates, err := b.raw.MessagesSendMedia(ctx, sendReq)
	if err != nil {
		return nil, fmt.Errorf("mtproto send document: %w", err)
	}

	msgID := extractSentMessageID(updates)

	return &converter.Message{
		MessageID: msgID,
		From:      b.GetMe(),
		Chat:      converter.Chat{ID: req.ChatID},
		Date:      int(time.Now().Unix()),
		Caption:   caption,
	}, nil
}

// SendVoice sends an audio voice note to a chat.
func (b *BotInstance) SendVoice(ctx context.Context, req *converter.SendVoiceRequest) (*converter.Message, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}

	var media tg.InputMediaClass
	if len(req.VoiceData) > 0 {
		fileName := req.VoiceFileName
		if fileName == "" {
			fileName = "voice.ogg"
		}
		u := uploader.NewUploader(b.raw)
		inputFile, err := u.FromBytes(ctx, fileName, req.VoiceData)
		if err != nil {
			return nil, fmt.Errorf("upload voice bytes: %w", err)
		}
		media = &tg.InputMediaUploadedDocument{
			File:     inputFile,
			MimeType: "audio/ogg",
			Attributes: []tg.DocumentAttributeClass{
				&tg.DocumentAttributeAudio{
					Voice:    true,
					Duration: req.Duration,
				},
			},
		}
	} else {
		media = &tg.InputMediaDocumentExternal{
			URL: req.Voice,
		}
	}

	randomID, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	sendReq := &tg.MessagesSendMediaRequest{
		Peer:       peer,
		Media:      media,
		Message:    req.Caption,
		RandomID:   randomID.Int64(),
		Noforwards: req.ProtectContent,
	}

	updates, err := b.raw.MessagesSendMedia(ctx, sendReq)
	if err != nil {
		return nil, fmt.Errorf("mtproto send voice: %w", err)
	}

	return &converter.Message{
		MessageID: extractSentMessageID(updates),
		From:      b.GetMe(),
		Chat:      converter.Chat{ID: req.ChatID},
		Date:      int(time.Now().Unix()),
		Caption:   req.Caption,
	}, nil
}

// SendVideoNote sends a rounded video note.
func (b *BotInstance) SendVideoNote(ctx context.Context, req *converter.SendVideoNoteRequest) (*converter.Message, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}

	var media tg.InputMediaClass
	if len(req.VideoNoteData) > 0 {
		fileName := req.VideoNoteFileName
		if fileName == "" {
			fileName = "video_note.mp4"
		}
		u := uploader.NewUploader(b.raw)
		inputFile, err := u.FromBytes(ctx, fileName, req.VideoNoteData)
		if err != nil {
			return nil, fmt.Errorf("upload video note bytes: %w", err)
		}
		media = &tg.InputMediaUploadedDocument{
			File:     inputFile,
			MimeType: "video/mp4",
			Attributes: []tg.DocumentAttributeClass{
				&tg.DocumentAttributeVideo{
					RoundMessage: true,
					Duration:     float64(req.Duration),
					W:            req.Length,
					H:            req.Length,
				},
			},
		}
	} else {
		media = &tg.InputMediaDocumentExternal{
			URL: req.VideoNote,
		}
	}

	randomID, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	sendReq := &tg.MessagesSendMediaRequest{
		Peer:       peer,
		Media:      media,
		RandomID:   randomID.Int64(),
		Noforwards: req.ProtectContent,
	}

	updates, err := b.raw.MessagesSendMedia(ctx, sendReq)
	if err != nil {
		return nil, fmt.Errorf("mtproto send video note: %w", err)
	}

	return &converter.Message{
		MessageID: extractSentMessageID(updates),
		From:      b.GetMe(),
		Chat:      converter.Chat{ID: req.ChatID},
		Date:      int(time.Now().Unix()),
	}, nil
}

// AnswerCallbackQuery answers an inline keyboard callback query.
func (b *BotInstance) AnswerCallbackQuery(ctx context.Context, req *converter.AnswerCallbackQueryRequest) (bool, error) {
	queryID, err := json.Number(req.CallbackQueryID).Int64()
	if err != nil {
		return false, fmt.Errorf("invalid callback_query_id: %w", err)
	}

	_, err = b.raw.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{
		QueryID:   queryID,
		Message:   req.Text,
		Alert:     req.ShowAlert,
		URL:       req.URL,
		CacheTime: req.CacheTime,
	})
	if err != nil {
		return false, fmt.Errorf("mtproto answer callback: %w", err)
	}
	return true, nil
}

// AnswerPreCheckoutQuery responds to a pre-checkout query for Telegram Payments.
func (b *BotInstance) AnswerPreCheckoutQuery(ctx context.Context, req *converter.AnswerPreCheckoutQueryRequest) (bool, error) {
	queryID, err := strconv.ParseInt(req.PreCheckoutQueryID, 10, 64)
	if err != nil {
		return false, fmt.Errorf("invalid pre_checkout_query_id: %w", err)
	}

	sendReq := &tg.MessagesSetBotPrecheckoutResultsRequest{
		QueryID: queryID,
		Success: req.OK,
		Error:   req.ErrorMessage,
	}

	res, err := b.raw.MessagesSetBotPrecheckoutResults(ctx, sendReq)
	if err != nil {
		return false, fmt.Errorf("mtproto answer pre-checkout query: %w", err)
	}
	return res, nil
}

// SetMyCommands updates the list of the bot's commands.
func (b *BotInstance) SetMyCommands(ctx context.Context, req *converter.SetMyCommandsRequest) (bool, error) {
	var commands []tg.BotCommand
	for _, c := range req.Commands {
		commands = append(commands, tg.BotCommand{
			Command:     c.Command,
			Description: c.Description,
		})
	}

	sendReq := &tg.BotsSetBotCommandsRequest{
		Commands: commands,
		Scope:    &tg.BotCommandScopeDefault{},
	}
	if req.LanguageCode != "" {
		sendReq.LangCode = req.LanguageCode
	}

	res, err := b.raw.BotsSetBotCommands(ctx, sendReq)
	if err != nil {
		return false, fmt.Errorf("mtproto set bot commands: %w", err)
	}
	return res, nil
}

// GetChatMember gets information about a member of a chat.
func (b *BotInstance) GetChatMember(ctx context.Context, req *converter.GetChatMemberRequest) (interface{}, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}
	switch chat := peer.(type) {
	case *tg.InputPeerChannel:
		participant, err := b.resolvePeer(req.UserID)
		if err != nil {
			return nil, fmt.Errorf("resolve user: %w", err)
		}
		res, err := b.raw.ChannelsGetParticipant(ctx, &tg.ChannelsGetParticipantRequest{
			Channel:     &tg.InputChannel{ChannelID: chat.ChannelID, AccessHash: chat.AccessHash},
			Participant: participant,
		})
		if err != nil {
			return nil, fmt.Errorf("get channel participant: %w", err)
		}
		entities := converter.NewEntityContext(res.Users, res.Chats)
		member := converter.ConvertChannelParticipant(res.Participant, req.UserID, entities)
		return &member, nil
	case *tg.InputPeerChat:
		res, err := b.raw.MessagesGetFullChat(ctx, chat.ChatID)
		if err != nil {
			return nil, fmt.Errorf("get full chat: %w", err)
		}
		full, ok := res.FullChat.(*tg.ChatFull)
		if !ok {
			return nil, fmt.Errorf("unexpected full chat response %T", res.FullChat)
		}
		participants, ok := full.Participants.(*tg.ChatParticipants)
		if !ok {
			member := converter.ConvertBasicParticipant(nil, req.UserID, converter.NewEntityContext(res.Users, res.Chats))
			return &member, nil
		}
		entities := converter.NewEntityContext(res.Users, res.Chats)
		for _, participant := range participants.Participants {
			member := converter.ConvertBasicParticipant(participant, req.UserID, entities)
			if member.User != nil && member.User.ID == req.UserID {
				return &member, nil
			}
		}
		member := converter.ConvertBasicParticipant(nil, req.UserID, entities)
		return &member, nil
	case *tg.InputPeerUser:
		member := converter.ChatMember{Status: "member", User: &converter.User{ID: req.UserID}}
		return &member, nil
	default:
		return nil, fmt.Errorf("unsupported chat peer %T", peer)
	}
}

// Stop terminates the MTProto client connection.
func (b *BotInstance) Stop() {
	if b.cancel != nil {
		b.cancel()
	}
}

func updateKind(upd *converter.Update) string {
	switch {
	case upd.Message != nil:
		if len(upd.Message.Photo) > 0 {
			return "photo"
		}
		if upd.Message.Video != nil {
			return "video"
		}
		if upd.Message.Document != nil {
			return "document"
		}
		if upd.Message.Voice != nil {
			return "voice"
		}
		return "message"
	case upd.EditedMessage != nil:
		return "edited_message"
	case upd.ChannelPost != nil:
		return "channel_post"
	case upd.EditedChannelPost != nil:
		return "edited_channel_post"
	case upd.BusinessConnection != nil:
		return "business_connection"
	case upd.BusinessMessage != nil:
		return "business_message"
	case upd.EditedBusinessMessage != nil:
		return "edited_business_message"
	case upd.DeletedBusinessMessages != nil:
		return "deleted_business_messages"
	case upd.CallbackQuery != nil:
		return "callback_query"
	case upd.InlineQuery != nil:
		return "inline_query"
	case upd.ChosenInlineResult != nil:
		return "chosen_inline_result"
	case upd.ShippingQuery != nil:
		return "shipping_query"
	case upd.PreCheckoutQuery != nil:
		return "pre_checkout_query"
	case upd.Poll != nil:
		return "poll"
	case upd.PollAnswer != nil:
		return "poll_answer"
	case upd.MyChatMember != nil:
		return "my_chat_member"
	case upd.ChatMember != nil:
		return "chat_member"
	case upd.ChatJoinRequest != nil:
		return "chat_join_request"
	case upd.ChatBoost != nil:
		return "chat_boost"
	case upd.MessageReaction != nil:
		return "message_reaction"
	case upd.MessageReactionCount != nil:
		return "message_reaction_count"
	case upd.PurchasedPaidMedia != nil:
		return "purchased_paid_media"
	default:
		return "update"
	}
}

func describeUpdateSender(upd *converter.Update) string {
	var user *converter.User
	var text string
	switch {
	case upd.Message != nil:
		user = upd.Message.From
		text = upd.Message.Text
		if text == "" && upd.Message.Caption != "" {
			text = upd.Message.Caption
		}
	case upd.BusinessMessage != nil:
		user = upd.BusinessMessage.From
		text = upd.BusinessMessage.Text
		if text == "" && upd.BusinessMessage.Caption != "" {
			text = upd.BusinessMessage.Caption
		}
	case upd.CallbackQuery != nil:
		user = &upd.CallbackQuery.From
		text = upd.CallbackQuery.Data
	}
	desc := ""
	if user != nil {
		if user.Username != "" {
			desc = "@" + user.Username
		} else if user.FirstName != "" {
			desc = user.FirstName
		} else {
			desc = fmt.Sprintf("id:%d", user.ID)
		}
	}
	if text != "" {
		if len(text) > 30 {
			text = text[:30] + "..."
		}
		if desc != "" {
			desc += fmt.Sprintf(" %q", text)
		} else {
			desc = fmt.Sprintf("%q", text)
		}
	}
	return desc
}

func extractSentMessageID(updates tg.UpdatesClass) int64 {
	switch u := updates.(type) {
	case *tg.Updates:
		for _, upd := range u.Updates {
			switch m := upd.(type) {
			case *tg.UpdateMessageID:
				return int64(m.ID)
			case *tg.UpdateNewMessage:
				if msg, ok := m.Message.(*tg.Message); ok {
					return int64(msg.ID)
				}
			case *tg.UpdateNewChannelMessage:
				if msg, ok := m.Message.(*tg.Message); ok {
					return int64(msg.ID)
				}
			}
		}
	case *tg.UpdateShortSentMessage:
		return int64(u.ID)
	case *tg.UpdateShortMessage:
		return int64(u.ID)
	case *tg.UpdatesCombined:
		for _, upd := range u.Updates {
			switch m := upd.(type) {
			case *tg.UpdateMessageID:
				return int64(m.ID)
			case *tg.UpdateNewMessage:
				if msg, ok := m.Message.(*tg.Message); ok {
					return int64(msg.ID)
				}
			case *tg.UpdateNewChannelMessage:
				if msg, ok := m.Message.(*tg.Message); ok {
					return int64(msg.ID)
				}
			}
		}
	}
	return 0
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}


// SendAudio sends an audio file.
func (b *BotInstance) SendAudio(ctx context.Context, req *converter.SendAudioRequest) (*converter.Message, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}

	caption := req.Caption
	var entities []tg.MessageEntityClass
	if len(req.CaptionEntities) > 0 {
		entities = converter.ConvertEntities(req.CaptionEntities)
	} else if req.ParseMode != "" {
		cleanCaption, parsedEntities, err := converter.ParseTextFormatting(req.Caption, req.ParseMode)
		if err == nil {
			caption = cleanCaption
			entities = parsedEntities
		}
	}

	var media tg.InputMediaClass
	if len(req.AudioData) > 0 {
		fileName := req.AudioFileName
		if fileName == "" {
			fileName = "audio.mp3"
		}
		u := uploader.NewUploader(b.raw)
		inputFile, err := u.FromBytes(ctx, fileName, req.AudioData)
		if err != nil {
			return nil, fmt.Errorf("upload audio bytes: %w", err)
		}
		media = &tg.InputMediaUploadedDocument{
			File:     inputFile,
			MimeType: "audio/mpeg",
			Attributes: []tg.DocumentAttributeClass{
				&tg.DocumentAttributeAudio{
					Duration:  req.Duration,
					Title:     req.Title,
					Performer: req.Performer,
				},
			},
		}
	} else if strings.HasPrefix(req.Audio, "http://") || strings.HasPrefix(req.Audio, "https://") {
		media = &tg.InputMediaDocumentExternal{
			URL: req.Audio,
		}
	} else {
		fid, err := fileid.DecodeFileID(req.Audio)
		if err == nil {
			media = &tg.InputMediaDocument{
				ID: &tg.InputDocument{
					ID:            fid.ID,
					AccessHash:    fid.AccessHash,
					FileReference: fid.FileReference,
				},
			}
		} else {
			media = &tg.InputMediaDocumentExternal{
				URL: req.Audio,
			}
		}
	}

	randomID, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	sendReq := &tg.MessagesSendMediaRequest{
		Peer:       peer,
		Media:      media,
		Message:    caption,
		Entities:   entities,
		RandomID:   randomID.Int64(),
		Noforwards: req.ProtectContent,
	}

	if len(req.ReplyMarkup) > 0 {
		markup, _ := converter.ParseReplyMarkup(req.ReplyMarkup)
		sendReq.ReplyMarkup = markup
	}

	updates, err := b.raw.MessagesSendMedia(ctx, sendReq)
	if err != nil {
		return nil, fmt.Errorf("mtproto send audio: %w", err)
	}

	return &converter.Message{
		MessageID: extractSentMessageID(updates),
		From:      b.GetMe(),
		Chat:      converter.Chat{ID: req.ChatID},
		Date:      int(time.Now().Unix()),
		Caption:   caption,
	}, nil
}

// SendSticker sends a sticker.
func (b *BotInstance) SendSticker(ctx context.Context, req *converter.SendStickerRequest) (*converter.Message, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}

	var media tg.InputMediaClass
	if len(req.StickerData) > 0 {
		fileName := req.StickerFileName
		if fileName == "" {
			fileName = "sticker.webp"
		}
		u := uploader.NewUploader(b.raw)
		inputFile, err := u.FromBytes(ctx, fileName, req.StickerData)
		if err != nil {
			return nil, fmt.Errorf("upload sticker bytes: %w", err)
		}
		media = &tg.InputMediaUploadedDocument{
			File:     inputFile,
			MimeType: "image/webp",
		}
	} else if strings.HasPrefix(req.Sticker, "http://") || strings.HasPrefix(req.Sticker, "https://") {
		media = &tg.InputMediaDocumentExternal{
			URL: req.Sticker,
		}
	} else {
		fid, err := fileid.DecodeFileID(req.Sticker)
		if err == nil {
			media = &tg.InputMediaDocument{
				ID: &tg.InputDocument{
					ID:            fid.ID,
					AccessHash:    fid.AccessHash,
					FileReference: fid.FileReference,
				},
			}
		} else {
			media = &tg.InputMediaDocumentExternal{
				URL: req.Sticker,
			}
		}
	}

	randomID, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	sendReq := &tg.MessagesSendMediaRequest{
		Peer:       peer,
		Media:      media,
		RandomID:   randomID.Int64(),
		Noforwards: req.ProtectContent,
	}

	if len(req.ReplyMarkup) > 0 {
		markup, _ := converter.ParseReplyMarkup(req.ReplyMarkup)
		sendReq.ReplyMarkup = markup
	}

	updates, err := b.raw.MessagesSendMedia(ctx, sendReq)
	if err != nil {
		return nil, fmt.Errorf("mtproto send sticker: %w", err)
	}

	return &converter.Message{
		MessageID: extractSentMessageID(updates),
		From:      b.GetMe(),
		Chat:      converter.Chat{ID: req.ChatID},
		Date:      int(time.Now().Unix()),
	}, nil
}

// SendAnimation sends an animation (GIF or silent MP4).
func (b *BotInstance) SendAnimation(ctx context.Context, req *converter.SendAnimationRequest) (*converter.Message, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}

	caption := req.Caption
	var entities []tg.MessageEntityClass
	if len(req.CaptionEntities) > 0 {
		entities = converter.ConvertEntities(req.CaptionEntities)
	} else if req.ParseMode != "" {
		cleanCaption, parsedEntities, err := converter.ParseTextFormatting(req.Caption, req.ParseMode)
		if err == nil {
			caption = cleanCaption
			entities = parsedEntities
		}
	}

	var media tg.InputMediaClass
	if len(req.AnimationData) > 0 {
		fileName := req.AnimationFileName
		if fileName == "" {
			fileName = "animation.mp4"
		}
		u := uploader.NewUploader(b.raw)
		inputFile, err := u.FromBytes(ctx, fileName, req.AnimationData)
		if err != nil {
			return nil, fmt.Errorf("upload animation bytes: %w", err)
		}
		media = &tg.InputMediaUploadedDocument{
			File:     inputFile,
			MimeType: "video/mp4",
			Attributes: []tg.DocumentAttributeClass{
				&tg.DocumentAttributeAnimated{},
				&tg.DocumentAttributeVideo{
					Duration: float64(req.Duration),
					W:        req.Width,
					H:        req.Height,
				},
			},
		}
	} else if isMediaURL(req.Animation) {
		if cachedDoc := b.getDocCache(ctx, req.Animation); cachedDoc != nil {
			media = &tg.InputMediaDocument{
				ID: cachedDoc,
			}
		} else {
			inputFile, mimeType, fileName, err := b.uploadFromURL(ctx, req.Animation)
			if err == nil {
				media = &tg.InputMediaUploadedDocument{
					File:     inputFile,
					MimeType: mimeType,
					Attributes: []tg.DocumentAttributeClass{
						&tg.DocumentAttributeAnimated{},
						&tg.DocumentAttributeVideo{
							Duration: float64(req.Duration),
							W:        req.Width,
							H:        req.Height,
						},
						&tg.DocumentAttributeFilename{FileName: fileName},
					},
				}
			} else {
				media = &tg.InputMediaDocumentExternal{
					URL: req.Animation,
				}
			}
		}
	} else {
		fid, err := fileid.DecodeFileID(req.Animation)
		if err == nil {
			media = &tg.InputMediaDocument{
				ID: &tg.InputDocument{
					ID:            fid.ID,
					AccessHash:    fid.AccessHash,
					FileReference: fid.FileReference,
				},
			}
		} else {
			media = &tg.InputMediaDocumentExternal{
				URL: req.Animation,
			}
		}
	}

	randomID, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	sendReq := &tg.MessagesSendMediaRequest{
		Peer:       peer,
		Media:      media,
		Message:    caption,
		Entities:   entities,
		RandomID:   randomID.Int64(),
		Noforwards: req.ProtectContent,
	}

	if len(req.ReplyMarkup) > 0 {
		markup, _ := converter.ParseReplyMarkup(req.ReplyMarkup)
		sendReq.ReplyMarkup = markup
	}

	updates, err := b.raw.MessagesSendMedia(ctx, sendReq)
	if err != nil {
		return nil, fmt.Errorf("mtproto send animation: %w", err)
	}

	msgID := extractSentMessageID(updates)

	if req.Animation != "" {
		if u, ok := updates.(*tg.Updates); ok {
			for _, upd := range u.Updates {
				if msgUpd, ok := upd.(*tg.UpdateNewMessage); ok {
					if m, ok := msgUpd.Message.(*tg.Message); ok {
						if dMedia, ok := m.Media.(*tg.MessageMediaDocument); ok {
							if doc, ok := dMedia.Document.(*tg.Document); ok {
								b.setDocCache(ctx, req.Animation, &tg.InputDocument{
									ID:            doc.ID,
									AccessHash:    doc.AccessHash,
									FileReference: doc.FileReference,
								})
							}
						}
					}
				}
			}
		}
	}

	return &converter.Message{
		MessageID: msgID,
		From:      b.GetMe(),
		Chat:      converter.Chat{ID: req.ChatID},
		Date:      int(time.Now().Unix()),
		Caption:   caption,
	}, nil
}

// GetFile returns basic info about a file by its file_id.
func (b *BotInstance) GetFile(ctx context.Context, fileIDStr string) (*converter.File, error) {
	fid, err := fileid.DecodeFileID(fileIDStr)
	if err != nil {
		return &converter.File{
			FileID:       fileIDStr,
			FileUniqueID: "unique_" + fileIDStr[:min(10, len(fileIDStr))],
			FileSize:     0,
			FilePath:     "files/" + fileIDStr,
		}, nil
	}

	return &converter.File{
		FileID:       fileIDStr,
		FileUniqueID: fmt.Sprintf("%d", fid.ID),
		FileSize:     0,
		FilePath:     "files/" + fileIDStr,
	}, nil
}

// DownloadFile streams a file by its file_id to the given writer.
func (b *BotInstance) DownloadFile(ctx context.Context, fileIDStr string, w io.Writer) error {
	fid, err := fileid.DecodeFileID(fileIDStr)
	if err != nil {
		return fmt.Errorf("decode file_id: %w", err)
	}

	loc, ok := fid.AsInputFileLocation()
	if !ok {
		return fmt.Errorf("cannot convert file_id to input file location")
	}

	d := downloader.NewDownloader()
	_, err = d.Download(b.raw, loc).Stream(ctx, w)
	return err
}

// GetUserProfilePhotos retrieves profile pictures of a user.
func (b *BotInstance) GetUserProfilePhotos(ctx context.Context, req *converter.GetUserProfilePhotosRequest) (*converter.UserProfilePhotos, error) {
	peer, err := b.resolvePeer(req.UserID)
	var inputUser tg.InputUserClass
	if err == nil {
		if pu, ok := peer.(*tg.InputPeerUser); ok {
			inputUser = &tg.InputUser{UserID: pu.UserID, AccessHash: pu.AccessHash}
		}
	}
	if inputUser == nil {
		inputUser = &tg.InputUser{UserID: req.UserID, AccessHash: 0}
	}

	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	photosRes, err := b.raw.PhotosGetUserPhotos(ctx, &tg.PhotosGetUserPhotosRequest{
		UserID: inputUser,
		Offset: req.Offset,
		Limit:  limit,
	})
	if err != nil {
		return &converter.UserProfilePhotos{
			TotalCount: 0,
			Photos:     [][]converter.PhotoSize{},
		}, nil
	}

	var photosList []tg.PhotoClass
	var totalCount int

	switch p := photosRes.(type) {
	case *tg.PhotosPhotos:
		photosList = p.Photos
		totalCount = len(p.Photos)
	case *tg.PhotosPhotosSlice:
		photosList = p.Photos
		totalCount = p.Count
	}

	resultPhotos := make([][]converter.PhotoSize, 0, len(photosList))
	for _, pc := range photosList {
		if photo, ok := pc.(*tg.Photo); ok {
			var sizes []converter.PhotoSize
			for _, sc := range photo.Sizes {
				switch s := sc.(type) {
				case *tg.PhotoSize:
					fid := fileid.FromPhoto(photo, []rune(s.Type)[0])
					encodedID, _ := fileid.EncodeFileID(fid)
					sizes = append(sizes, converter.PhotoSize{
						FileID:       encodedID,
						FileUniqueID: fmt.Sprintf("%d_%s", photo.ID, s.Type),
						Width:        s.W,
						Height:       s.H,
						FileSize:     s.Size,
					})
				case *tg.PhotoSizeProgressive:
					fid := fileid.FromPhoto(photo, []rune(s.Type)[0])
					encodedID, _ := fileid.EncodeFileID(fid)
					sizes = append(sizes, converter.PhotoSize{
						FileID:       encodedID,
						FileUniqueID: fmt.Sprintf("%d_%s", photo.ID, s.Type),
						Width:        s.W,
						Height:       s.H,
						FileSize:     0,
					})
				}
			}
			if len(sizes) > 0 {
				resultPhotos = append(resultPhotos, sizes)
			}
		}
	}

	return &converter.UserProfilePhotos{
		TotalCount: totalCount,
		Photos:     resultPhotos,
	}, nil
}

// resolveInputSingleMedia converts an InputMediaItem into an InputMediaClass suitable for MessagesSendMultiMedia.
// MTProto requires all items in sendMultiMedia to be InputMediaPhoto or InputMediaDocument (with server-side IDs).
func (b *BotInstance) resolveInputSingleMedia(ctx context.Context, peer tg.InputPeerClass, businessConnectionID string, item converter.InputMediaItem, files map[string][]byte, fileNames map[string]string) (tg.InputMediaClass, error) {
	isURL := strings.HasPrefix(item.Media, "http://") || strings.HasPrefix(item.Media, "https://")
	isAttach := strings.HasPrefix(item.Media, "attach://")
	isVideo := item.Type == "video" || item.Type == "animation"
	isAudio := item.Type == "audio"
	isDoc := item.Type == "document" || (!isVideo && !isAudio && item.Type != "photo" && item.Type != "")
	cacheKey := item.Type + "\x00" + item.Media
	if businessConnectionID != "" {
		// Media uploaded for a business connection can only be reused by that connection.
		cacheKey = businessConnectionID + "\x00" + cacheKey
	}

	// 1. Check in-memory & Redis cache if URL
	if isURL {
		if isVideo || isAudio || isDoc {
			if doc := b.getDocCache(ctx, cacheKey); doc != nil {
				return &tg.InputMediaDocument{
					ID:      doc,
					Spoiler: item.HasSpoiler,
				}, nil
			}
		} else {
			if photo := b.getPhotoCache(ctx, cacheKey); photo != nil {
				return &tg.InputMediaPhoto{
					ID:      photo,
					Spoiler: item.HasSpoiler,
				}, nil
			}
		}
	}

	// 2. Check if it's an existing file_id
	if !isURL && !isAttach {
		fid, err := fileid.DecodeFileID(item.Media)
		if err == nil {
			if fid.Type == fileid.Photo || fid.Type == fileid.Thumbnail {
				return &tg.InputMediaPhoto{
					ID: &tg.InputPhoto{
						ID:            fid.ID,
						AccessHash:    fid.AccessHash,
						FileReference: fid.FileReference,
					},
					Spoiler: item.HasSpoiler,
				}, nil
			}
			return &tg.InputMediaDocument{
				ID: &tg.InputDocument{
					ID:            fid.ID,
					AccessHash:    fid.AccessHash,
					FileReference: fid.FileReference,
				},
				Spoiler: item.HasSpoiler,
			}, nil
		}
	}

	// 3. If it's a photo URL, try MessagesUploadMedia with PhotoExternal first
	if isURL && !isVideo && !isAudio && !isDoc {
		res, err := b.raw.MessagesUploadMedia(ctx, &tg.MessagesUploadMediaRequest{
			BusinessConnectionID: businessConnectionID,
			Peer:                 peer,
			Media: &tg.InputMediaPhotoExternal{
				URL:     item.Media,
				Spoiler: item.HasSpoiler,
			},
		})
		if err == nil {
			if m, ok := res.(*tg.MessageMediaPhoto); ok {
				if p, ok := m.Photo.(*tg.Photo); ok {
					inputPhoto := &tg.InputPhoto{
						ID:            p.ID,
						AccessHash:    p.AccessHash,
						FileReference: p.FileReference,
					}
					b.setPhotoCache(ctx, cacheKey, inputPhoto)
					return &tg.InputMediaPhoto{
						ID:      inputPhoto,
						Spoiler: item.HasSpoiler,
					}, nil
				}
			}
		} else {
			b.logger.Warn("MessagesUploadMedia with PhotoExternal failed, falling back to download & upload",
				zap.String("url", item.Media),
				zap.Error(err),
			)
		}
	}

	// 4. Resolve and upload file data.
	// For attach:// the data is already in memory (multipart body), so FromBytes is fine.
	// For URL we stream directly HTTP → Telegram MTProto without buffering.
	if isURL {
		inputFile, detectedMime, detectedName, err := b.uploadFromURL(ctx, item.Media)
		if err != nil {
			return nil, fmt.Errorf("streaming upload from URL %q: %w", item.Media, err)
		}

		if isVideo || isAudio || isDoc || !isImage(detectedMime) {
			// Document branch
			mimeType := detectedMime
			fileName := detectedName
			if isVideo {
				if mimeType == "" || mimeType == "application/octet-stream" {
					mimeType = "video/mp4"
				}
				if fileName == "" {
					fileName = "video.mp4"
				}
			} else if isAudio {
				if mimeType == "" || mimeType == "application/octet-stream" {
					mimeType = "audio/mpeg"
				}
				if fileName == "" {
					fileName = "audio.mp3"
				}
			} else {
				if fileName == "" {
					fileName = "document.dat"
				}
			}

			var attrs []tg.DocumentAttributeClass
			if isVideo {
				attrs = append(attrs,
					&tg.DocumentAttributeVideo{
						Duration:          float64(item.Duration),
						W:                 item.Width,
						H:                 item.Height,
						SupportsStreaming: true,
					},
					&tg.DocumentAttributeFilename{FileName: fileName},
				)
			} else if isAudio {
				attrs = append(attrs,
					&tg.DocumentAttributeAudio{},
					&tg.DocumentAttributeFilename{FileName: fileName},
				)
			} else {
				attrs = append(attrs, &tg.DocumentAttributeFilename{FileName: fileName})
			}

			res, err := b.raw.MessagesUploadMedia(ctx, &tg.MessagesUploadMediaRequest{
				BusinessConnectionID: businessConnectionID,
				Peer:                 peer,
				Media: &tg.InputMediaUploadedDocument{
					File:         inputFile,
					MimeType:     mimeType,
					NosoundVideo: isVideo,
					Spoiler:      item.HasSpoiler,
					Attributes:   attrs,
				},
			})
			if err != nil {
				return nil, fmt.Errorf("upload document to Telegram: %w", err)
			}
			if m, ok := res.(*tg.MessageMediaDocument); ok {
				if d, ok := m.Document.(*tg.Document); ok {
					inputDoc := &tg.InputDocument{
						ID:            d.ID,
						AccessHash:    d.AccessHash,
						FileReference: d.FileReference,
					}
					b.setDocCache(ctx, cacheKey, inputDoc)
					return &tg.InputMediaDocument{
						ID:      inputDoc,
						Spoiler: item.HasSpoiler,
					}, nil
				}
			}
			return nil, fmt.Errorf("unexpected media type for document upload: %T", res)
		}

		// Photo branch (image/* detected from Content-Type)
		res, err := b.raw.MessagesUploadMedia(ctx, &tg.MessagesUploadMediaRequest{
			BusinessConnectionID: businessConnectionID,
			Peer:                 peer,
			Media: &tg.InputMediaUploadedPhoto{
				File:    inputFile,
				Spoiler: item.HasSpoiler,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("upload photo to Telegram: %w", err)
		}
		if m, ok := res.(*tg.MessageMediaPhoto); ok {
			if p, ok := m.Photo.(*tg.Photo); ok {
				inputPhoto := &tg.InputPhoto{
					ID:            p.ID,
					AccessHash:    p.AccessHash,
					FileReference: p.FileReference,
				}
				b.setPhotoCache(ctx, cacheKey, inputPhoto)
				return &tg.InputMediaPhoto{
					ID:      inputPhoto,
					Spoiler: item.HasSpoiler,
				}, nil
			}
		}
		return nil, fmt.Errorf("unexpected media type for photo upload: %T", res)
	}

	// attach:// branch — data already in RAM from multipart
	var data []byte
	var fileName string
	{
		attachKey := strings.TrimPrefix(item.Media, "attach://")
		var ok bool
		data, ok = files[attachKey]
		if !ok {
			return nil, fmt.Errorf("attachment %q not found in request files", attachKey)
		}
		if fileNames != nil {
			fileName = fileNames[attachKey]
		}
	}

	u := uploader.NewUploader(b.raw)
	if isVideo || isAudio || isDoc {
		mimeType := "video/mp4"
		if fileName == "" {
			if isVideo {
				fileName = "video.mp4"
			} else if isAudio {
				fileName = "audio.mp3"
			} else {
				fileName = "document.dat"
			}
		}

		var attrs []tg.DocumentAttributeClass
		if isVideo {
			attrs = append(attrs,
				&tg.DocumentAttributeVideo{
					Duration:          float64(item.Duration),
					W:                 item.Width,
					H:                 item.Height,
					SupportsStreaming: true,
				},
				&tg.DocumentAttributeFilename{FileName: fileName},
			)
		} else if isAudio {
			mimeType = "audio/mpeg"
			attrs = append(attrs,
				&tg.DocumentAttributeAudio{},
				&tg.DocumentAttributeFilename{FileName: fileName},
			)
		} else {
			mimeType = "application/octet-stream"
			attrs = append(attrs, &tg.DocumentAttributeFilename{FileName: fileName})
		}

		inputFile, err := u.FromBytes(ctx, fileName, data)
		if err != nil {
			return nil, fmt.Errorf("upload bytes: %w", err)
		}

		res, err := b.raw.MessagesUploadMedia(ctx, &tg.MessagesUploadMediaRequest{
			BusinessConnectionID: businessConnectionID,
			Peer:                 peer,
			Media: &tg.InputMediaUploadedDocument{
				File:         inputFile,
				MimeType:     mimeType,
				NosoundVideo: isVideo,
				Spoiler:      item.HasSpoiler,
				Attributes:   attrs,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("upload document to Telegram: %w", err)
		}

		if m, ok := res.(*tg.MessageMediaDocument); ok {
			if d, ok := m.Document.(*tg.Document); ok {
				inputDoc := &tg.InputDocument{
					ID:            d.ID,
					AccessHash:    d.AccessHash,
					FileReference: d.FileReference,
				}
				return &tg.InputMediaDocument{
					ID:      inputDoc,
					Spoiler: item.HasSpoiler,
				}, nil
			}
		}
		return nil, fmt.Errorf("unexpected media type for document upload: %T", res)
	}

	// Photo upload (attach://)
	if fileName == "" {
		fileName = "photo.jpg"
	}
	inputFile, err := u.FromBytes(ctx, fileName, data)
	if err != nil {
		return nil, fmt.Errorf("upload photo bytes: %w", err)
	}

	res, err := b.raw.MessagesUploadMedia(ctx, &tg.MessagesUploadMediaRequest{
		BusinessConnectionID: businessConnectionID,
		Peer:                 peer,
		Media: &tg.InputMediaUploadedPhoto{
			File:    inputFile,
			Spoiler: item.HasSpoiler,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("upload photo to Telegram: %w", err)
	}

	if m, ok := res.(*tg.MessageMediaPhoto); ok {
		if p, ok := m.Photo.(*tg.Photo); ok {
			inputPhoto := &tg.InputPhoto{
				ID:            p.ID,
				AccessHash:    p.AccessHash,
				FileReference: p.FileReference,
			}
			return &tg.InputMediaPhoto{
				ID:      inputPhoto,
				Spoiler: item.HasSpoiler,
			}, nil
		}
	}

	return nil, fmt.Errorf("unexpected media type for photo upload: %T", res)
}

// SendMediaGroup sends a group of photos, videos, documents or audios as an album.
func (b *BotInstance) SendMediaGroup(ctx context.Context, req *converter.SendMediaGroupRequest) ([]*converter.Message, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}

	var items []converter.InputMediaItem
	if err := json.Unmarshal(req.Media, &items); err != nil {
		return nil, fmt.Errorf("unmarshal media group: %w", err)
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("media group must not be empty")
	}

	var multiMedia []tg.InputSingleMedia
	for _, item := range items {
		media, err := b.resolveInputSingleMedia(ctx, peer, req.BusinessConnectionID, item, req.Files, req.FileNames)
		if err != nil {
			return nil, fmt.Errorf("resolve media item (%s): %w", item.Type, err)
		}

		caption := item.Caption
		var entities []tg.MessageEntityClass
		if len(item.CaptionEntities) > 0 {
			entities = converter.ConvertEntities(item.CaptionEntities)
		} else if item.ParseMode != "" {
			cleanCaption, parsedEntities, err := converter.ParseTextFormatting(item.Caption, item.ParseMode)
			if err == nil {
				caption = cleanCaption
				entities = parsedEntities
			}
		}

		randomID, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
		multiMedia = append(multiMedia, tg.InputSingleMedia{
			Media:    media,
			RandomID: randomID.Int64(),
			Message:  caption,
			Entities: entities,
		})
	}

	sendReq := &tg.MessagesSendMultiMediaRequest{
		Peer:       peer,
		MultiMedia: multiMedia,
		Noforwards: req.ProtectContent,
		Silent:     req.DisableNotification,
	}

	var updates tg.UpdatesClass
	if req.BusinessConnectionID != "" {
		var box tg.UpdatesBox
		err = b.client.Invoke(ctx, &tg.InvokeWithBusinessConnectionRequest{
			ConnectionID: req.BusinessConnectionID,
			Query:        sendReq,
		}, &box)
		if err != nil {
			return nil, fmt.Errorf("mtproto business send media group: %w", err)
		}
		updates = box.Updates
	} else {
		updates, err = b.raw.MessagesSendMultiMedia(ctx, sendReq)
		if err != nil {
			return nil, fmt.Errorf("mtproto send media group: %w", err)
		}
	}

	var messages []*converter.Message
	now := int(time.Now().Unix())
	switch u := updates.(type) {
	case *tg.Updates:
		for _, upd := range u.Updates {
			if msgUpd, ok := upd.(*tg.UpdateNewMessage); ok {
				if m, ok := msgUpd.Message.(*tg.Message); ok {
					messages = append(messages, &converter.Message{
						MessageID: int64(m.ID),
						From:      b.GetMe(),
						Chat:      converter.Chat{ID: req.ChatID},
						Date:      now,
					})
				}
			}
		}
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("send media group succeeded without message ids")
	}

	return messages, nil
}

// SetBusinessAccountBio updates the bio of a business connection account.
func (b *BotInstance) SetBusinessAccountBio(ctx context.Context, connectionID, bio string) (bool, error) {
	request := &tg.AccountUpdateProfileRequest{}
	request.SetAbout(bio)
	var user tg.UserBox
	err := b.client.Invoke(ctx, &tg.InvokeWithBusinessConnectionRequest{ConnectionID: connectionID, Query: request}, &user)
	return err == nil, err
}

// SetBusinessAccountName updates the first name and last name of a business connection account.
func (b *BotInstance) SetBusinessAccountName(ctx context.Context, connectionID, firstName, lastName string) (bool, error) {
	request := &tg.AccountUpdateProfileRequest{}
	request.SetFirstName(firstName)
	request.SetLastName(lastName)
	var user tg.UserBox
	err := b.client.Invoke(ctx, &tg.InvokeWithBusinessConnectionRequest{ConnectionID: connectionID, Query: request}, &user)
	return err == nil, err
}

// SetBusinessAccountUsername changes or clears the username of a connected business account.
func (b *BotInstance) SetBusinessAccountUsername(ctx context.Context, req *converter.SetBusinessAccountUsernameRequest) (bool, error) {
	var result tg.UserBox
	if err := b.client.Invoke(ctx, &tg.InvokeWithBusinessConnectionRequest{
		ConnectionID: req.BusinessConnectionID, Query: &tg.AccountUpdateUsernameRequest{Username: req.Username},
	}, &result); err != nil {
		return false, err
	}
	return true, nil
}

// RemoveBusinessAccountProfilePhoto removes the main or public profile photo.
func (b *BotInstance) RemoveBusinessAccountProfilePhoto(ctx context.Context, req *converter.RemoveBusinessAccountProfilePhotoRequest) (bool, error) {
	request := &tg.PhotosUpdateProfilePhotoRequest{Fallback: req.IsPublic, ID: &tg.InputPhotoEmpty{}}
	var result tg.PhotosPhoto
	if err := b.client.Invoke(ctx, &tg.InvokeWithBusinessConnectionRequest{
		ConnectionID: req.BusinessConnectionID, Query: request,
	}, &result); err != nil {
		return false, err
	}
	return true, nil
}

// PostStory posts a story on behalf of a business account or bot.
func (b *BotInstance) PostStory(ctx context.Context, req *converter.PostStoryRequest) (interface{}, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, err
	}
	var content struct {
		Type     string `json:"type"`
		Photo    string `json:"photo"`
		Video    string `json:"video"`
		Duration int    `json:"duration"`
		Width    int    `json:"width"`
		Height   int    `json:"height"`
	}
	if err := json.Unmarshal(req.Content, &content); err != nil {
		return nil, fmt.Errorf("invalid story content: %w", err)
	}
	mediaID := content.Photo
	if content.Type == "video" {
		mediaID = content.Video
	}
	media, err := b.resolveInputSingleMedia(ctx, peer, req.BusinessConnectionID, converter.InputMediaItem{Type: content.Type,
		Media: mediaID, Duration: content.Duration, Width: content.Width, Height: content.Height}, req.Files, req.FileNames)
	if err != nil {
		return nil, err
	}
	caption, entities, err := inlineMessageEntities(req.Caption, req.ParseMode, req.CaptionEntities)
	if err != nil {
		return nil, err
	}
	randomID, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	request := &tg.StoriesSendStoryRequest{Peer: peer, Media: media, PrivacyRules: []tg.InputPrivacyRuleClass{&tg.InputPrivacyValueAllowAll{}},
		RandomID: randomID.Int64(), Pinned: req.PostToChatPage, Noforwards: req.ProtectContent}
	if caption != "" {
		request.SetCaption(caption)
		request.SetEntities(entities)
	}
	if req.ActivePeriod != 0 {
		request.SetPeriod(req.ActivePeriod)
	}
	var box tg.UpdatesBox
	if err := b.client.Invoke(ctx, &tg.InvokeWithBusinessConnectionRequest{ConnectionID: req.BusinessConnectionID, Query: request}, &box); err != nil {
		return nil, err
	}
	storyID := 0
	switch updates := box.Updates.(type) {
	case *tg.Updates:
		for _, update := range updates.Updates {
			switch value := update.(type) {
			case *tg.UpdateStoryID:
				storyID = value.ID
			case *tg.UpdateStory:
				storyID = value.Story.GetID()
			}
		}
	case *tg.UpdatesCombined:
		for _, update := range updates.Updates {
			switch value := update.(type) {
			case *tg.UpdateStoryID:
				storyID = value.ID
			case *tg.UpdateStory:
				storyID = value.Story.GetID()
			}
		}
	}
	if storyID == 0 {
		return nil, fmt.Errorf("story sent without story id")
	}
	return map[string]interface{}{"id": storyID, "chat": converter.Chat{ID: req.ChatID}, "date": int(time.Now().Unix())}, nil
}

// DeleteStory deletes a story previously posted through a business connection.
func (b *BotInstance) DeleteStory(ctx context.Context, req *converter.DeleteStoryRequest) (bool, error) {
	connection, err := b.GetBusinessConnection(ctx, req.BusinessConnectionID)
	if err != nil {
		return false, err
	}
	peer, err := b.resolvePeer(connection.UserChatID)
	if err != nil {
		return false, fmt.Errorf("resolve business peer: %w", err)
	}
	request := &tg.StoriesDeleteStoriesRequest{Peer: peer, ID: []int{req.StoryID}}
	var result tg.IntVector
	if err := b.client.Invoke(ctx, &tg.InvokeWithBusinessConnectionRequest{
		ConnectionID: req.BusinessConnectionID, Query: request,
	}, &result); err != nil {
		return false, err
	}
	return true, nil
}

// ReadBusinessMessage marks messages through message_id as read for a business account.
func (b *BotInstance) ReadBusinessMessage(ctx context.Context, req *converter.ReadBusinessMessageRequest) (bool, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}
	request := &tg.MessagesReadHistoryRequest{Peer: peer, MaxID: int(req.MessageID)}
	var result tg.MessagesAffectedMessages
	if err := b.client.Invoke(ctx, &tg.InvokeWithBusinessConnectionRequest{
		ConnectionID: req.BusinessConnectionID, Query: request,
	}, &result); err != nil {
		return false, err
	}
	return true, nil
}
