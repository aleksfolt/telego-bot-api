package logging_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"telego-bot-api/internal/logging"
)

func TestNewLogger_Console(t *testing.T) {
	log, err := logging.NewLogger("debug", "console")
	require.NoError(t, err)
	require.NotNil(t, log)

	log.Info("Test info message", zap.String("module", "test"))
	log.Debug("Test debug message", zap.Int("counter", 42))
}

func TestNewLogger_JSON(t *testing.T) {
	log, err := logging.NewLogger("info", "json")
	require.NoError(t, err)
	require.NotNil(t, log)

	log.Info("Test json info message", zap.String("service", "telego"))
}

func TestMTProtoLogger_Filtered(t *testing.T) {
	root, err := logging.NewLogger("debug", "console")
	require.NoError(t, err)

	filtered := logging.MTProtoLogger(root, false)
	require.NotNil(t, filtered)
	// Must not panic
	filtered.Info("Filtered info")
}
