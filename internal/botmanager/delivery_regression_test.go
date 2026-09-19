package botmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/redis/go-redis/v9"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"telego-bot-api/internal/converter"
	"telego-bot-api/internal/storage"
	"telego-bot-api/internal/webhook"
)

func deliveryTestBot(t *testing.T, id int64) *BotInstance {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("set REDIS_ADDR to an isolated test Redis")
	}
	store := storage.NewRedisStore(addr, "", 15)
	require.NoError(t, store.Ping(context.Background()))
	rdb := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { rdb.Close() })
	require.NoError(t, rdb.Del(context.Background(), fmt.Sprintf("telego:updates:%d", id), fmt.Sprintf("telego:update_id:%d", id)).Err())
	t.Cleanup(func() { store.DropPendingUpdates(context.Background(), id); store.Close() })
	b := mediaTestBot(nil)
	b.redisStore = store
	b.botID = id
	b.token = fmt.Sprintf("delivery-test:%d", id)
	b.updateNotify = make(chan struct{}, 1)
	return b
}

func appendDeliveryTestUpdate(t *testing.T, b *BotInstance, update converter.Update) int {
	t.Helper()
	ctx := context.Background()
	id, err := b.redisStore.NextUpdateID(ctx, b.botID)
	require.NoError(t, err)
	update.UpdateID = id
	payload, err := json.Marshal(update)
	require.NoError(t, err)
	require.NoError(t, b.redisStore.AppendUpdate(ctx, b.botID, id, payload))
	return id
}

func TestDeliveryRegressionAllowedUpdatesDoesNotStarve(t *testing.T) {
	b := deliveryTestBot(t, 801)
	appendDeliveryTestUpdate(t, b, converter.Update{Message: &converter.Message{MessageID: 1}})
	wantID := appendDeliveryTestUpdate(t, b, converter.Update{CallbackQuery: &converter.CallbackQuery{ID: "query"}})
	updates, err := b.GetUpdates(context.Background(), 0, 1, 0, []string{"callback_query"})
	require.NoError(t, err)
	require.Len(t, updates, 1, "an excluded message must not hide a queued callback")
	require.Equal(t, wantID, updates[0].UpdateID)
}

func TestDeliveryRegressionDropPendingDoesNotInvalidateOffset(t *testing.T) {
	b := deliveryTestBot(t, 802)
	ctx := context.Background()
	oldID := appendDeliveryTestUpdate(t, b, converter.Update{Message: &converter.Message{MessageID: 1}})
	require.NoError(t, b.DropPendingUpdates(ctx))
	appendDeliveryTestUpdate(t, b, converter.Update{Message: &converter.Message{MessageID: 2}})
	updates, err := b.GetUpdates(ctx, oldID+1, 100, 0, nil)
	require.NoError(t, err)
	require.Len(t, updates, 1, "old polling offset must not delete freshly received messages after dropping the queue")
}

func TestDeliveryRegressionBusinessDeleteDecodesSuccess(t *testing.T) {
	b := mediaTestBot(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		wrapped := input.(*tg.InvokeWithBusinessConnectionRequest)
		require.IsType(t, &tg.MessagesDeleteMessagesRequest{}, wrapped.Query)
		var wire bin.Buffer
		response := &tg.MessagesAffectedMessages{Pts: 100, PtsCount: 1}
		require.NoError(t, response.Encode(&wire))
		return output.Decode(&wire)
	})
	ok, err := b.DeleteMessage(context.Background(), &converter.DeleteMessageRequest{
		BusinessConnectionID: "business-test", ChatID: 123, MessageID: 42,
	})
	require.NoError(t, err)
	require.True(t, ok)
}

