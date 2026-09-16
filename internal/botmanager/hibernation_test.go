package botmanager

import (
	"testing"
	"time"

	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"telego-bot-api/internal/config"
	"telego-bot-api/internal/converter"
	"telego-bot-api/internal/peer"
)

func TestBotInstance_Hibernation(t *testing.T) {
	bot := &BotInstance{
		botID:      12345678,
		peers:      peer.NewStorage(),
		mediaCache: make(map[string]*tg.InputPhoto),
		docCache:   make(map[string]*tg.InputDocument),
		busConns:   make(map[string]*converter.BusinessConnection),
		logger:     zap.NewNop(),
	}
	bot.lastActive.Store(time.Now().UnixNano())

	// Populate in-memory caches
	bot.peers.SaveUser(111, 222)
	bot.peers.SaveChannel(333, 444)
	bot.mediaCache["test-photo"] = &tg.InputPhoto{ID: 100}
	bot.docCache["test-doc"] = &tg.InputDocument{ID: 200}

	assert.False(t, bot.IsHibernated())

	// Enter hibernation
	bot.Hibernate()

	assert.True(t, bot.IsHibernated())

	// In-memory caches must be cleared
	bot.mediaCacheMu.RLock()
	assert.Empty(t, bot.mediaCache, "mediaCache must be empty after hibernation")
	assert.Empty(t, bot.docCache, "docCache must be empty after hibernation")
	bot.mediaCacheMu.RUnlock()

	// Channel peer storage must be cleared
	_, err := bot.peers.ResolvePeer(-1000000000333)
	assert.Error(t, err, "channel peer must not be resolvable after peer storage is cleared")

	// Wake up
	bot.WakeUp()
	assert.False(t, bot.IsHibernated())
}

func TestBotInstance_TouchWakesUp(t *testing.T) {
	bot := &BotInstance{
		botID:      12345678,
		peers:      peer.NewStorage(),
		mediaCache: make(map[string]*tg.InputPhoto),
		docCache:   make(map[string]*tg.InputDocument),
		logger:     zap.NewNop(),
	}
	bot.lastActive.Store(time.Now().Add(-10 * time.Minute).UnixNano())

	bot.Hibernate()
	require.True(t, bot.IsHibernated())

	oldActive := bot.LastActive()

	// Touching must update timestamp and wake up the bot
	bot.touch()
	assert.False(t, bot.IsHibernated(), "touch() must wake up hibernated bot")
	assert.True(t, bot.LastActive().After(oldActive), "touch() must update lastActive")
}

func TestManager_SweepIdleBots(t *testing.T) {
	m := &Manager{
		bots:   make(map[string]*BotInstance),
		cfg:    &config.Config{},
		logger: zap.NewNop(),
	}

	activeBot := &BotInstance{
		botID:      1001,
		peers:      peer.NewStorage(),
		mediaCache: make(map[string]*tg.InputPhoto),
		docCache:   make(map[string]*tg.InputDocument),
		logger:     zap.NewNop(),
	}
	activeBot.lastActive.Store(time.Now().UnixNano())

	idleBot := &BotInstance{
		botID:      1002,
		peers:      peer.NewStorage(),
		mediaCache: make(map[string]*tg.InputPhoto),
		docCache:   make(map[string]*tg.InputDocument),
		logger:     zap.NewNop(),
	}
	// Idle for 10 minutes
	idleBot.lastActive.Store(time.Now().Add(-10 * time.Minute).UnixNano())

	m.bots["token_active"] = activeBot
	m.bots["token_idle"] = idleBot

	// Sweep with 5-minute timeout
	m.sweepIdleBots(5 * time.Minute)

	assert.False(t, activeBot.IsHibernated(), "active bot must not be hibernated")
	assert.True(t, idleBot.IsHibernated(), "idle bot must be hibernated")

	// Calling Get on idleBot must wake it up
	got := m.Get("token_idle")
	assert.NotNil(t, got)
	assert.False(t, got.IsHibernated(), "m.Get must touch and wake up idle bot")
}
