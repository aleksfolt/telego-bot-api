package api

import (
	"encoding/json"
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

func TestBindBusinessProfilePhotoObject(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.Header.SetContentType("application/json")
	ctx.Request.SetBodyString(`{
		"business_connection_id":"connection",
		"photo":{"type":"animated","animation":"attach://avatar","main_frame_timestamp":1.25}
	}`)

	var req converter.SetBusinessAccountProfilePhotoRequest
	assert.NoError(t, bindRequest(ctx, &req))
	var photo converter.InputProfilePhoto
	assert.NoError(t, json.Unmarshal(req.Photo, &photo))
	assert.Equal(t, "animated", photo.Type)
	assert.Equal(t, "attach://avatar", photo.Animation)
	assert.Equal(t, 1.25, photo.MainFrameTimestamp)
}

func TestParseContentDisposition(t *testing.T) {
	// Standard quoted format
	cd1 := `form-data; name="document"; filename="file.pdf"`
	name1, fn1 := ParseContentDisposition(cd1)
	assert.Equal(t, "document", name1)
	assert.Equal(t, "file.pdf", fn1)

	// Unquoted format with colons and URLs (e.g. grammY / web clients)
	cd2 := `form-data; name="ihxt8gcjii1n6hln"; filename=blob:http://localhost:5173/bf02517d-e64e-4866-9903-820847cf57c9`
	name2, fn2 := ParseContentDisposition(cd2)
	assert.Equal(t, "ihxt8gcjii1n6hln", name2)
	assert.Equal(t, "blob:http://localhost:5173/bf02517d-e64e-4866-9903-820847cf57c9", fn2)

	// Unquoted filename without quotes and extra semicolons
	cd3 := `form-data; name=photo; filename=test.jpg; size=1234`
	name3, fn3 := ParseContentDisposition(cd3)
	assert.Equal(t, "photo", name3)
	assert.Equal(t, "test.jpg", fn3)
}

func TestBindRequestThumbnailAlias(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.Header.SetContentType("application/x-www-form-urlencoded")
	ctx.Request.SetBodyString(`chat_id=12345&document=attach://doc1&thumb=attach://thumb1`)

	var req converter.SendDocumentRequest
	err := bindRequest(ctx, &req)
	assert.NoError(t, err)
	assert.Equal(t, int64(12345), req.ChatID)
	assert.Equal(t, "attach://doc1", req.Document)
	assert.Equal(t, "attach://thumb1", req.Thumbnail)
}
