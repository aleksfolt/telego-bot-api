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
	PprofAddr     string
}

// Load loads configuration from environment variables and command-line flags.
func Load() *Config {
	cfg := &Config{
		AppID:         2040,
		AppHash:       "b18441a1ff607e10a989891a5462e627",
		HTTPAddr:      "0.0.0.0:8081",
		OutboundIPs:   nil,
		RedisAddr:     "127.0.0.1:6379",
		RedisPassword: "",
		RedisDB:       0,
		LogLevel:      "info",
		LogFormat:     "console",
		MTProtoDebug:  false,
	}

	if val := getEnv("TELEGO_API_ID", "TELEGRAM_API_ID", "API_ID"); val != "" {
		if id, err := strconv.Atoi(val); err == nil {
			cfg.AppID = id
		}
	}
	if val := getEnv("TELEGO_API_HASH", "TELEGRAM_API_HASH", "API_HASH"); val != "" {
		cfg.AppHash = val
	}
	if val := getEnv("TELEGO_HTTP_ADDR", "HTTP_ADDR"); val != "" {
		cfg.HTTPAddr = val
	}
	if val := getEnv("TELEGO_OUTBOUND_IPS", "OUTBOUND_IPS"); val != "" {
		cfg.OutboundIPs = strings.Split(val, ",")
	}
	if val := getEnv("TELEGO_REDIS_ADDR", "REDIS_ADDR"); val != "" {
		cfg.RedisAddr = val
	}
	if val := getEnv("TELEGO_REDIS_PASSWORD", "TELEGO_REDIS_PASS", "REDIS_PASSWORD", "REDIS_PASS"); val != "" {
		cfg.RedisPassword = val
	}
	if val := getEnv("TELEGO_REDIS_DB", "REDIS_DB"); val != "" {
		if db, err := strconv.Atoi(val); err == nil {
			cfg.RedisDB = db
		}
	}
	if val := getEnv("TELEGO_LOG_LEVEL", "LOG_LEVEL"); val != "" {
		cfg.LogLevel = val
	}
	if val := getEnv("TELEGO_LOG_FORMAT", "LOG_FORMAT"); val != "" {
		cfg.LogFormat = val
	}
	if val := getEnv("TELEGO_MTPROTO_DEBUG", "MTPROTO_DEBUG"); val == "true" || val == "1" {
		cfg.MTProtoDebug = true
	}
	if val := getEnv("TELEGO_PPROF_ADDR", "PPROF_ADDR"); val != "" {
		cfg.PprofAddr = val
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
	flag.StringVar(&cfg.PprofAddr, "pprof-addr", cfg.PprofAddr, "Optional address for pprof HTTP server (e.g. 127.0.0.1:6060)")
	flag.Parse()

	if ipsFlag != "" {
		cfg.OutboundIPs = strings.Split(ipsFlag, ",")
	}

	return cfg
}

func getEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