func TestDeliveryRegressionWebhookAckKeepsUndeliveredUpdate(t *testing.T) {
	b := deliveryTestBot(t, 803)
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var update converter.Update
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			w.WriteHeader(400)
			return
		}
		if update.UpdateID == 1 {
			w.WriteHeader(400)
			return
		}
		w.WriteHeader(200)
	}))
	t.Cleanup(server.Close)
	b.dispatcher = webhook.NewDispatcher(1, 10, zap.NewNop())
	t.Cleanup(func() { b.Stop(); b.dispatcher.Stop() })
	require.NoError(t, b.SetWebhook(ctx, server.URL, "", false))
	require.NoError(t, b.Handle(ctx, &tg.Updates{Updates: []tg.UpdateClass{
		&tg.UpdateNewMessage{Message: &tg.Message{ID: 1, PeerID: &tg.PeerUser{UserID: 123}, Message: "first"}},
		&tg.UpdateNewMessage{Message: &tg.Message{ID: 2, PeerID: &tg.PeerUser{UserID: 123}, Message: "second"}},
	}}))
	var entries []storage.UpdateEntry
	require.Eventually(t, func() bool {
		entries, _ = b.redisStore.ReadUpdates(ctx, b.botID, 0, 100, 0)
		return len(entries) == 1
	}, 3*time.Second, 10*time.Millisecond)
	require.Equal(t, 1, entries[0].UpdateID, "retain the failed update, remove the successful one")
}

func TestDeliveryRegressionSetWebhookDeliversPendingUpdates(t *testing.T) {
	b := deliveryTestBot(t, 804)
	appendDeliveryTestUpdate(t, b, converter.Update{Message: &converter.Message{MessageID: 1}})
	b.dispatcher = webhook.NewDispatcher(0, 10, zap.NewNop())
	t.Cleanup(func() { b.Stop(); b.dispatcher.Stop() })
	require.NoError(t, b.SetWebhook(context.Background(), "http://127.0.0.1:1/unused", "", false))
	require.Eventually(t, func() bool { return b.dispatcher.QueueSize() == 1 }, time.Second, 10*time.Millisecond, "existing pending update must be scheduled when installing a webhook")
}

func TestWebhookRetryAfterReceiverRecovers(t *testing.T) {
	b := deliveryTestBot(t, 805)
	var healthy atomic.Bool
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if !healthy.Load() {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
	}))
	t.Cleanup(server.Close)
	b.dispatcher = webhook.NewDispatcher(1, 10, zap.NewNop())
	t.Cleanup(func() { b.Stop(); b.dispatcher.Stop() })
	appendDeliveryTestUpdate(t, b, converter.Update{Message: &converter.Message{MessageID: 1}})
	ctx := context.Background()
	require.NoError(t, b.SetWebhook(ctx, server.URL, "", false))
	require.Eventually(t, func() bool {
		wh, err := b.redisStore.GetWebhook(ctx, b.token)
		return err == nil && wh != nil && wh.LastErrorMessage != ""
	}, 3*time.Second, 10*time.Millisecond)
	count, err := b.redisStore.PendingUpdatesCount(ctx, b.botID)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	healthy.Store(true)
	require.Eventually(t, func() bool {
		count, err := b.redisStore.PendingUpdatesCount(ctx, b.botID)
		return err == nil && count == 0
	}, 5*time.Second, 10*time.Millisecond)
	require.GreaterOrEqual(t, calls.Load(), int32(4))
}

func TestWebhookReplayAfterRestart(t *testing.T) {
	b := deliveryTestBot(t, 806)
	var deliveries atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deliveries.Add(1)
		w.WriteHeader(200)
	}))
	t.Cleanup(server.Close)
	b.dispatcher = webhook.NewDispatcher(0, 10, zap.NewNop())
	t.Cleanup(func() { b.Stop(); b.dispatcher.Stop() })
	appendDeliveryTestUpdate(t, b, converter.Update{Message: &converter.Message{MessageID: 1}})
	ctx := context.Background()
	require.NoError(t, b.SetWebhook(ctx, server.URL, "", false))
	require.Eventually(t, func() bool { return b.dispatcher.QueueSize() == 1 }, time.Second, 10*time.Millisecond)
	b.Stop()
	// A fresh BotInstance restores exactly the same state/lifecycle hooks as Start.
	restarted := mediaTestBot(nil)
	restarted.botID, restarted.token, restarted.redisStore = b.botID, b.token, b.redisStore
	restarted.dispatcher = webhook.NewDispatcher(1, 10, zap.NewNop())
	t.Cleanup(func() { restarted.Stop(); restarted.dispatcher.Stop() })
	restarted.restoreWebhookConfig(ctx)
	restarted.restartWebhookDelivery()
	require.Eventually(t, func() bool {
		count, err := b.redisStore.PendingUpdatesCount(ctx, b.botID)
		return err == nil && count == 0
	}, 3*time.Second, 10*time.Millisecond)
	require.Equal(t, int32(1), deliveries.Load())
}

