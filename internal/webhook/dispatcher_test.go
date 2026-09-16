package webhook

import (
	"net"
	"sync/atomic"
	"testing"
	"time"

	"telego-bot-api/internal/converter"

	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

func TestDispatcher_DeliveryAndCallbacks(t *testing.T) {
	logger := zap.NewNop()

	var receivedCount atomic.Int32
	var receivedSecret atomic.Value

	server := &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {
			receivedCount.Add(1)
			secret := string(ctx.Request.Header.Peek("X-Telegram-Bot-Api-Secret-Token"))
			receivedSecret.Store(secret)
			ctx.SetStatusCode(200)
			ctx.SetBodyString(`{"ok":true}`)
		},
	}

	fastLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("skipping local listener test:", err)
		return
	}
	defer fastLn.Close()

	go server.Serve(fastLn) //nolint:errcheck

	addr := fastLn.Addr().String()
	targetURL := "http://" + addr + "/webhook"

	// Verify local dialing is permitted in this environment
	testConn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
	if err != nil {
		t.Skip("skipping webhook network test: local TCP dial not permitted in sandbox:", err)
		return
	}
	_ = testConn.Close()

	d := NewDispatcher(2, 100, logger)

	var successCalled atomic.Bool
	d.EnqueueTask(&Task{
		BotID:       12345,
		URL:         targetURL,
		SecretToken: "my_secret_token_123",
		Update: &converter.Update{
			UpdateID: 42,
		},
		OnSuccess: func(botID int64, updateID int) {
			require.Equal(t, int64(12345), botID)
			require.Equal(t, 42, updateID)
			successCalled.Store(true)
		},
	})

	require.Eventually(t, func() bool {
		return successCalled.Load() && receivedCount.Load() == 1
	}, 3*time.Second, 50*time.Millisecond)

	require.Equal(t, "my_secret_token_123", receivedSecret.Load())

	d.Stop()
}

func TestDispatcher_QueueOperations(t *testing.T) {
	logger := zap.NewNop()
	// Workers = 0 so nothing gets popped automatically
	d := &Dispatcher{
		logger: logger,
		queue:  make(chan *Task, 5),
		quit:   make(chan struct{}),
	}

	require.Equal(t, 0, d.QueueSize())

	d.Enqueue("http://localhost", "secret", &converter.Update{UpdateID: 1})
	d.Enqueue("http://localhost", "secret", &converter.Update{UpdateID: 2})

	require.Equal(t, 2, d.QueueSize())

	task := <-d.queue
	require.Equal(t, 1, task.Update.UpdateID)
	require.Equal(t, 1, d.QueueSize())
}

