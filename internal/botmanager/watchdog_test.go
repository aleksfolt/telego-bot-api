package botmanager

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/require"
)

func blockingInvoker(started chan<- struct{}) telegram.InvokeFunc {
	return func(ctx context.Context, _ bin.Encoder, _ bin.Decoder) error {
		if started != nil {
			started <- struct{}{}
		}
		<-ctx.Done()
		return ctx.Err()
	}
}

func TestSyncDeadlineAppliesOnlyToUpdatesSync(t *testing.T) {
	oldTimeout, oldStall := updatesSyncTimeout, syncStallAfter
	updatesSyncTimeout, syncStallAfter = 100*time.Millisecond, 50*time.Millisecond
	t.Cleanup(func() { updatesSyncTimeout, syncStallAfter = oldTimeout, oldStall })
	b := mediaTestBot(nil)
	invoker := syncDeadlineMiddleware{bot: b}.Handle(blockingInvoker(nil))

	for _, req := range []bin.Encoder{&tg.UpdatesGetDifferenceRequest{}, &tg.UpdatesGetChannelDifferenceRequest{}, &tg.UpdatesGetStateRequest{}} {
		start := time.Now()
		err := invoker.Invoke(context.Background(), req, nil)
		require.ErrorIs(t, err, context.DeadlineExceeded, "%T", req)
		require.Less(t, time.Since(start), time.Second)
	}
	b.watchdog.mu.Lock()
	require.False(t, b.watchdog.lastStall.IsZero(), "a stalled sync request is reported to the watchdog")
	b.watchdog.mu.Unlock()

	// Other methods (file downloads) keep their caller's deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := invoker.Invoke(ctx, &tg.UploadGetFileRequest{}, nil)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.GreaterOrEqual(t, time.Since(start), 350*time.Millisecond, "the sync timeout must not apply to upload.getFile")
}

// stuckDiffAPI answers getState and hangs in getDifference, like a dead connection.
type stuckDiffAPI struct{ started chan struct{} }

func (a *stuckDiffAPI) UpdatesGetState(context.Context) (*tg.UpdatesState, error) {
	return &tg.UpdatesState{Pts: 100, Qts: 1, Date: 1, Seq: 1}, nil
}

