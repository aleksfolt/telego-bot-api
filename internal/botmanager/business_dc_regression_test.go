package botmanager

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/fileid"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/require"
	"telego-bot-api/internal/converter"
)

func TestRegressionBusinessSendUsesConnectionDatacenter(t *testing.T) {
	var requestedDC int
	invoke := telegram.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		wrapped, ok := input.(*tg.InvokeWithBusinessConnectionRequest)
		require.True(t, ok)
		require.Equal(t, "business-test", wrapped.ConnectionID)
		require.IsType(t, &tg.MessagesSendMessageRequest{}, wrapped.Query)
		output.(*tg.UpdatesBox).Updates = &tg.UpdateShortSentMessage{ID: 42}
		return nil
	})
	b := mediaTestBot(invoke)
	b.busConnDCs["business-test"] = 7
	b.businessDCFactory = func(ctx context.Context, dcID int) (telegram.CloseInvoker, error) {
		requestedDC = dcID
		return testCloseInvoker{InvokeFunc: invoke}, nil
	}

	message, err := b.SendMessage(context.Background(), &converter.SendMessageRequest{
		BusinessConnectionID: "business-test",
		ChatID:               123,
		Text:                 "hello",
	})
	require.NoError(t, err)
	require.Equal(t, 7, requestedDC)
	require.Equal(t, int64(42), message.MessageID)
}

func TestRegressionBusinessStoryIsNotWrapped(t *testing.T) {
	photoID, err := fileid.EncodeFileID(fileid.FromPhoto(&tg.Photo{ID: 123, AccessHash: 456, DCID: 2}, 'x'))
	require.NoError(t, err)
	content, err := json.Marshal(map[string]string{"type": "photo", "photo": photoID})
	require.NoError(t, err)

	b := mediaTestBot(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		request, ok := input.(*tg.StoriesSendStoryRequest)
		require.True(t, ok, "stories.sendStory must be sent directly")
		peer := request.Peer.(*tg.InputPeerUser)
		require.Equal(t, int64(123), peer.UserID)
		output.(*tg.UpdatesBox).Updates = &tg.Updates{Updates: []tg.UpdateClass{&tg.UpdateStoryID{ID: 99}}}
		return nil
	})
	b.busConns["business-test"].UserChatID = 123

	result, err := b.PostStory(context.Background(), &converter.PostStoryRequest{
		BusinessConnectionID: "business-test",
		ChatID:               999,
		Content:              content,
	})
	require.NoError(t, err)
	response := result.(map[string]interface{})
	require.Equal(t, 99, response["id"])
	require.Equal(t, int64(123), response["chat"].(converter.Chat).ID)
}

func TestStoredBusinessConnectionIncludesRoutingMetadata(t *testing.T) {
	record := storedBusinessConnection{
		BusinessConnection: converter.BusinessConnection{ID: "business-test", IsEnabled: true},
		DCID:               7,
	}
	data, err := json.Marshal(record)
	require.NoError(t, err)
	require.JSONEq(t, `{"id":"business-test","user":{"id":0,"is_bot":false,"first_name":""},"user_chat_id":0,"date":0,"can_reply":false,"is_enabled":true,"_dc_id":7}`, string(data))

	var restored storedBusinessConnection
	require.NoError(t, json.Unmarshal(data, &restored))
	require.Equal(t, 7, restored.DCID)
	require.Equal(t, "business-test", restored.ID)
}
