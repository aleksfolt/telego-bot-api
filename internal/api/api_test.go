package api_test

import (
	"encoding/json"
	"testing"

	"telego-bot-api/internal/api"
	"telego-bot-api/internal/botmanager"
	"telego-bot-api/internal/config"
	"telego-bot-api/internal/converter"
	"telego-bot-api/internal/storage"
	"telego-bot-api/internal/webhook"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

func TestServerRouting(t *testing.T) {
	logger := zap.NewNop()
	cfg := &config.Config{
		AppID:    12345,
		AppHash:  "test_hash",
		HTTPAddr: "127.0.0.1:0",
	}

	rdb := storage.NewRedisStore("127.0.0.1:6379", "", 0)
	dispatcher := webhook.NewDispatcher(1, 10, logger)
	defer dispatcher.Stop()

	bm := botmanager.NewManager(cfg, rdb, dispatcher, logger)
	srv := api.NewServer(cfg.HTTPAddr, bm, dispatcher, logger)

	// 1. Invalid path (no /bot prefix)
	{
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/some/invalid/path")
		srv.HandleRequest(ctx)

		assert.Equal(t, 404, ctx.Response.StatusCode())

		var resp converter.ApiResponse
		err := json.Unmarshal(ctx.Response.Body(), &resp)
		require.NoError(t, err)
		assert.False(t, resp.OK)
		assert.Equal(t, 404, resp.ErrorCode)
		assert.Equal(t, "Not Found", resp.Description)
	}

	// 2. Malformed bot path
	{
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/bot12345")
		srv.HandleRequest(ctx)

		assert.Equal(t, 400, ctx.Response.StatusCode())

		var resp converter.ApiResponse
		err := json.Unmarshal(ctx.Response.Body(), &resp)
		require.NoError(t, err)
		assert.False(t, resp.OK)
		assert.Equal(t, 400, resp.ErrorCode)
	}

	// 3. /health and /status endpoints
	{
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/health")
		srv.HandleRequest(ctx)

		assert.Equal(t, 200, ctx.Response.StatusCode())
		var res map[string]any
		err := json.Unmarshal(ctx.Response.Body(), &res)
		require.NoError(t, err)
		assert.Equal(t, true, res["ok"])
		assert.Equal(t, "healthy", res["status"])
	}

	// 4. /metrics endpoint
	{
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/metrics")
		srv.HandleRequest(ctx)

		assert.Equal(t, 200, ctx.Response.StatusCode())
		assert.Contains(t, string(ctx.Response.Body()), "telego_uptime_seconds")
		assert.Contains(t, string(ctx.Response.Body()), "telego_bots_total")
	}
}

func TestApiResponse_ParametersSerialization(t *testing.T) {
	resp := converter.ApiResponse{
		OK:          false,
		ErrorCode:   429,
		Description: "Too Many Requests: retry after 10",
		Parameters: &converter.ResponseParameters{
			RetryAfter: 10,
		},
	}
	bytes, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded converter.ApiResponse
	err = json.Unmarshal(bytes, &decoded)
	require.NoError(t, err)
	require.False(t, decoded.OK)
	require.Equal(t, 429, decoded.ErrorCode)
	require.NotNil(t, decoded.Parameters)
	require.Equal(t, 10, decoded.Parameters.RetryAfter)
}

