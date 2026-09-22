package api_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"os"
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

func TestExtractFileFromRequest(t *testing.T) {
	// 1. Test attach:// with multipart file header (grammY InputFile behavior)
	{
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)

		err := writer.WriteField("chat_id", "12345")
		require.NoError(t, err)
		err = writer.WriteField("document", "attach://upload_part_123")
		require.NoError(t, err)

		part, err := writer.CreateFormFile("upload_part_123", "archive.zip")
		require.NoError(t, err)
		_, err = part.Write([]byte("dummy zip content"))
		require.NoError(t, err)

		err = writer.Close()
		require.NoError(t, err)

		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetContentType(writer.FormDataContentType())
		ctx.Request.SetBody(body.Bytes())

		data, filename := api.ExtractFileFromRequest(ctx, "document", "attach://upload_part_123")
		assert.Equal(t, "archive.zip", filename)
		assert.Equal(t, []byte("dummy zip content"), data)
	}

	// 2. Test direct multipart field name
	{
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)

		part, err := writer.CreateFormFile("photo", "image.png")
		require.NoError(t, err)
		_, err = part.Write([]byte("image bytes"))
		require.NoError(t, err)

		err = writer.Close()
		require.NoError(t, err)

		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetContentType(writer.FormDataContentType())
		ctx.Request.SetBody(body.Bytes())

		data, filename := api.ExtractFileFromRequest(ctx, "photo", "")
		assert.Equal(t, "image.png", filename)
		assert.Equal(t, []byte("image bytes"), data)
	}

	// 3. Test local file path
	{
		tmpFile, err := os.CreateTemp("", "test_doc_*.txt")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())

		_, err = tmpFile.WriteString("local file data")
		require.NoError(t, err)
		_ = tmpFile.Close()

		ctx := &fasthttp.RequestCtx{}
		data, filename := api.ExtractFileFromRequest(ctx, "document", tmpFile.Name())
		assert.Equal(t, []byte("local file data"), data)
		assert.NotEmpty(t, filename)
	}

	// 4. Test missing attachment returns empty
	{
		ctx := &fasthttp.RequestCtx{}
		data, filename := api.ExtractFileFromRequest(ctx, "document", "attach://non_existent")
		assert.Nil(t, data)
		assert.Empty(t, filename)
	}

	// 5. Test grammY exact raw multipart stream (lowercase content-disposition, no spaces, unquoted filename)
	{
		boundary := "----------1234567890"
		rawBody := "--" + boundary + "\r\n" +
			"content-disposition:form-data;name=\"chat_id\"\r\n\r\n" +
			"12345\r\n" +
			"--" + boundary + "\r\n" +
			"content-disposition:form-data;name=\"document\"\r\n\r\n" +
			"attach://ihxt8gcjii1n6hln\r\n" +
			"--" + boundary + "\r\n" +
			"content-disposition:form-data;name=\"ihxt8gcjii1n6hln\";filename=test.zip\r\n" +
			"content-type:application/octet-stream\r\n\r\n" +
			"ZIP_CONTENT_HERE\r\n" +
			"--" + boundary + "--\r\n"

		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetContentType("multipart/form-data; boundary=" + boundary)
		ctx.Request.SetBody([]byte(rawBody))

		data, filename := api.ExtractFileFromRequest(ctx, "document", "attach://ihxt8gcjii1n6hln")
		t.Logf("Case 5 result - data: %q, filename: %q", string(data), filename)
		assert.Equal(t, "test.zip", filename)
		assert.Equal(t, []byte("ZIP_CONTENT_HERE"), data)
	}

	// 6. Test grammY with COLON in unquoted filename: filename=8907195935:7026718333.zip
	{
		boundary := "----------1234567890"
		rawBody := "--" + boundary + "\r\n" +
			"content-disposition:form-data;name=\"chat_id\"\r\n\r\n" +
			"12345\r\n" +
			"--" + boundary + "\r\n" +
			"content-disposition:form-data;name=\"document\"\r\n\r\n" +
			"attach://ihxt8gcjii1n6hln\r\n" +
			"--" + boundary + "\r\n" +
			"content-disposition:form-data;name=\"ihxt8gcjii1n6hln\";filename=8907195935:7026718333.zip\r\n" +
			"content-type:application/octet-stream\r\n\r\n" +
			"ZIP_WITH_COLON_HERE\r\n" +
			"--" + boundary + "--\r\n"

		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetContentType("multipart/form-data; boundary=" + boundary)
		ctx.Request.SetBody([]byte(rawBody))

		data, filename := api.ExtractFileFromRequest(ctx, "document", "attach://ihxt8gcjii1n6hln")
		assert.Equal(t, "8907195935:7026718333.zip", filename)
		assert.Equal(t, []byte("ZIP_WITH_COLON_HERE"), data)
	}

	// 7. Test ParseContentDisposition edge cases
	{
		name, fn := api.ParseContentDisposition("form-data; name=\"doc\"; filename=8907195935:7026718333.zip")
		assert.Equal(t, "doc", name)
		assert.Equal(t, "8907195935:7026718333.zip", fn)

		name, fn = api.ParseContentDisposition("form-data; filename=\"cool:file.txt\"; name=\"upload\"")
		assert.Equal(t, "upload", name)
		assert.Equal(t, "cool:file.txt", fn)

		name, fn = api.ParseContentDisposition("form-data; name=test; filename=archive.tar.gz; extra=1")
		assert.Equal(t, "test", name)
		assert.Equal(t, "archive.tar.gz", fn)
	}
}

