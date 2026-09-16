package converter_test

import (
	"encoding/json"
	"testing"

	"telego-bot-api/internal/converter"

	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseInlineKeyboard(t *testing.T) {
	raw := []byte(`{
		"inline_keyboard": [
			[
				{"text": "Button 1", "callback_data": "action_1"},
				{"text": "Google", "url": "https://google.com"}
			]
		]
	}`)

	markup, err := converter.ParseReplyMarkup(raw)
	require.NoError(t, err)
	require.NotNil(t, markup)

	inlineMarkup, ok := markup.(*tg.ReplyInlineMarkup)
	require.True(t, ok)
	require.Len(t, inlineMarkup.Rows, 1)
	require.Len(t, inlineMarkup.Rows[0].Buttons, 2)

	btn1, ok := inlineMarkup.Rows[0].Buttons[0].(*tg.KeyboardButtonCallback)
	require.True(t, ok)
	assert.Equal(t, "Button 1", btn1.Text)
	assert.Equal(t, []byte("action_1"), btn1.Data)

	btn2, ok := inlineMarkup.Rows[0].Buttons[1].(*tg.KeyboardButtonURL)
	require.True(t, ok)
	assert.Equal(t, "Google", btn2.Text)
	assert.Equal(t, "https://google.com", btn2.URL)
}

func TestParseInlineKeyboard_WebApp(t *testing.T) {
	raw := []byte(`{
		"inline_keyboard": [
			[
				{"text": "Mini App", "web_app": {"url": "https://webapp.example.com"}}
			]
		]
	}`)

	markup, err := converter.ParseReplyMarkup(raw)
	require.NoError(t, err)
	require.NotNil(t, markup)

	inlineMarkup, ok := markup.(*tg.ReplyInlineMarkup)
	require.True(t, ok)
	require.Len(t, inlineMarkup.Rows, 1)

	btn, ok := inlineMarkup.Rows[0].Buttons[0].(*tg.KeyboardButtonWebView)
	require.True(t, ok)
	assert.Equal(t, "Mini App", btn.Text)
	assert.Equal(t, "https://webapp.example.com", btn.URL)
}

func TestParseReplyKeyboard(t *testing.T) {
	raw := []byte(`{
		"keyboard": [
			[{"text": "Contact", "request_contact": true}],
			[{"text": "Regular"}]
		],
		"resize_keyboard": true,
		"one_time_keyboard": true
	}`)

	markup, err := converter.ParseReplyMarkup(raw)
	require.NoError(t, err)
	require.NotNil(t, markup)

	replyMarkup, ok := markup.(*tg.ReplyKeyboardMarkup)
	require.True(t, ok)
	assert.True(t, replyMarkup.Resize)
	assert.True(t, replyMarkup.SingleUse)
	require.Len(t, replyMarkup.Rows, 2)

	_, ok = replyMarkup.Rows[0].Buttons[0].(*tg.KeyboardButtonRequestPhone)
	assert.True(t, ok)
}

func TestConvertEntities(t *testing.T) {
	entities := []converter.MessageEntity{
		{Type: "bold", Offset: 0, Length: 5},
		{Type: "italic", Offset: 6, Length: 10},
		{Type: "text_link", Offset: 17, Length: 4, URL: "https://t.me"},
	}

	converted := converter.ConvertEntities(entities)
	require.Len(t, converted, 3)

	_, ok := converted[0].(*tg.MessageEntityBold)
	assert.True(t, ok)
	_, ok = converted[1].(*tg.MessageEntityItalic)
	assert.True(t, ok)
	link, ok := converted[2].(*tg.MessageEntityTextURL)
	assert.True(t, ok)
	assert.Equal(t, "https://t.me", link.URL)
}

func TestConvertUpdate(t *testing.T) {
	c := converter.NewMTProtoConverter()
	entities := converter.NewEntityContext([]tg.UserClass{
		&tg.User{ID: 12345, FirstName: "Alice", Username: "alice_bot", Bot: true},
	}, nil)

	upd := &tg.UpdateNewMessage{
		Message: &tg.Message{
			ID:      42,
			Date:    1700000000,
			Message: "Hello from user",
			PeerID:  &tg.PeerUser{UserID: 12345},
			FromID:  &tg.PeerUser{UserID: 12345},
		},
	}

	converted, err := c.ConvertUpdate(1, upd, entities)
	require.NoError(t, err)
	require.NotNil(t, converted)

	assert.Equal(t, 1, converted.UpdateID)
	require.NotNil(t, converted.Message)
	assert.Equal(t, int64(42), converted.Message.MessageID)
	assert.Equal(t, "Hello from user", converted.Message.Text)
	assert.Equal(t, int64(12345), converted.Message.Chat.ID)
	assert.Equal(t, "Alice", converted.Message.From.FirstName)

	// Test JSON serialization matches grammY expectation
	data, err := json.Marshal(converted)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"update_id":1`)
	assert.Contains(t, string(data), `"message_id":42`)
	assert.Contains(t, string(data), `"first_name":"Alice"`)
}

func TestConvertUpdate_BusinessConnection(t *testing.T) {
	c := converter.NewMTProtoConverter()
	entities := converter.NewEntityContext([]tg.UserClass{
		&tg.User{ID: 835716625, FirstName: "Business Owner", Username: "bizowner"},
	}, nil)

	updEnabled := &tg.UpdateBotBusinessConnect{
		Connection: tg.BotBusinessConnection{
			ConnectionID: "conn_12345",
			UserID:       835716625,
			Date:         1700000000,
			Rights:       tg.BusinessBotRights{Reply: true},
			Disabled:     false,
		},
	}

	converted, err := c.ConvertUpdate(10, updEnabled, entities)
	require.NoError(t, err)
	require.NotNil(t, converted)
	require.NotNil(t, converted.BusinessConnection)
	assert.Equal(t, "conn_12345", converted.BusinessConnection.ID)
	assert.Equal(t, int64(835716625), converted.BusinessConnection.UserChatID)
	assert.Equal(t, int64(835716625), converted.BusinessConnection.User.ID)
	assert.Equal(t, "Business Owner", converted.BusinessConnection.User.FirstName)
	assert.True(t, converted.BusinessConnection.CanReply)
	assert.True(t, converted.BusinessConnection.IsEnabled)

	data, err := json.Marshal(converted)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"is_enabled":true`)
	assert.Contains(t, string(data), `"business_connection"`)

	// Test disabled connection
	updDisabled := &tg.UpdateBotBusinessConnect{
		Connection: tg.BotBusinessConnection{
			ConnectionID: "conn_12345",
			UserID:       835716625,
			Date:         1700000010,
			Rights:       tg.BusinessBotRights{Reply: false},
			Disabled:     true,
		},
	}

	convertedDisabled, err := c.ConvertUpdate(11, updDisabled, entities)
	require.NoError(t, err)
	require.NotNil(t, convertedDisabled)
	require.NotNil(t, convertedDisabled.BusinessConnection)
	assert.False(t, convertedDisabled.BusinessConnection.IsEnabled)
	assert.False(t, convertedDisabled.BusinessConnection.CanReply)

	dataDisabled, err := json.Marshal(convertedDisabled)
	require.NoError(t, err)
	assert.Contains(t, string(dataDisabled), `"is_enabled":false`)
}

