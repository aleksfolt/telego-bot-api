package config

import (
	"flag"
	"os"
	"strconv"
	"strings"
)

// Config contains runtime configuration parameters for telego-bot-api.
type Config struct {
	AppID         int
	AppHash       string
	HTTPAddr      string
	OutboundIPs   []string
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	LogLevel      string
	LogFormat     string
	MTProtoDebug  bool
}

// Load loads configuration from environment variables and command-line flags.
func Load() *Config {
	cfg := &Config{
		AppID:         0,
		AppHash:       "",
		HTTPAddr:      "0.0.0.0:8081",
		OutboundIPs:   nil,
		RedisAddr:     "127.0.0.1:6379",
		RedisPassword: "",
		RedisDB:       0,
		LogLevel:      "info",
		LogFormat:     "console",
		MTProtoDebug:  false,
	}

	if val := os.Getenv("TELEGRAM_API_ID"); val != "" {
		if id, err := strconv.Atoi(val); err == nil {
			cfg.AppID = id
		}
	}
	if val := os.Getenv("TELEGRAM_API_HASH"); val != "" {
		cfg.AppHash = val
	}
	if val := os.Getenv("HTTP_ADDR"); val != "" {
		cfg.HTTPAddr = val
	}
	if val := os.Getenv("OUTBOUND_IPS"); val != "" {
		cfg.OutboundIPs = strings.Split(val, ",")
	}
	if val := os.Getenv("REDIS_ADDR"); val != "" {
		cfg.RedisAddr = val
	}
	if val := os.Getenv("REDIS_PASSWORD"); val != "" {
		cfg.RedisPassword = val
	}
	if val := os.Getenv("REDIS_DB"); val != "" {
		if db, err := strconv.Atoi(val); err == nil {
			cfg.RedisDB = db
		}
	}
	if val := os.Getenv("LOG_LEVEL"); val != "" {
		cfg.LogLevel = val
	}
	if val := os.Getenv("LOG_FORMAT"); val != "" {
		cfg.LogFormat = val
	}
	if val := os.Getenv("TELEGO_MTPROTO_DEBUG"); val == "true" || val == "1" {
		cfg.MTProtoDebug = true
	}

	var ipsFlag string
	flag.IntVar(&cfg.AppID, "api-id", cfg.AppID, "Telegram API ID (from my.telegram.org)")
	flag.StringVar(&cfg.AppHash, "api-hash", cfg.AppHash, "Telegram API Hash")
	flag.StringVar(&cfg.HTTPAddr, "http-addr", cfg.HTTPAddr, "HTTP listen address")
	flag.StringVar(&ipsFlag, "outbound-ips", "", "Comma-separated list of local outbound IPs for MTProto connections")
	flag.StringVar(&cfg.RedisAddr, "redis-addr", cfg.RedisAddr, "Redis server address (e.g. 127.0.0.1:6379)")
	flag.StringVar(&cfg.RedisPassword, "redis-pass", cfg.RedisPassword, "Redis server password")
	flag.IntVar(&cfg.RedisDB, "redis-db", cfg.RedisDB, "Redis database number")
	flag.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "Log level: debug, info, warn, error")
	flag.StringVar(&cfg.LogFormat, "log-format", cfg.LogFormat, "Log format: console (human-readable colors) or json (production)")
	flag.BoolVar(&cfg.MTProtoDebug, "mtproto-debug", cfg.MTProtoDebug, "Enable verbose wire-level MTProto transport debug dumps (default: false)")
	flag.Parse()

	if ipsFlag != "" {
		cfg.OutboundIPs = strings.Split(ipsFlag, ",")
	}

	return cfg
}