func getKeys(m map[string][]string) []string {
	var res []string
	for k := range m {
		res = append(res, k)
	}
	return res
}

func getFileKeys(m map[string][]*multipart.FileHeader) []string {
	var res []string
	for k := range m {
		res = append(res, k)
	}
	return res
}

func TestServerFastHTTPErrorHandler(t *testing.T) {
	logger := zap.NewNop()
	cfg := &config.Config{
		HTTPAddr: "127.0.0.1:0",
	}
	rdb := storage.NewRedisStore("127.0.0.1:6379", "", 0)
	dispatcher := webhook.NewDispatcher(1, 10, logger)
	defer dispatcher.Stop()
	bm := botmanager.NewManager(cfg, rdb, dispatcher, logger)
	srv := api.NewServer(cfg.HTTPAddr, bm, dispatcher, logger, cfg)

	// 1. Timeout error returns 408 JSON
	{
		ctx := &fasthttp.RequestCtx{}
		srv.HandleFastHTTPError(ctx, errors.New("connection read timeout: deadline exceeded"))

		assert.Equal(t, 408, ctx.Response.StatusCode())
		assert.Equal(t, "application/json", string(ctx.Response.Header.ContentType()))

		var resp converter.ApiResponse
		err := json.Unmarshal(ctx.Response.Body(), &resp)
		require.NoError(t, err)
		assert.False(t, resp.OK)
		assert.Equal(t, 408, resp.ErrorCode)
		assert.Equal(t, "Request Timeout", resp.Description)
	}

	// 2. Request body too large returns 413 JSON
	{
		ctx := &fasthttp.RequestCtx{}
		srv.HandleFastHTTPError(ctx, fasthttp.ErrBodyTooLarge)

		assert.Equal(t, 413, ctx.Response.StatusCode())
		assert.Equal(t, "application/json", string(ctx.Response.Header.ContentType()))

		var resp converter.ApiResponse
		err := json.Unmarshal(ctx.Response.Body(), &resp)
		require.NoError(t, err)
		assert.False(t, resp.OK)
		assert.Equal(t, 413, resp.ErrorCode)
		assert.Equal(t, "Request Entity Too Large", resp.Description)
	}

	// 3. Generic bad request returns 400 JSON
	{
		ctx := &fasthttp.RequestCtx{}
		srv.HandleFastHTTPError(ctx, errors.New("malformed HTTP headers"))

		assert.Equal(t, 400, ctx.Response.StatusCode())
		assert.Equal(t, "application/json", string(ctx.Response.Header.ContentType()))

		var resp converter.ApiResponse
		err := json.Unmarshal(ctx.Response.Body(), &resp)
		require.NoError(t, err)
		assert.False(t, resp.OK)
		assert.Equal(t, 400, resp.ErrorCode)
		assert.Contains(t, resp.Description, "malformed HTTP headers")
	}
}