func TestParseInlineKeyboard_CopyTextAndEmptyData(t *testing.T) {
	raw := []byte(`{
		"inline_keyboard": [
			[
				{"text": "Copy Username", "copy_text": {"text": "@ShadyCloudTestBot"}},
				{"text": "Empty Button"}
			]
		]
	}`)

	markup, err := converter.ParseReplyMarkup(raw)
	require.NoError(t, err)
	require.NotNil(t, markup)

	inlineMarkup, ok := markup.(*tg.ReplyInlineMarkup)
	require.True(t, ok)
	require.Len(t, inlineMarkup.Rows, 1)
	require.Len(t, inlineMarkup.Rows[0].Buttons, 2)

	btn1, ok := inlineMarkup.Rows[0].Buttons[0].(*tg.KeyboardButtonCopy)
	require.True(t, ok)
	assert.Equal(t, "Copy Username", btn1.Text)
	assert.Equal(t, "@ShadyCloudTestBot", btn1.CopyText)

	btn2, ok := inlineMarkup.Rows[0].Buttons[1].(*tg.KeyboardButtonCallback)
	require.True(t, ok)
	assert.Equal(t, "Empty Button", btn2.Text)
	assert.NotEmpty(t, btn2.Data) // Must never be empty (BUTTON_DATA_INVALID protection)
}

