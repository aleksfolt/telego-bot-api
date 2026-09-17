package botmanager

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBotFlight_Deduplication(t *testing.T) {
	flight := newBotFlight()

	var execCount int64
	var wg sync.WaitGroup

	// Launch 10 concurrent requests for the same key
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := flight.Do("bot123", func() (*BotInstance, error) {
				atomic.AddInt64(&execCount, 1)
				time.Sleep(50 * time.Millisecond)
				return &BotInstance{token: "bot123"}, nil
			})
			assert.NoError(t, err)
			assert.NotNil(t, res)
			assert.Equal(t, "bot123", res.token)
		}()
	}

	wg.Wait()
	assert.Equal(t, int64(1), atomic.LoadInt64(&execCount), "flight.Do must execute fn exactly once for concurrent calls on same key")
}

func TestBotFlight_ConcurrentDifferentKeys(t *testing.T) {
	flight := newBotFlight()

	var execCount int64
	var wg sync.WaitGroup

	for i := 0; i < 5; i++ {
		key := string(rune('a' + i))
		wg.Add(1)
		go func(k string) {
			defer wg.Done()
			res, err := flight.Do(k, func() (*BotInstance, error) {
				atomic.AddInt64(&execCount, 1)
				time.Sleep(10 * time.Millisecond)
				return &BotInstance{token: k}, nil
			})
			assert.NoError(t, err)
			assert.Equal(t, k, res.token)
		}(key)
	}

	wg.Wait()
	assert.Equal(t, int64(5), atomic.LoadInt64(&execCount), "flight.Do must run concurrently for different keys")
}
