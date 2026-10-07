package diag

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupDumps(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	Configure(dir, zap.NewNop())
	lastDump.Store(0)
	t.Cleanup(func() { Configure("", zap.NewNop()); lastDump.Store(0) })
	return dir
}

func dumpFiles(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, dumpPrefix+"*.txt"))
	require.NoError(t, err)
	return files
}

func TestWatchDumpsGoroutinesWhileStalled(t *testing.T) {
	dir := setupDumps(t)
	oldStall := StallThreshold
	StallThreshold = 20 * time.Millisecond
	t.Cleanup(func() { StallThreshold = oldStall })

	done := Watch("telegram_request", "messages.sendMessage", zap.Int64("bot_id", 42))
	require.Eventually(t, func() bool { return len(dumpFiles(t, dir)) == 1 }, 2*time.Second, 10*time.Millisecond)
	done()

	data, err := os.ReadFile(dumpFiles(t, dir)[0])
	require.NoError(t, err)
	require.Contains(t, string(data), "reason: telegram_request messages.sendMessage")
	require.Contains(t, string(data), "goroutine ")
}

func TestWatchSlowNeverDumps(t *testing.T) {
	dir := setupDumps(t)
	oldStall := StallThreshold
	StallThreshold = 10 * time.Millisecond
	t.Cleanup(func() { StallThreshold = oldStall })

	done := WatchSlow("api_request", "sendvideo")
	time.Sleep(50 * time.Millisecond)
	done()
	require.Empty(t, dumpFiles(t, dir))
}

func TestDumpIsRateLimitedAndPruned(t *testing.T) {
	dir := setupDumps(t)
	Dump("first")
	Dump("second")
	require.Len(t, dumpFiles(t, dir), 1, "dumps are rate limited")

	for i := 0; i < maxDumps+5; i++ {
		name := filepath.Join(dir, fmt.Sprintf("%s20200101-%06d.txt", dumpPrefix, i))
		require.NoError(t, os.WriteFile(name, []byte("old"), 0o600))
	}
	pruneDumps(dir)
	require.Len(t, dumpFiles(t, dir), maxDumps)
}

func TestDumpDisabledWithoutDirectory(t *testing.T) {
	Configure("", zap.NewNop())
	lastDump.Store(0)
	Dump("nothing")
	require.Zero(t, lastDump.Load())
}