func TestParseInlineKeyboard_AllTypes(t *testing.T) {
	raw := []byte(`{
		"inline_keyboard": [
			[
				{"text": "Game", "callback_game": {}},
				{"text": "Pay", "pay": true},
				{"text": "Chosen Chat", "switch_inline_query_chosen_chat": {"query": "search", "allow_user_chats": true, "allow_channel_chats": true}}
			]
		]
	}`)

	markup, err := converter.ParseReplyMarkup(raw)
	require.NoError(t, err)
	require.NotNil(t, markup)

	inlineMarkup, ok := markup.(*tg.ReplyInlineMarkup)
	require.True(t, ok)
	require.Len(t, inlineMarkup.Rows[0].Buttons, 3)

	_, ok = inlineMarkup.Rows[0].Buttons[0].(*tg.KeyboardButtonGame)
	assert.True(t, ok)

	_, ok = inlineMarkup.Rows[0].Buttons[1].(*tg.KeyboardButtonBuy)
	assert.True(t, ok)

	sw, ok := inlineMarkup.Rows[0].Buttons[2].(*tg.KeyboardButtonSwitchInline)
	require.True(t, ok)
	assert.Equal(t, "search", sw.Query)
	types, ok := sw.GetPeerTypes()
	assert.True(t, ok)
	assert.Len(t, types, 2)
}

func TestParseReplyKeyboard_RequestTypes(t *testing.T) {
	raw := []byte(`{
		"keyboard": [
			[
				{"text": "Poll", "request_poll": {"type": "quiz"}},
				{"text": "Users", "request_users": {"request_id": 1, "max_quantity": 3, "request_name": true}},
				{"text": "Chat", "request_chat": {"request_id": 2, "chat_is_channel": true}}
			]
		]
	}`)

	markup, err := converter.ParseReplyMarkup(raw)
	require.NoError(t, err)
	require.NotNil(t, markup)

	replyMarkup, ok := markup.(*tg.ReplyKeyboardMarkup)
	require.True(t, ok)
	require.Len(t, replyMarkup.Rows[0].Buttons, 3)

	pollBtn, ok := replyMarkup.Rows[0].Buttons[0].(*tg.KeyboardButtonRequestPoll)
	require.True(t, ok)
	quiz, ok := pollBtn.GetQuiz()
	assert.True(t, ok)
	assert.True(t, quiz)

	userBtn, ok := replyMarkup.Rows[0].Buttons[1].(*tg.InputKeyboardButtonRequestPeer)
	require.True(t, ok)
	assert.Equal(t, 1, userBtn.ButtonID)
	assert.Equal(t, 3, userBtn.MaxQuantity)
	assert.True(t, userBtn.NameRequested)

	chatBtn, ok := replyMarkup.Rows[0].Buttons[2].(*tg.InputKeyboardButtonRequestPeer)
	require.True(t, ok)
	assert.Equal(t, 2, chatBtn.ButtonID)
	_, isBroadcast := chatBtn.PeerType.(*tg.RequestPeerTypeBroadcast)
	assert.True(t, isBroadcast)
}

