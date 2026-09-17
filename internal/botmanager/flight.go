package botmanager

import (
	"sync"
)

type flightCall struct {
	wg  sync.WaitGroup
	val *BotInstance
	err error
}

type botFlight struct {
	mu sync.Mutex
	m  map[string]*flightCall
}

func newBotFlight() *botFlight {
	return &botFlight{
		m: make(map[string]*flightCall),
	}
}

// Do executes and returns the results of the given function, making
// sure that only one execution is in-flight for a given key at a
// time. If a duplicate comes in, the duplicate caller waits for the
// original to complete and receives the same results.
func (g *botFlight) Do(key string, fn func() (*BotInstance, error)) (*BotInstance, error) {
	g.mu.Lock()
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}
	c := new(flightCall)
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	c.val, c.err = fn()
	c.wg.Done()

	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()

	return c.val, c.err
}
