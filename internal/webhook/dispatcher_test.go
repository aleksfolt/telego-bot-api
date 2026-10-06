package webhook

import (
	"context"
	"errors"
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

func TestDispatcherCancelledTaskIsNotSent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d := &Dispatcher{logger: zap.NewNop(), client: &fasthttp.Client{
		Dial: func(string) (net.Conn, error) { t.Fatal("cancelled webhook attempted a connection"); return nil, nil },
	}}
	req, resp := fasthttp.AcquireRequest(), fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)
	d.processTask(req, resp, &Task{
		Context: ctx, URL: "http://127.0.0.1/unused", Update: &converter.Update{UpdateID: 1},
		OnSuccess: func(int64, int) { t.Fatal("cancelled update must remain pending") },
	})
}

func TestDispatcherDoesNotResendAfterApplicationError(t *testing.T) {
	var calls atomic.Int32
	server := &fasthttp.Server{Handler: func(ctx *fasthttp.RequestCtx) {
		calls.Add(1)
		ctx.SetStatusCode(500)
	}}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("skipping local listener test:", err)
	}
	defer ln.Close()
	go server.Serve(ln) //nolint:errcheck

	d := NewDispatcher(1, 1, zap.NewNop())
	defer d.Stop()
	failed := make(chan error, 1)
	d.EnqueueTask(&Task{URL: "http://" + ln.Addr().String() + "/hook", Update: &converter.Update{UpdateID: 1},
		OnError: func(_ int64, err error) { failed <- err }})
	select {
	case err := <-failed:
		require.ErrorContains(t, err, "unexpected status code: 500")
	case <-time.After(5 * time.Second):
		t.Fatal("delivery did not finish")
	}
	// The application already received the update; resending it immediately would duplicate it.
	require.Equal(t, int32(1), calls.Load())
}

func TestIsNotDeliveredError(t *testing.T) {
	require.True(t, isNotDeliveredError(fasthttp.ErrConnectionClosed))
	require.True(t, isNotDeliveredError(errors.New("error when dialing 127.0.0.1:9003: dial tcp4 127.0.0.1:9003: connect: connection refused")))
	require.False(t, isNotDeliveredError(fasthttp.ErrTimeout))
}
