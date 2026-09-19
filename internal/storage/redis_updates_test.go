package storage_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"telego-bot-api/internal/storage"
)

// redisAddr returns the Redis address from env REDIS_ADDR or falls back to localhost:6379.
func redisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	return "127.0.0.1:6379"
}

// redisPass returns the Redis password from env REDIS_PASS.
func redisPass() string {
	return os.Getenv("REDIS_PASS")
}

// newTestStore creates a RedisStore and pings it, skipping the test if Redis is unavailable.
func newTestStore(t *testing.T) *storage.RedisStore {
	t.Helper()
	store := storage.NewRedisStore(redisAddr(), redisPass(), 15) // DB 15 reserved for tests
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := store.Ping(ctx); err != nil {
		t.Skipf("Redis not available at %s: %v", redisAddr(), err)
	}
	return store
}

// cleanupBot removes all telego keys for the given bot_id in test DB.
func cleanupBot(t *testing.T, botID int64) {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddr(),
		Password: redisPass(),
		DB:       15,
	})
	defer rdb.Close()
	ctx := context.Background()
	// Delete stream, update_id counter, and media cache
	rdb.Del(ctx,
		fmt.Sprintf("telego:updates:%d", botID),
		fmt.Sprintf("telego:update_id:%d", botID),
		fmt.Sprintf("telego:mediacache:photo:%d", botID),
		fmt.Sprintf("telego:mediacache:doc:%d", botID),
	)
}

// appendUpdate is a convenience helper for tests.
func appendUpdate(t *testing.T, store *storage.RedisStore, botID int64, payload any) int {
	t.Helper()
	ctx := context.Background()
	uid, err := store.NextUpdateID(ctx, botID)
	require.NoError(t, err)
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	require.NoError(t, store.AppendUpdate(ctx, botID, uid, raw))
	return uid
}

// ────────────────────────────────────────────────────────────────────────────

func TestNextUpdateID_Monotonic(t *testing.T) {
	store := newTestStore(t)
	const botID int64 = 90001
	cleanupBot(t, botID)
	t.Cleanup(func() { cleanupBot(t, botID) })

	ctx := context.Background()
	prev := 0
	for i := 0; i < 10; i++ {
		uid, err := store.NextUpdateID(ctx, botID)
		require.NoError(t, err)
		assert.Greater(t, uid, prev, "update_id must be strictly increasing")
		prev = uid
	}
}

func TestNextUpdateID_PersistsAcrossRestarts(t *testing.T) {
	store1 := newTestStore(t)
	const botID int64 = 90002
	cleanupBot(t, botID)
	t.Cleanup(func() { cleanupBot(t, botID) })

	ctx := context.Background()
	uid1, err := store1.NextUpdateID(ctx, botID)
	require.NoError(t, err)

	// Simulate restart: fresh store on same Redis
	store2 := storage.NewRedisStore(redisAddr(), redisPass(), 15)
	uid2, err := store2.NextUpdateID(ctx, botID)
	require.NoError(t, err)

	assert.Greater(t, uid2, uid1, "update_id must not reset after restart")
}

func TestAppendAndReadUpdates_Basic(t *testing.T) {
	store := newTestStore(t)
	const botID int64 = 90003
	cleanupBot(t, botID)
	t.Cleanup(func() { cleanupBot(t, botID) })

	uid1 := appendUpdate(t, store, botID, map[string]any{"update_id": 1, "text": "hello"})
	uid2 := appendUpdate(t, store, botID, map[string]any{"update_id": 2, "text": "world"})

	ctx := context.Background()
	entries, err := store.ReadUpdates(ctx, botID, 0, 100, 0)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, uid1, entries[0].UpdateID)
	assert.Equal(t, uid2, entries[1].UpdateID)
}

func TestReadUpdates_OffsetFiltering(t *testing.T) {
	store := newTestStore(t)
	const botID int64 = 90004
	cleanupBot(t, botID)
	t.Cleanup(func() { cleanupBot(t, botID) })

	uid1 := appendUpdate(t, store, botID, "msg1")
	uid2 := appendUpdate(t, store, botID, "msg2")
	uid3 := appendUpdate(t, store, botID, "msg3")
	_ = uid1

	ctx := context.Background()
	// offset = uid2 should skip uid1
	entries, err := store.ReadUpdates(ctx, botID, uid2, 100, 0)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, uid2, entries[0].UpdateID)
	assert.Equal(t, uid3, entries[1].UpdateID)
}

func TestAckUpdates_RemovesOldEntries(t *testing.T) {
	store := newTestStore(t)
	const botID int64 = 90005
	cleanupBot(t, botID)
	t.Cleanup(func() { cleanupBot(t, botID) })

	uid1 := appendUpdate(t, store, botID, "a")
	uid2 := appendUpdate(t, store, botID, "b")
	uid3 := appendUpdate(t, store, botID, "c")
	_, _ = uid1, uid2

	ctx := context.Background()
	// Ack uid1 and uid2 → only uid3 should remain
	err := store.AckUpdates(ctx, botID, uid3)
	require.NoError(t, err)

	entries, err := store.ReadUpdates(ctx, botID, 0, 100, 0)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, uid3, entries[0].UpdateID)
}

func TestAckUpdates_EmptyStreamIsNoop(t *testing.T) {
	store := newTestStore(t)
	const botID int64 = 90006
	cleanupBot(t, botID)
	t.Cleanup(func() { cleanupBot(t, botID) })

	ctx := context.Background()
	err := store.AckUpdates(ctx, botID, 999)
	assert.NoError(t, err)
}

