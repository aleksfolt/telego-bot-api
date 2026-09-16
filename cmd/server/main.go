package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"telego-bot-api/internal/api"
	"telego-bot-api/internal/botmanager"
	"telego-bot-api/internal/config"
	"telego-bot-api/internal/logging"
	"telego-bot-api/internal/storage"
	"telego-bot-api/internal/webhook"

	"go.uber.org/zap"
)

func main() {
	cfg := config.Load()

	logger, err := logging.NewLogger(cfg.LogLevel, cfg.LogFormat)
	if err != nil {
		panic(err)
	}
	defer logger.Sync() //nolint:errcheck

	logger.Info("Initializing telego-bot-api gateway",
		zap.Int("app_id", cfg.AppID),
		zap.String("http_addr", cfg.HTTPAddr),
		zap.String("redis_addr", cfg.RedisAddr),
		zap.Int("outbound_ips", len(cfg.OutboundIPs)),
	)

	// 1. Initialize Redis Store
	rdb := storage.NewRedisStore(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 3*time.Second)
	if err := rdb.Ping(pingCtx); err != nil {
		logger.Warn("Redis ping failed, continuing anyway", zap.Error(err), zap.String("addr", cfg.RedisAddr))
	} else {
		logger.Info("Connected to Redis successfully", zap.String("addr", cfg.RedisAddr))
	}
	pingCancel()

	// 2. High-throughput in-memory webhook dispatcher (500 concurrent workers, 50k queue)
	dispatcher := webhook.NewDispatcher(500, 50000, logger)

	// 3. Multi-bot MTProto session manager backed by Redis
	bm := botmanager.NewManager(cfg, rdb, dispatcher, logger)

	// 4. Restore and keep-alive all registered bots from Redis in background
	go func() {
		restoreCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := bm.LoadAndStartAll(restoreCtx); err != nil {
			logger.Warn("Failed to restore all bots from Redis", zap.Error(err))
		}
	}()

	// 5. FastHTTP server for grammY requests
	srv := api.NewServer(cfg.HTTPAddr, bm, logger)

	// Setup graceful shutdown listener for SIGINT and SIGTERM
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	serverErr := make(chan error, 1)
	go func() {
		if err := srv.Start(); err != nil {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		logger.Fatal("HTTP server failed unexpectedly", zap.Error(err))
	case sig := <-sigChan:
		logger.Warn("Received termination signal, starting graceful shutdown...", zap.String("signal", sig.String()))
	}

	shutdownStart := time.Now()

	// 1. Stop accepting new HTTP requests and wait for in-flight requests to complete
	if err := srv.Shutdown(); err != nil {
		logger.Warn("HTTP server shutdown encountered error", zap.Error(err))
	} else {
		logger.Info("HTTP server stopped gracefully")
	}

	// 2. Stop webhook dispatcher (flush and terminate workers)
	dispatcher.Stop()
	logger.Info("Webhook dispatcher stopped")

	// 3. Close all MTProto bot sessions
	bm.StopAll()
	logger.Info("Bot MTProto sessions closed")

	// 4. Close Redis connection pool
	if err := rdb.Close(); err != nil {
		logger.Warn("Redis store close encountered error", zap.Error(err))
	} else {
		logger.Info("Redis connection pool closed")
	}

	logger.Info("Telego Bot API gateway stopped cleanly", zap.Duration("shutdown_duration", time.Since(shutdownStart)))
}
