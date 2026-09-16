package botmanager

import (
	"context"
	"fmt"
	"sync"
	"time"

	"telego-bot-api/internal/config"
	"telego-bot-api/internal/netpool"
	"telego-bot-api/internal/storage"
	"telego-bot-api/internal/webhook"

	"go.uber.org/zap"
)

// Manager manages active bot sessions.
type Manager struct {
	mu            sync.RWMutex
	bots          map[string]*BotInstance
	cfg           *config.Config
	redisStore    *storage.RedisStore
	dispatcher    *webhook.Dispatcher
	dialer        *netpool.PoolDialer
	logger        *zap.Logger
	janitorCancel context.CancelFunc
}

// NewManager creates a bot manager instance and starts the idle session janitor.
func NewManager(cfg *config.Config, rdb *storage.RedisStore, dispatcher *webhook.Dispatcher, logger *zap.Logger) *Manager {
	var dialer *netpool.PoolDialer
	if len(cfg.OutboundIPs) > 0 {
		dialer = netpool.NewPoolDialer(cfg.OutboundIPs)
	}

	ctx, cancel := context.WithCancel(context.Background())

	m := &Manager{
		bots:          make(map[string]*BotInstance),
		cfg:           cfg,
		redisStore:    rdb,
		dispatcher:    dispatcher,
		dialer:        dialer,
		logger:        logger,
		janitorCancel: cancel,
	}

	// Periodically sweep and hibernate inactive bots (idle for > 5 minutes)
	m.StartJanitor(ctx, 5*time.Minute)

	return m
}

// LoadAndStartAll restores all registered bots from Redis and runs them 24/7.
func (m *Manager) LoadAndStartAll(ctx context.Context) error {
	tokens, err := m.redisStore.GetRegisteredBots(ctx)
	if err != nil {
		return fmt.Errorf("read bots from redis: %w", err)
	}

	m.logger.Info("Restoring active bot sessions from Redis", zap.Int("count", len(tokens)))

	var wg sync.WaitGroup
	// Concurrency limiter for startup burst
	semaphore := make(chan struct{}, 50)

	for _, token := range tokens {
		wg.Add(1)
		go func(t string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			botCtx, cancel := context.WithCancel(context.Background())
			_ = cancel // preserved in BotInstance
			if _, err := m.GetOrCreate(botCtx, t); err != nil {
				m.logger.Warn("Failed to restore bot session", zap.String("token_prefix", t[:min(10, len(t))]), zap.Error(err))
			}
		}(token)
	}

	wg.Wait()
	m.logger.Info("All registered bots restored and listening for updates")
	return nil
}

// GetOrCreate returns an existing bot instance or initializes a new one.
func (m *Manager) GetOrCreate(ctx context.Context, token string) (*BotInstance, error) {
	m.mu.RLock()
	bot, ok := m.bots[token]
	m.mu.RUnlock()
	if ok {
		bot.touch()
		return bot, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check under write lock
	if bot, ok = m.bots[token]; ok {
		bot.touch()
		return bot, nil
	}

	if m.cfg.AppID == 0 || m.cfg.AppHash == "" {
		return nil, fmt.Errorf("TELEGRAM_API_ID and TELEGRAM_API_HASH must be configured")
	}

	bot = NewBotInstance(token, m.cfg.AppID, m.cfg.AppHash, m.dispatcher, m.redisStore, m.logger, m.dialer, m.cfg.MTProtoDebug)
	if err := bot.Start(ctx); err != nil {
		return nil, fmt.Errorf("start bot session: %w", err)
	}

	m.bots[token] = bot
	_ = m.redisStore.RegisterBot(context.Background(), token)

	m.logger.Info("Registered active bot", zap.Int("total_active_bots", len(m.bots)))
	return bot, nil
}

// Get returns an existing bot instance if registered and notifies it of activity.
func (m *Manager) Get(token string) *BotInstance {
	m.mu.RLock()
	bot := m.bots[token]
	m.mu.RUnlock()
	if bot != nil {
		bot.touch()
	}
	return bot
}

// CloseBot closes and unloads a bot session.
func (m *Manager) CloseBot(token string) bool {
	m.mu.Lock()
	bot, ok := m.bots[token]
	if ok {
		delete(m.bots, token)
	}
	m.mu.Unlock()

	if ok {
		bot.Stop()
		_ = m.redisStore.UnregisterBot(context.Background(), token)
		m.logger.Info("Bot session closed and unregistered", zap.Int("remaining_bots", len(m.bots)))
		return true
	}
	return false
}

// StopAll gracefully shuts down all active bot instances and stops background janitors.
func (m *Manager) StopAll() {
	if m.janitorCancel != nil {
		m.janitorCancel()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, bot := range m.bots {
		bot.Stop()
	}
	m.bots = make(map[string]*BotInstance)
}

// StartJanitor launches a background loop that puts idle bots into hibernation mode.
func (m *Manager) StartJanitor(ctx context.Context, idleTimeout time.Duration) {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.sweepIdleBots(idleTimeout)
			}
		}
	}()
}

// sweepIdleBots finds bots with no activity for > idleTimeout and evicts their RAM caches.
func (m *Manager) sweepIdleBots(idleTimeout time.Duration) {
	m.mu.RLock()
	bots := make([]*BotInstance, 0, len(m.bots))
	for _, b := range m.bots {
		bots = append(bots, b)
	}
	m.mu.RUnlock()

	now := time.Now()
	for _, b := range bots {
		if now.Sub(b.LastActive()) > idleTimeout && !b.IsHibernated() {
			b.Hibernate()
		}
	}
}

// BotStatus represents real-time runtime state of a bot for observability.
type BotStatus struct {
	BotID       int64  `json:"bot_id"`
	Username    string `json:"username,omitempty"`
	Hibernated  bool   `json:"hibernated"`
	IdleSeconds int    `json:"idle_seconds"`
	LastActive  string `json:"last_active"`
}

// GetBotsStatus returns real-time status of all managed bots.
func (m *Manager) GetBotsStatus() []BotStatus {
	m.mu.RLock()
	bots := make([]*BotInstance, 0, len(m.bots))
	for _, b := range m.bots {
		bots = append(bots, b)
	}
	m.mu.RUnlock()

	res := make([]BotStatus, 0, len(bots))
	now := time.Now()
	for _, b := range bots {
		last := b.LastActive()
		idle := int(now.Sub(last).Seconds())
		if idle < 0 {
			idle = 0
		}
		username := ""
		if me := b.GetMe(); me != nil {
			username = me.Username
		}
		res = append(res, BotStatus{
			BotID:       b.botID,
			Username:    username,
			Hibernated:  b.IsHibernated(),
			IdleSeconds: idle,
			LastActive:  last.Format(time.RFC3339),
		})
	}
	return res
}

// GetBotCounts returns current counts of (active, hibernated) bots.
func (m *Manager) GetBotCounts() (active int, hibernated int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, b := range m.bots {
		if b.IsHibernated() {
			hibernated++
		} else {
			active++
		}
	}
	return active, hibernated
}

// PingRedis tests connectivity to Redis store.
func (m *Manager) PingRedis(ctx context.Context) error {
	if m.redisStore == nil {
		return fmt.Errorf("redis store not initialized")
	}
	return m.redisStore.Ping(ctx)
}