func (a *stuckDiffAPI) UpdatesGetDifference(ctx context.Context, _ *tg.UpdatesGetDifferenceRequest) (tg.UpdatesDifferenceClass, error) {
	select {
	case a.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (a *stuckDiffAPI) UpdatesGetChannelDifference(ctx context.Context, _ *tg.UpdatesGetChannelDifferenceRequest) (tg.UpdatesChannelDifferenceClass, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// The incident: the gap manager hangs in getDifference, its queue fills up and the
// connection's update handlers must still return, so the read loop can close.
func TestUpdateHandlerDoesNotBlockOnStuckGapManager(t *testing.T) {
	b := mediaTestBot(nil)
	// botID stays 0: updates the manager flushes on shutdown are skipped before they
	// reach Redis, which this bot does not have.
	b.gaps = newGapManager(b)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(b.stopUpdateWorker)
	defer cancel()
	api := &stuckDiffAPI{started: make(chan struct{}, 1)}
	go func() { _ = b.gaps.Run(ctx, api, 7467090745, updates.AuthOptions{IsBot: true}) }()
	select {
	case <-api.started:
	case <-time.After(3 * time.Second):
		t.Fatal("gap manager did not start getDifference")
	}
	go b.runIntake(ctx)

	handler := intakeHandler{bot: b}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 1000; i++ {
			_ = handler.Handle(ctx, &tg.UpdateShort{Update: &tg.UpdateNewMessage{
				Message: &tg.Message{ID: i + 1, PeerID: &tg.PeerUser{UserID: 123}}, Pts: 101 + i, PtsCount: 1}})
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the MTProto update handler blocked on a stuck gap manager")
	}
	require.Eventually(t, func() bool { return b.intake.stuckFor(time.Now().Add(time.Minute)) > 0 },
		time.Second, 10*time.Millisecond, "the intake reports that the gap manager does not accept updates")
}

func TestWatchdogRestartsBusyStuckBotOnce(t *testing.T) {
	var w clientWatchdog
	start := time.Unix(1_800_000_000, 0)
	for i := 0; i < 120; i++ {
		w.noteUpdates(start.Add(time.Duration(i)*time.Second), 1)
	}
	lastUpdate := start.Add(119 * time.Second)

	_, restart := w.check(lastUpdate.Add(time.Minute), 0)
	require.False(t, restart, "a short silence is normal")
	_, restart = w.check(lastUpdate.Add(5*time.Minute), 0)
	require.False(t, restart, "silence without a stalled request is not a hang")

	w.mu.Lock()
	w.lastStall = lastUpdate.Add(30 * time.Second)
	w.mu.Unlock()
	reason, restart := w.check(lastUpdate.Add(5*time.Minute), 0)
	require.True(t, restart)
	require.NotEmpty(t, reason)
	for i := 1; i <= 10; i++ {
		_, restart = w.check(lastUpdate.Add(5*time.Minute+time.Duration(i)*30*time.Second), 0)
		require.False(t, restart, "the client is recreated exactly once per restart interval")
	}
}

func TestWatchdogRestartsWhenIntakeIsStuck(t *testing.T) {
	var w clientWatchdog
	now := time.Unix(1_800_000_000, 0)
	w.noteUpdates(now, 500)
	_, restart := w.check(now.Add(5*time.Minute), time.Minute)
	require.False(t, restart)
	_, restart = w.check(now.Add(5*time.Minute), 3*time.Minute)
	require.True(t, restart)
}

func TestWatchdogIgnoresQuietBots(t *testing.T) {
	var w clientWatchdog
	now := time.Unix(1_800_000_000, 0)
	w.noteUpdates(now, 10)
	w.noteSyncStall()
	w.mu.Lock()
	w.lastStall = now.Add(time.Minute)
	w.mu.Unlock()
	_, restart := w.check(now.Add(time.Hour), 10*time.Minute)
	require.False(t, restart, "a bot without traffic is never restarted")
}

func TestRestartClientCancelsCurrentRun(t *testing.T) {
	b := mediaTestBot(nil)
	ctx, cancel := context.WithCancel(context.Background())
	b.runCancel = cancel
	b.restartClient()
	require.ErrorIs(t, ctx.Err(), context.Canceled)
}

func TestReplaceClientCreatesNewClient(t *testing.T) {
	b := mediaTestBot(nil)
	old := b.mtClient()
	b.businessDCPools[4] = testCloseInvoker{InvokeFunc: blockingInvoker(nil)}
	fresh := b.replaceClient()
	require.NotSame(t, old, fresh)
	require.Same(t, fresh, b.mtClient())
	require.Empty(t, b.businessDCPools, "business pools of the old client are closed")
}

func TestDownloadStallIsAborted(t *testing.T) {
	old := downloadIdleTimeout
	downloadIdleTimeout = 100 * time.Millisecond
	t.Cleanup(func() { downloadIdleTimeout = old })

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	pw := &progressWriter{w: &bytes.Buffer{}}
	stop := watchDownloadProgress(ctx, pw, cancel)
	defer stop()
	select {
	case <-ctx.Done():
		require.True(t, errors.Is(context.Cause(ctx), ErrDownloadStalled))
	case <-time.After(time.Second):
		t.Fatal("a download without progress was not aborted")
	}

	ctx2, cancel2 := context.WithCancelCause(context.Background())
	defer cancel2(nil)
	pw2 := &progressWriter{w: &bytes.Buffer{}}
	stop2 := watchDownloadProgress(ctx2, pw2, cancel2)
	defer stop2()
	for i := 0; i < 10; i++ {
		_, _ = pw2.Write([]byte("part"))
		time.Sleep(40 * time.Millisecond)
	}
	require.NoError(t, ctx2.Err(), "a download that makes progress keeps running")
}

func TestDownloadSlotsAreBounded(t *testing.T) {
	b := mediaTestBot(nil)
	var releases []func()
	for i := 0; i < maxParallelDownloads; i++ {
		release, err := b.acquireDownloadSlot(context.Background())
		require.NoError(t, err)
		releases = append(releases, release)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := b.acquireDownloadSlot(ctx)
	require.Error(t, err, "no slot is available while all downloads run")
	releases[0]()
	release, err := b.acquireDownloadSlot(context.Background())
	require.NoError(t, err)
	release()
}
