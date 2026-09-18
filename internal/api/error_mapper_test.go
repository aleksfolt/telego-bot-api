package api

import (
	"context"
	"fmt"
	"testing"

	"github.com/gotd/td/tgerr"
	"github.com/stretchr/testify/require"
)

func TestMapRpcError_FloodWait(t *testing.T) {
	// From gotd tgerr
	err := tgerr.New(420, "FLOOD_WAIT_15")
	code, desc, params := MapRpcError(err)
	require.Equal(t, 429, code)
	require.Equal(t, "Too Many Requests: retry after 15", desc)
	require.NotNil(t, params)
	require.Equal(t, 15, params.RetryAfter)

	// From generic error string
	errStr := fmt.Errorf("send message: rpc error code 420: FLOOD_WAIT_30")
	code2, desc2, params2 := MapRpcError(errStr)
	require.Equal(t, 429, code2)
	require.Equal(t, "Too Many Requests: retry after 30", desc2)
	require.NotNil(t, params2)
	require.Equal(t, 30, params2.RetryAfter)
}

func TestMapRpcError_Forbidden(t *testing.T) {
	err := tgerr.New(403, "BOT_BLOCKED")
	code, desc, params := MapRpcError(err)
	require.Equal(t, 403, code)
	require.Equal(t, "Forbidden: bot was blocked by the user", desc)
	require.Nil(t, params)

	errDeact := tgerr.New(400, "USER_DEACTIVATED")
	code2, desc2, _ := MapRpcError(errDeact)
	require.Equal(t, 403, code2)
	require.Equal(t, "Forbidden: user is deactivated", desc2)
}

func TestMapRpcError_MigrateToChatID(t *testing.T) {
	err := tgerr.New(400, "MIGRATE_TO_123456789")
	code, desc, params := MapRpcError(err)
	require.Equal(t, 400, code)
	require.Equal(t, "Bad Request: group chat was upgraded to a supergroup chat", desc)
	require.NotNil(t, params)
	require.Equal(t, int64(-1000123456789), params.MigrateToChatID)
}

func TestMapRpcError_StandardErrors(t *testing.T) {
	tests := []struct {
		rpcErr       error
		expectedCode int
		expectedDesc string
	}{
		{
			rpcErr:       tgerr.New(400, "CHAT_NOT_FOUND"),
			expectedCode: 400,
			expectedDesc: "Bad Request: chat not found",
		},
		{
			rpcErr:       tgerr.New(400, "MESSAGE_NOT_MODIFIED"),
			expectedCode: 400,
			expectedDesc: "Bad Request: message is not modified: specified new message content and reply markup are exactly the same as a current content and reply markup of the message",
		},
		{
			rpcErr:       tgerr.New(400, "MESSAGE_ID_INVALID"),
			expectedCode: 400,
			expectedDesc: "Bad Request: message to edit not found",
		},
		{
			rpcErr:       context.DeadlineExceeded,
			expectedCode: 504,
			expectedDesc: "Gateway Timeout: request timed out",
		},
		{
			rpcErr:       tgerr.New(401, "AUTH_KEY_UNREGISTERED"),
			expectedCode: 401,
			expectedDesc: "Unauthorized: bot token is invalid or revoked",
		},
		{
			rpcErr:       fmt.Errorf("rpcDoRequest: rpc error code 401: AUTH_KEY_UNREGISTERED"),
			expectedCode: 401,
			expectedDesc: "Unauthorized: bot token is invalid or revoked",
		},
		{
			rpcErr:       fmt.Errorf("business connection invalid: rpc error code 401: AUTH_KEY_UNREGISTERED"),
			expectedCode: 400,
			expectedDesc: "Bad Request: BUSINESS_CONNECTION_INVALID",
		},
		{
			rpcErr:       tgerr.New(401, "TOKEN_INVALID"),
			expectedCode: 401,
			expectedDesc: "Unauthorized: bot token is invalid or revoked",
		},
		{
			rpcErr:       tgerr.New(400, "BUSINESS_CONNECTION_INVALID"),
			expectedCode: 400,
			expectedDesc: "Bad Request: BUSINESS_CONNECTION_INVALID",
		},
		{
			rpcErr:       tgerr.New(400, "BUSINESS_PEER_INVALID"),
			expectedCode: 400,
			expectedDesc: "Bad Request: BUSINESS_PEER_INVALID",
		},
	}

	for _, tc := range tests {
		code, desc, _ := MapRpcError(tc.rpcErr)
		require.Equal(t, tc.expectedCode, code)
		require.Equal(t, tc.expectedDesc, desc)
	}
}
