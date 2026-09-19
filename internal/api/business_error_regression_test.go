package api

import (
	"fmt"
	"testing"

	"github.com/gotd/td/tgerr"
	"github.com/stretchr/testify/require"
)

func TestRegressionWrappedBusinessAuthError(t *testing.T) {
	// Match the %w wrapping used by businessErrorMiddleware and the caller.
	err := fmt.Errorf("mtproto business edit message text: %w", fmt.Errorf("business connection invalid: %w", tgerr.New(401, "AUTH_KEY_UNREGISTERED")))
	code, desc, _ := MapRpcError(err)
	require.Equal(t, 400, code, desc)
	require.Equal(t, "Bad Request: BUSINESS_CONNECTION_INVALID", desc)
}
