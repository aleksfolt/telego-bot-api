package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gotd/td/session"
	"github.com/redis/go-redis/v9"
)

// UpdateEntry is a single update read from the Redis Stream.
type UpdateEntry struct {
	// StreamID is the Redis Stream entry ID (e.g. "1234567890123-0").
	StreamID string
	// UpdateID is the Bot API update_id.
	UpdateID int
	// Payload is the raw JSON-encoded converter.Update.
	Payload []byte
}

const (
	// streamMaxLen is the approximate cap for updates per bot stream (MAXLEN ~).
	streamMaxLen = 10_000
)

// updateStreamKey returns the Redis Stream key for a bot by its user ID.
func updateStreamKey(botID int64) string {
	return fmt.Sprintf("telego:updates:%d", botID)
}

// updateIDKey returns the Redis key for the monotonic update_id counter.
func updateIDKey(botID int64) string {
	return fmt.Sprintf("telego:update_id:%d", botID)
}

// WebhookData stores webhook configuration for a bot.
type WebhookData struct {
	URL              string    `json:"url"`
	SecretToken      string    `json:"secret_token,omitempty"`
	MaxConnections   int       `json:"max_connections,omitempty"`
	AllowedUpdates   []string  `json:"allowed_updates,omitempty"`
	LastErrorDate    int64     `json:"last_error_date,omitempty"`
	LastErrorMessage string    `json:"last_error_message,omitempty"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// RedisStore provides persistent storage for bot sessions, webhooks, and peer caches.
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore initializes connection to Redis.
func NewRedisStore(addr, password string, db int) *RedisStore {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	return &RedisStore{
		client: rdb,
	}
}

// Close terminates all active connections in the Redis connection pool.
func (r *RedisStore) Close() error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

// Ping checks Redis connectivity.
func (r *RedisStore) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// RegisterBot adds a bot token to the active registry set.
func (r *RedisStore) RegisterBot(ctx context.Context, token string) error {
	return r.client.SAdd(ctx, "telego:bots", token).Err()
}

// UnregisterBot removes a bot token from the active registry set.
func (r *RedisStore) UnregisterBot(ctx context.Context, token string) error {
	return r.client.SRem(ctx, "telego:bots", token).Err()
}

// GetRegisteredBots returns all registered bot tokens.
func (r *RedisStore) GetRegisteredBots(ctx context.Context) ([]string, error) {
	return r.client.SMembers(ctx, "telego:bots").Result()
}

// SaveWebhook saves webhook configuration for a bot.
func (r *RedisStore) SaveWebhook(ctx context.Context, token string, data *WebhookData) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	pipe := r.client.Pipeline()
	pipe.Set(ctx, fmt.Sprintf("telego:webhook:%s", token), raw, 0)
	pipe.SAdd(ctx, "telego:webhooks", token)
	_, err = pipe.Exec(ctx)
	return err
}

// GetWebhook returns webhook configuration for a bot.
func (r *RedisStore) GetWebhook(ctx context.Context, token string) (*WebhookData, error) {
	val, err := r.client.Get(ctx, fmt.Sprintf("telego:webhook:%s", token)).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var data WebhookData
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		return nil, err
	}
	return &data, nil
}

// DeleteWebhook removes webhook configuration.
func (r *RedisStore) DeleteWebhook(ctx context.Context, token string) error {
	pipe := r.client.Pipeline()
	pipe.Del(ctx, fmt.Sprintf("telego:webhook:%s", token))
	pipe.SRem(ctx, "telego:webhooks", token)
	_, err := pipe.Exec(ctx)
	return err
}

// GetWebhookTokens returns tokens of all bots with active webhooks.
func (r *RedisStore) GetWebhookTokens(ctx context.Context) ([]string, error) {
	tokens, err := r.client.SMembers(ctx, "telego:webhooks").Result()
	if err == nil && len(tokens) > 0 {
		return tokens, nil
	}

	// Fallback: scan telego:webhook:* keys for existing databases
	var result []string
	var cursor uint64
	for {
		keys, nextCursor, err := r.client.Scan(ctx, cursor, "telego:webhook:*", 100).Result()
		if err != nil {
			break
		}
		for _, k := range keys {
			t := strings.TrimPrefix(k, "telego:webhook:")
			if t != "" && t != "telego:webhooks" {
				result = append(result, t)
				_ = r.client.SAdd(ctx, "telego:webhooks", t)
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return result, nil
}

// UpdateWebhookDeliveryError records the last webhook delivery failure.
func (r *RedisStore) UpdateWebhookDeliveryError(ctx context.Context, token string, errMsg string) error {
	wh, err := r.GetWebhook(ctx, token)
	if err != nil || wh == nil {
		return err
	}
	wh.LastErrorDate = time.Now().Unix()
	wh.LastErrorMessage = errMsg
	return r.SaveWebhook(ctx, token, wh)
}

// PendingUpdatesCount returns the number of pending updates in the Redis Stream.
func (r *RedisStore) PendingUpdatesCount(ctx context.Context, botID int64) (int, error) {
	stream := updateStreamKey(botID)
	length, err := r.client.XLen(ctx, stream).Result()
	if err != nil {
		return 0, err
	}
	return int(length), nil
}

// SavePeer stores chat_id -> access_hash and peer_type in Redis.
func (r *RedisStore) SavePeer(ctx context.Context, token string, chatID, accessHash int64, peerType string) error {
	val := fmt.Sprintf("%d:%s", accessHash, peerType)
	return r.client.HSet(ctx, fmt.Sprintf("telego:peers:%s", token), strconv.FormatInt(chatID, 10), val).Err()
}

// GetPeer retrieves access_hash and peer_type for a chat_id.
func (r *RedisStore) GetPeer(ctx context.Context, token string, chatID int64) (int64, string, bool, error) {
	val, err := r.client.HGet(ctx, fmt.Sprintf("telego:peers:%s", token), strconv.FormatInt(chatID, 10)).Result()
	if errors.Is(err, redis.Nil) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, err
	}

	parts := strings.SplitN(val, ":", 2)
	if len(parts) != 2 {
		return 0, "", false, nil
	}

	hash, _ := strconv.ParseInt(parts[0], 10, 64)
	return hash, parts[1], true, nil
}

// ------------------------------------------------------------------------------------------------
// Media Reference Cache (InputPhoto / InputDocument)
// ------------------------------------------------------------------------------------------------

// cachedInputPhoto is the serialisable form of tg.InputPhoto.
type cachedInputPhoto struct {
	ID            int64  `json:"id"`
	AccessHash    int64  `json:"ah"`
	FileReference []byte `json:"fr"`
}

// cachedInputDoc is the serialisable form of tg.InputDocument.
type cachedInputDoc struct {
	ID            int64  `json:"id"`
	AccessHash    int64  `json:"ah"`
	FileReference []byte `json:"fr"`
}

const mediaCacheTTL = 7 * 24 * time.Hour

// mediaCacheKey returns the Redis Hash key that stores all photo cache entries for a bot.
func mediaCacheKey(botID int64) string {
	return fmt.Sprintf("telego:mediacache:photo:%d", botID)
}

// docCacheKey returns the Redis Hash key that stores all document cache entries for a bot.
func docCacheKey(botID int64) string {
	return fmt.Sprintf("telego:mediacache:doc:%d", botID)
}

// SavePhotoCache persists a URL→InputPhoto mapping to Redis for the given bot.
// urlKey is the raw URL string (used as hash field).
func (r *RedisStore) SavePhotoCache(ctx context.Context, botID int64, urlKey string, id, accessHash int64, fileRef []byte) error {
	val, err := json.Marshal(cachedInputPhoto{ID: id, AccessHash: accessHash, FileReference: fileRef})
	if err != nil {
		return err
	}
	pipe := r.client.Pipeline()
	pipe.HSet(ctx, mediaCacheKey(botID), urlKey, val)
	pipe.Expire(ctx, mediaCacheKey(botID), mediaCacheTTL)
	_, err = pipe.Exec(ctx)
	return err
}

// GetPhotoCache retrieves an InputPhoto from Redis for the given bot and URL key.
// Returns (id, accessHash, fileRef, true) on hit, (0, 0, nil, false) on miss.
func (r *RedisStore) GetPhotoCache(ctx context.Context, botID int64, urlKey string) (int64, int64, []byte, bool) {
	val, err := r.client.HGet(ctx, mediaCacheKey(botID), urlKey).Bytes()
	if err != nil {
		return 0, 0, nil, false
	}
	var c cachedInputPhoto
	if err := json.Unmarshal(val, &c); err != nil {
		return 0, 0, nil, false
	}
	return c.ID, c.AccessHash, c.FileReference, true
}

// SaveDocCache persists a URL→InputDocument mapping to Redis for the given bot.
func (r *RedisStore) SaveDocCache(ctx context.Context, botID int64, urlKey string, id, accessHash int64, fileRef []byte) error {
	val, err := json.Marshal(cachedInputDoc{ID: id, AccessHash: accessHash, FileReference: fileRef})
	if err != nil {
		return err
	}
	pipe := r.client.Pipeline()
	pipe.HSet(ctx, docCacheKey(botID), urlKey, val)
	pipe.Expire(ctx, docCacheKey(botID), mediaCacheTTL)
	_, err = pipe.Exec(ctx)
	return err
}

// GetDocCache retrieves an InputDocument from Redis for the given bot and URL key.
func (r *RedisStore) GetDocCache(ctx context.Context, botID int64, urlKey string) (int64, int64, []byte, bool) {
	val, err := r.client.HGet(ctx, docCacheKey(botID), urlKey).Bytes()
	if err != nil {
		return 0, 0, nil, false
	}
	var c cachedInputDoc
	if err := json.Unmarshal(val, &c); err != nil {
		return 0, 0, nil, false
	}
	return c.ID, c.AccessHash, c.FileReference, true
}

// ------------------------------------------------------------------------------------------------
// Update Queue (Redis Streams)
// ------------------------------------------------------------------------------------------------

// NextUpdateID atomically increments the persistent update_id counter for a bot
// and returns the new value. This survives process restarts.
func (r *RedisStore) NextUpdateID(ctx context.Context, botID int64) (int, error) {
	n, err := r.client.Incr(ctx, updateIDKey(botID)).Result()
	if err != nil {
		return 0, fmt.Errorf("incr update_id: %w", err)
	}
	return int(n), nil
}

// AppendUpdate writes a serialized update payload to the bot's Redis Stream.
// updateID must be obtained via NextUpdateID to guarantee monotonicity across restarts.
// The stream is capped at streamMaxLen entries (~).
func (r *RedisStore) AppendUpdate(ctx context.Context, botID int64, updateID int, payload []byte) error {
	args := &redis.XAddArgs{
		Stream: updateStreamKey(botID),
		MaxLen: streamMaxLen,
		Approx: true,
		Values: map[string]any{
			"uid":  strconv.Itoa(updateID),
			"data": string(payload),
		},
	}
	return r.client.XAdd(ctx, args).Err()
}

// ReadUpdates reads up to limit updates from the bot's stream, starting from the entry
// whose update_id >= offset (Bot API semantics). If blockDuration > 0 and the stream is
// empty or all entries are below offset, the call blocks up to blockDuration.
// Returns an empty slice (not nil) when no updates are available.
func (r *RedisStore) ReadUpdates(ctx context.Context, botID int64, offset, limit int, blockDuration time.Duration) ([]UpdateEntry, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	streamKey := updateStreamKey(botID)

	// First, do a non-blocking XRANGE to pick up already-present entries.
	entries, err := r.client.XRangeN(ctx, streamKey, "-", "+", int64(limit+100)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("xrange: %w", err)
	}

	result := filterEntries(entries, offset, limit)
	if len(result) > 0 || blockDuration <= 0 {
		return result, nil
	}

	// Nothing yet — do a blocking XREAD for new messages.
	// We use "$" to listen only for messages added after this point, but since we already
	// read the full stream above and got nothing past offset, this is safe.
	blockArgs := &redis.XReadArgs{
		Streams: []string{streamKey, "$"},
		Count:   int64(limit),
		Block:   blockDuration,
	}
	streams, err := r.client.XRead(ctx, blockArgs).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) || strings.Contains(err.Error(), "redis: nil") {
			return []UpdateEntry{}, nil
		}
		if ctx.Err() != nil {
			return []UpdateEntry{}, nil
		}
		return nil, fmt.Errorf("xread block: %w", err)
	}
	if len(streams) == 0 {
		return []UpdateEntry{}, nil
	}
	return filterEntries(streams[0].Messages, offset, limit), nil
}

// filterEntries converts raw Redis Stream messages to UpdateEntry, keeping only
// those whose update_id >= offset and capping at limit results.
func filterEntries(messages []redis.XMessage, offset, limit int) []UpdateEntry {
	var out []UpdateEntry
	for _, msg := range messages {
		uidStr, _ := msg.Values["uid"].(string)
		data, _ := msg.Values["data"].(string)
		uid, _ := strconv.Atoi(uidStr)
		if offset > 0 && uid < offset {
			continue
		}
		out = append(out, UpdateEntry{
			StreamID: msg.ID,
			UpdateID: uid,
			Payload:  []byte(data),
		})
		if len(out) >= limit {
			break
		}
	}
	if out == nil {
		return []UpdateEntry{}
	}
	return out
}

// AckUpdates trims the stream by removing all entries whose update_id < offset,
// implementing the Bot API offset acknowledgement contract.
// It reads the stream, collects IDs of entries with uid < offset, and deletes them.
func (r *RedisStore) AckUpdates(ctx context.Context, botID int64, offset int) error {
	if offset <= 0 {
		return nil
	}
	streamKey := updateStreamKey(botID)
	// Read all messages (bounded by streamMaxLen which caps the stream)
	msgs, err := r.client.XRange(ctx, streamKey, "-", "+").Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil
		}
		return fmt.Errorf("xrange for ack: %w", err)
	}
	var toDelete []string
	for _, msg := range msgs {
		uidStr, _ := msg.Values["uid"].(string)
		uid, _ := strconv.Atoi(uidStr)
		if uid < offset {
			toDelete = append(toDelete, msg.ID)
		}
	}
	if len(toDelete) == 0 {
		return nil
	}
	return r.client.XDel(ctx, streamKey, toDelete...).Err()
}

// DropPendingUpdates deletes the entire update stream and resets the update_id counter for a bot.
func (r *RedisStore) DropPendingUpdates(ctx context.Context, botID int64) error {
	pipe := r.client.Pipeline()
	pipe.Del(ctx, updateStreamKey(botID))
	pipe.Del(ctx, updateIDKey(botID))
	_, err := pipe.Exec(ctx)
	return err
}

// ------------------------------------------------------------------------------------------------
// Unmarshalling helper
// ------------------------------------------------------------------------------------------------

// UnmarshalUpdatePayload is a convenience helper to decode an UpdateEntry payload.
func UnmarshalUpdatePayload[T any](entry UpdateEntry) (*T, error) {
	var v T
	if err := json.Unmarshal(entry.Payload, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// BotSessionStorage implements gotd session.Storage backed by Redis.

type BotSessionStorage struct {
	client *redis.Client
	key    string
}

// SessionStorage returns a session.Storage implementation for a specific bot token.
func (r *RedisStore) SessionStorage(token string) session.Storage {
	return &BotSessionStorage{
		client: r.client,
		key:    fmt.Sprintf("telego:session:%s", token),
	}
}

// LoadSession loads MTProto session bytes from Redis.
func (s *BotSessionStorage) LoadSession(ctx context.Context) ([]byte, error) {
	val, err := s.client.Get(ctx, s.key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, session.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return val, nil
}

// StoreSession saves MTProto session bytes to Redis.
func (s *BotSessionStorage) StoreSession(ctx context.Context, data []byte) error {
	return s.client.Set(ctx, s.key, data, 0).Err()
}

// SaveBotProfile caches the bot User profile JSON in Redis for instant getMe responses.
func (r *RedisStore) SaveBotProfile(ctx context.Context, token string, profile []byte) error {
	return r.client.Set(ctx, fmt.Sprintf("telego:profile:%s", token), profile, 0).Err()
}

// GetBotProfile retrieves cached bot User profile JSON from Redis.
func (r *RedisStore) GetBotProfile(ctx context.Context, token string) ([]byte, error) {
	val, err := r.client.Get(ctx, fmt.Sprintf("telego:profile:%s", token)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	return val, err
}

// SaveBusinessConnection caches a serialized BusinessConnection in Redis.
func (r *RedisStore) SaveBusinessConnection(ctx context.Context, connectionID string, data []byte) error {
	return r.client.Set(ctx, fmt.Sprintf("telego:busconn:%s", connectionID), data, 30*24*time.Hour).Err()
}

// GetBusinessConnection retrieves a serialized BusinessConnection from Redis.
func (r *RedisStore) GetBusinessConnection(ctx context.Context, connectionID string) ([]byte, error) {
	val, err := r.client.Get(ctx, fmt.Sprintf("telego:busconn:%s", connectionID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	return val, err
}

