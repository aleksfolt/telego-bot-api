package logging

import (
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger creates an enterprise-grade structured zap logger.
// format: "console" (colored, human-readable for development) or "json" (production Loki/ELK).
// level: "debug", "info", "warn", "error".
func NewLogger(levelStr, format string) (*zap.Logger, error) {
	level := zap.InfoLevel
	switch strings.ToLower(levelStr) {
	case "debug":
		level = zap.DebugLevel
	case "warn", "warning":
		level = zap.WarnLevel
	case "error":
		level = zap.ErrorLevel
	}

	if strings.ToLower(format) == "json" {
		cfg := zap.NewProductionConfig()
		cfg.Level = zap.NewAtomicLevelAt(level)
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		return cfg.Build()
	}

	// Console encoder configuration: clean, colored, human-readable
	encCfg := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalColorLevelEncoder,
		EncodeTime:     customConsoleTimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   customCallerEncoder,
	}

	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encCfg),
		zapcore.AddSync(os.Stdout),
		level,
	)

	return zap.New(core, zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel)), nil
}

// customConsoleTimeEncoder formats timestamps cleanly: "15:04:05.000"
func customConsoleTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("15:04:05.000"))
}

// customCallerEncoder shortens the caller path to "pkg/file.go:line"
func customCallerEncoder(caller zapcore.EntryCaller, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(caller.TrimmedPath())
}

// MTProtoLogger creates a filtered logger for gotd's internal MTProto transport.
// If enableWireDebug is false, low-level MTProto protocol tracing (msgs_ack, invokeWithLayer,
// frame dumps) is silenced to Info/Warn level, keeping application debug logs clear.
func MTProtoLogger(root *zap.Logger, enableWireDebug bool) *zap.Logger {
	if enableWireDebug {
		return root.Named("mtproto")
	}
	// Silence noisy transport debug packets
	return root.Named("mtproto").WithOptions(zap.IncreaseLevel(zap.InfoLevel))
}
