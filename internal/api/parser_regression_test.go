package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
	"telego-bot-api/internal/converter"
)

func TestRegressionLiteralFormText(t *testing.T) {
	for _, text := range []string{"123", "true", "false", "1.5", "[1,2]", `{"hello":1}`} {
		for _, format := range []string{"form", "query", "multipart", "json_with_query"} {
			t.Run(format+"/"+text, func(t *testing.T) {
				ctx := &fasthttp.RequestCtx{}
				ctx.Request.Header.SetMethod("POST")
				values := url.Values{"chat_id": {"123"}, "text": {text}}
				switch format {
				case "form":
					ctx.Request.Header.SetContentType("application/x-www-form-urlencoded")
					ctx.Request.SetBodyString(values.Encode())
				case "query":
					ctx.Request.SetRequestURI("/sendMessage?" + values.Encode())
				case "multipart":
					var body bytes.Buffer
					writer := multipart.NewWriter(&body)
					require.NoError(t, writer.WriteField("chat_id", "123"))
					require.NoError(t, writer.WriteField("text", text))
					require.NoError(t, writer.Close())
					ctx.Request.Header.SetContentType(writer.FormDataContentType())
					ctx.Request.SetBody(body.Bytes())
				case "json_with_query":
					ctx.Request.SetRequestURI("/sendMessage?chat_id=123")
					ctx.Request.Header.SetContentType("application/json")
					body, err := json.Marshal(map[string]string{"text": text})
					require.NoError(t, err)
					ctx.Request.SetBody(body)
				}
				var req converter.SendMessageRequest
				require.NoError(t, bindRequest(ctx, &req))
				require.Equal(t, text, req.Text)
			})
		}
	}
}

func TestParserTypedFieldsAndLargeIdentifiers(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.Header.SetContentType("application/x-www-form-urlencoded")
	ctx.Request.SetBodyString(url.Values{
		"chat_id": {"123"}, "text": {"true"}, "protect_content": {"true"},
		"business_connection_id": {"000123"}, "reply_parameters": {`{"message_id":9007199254740993}`},
	}.Encode())
	var req converter.SendMessageRequest
	require.NoError(t, bindRequest(ctx, &req))
	require.True(t, req.ProtectContent)
	require.Equal(t, "000123", req.BusinessConnectionID)
	require.Equal(t, "true", req.Text)
	require.NotNil(t, req.ReplyParameters)
	require.Equal(t, int64(9007199254740993), req.ReplyParameters.MessageID)
}
