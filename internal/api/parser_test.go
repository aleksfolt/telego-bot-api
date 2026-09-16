package api

import (
	"testing"

	"telego-bot-api/internal/converter"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
)

func TestBindRequestJSON(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.Header.SetContentType("application/json")
	ctx.Request.SetBodyString(`{"chat_id": 12345, "text": "Hello world"}`)

	var req converter.SendMessageRequest
	err := bindRequest(ctx, &req)
	assert.NoError(t, err)
	assert.Equal(t, int64(12345), req.ChatID)
	assert.Equal(t, "Hello world", req.Text)
}

func TestBindRequestQueryArgs(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/bot123/setWebhook?url=https://example.com/webhook&secret_token=sec123")

	var req converter.SetWebhookRequest
	err := bindRequest(ctx, &req)
	assert.NoError(t, err)
	assert.Equal(t, "https://example.com/webhook", req.URL)
	assert.Equal(t, "sec123", req.SecretToken)
}

func TestBindRequestFormUrlencoded(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.Header.SetContentType("application/x-www-form-urlencoded")
	ctx.Request.SetBodyString(`chat_id=987654&text=Form+Text&protect_content=true`)

	var req converter.SendMessageRequest
	err := bindRequest(ctx, &req)
	assert.NoError(t, err)
	assert.Equal(t, int64(987654), req.ChatID)
	assert.Equal(t, "Form Text", req.Text)
	assert.True(t, req.ProtectContent)
}

func TestBindRequestNestedJSONInForm(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.Header.SetContentType("application/x-www-form-urlencoded")
	ctx.Request.SetBodyString(`chat_id=123&text=Inline&reply_markup={"inline_keyboard":[[{"text":"click","callback_data":"btn"}]]}`)

	var req converter.SendMessageRequest
	err := bindRequest(ctx, &req)
	assert.NoError(t, err)
	assert.Equal(t, int64(123), req.ChatID)
	assert.NotEmpty(t, req.ReplyMarkup)
	assert.Contains(t, string(req.ReplyMarkup), "callback_data")
}