func TestWebhookQueueOverflowRetainsUpdates(t *testing.T) {
	b := deliveryTestBot(t, 807)
	var mu sync.Mutex
	received := make(map[int]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var update converter.Update
		if json.NewDecoder(r.Body).Decode(&update) != nil {
			w.WriteHeader(400)
			return
		}
		// One small dispatcher queue cannot hold the entire replay batch.
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		received[update.UpdateID]++
		mu.Unlock()
		w.WriteHeader(200)
	}))
	t.Cleanup(server.Close)
	b.dispatcher = webhook.NewDispatcher(1, 1, zap.NewNop())
	t.Cleanup(func() { b.Stop(); b.dispatcher.Stop() })
	for i := 0; i < 5; i++ {
		appendDeliveryTestUpdate(t, b, converter.Update{Message: &converter.Message{MessageID: int64(i + 1)}})
	}
	ctx := context.Background()
	require.NoError(t, b.SetWebhook(ctx, server.URL, "", false))
	require.Eventually(t, func() bool {
		count, err := b.redisStore.PendingUpdatesCount(ctx, b.botID)
		return err == nil && count == 0
	}, 10*time.Second, 20*time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, received, 5)
	for _, count := range received {
		require.Equal(t, 1, count)
	}
}

func TestDeleteWebhookPreservesPendingForPolling(t *testing.T) {
	b := deliveryTestBot(t, 808)
	b.dispatcher = webhook.NewDispatcher(0, 10, zap.NewNop())
	t.Cleanup(func() { b.Stop(); b.dispatcher.Stop() })
	id := appendDeliveryTestUpdate(t, b, converter.Update{Message: &converter.Message{MessageID: 1}})
	ctx := context.Background()
	require.NoError(t, b.SetWebhook(ctx, "http://127.0.0.1:1/unused", "", false))
	require.Eventually(t, func() bool { return b.dispatcher.QueueSize() == 1 }, time.Second, 10*time.Millisecond)
	require.NoError(t, b.DeleteWebhook(ctx, false))
	require.False(t, b.HasWebhook())
	updates, err := b.GetUpdates(ctx, 0, 100, 0, nil)
	require.NoError(t, err)
	require.Len(t, updates, 1)
	require.Equal(t, id, updates[0].UpdateID)
}

func TestFilteredPollingWaitsForNewMatchingUpdate(t *testing.T) {
	b := deliveryTestBot(t, 809)
	appendDeliveryTestUpdate(t, b, converter.Update{Message: &converter.Message{MessageID: 1}})
	type result struct {
		updates []*converter.Update
		err     error
	}
	resultCh := make(chan result, 1)
	go func() {
		updates, err := b.GetUpdates(context.Background(), 0, 1, 2, []string{"callback_query"})
		resultCh <- result{updates, err}
	}()
	time.Sleep(100 * time.Millisecond)
	id := appendDeliveryTestUpdate(t, b, converter.Update{CallbackQuery: &converter.CallbackQuery{ID: "new"}})
	select {
	case got := <-resultCh:
		require.NoError(t, got.err)
		require.Len(t, got.updates, 1)
		require.Equal(t, id, got.updates[0].UpdateID)
	case <-time.After(3 * time.Second):
		t.Fatal("matching update was not delivered")
	}
}