func TestDropPendingUpdates(t *testing.T) {
	store := newTestStore(t)
	const botID int64 = 90007
	cleanupBot(t, botID)
	t.Cleanup(func() { cleanupBot(t, botID) })

	appendUpdate(t, store, botID, "x")
	appendUpdate(t, store, botID, "y")

	ctx := context.Background()
	require.NoError(t, store.DropPendingUpdates(ctx, botID))

	entries, err := store.ReadUpdates(ctx, botID, 0, 100, 0)
	require.NoError(t, err)
	assert.Empty(t, entries)

	// Dropping pending events must not invalidate an existing polling offset.
	uid, err := store.NextUpdateID(ctx, botID)
	require.NoError(t, err)
	assert.Equal(t, 3, uid)
}

func TestReadUpdates_EmptyStreamReturnsEmpty(t *testing.T) {
	store := newTestStore(t)
	const botID int64 = 90008
	cleanupBot(t, botID)
	t.Cleanup(func() { cleanupBot(t, botID) })

	ctx := context.Background()
	entries, err := store.ReadUpdates(ctx, botID, 0, 100, 0)
	require.NoError(t, err)
	assert.NotNil(t, entries)
	assert.Empty(t, entries)
}

func TestReadUpdates_LimitRespected(t *testing.T) {
	store := newTestStore(t)
	const botID int64 = 90009
	cleanupBot(t, botID)
	t.Cleanup(func() { cleanupBot(t, botID) })

	for i := 0; i < 10; i++ {
		appendUpdate(t, store, botID, strconv.Itoa(i))
	}

	ctx := context.Background()
	entries, err := store.ReadUpdates(ctx, botID, 0, 3, 0)
	require.NoError(t, err)
	assert.Len(t, entries, 3)
}

func TestPersistAcrossRestartSimulation(t *testing.T) {
	store1 := newTestStore(t)
	const botID int64 = 90010
	cleanupBot(t, botID)
	t.Cleanup(func() { cleanupBot(t, botID) })

	uid1 := appendUpdate(t, store1, botID, "persistent")

	// New store = simulated process restart
	store2 := storage.NewRedisStore(redisAddr(), redisPass(), 15)
	ctx := context.Background()
	entries, err := store2.ReadUpdates(ctx, botID, 0, 100, 0)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, uid1, entries[0].UpdateID)
	assert.Equal(t, `"persistent"`, string(entries[0].Payload))
}

func TestReadUpdates_BlockingReceivesNewMessage(t *testing.T) {
	store := newTestStore(t)
	const botID int64 = 90011
	cleanupBot(t, botID)
	t.Cleanup(func() { cleanupBot(t, botID) })

	// Write a message *after* a short delay while ReadUpdates is blocking
	go func() {
		time.Sleep(200 * time.Millisecond)
		appendUpdate(t, store, botID, "late arrival")
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	start := time.Now()
	entries, err := store.ReadUpdates(ctx, botID, 0, 100, 2*time.Second)
	elapsed := time.Since(start)

	require.NoError(t, err)
	// Should have received the message without waiting the full 2s
	assert.Len(t, entries, 1, "blocking read should receive the new message")
	assert.Less(t, elapsed, 2*time.Second, "should not have waited the full block duration")
}

func TestUnmarshalUpdatePayload(t *testing.T) {
	type Msg struct {
		Text string `json:"text"`
	}
	payload, _ := json.Marshal(Msg{Text: "hi"})
	entry := storage.UpdateEntry{StreamID: "x", UpdateID: 1, Payload: payload}

	msg, err := storage.UnmarshalUpdatePayload[Msg](entry)
	require.NoError(t, err)
	assert.Equal(t, "hi", msg.Text)
}

func TestPhotoCache_PersistAcrossRestarts(t *testing.T) {
	store1 := newTestStore(t)
	const botID int64 = 90012
	cleanupBot(t, botID)
	t.Cleanup(func() { cleanupBot(t, botID) })

	ctx := context.Background()
	urlKey := "https://example.com/test-photo.jpg"
	err := store1.SavePhotoCache(ctx, botID, urlKey, 123456, 789012, []byte("file-ref-photo"))
	require.NoError(t, err)

	// Simulate restart with store2
	store2 := storage.NewRedisStore(redisAddr(), redisPass(), 15)
	id, ah, fr, hit := store2.GetPhotoCache(ctx, botID, urlKey)
	require.True(t, hit, "photo cache must be found after restart")
	assert.Equal(t, int64(123456), id)
	assert.Equal(t, int64(789012), ah)
	assert.Equal(t, []byte("file-ref-photo"), fr)

	// Non-existent key should return false
	_, _, _, hit = store2.GetPhotoCache(ctx, botID, "https://example.com/non-existent.jpg")
	assert.False(t, hit)
}

func TestDocCache_PersistAcrossRestarts(t *testing.T) {
	store1 := newTestStore(t)
	const botID int64 = 90013
	cleanupBot(t, botID)
	t.Cleanup(func() { cleanupBot(t, botID) })

	ctx := context.Background()
	urlKey := "video\x00https://example.com/test-video.mp4"
	err := store1.SaveDocCache(ctx, botID, urlKey, 654321, 210987, []byte("file-ref-doc"))
	require.NoError(t, err)

	// Simulate restart with store2
	store2 := storage.NewRedisStore(redisAddr(), redisPass(), 15)
	id, ah, fr, hit := store2.GetDocCache(ctx, botID, urlKey)
	require.True(t, hit, "doc cache must be found after restart")
	assert.Equal(t, int64(654321), id)
	assert.Equal(t, int64(210987), ah)
	assert.Equal(t, []byte("file-ref-doc"), fr)

	// Non-existent key should return false
	_, _, _, hit = store2.GetDocCache(ctx, botID, "video\x00https://example.com/non-existent.mp4")
	assert.False(t, hit)
}
