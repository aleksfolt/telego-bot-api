package logging

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSamplePerBot(t *testing.T) {
	ConfigureSampling(5, 100)
	t.Cleanup(func() { ConfigureSampling(5, 100) })
	now := time.Unix(1_800_000_000, 0)

	written, skippedTotal := 0, 0
	for i := 0; i < 1000; i++ {
		if ok, skipped := sampleAt("test", 1, now); ok {
			written++
			skippedTotal += skipped
		}
	}
	require.Equal(t, 5+9, written, "first 5 in full, then every 100th")
	// Lines skipped after the last written one (906..1000) are reported with the next.
	require.Equal(t, 1000-14-95, skippedTotal, "skipped counts are reported with written lines")

	ok, _ := sampleAt("test", 2, now)
	require.True(t, ok, "a busy bot does not use up the budget of another bot")
	ok, _ = sampleAt("other", 1, now)
	require.True(t, ok, "kinds are sampled independently")
	ok, _ = sampleAt("test", 1, now.Add(time.Second))
	require.True(t, ok, "the budget resets every second")
}

func TestSamplingDisabled(t *testing.T) {
	ConfigureSampling(0, 1)
	t.Cleanup(func() { ConfigureSampling(5, 100) })
	for i := 0; i < 100; i++ {
		ok, _ := sampleAt("disabled", 1, time.Unix(1_800_000_000, 0))
		require.True(t, ok)
	}
}
