package converter

import (
	"bytes"
	"encoding/json"

	"github.com/gotd/td/tg"
)

// RawWebAppInfo represents a Bot API WebAppInfo object.
type RawWebAppInfo struct {
	URL string `json:"url"`
}

// RawLoginURL represents a Bot API LoginUrl object.
type RawLoginURL struct {
	URL                string `json:"url"`
	ForwardText        string `json:"forward_text,omitempty"`
	BotUsername        string `json:"bot_username,omitempty"`
	RequestWriteAccess bool   `json:"request_write_access,omitempty"`
}

// RawCopyTextButton represents a Bot API copy text button.
type RawCopyTextButton struct {
	Text string `json:"text"`
}

// RawSwitchInlineQueryChosenChat represents a Bot API switch inline query chosen chat.
type RawSwitchInlineQueryChosenChat struct {
	Query             string `json:"query,omitempty"`
	AllowUserChats    bool   `json:"allow_user_chats,omitempty"`
	AllowBotChats     bool   `json:"allow_bot_chats,omitempty"`
	AllowGroupChats   bool   `json:"allow_group_chats,omitempty"`
	AllowChannelChats bool   `json:"allow_channel_chats,omitempty"`
}

// RawCallbackGame represents a Bot API callback game.
type RawCallbackGame struct{}

// RawKeyboardButtonPollType represents a Bot API poll type request.
type RawKeyboardButtonPollType struct {
	Type string `json:"type,omitempty"`
}

// RawKeyboardButtonRequestUsers represents a Bot API request users button.
type RawKeyboardButtonRequestUsers struct {
	RequestID       int   `json:"request_id"`
	UserIsBot       *bool `json:"user_is_bot,omitempty"`
	UserIsPremium   *bool `json:"user_is_premium,omitempty"`
	MaxQuantity     int   `json:"max_quantity,omitempty"`
	RequestName     bool  `json:"request_name,omitempty"`
	RequestUsername bool  `json:"request_username,omitempty"`
	RequestPhoto    bool  `json:"request_photo,omitempty"`
}

// RawKeyboardButtonRequestChat represents a Bot API request chat button.
type RawKeyboardButtonRequestChat struct {
	RequestID       int   `json:"request_id"`
	ChatIsChannel   bool  `json:"chat_is_channel"`
	ChatIsForum     *bool `json:"chat_is_forum,omitempty"`
	ChatHasUsername *bool `json:"chat_has_username,omitempty"`
	ChatIsCreated   bool  `json:"chat_is_created,omitempty"`
	RequestTitle    bool  `json:"request_title,omitempty"`
	RequestUsername bool  `json:"request_username,omitempty"`
	RequestPhoto    bool  `json:"request_photo,omitempty"`
}

// RawInlineKeyboardButton represents a Bot API inline keyboard button.
type RawInlineKeyboardButton struct {
	Text                         string                          `json:"text"`
	URL                          string                          `json:"url,omitempty"`
	CallbackData                 string                          `json:"callback_data,omitempty"`
	WebApp                       *RawWebAppInfo                  `json:"web_app,omitempty"`
	LoginURL                     *RawLoginURL                    `json:"login_url,omitempty"`
	SwitchInlineQuery            *string                         `json:"switch_inline_query,omitempty"`
	SwitchInlineQueryCurrentChat *string                         `json:"switch_inline_query_current_chat,omitempty"`
	SwitchInlineQueryChosenChat  *RawSwitchInlineQueryChosenChat `json:"switch_inline_query_chosen_chat,omitempty"`
	CopyText                     *RawCopyTextButton              `json:"copy_text,omitempty"`
	CallbackGame                 *RawCallbackGame                `json:"callback_game,omitempty"`
	Pay                          bool                            `json:"pay,omitempty"`
}

// RawInlineKeyboardMarkup represents a Bot API inline keyboard markup.
type RawInlineKeyboardMarkup struct {
	InlineKeyboard [][]RawInlineKeyboardButton `json:"inline_keyboard"`
}

// RawKeyboardButton represents a Bot API reply keyboard button.
type RawKeyboardButton struct {
	Text            string                         `json:"text"`
	RequestContact  bool                           `json:"request_contact,omitempty"`
	RequestLocation bool                           `json:"request_location,omitempty"`
	RequestPoll     *RawKeyboardButtonPollType     `json:"request_poll,omitempty"`
	RequestUsers    *RawKeyboardButtonRequestUsers `json:"request_users,omitempty"`
	RequestChat     *RawKeyboardButtonRequestChat  `json:"request_chat,omitempty"`
	WebApp          *RawWebAppInfo                 `json:"web_app,omitempty"`
}

// RawReplyKeyboardMarkup represents a Bot API reply keyboard markup.
type RawReplyKeyboardMarkup struct {
	Keyboard              [][]RawKeyboardButton `json:"keyboard"`
	IsPersistent          bool                  `json:"is_persistent,omitempty"`
	ResizeKeyboard        bool                  `json:"resize_keyboard,omitempty"`
	OneTimeKeyboard       bool                  `json:"one_time_keyboard,omitempty"`
	InputFieldPlaceholder string                `json:"input_field_placeholder,omitempty"`
	Selective             bool                  `json:"selective,omitempty"`
}

// RawReplyKeyboardRemove represents a Bot API keyboard removal.
type RawReplyKeyboardRemove struct {
	RemoveKeyboard bool `json:"remove_keyboard"`
	Selective      bool `json:"selective,omitempty"`
}

// RawForceReply represents a Bot API ForceReply.
type RawForceReply struct {
	ForceReply            bool   `json:"force_reply"`
	InputFieldPlaceholder string `json:"input_field_placeholder,omitempty"`
	Selective             bool   `json:"selective,omitempty"`
}

// ParseReplyMarkup parses JSON raw message into a tg.ReplyMarkupClass for MTProto.
func ParseReplyMarkup(raw []byte) (tg.ReplyMarkupClass, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	// If raw is a JSON-encoded string (e.g. "\"{\\\"inline_keyboard\\\":...}\""), unquote it first
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) >= 2 && trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"' {
		var unquoted string
		if err := json.Unmarshal(trimmed, &unquoted); err == nil {
			raw = []byte(unquoted)
		}
	}

	// 1. Try Inline Keyboard
	var inline RawInlineKeyboardMarkup
	if err := json.Unmarshal(raw, &inline); err == nil && len(inline.InlineKeyboard) > 0 {
		var rows []tg.KeyboardButtonRow
		for _, rawRow := range inline.InlineKeyboard {
			var buttons []tg.KeyboardButtonClass
			for _, btn := range rawRow {
				if btn.WebApp != nil && btn.WebApp.URL != "" {
					buttons = append(buttons, &tg.KeyboardButtonWebView{
						Text: btn.Text,
						URL:  btn.WebApp.URL,
					})
				} else if btn.CallbackData != "" {
					data := btn.CallbackData
					if len(data) > 64 {
						data = data[:64]
					}
					buttons = append(buttons, &tg.KeyboardButtonCallback{
						Text: btn.Text,
						Data: []byte(data),
					})
				} else if btn.URL != "" {
					buttons = append(buttons, &tg.KeyboardButtonURL{
						Text: btn.Text,
						URL:  btn.URL,
					})
				} else if btn.CopyText != nil {
					buttons = append(buttons, &tg.KeyboardButtonCopy{
						Text:     btn.Text,
						CopyText: btn.CopyText.Text,
					})
				} else if btn.LoginURL != nil && btn.LoginURL.URL != "" {
					auth := &tg.InputKeyboardButtonURLAuth{
						Text:               btn.Text,
						URL:                btn.LoginURL.URL,
						Bot:                &tg.InputUserSelf{},
						RequestWriteAccess: btn.LoginURL.RequestWriteAccess,
					}
					if btn.LoginURL.ForwardText != "" {
						auth.SetFwdText(btn.LoginURL.ForwardText)
					}
					if btn.LoginURL.RequestWriteAccess {
						auth.SetRequestWriteAccess(true)
					}
					buttons = append(buttons, auth)
				} else if btn.SwitchInlineQuery != nil {
					buttons = append(buttons, &tg.KeyboardButtonSwitchInline{
						Text:  btn.Text,
						Query: *btn.SwitchInlineQuery,
					})
				} else if btn.SwitchInlineQueryCurrentChat != nil {
					buttons = append(buttons, &tg.KeyboardButtonSwitchInline{
						Text:     btn.Text,
						Query:    *btn.SwitchInlineQueryCurrentChat,
						SamePeer: true,
					})
				} else if btn.SwitchInlineQueryChosenChat != nil {
					var peerTypes []tg.InlineQueryPeerTypeClass
					if btn.SwitchInlineQueryChosenChat.AllowUserChats {
						peerTypes = append(peerTypes, &tg.InlineQueryPeerTypePM{})
					}
					if btn.SwitchInlineQueryChosenChat.AllowBotChats {
						peerTypes = append(peerTypes, &tg.InlineQueryPeerTypeBotPM{})
					}
					if btn.SwitchInlineQueryChosenChat.AllowGroupChats {
						peerTypes = append(peerTypes, &tg.InlineQueryPeerTypeChat{}, &tg.InlineQueryPeerTypeMegagroup{})
					}
					if btn.SwitchInlineQueryChosenChat.AllowChannelChats {
						peerTypes = append(peerTypes, &tg.InlineQueryPeerTypeBroadcast{})
					}
					sw := &tg.KeyboardButtonSwitchInline{
						Text:  btn.Text,
						Query: btn.SwitchInlineQueryChosenChat.Query,
					}
					if len(peerTypes) > 0 {
						sw.SetPeerTypes(peerTypes)
					}
					buttons = append(buttons, sw)
				} else if btn.CallbackGame != nil {
					buttons = append(buttons, &tg.KeyboardButtonGame{
						Text: btn.Text,
					})
				} else if btn.Pay {
					buttons = append(buttons, &tg.KeyboardButtonBuy{
						Text: btn.Text,
					})
				} else {
					// Fallback for inline keyboard: MUST NEVER use standard tg.KeyboardButton.
					// Standard KeyboardButton inside ReplyInlineMarkup triggers BUTTON_TYPE_INVALID in MTProto.
					// And Data must NEVER be empty (length 0 triggers BUTTON_DATA_INVALID).
					data := btn.CallbackData
					if data == "" {
						data = "noop"
					}
					if len(data) > 64 {
						data = data[:64]
					}
					buttons = append(buttons, &tg.KeyboardButtonCallback{
						Text: btn.Text,
						Data: []byte(data),
					})
				}
			}
			rows = append(rows, tg.KeyboardButtonRow{Buttons: buttons})
		}
		return &tg.ReplyInlineMarkup{Rows: rows}, nil
	}

	// 2. Try Reply Keyboard Remove
	var remove RawReplyKeyboardRemove
	if err := json.Unmarshal(raw, &remove); err == nil && remove.RemoveKeyboard {
		return &tg.ReplyKeyboardHide{
			Selective: remove.Selective,
		}, nil
	}

	// 3. Try Force Reply
	var forceReply RawForceReply
	if err := json.Unmarshal(raw, &forceReply); err == nil && forceReply.ForceReply {
		res := &tg.ReplyKeyboardForceReply{
			SingleUse: true,
			Selective: forceReply.Selective,
		}
		if forceReply.InputFieldPlaceholder != "" {
			res.SetPlaceholder(forceReply.InputFieldPlaceholder)
		}
		return res, nil
	}

	// 4. Try Reply Keyboard
	var reply RawReplyKeyboardMarkup
	if err := json.Unmarshal(raw, &reply); err == nil && len(reply.Keyboard) > 0 {
		var rows []tg.KeyboardButtonRow
		for _, rawRow := range reply.Keyboard {
			var buttons []tg.KeyboardButtonClass
			for _, btn := range rawRow {
				if btn.RequestContact {
					buttons = append(buttons, &tg.KeyboardButtonRequestPhone{
						Text: btn.Text,
					})
				} else if btn.RequestLocation {
					buttons = append(buttons, &tg.KeyboardButtonRequestGeoLocation{
						Text: btn.Text,
					})
				} else if btn.RequestPoll != nil {
					pollBtn := &tg.KeyboardButtonRequestPoll{
						Text: btn.Text,
					}
					if btn.RequestPoll.Type == "quiz" {
						pollBtn.SetQuiz(true)
					}
					buttons = append(buttons, pollBtn)
				} else if btn.RequestUsers != nil {
					reqUser := &tg.RequestPeerTypeUser{}
					if btn.RequestUsers.UserIsBot != nil {
						reqUser.SetBot(*btn.RequestUsers.UserIsBot)
					}
					if btn.RequestUsers.UserIsPremium != nil {
						reqUser.SetPremium(*btn.RequestUsers.UserIsPremium)
					}
					maxQ := btn.RequestUsers.MaxQuantity
					if maxQ <= 0 {
						maxQ = 1
					}
					btnReq := &tg.InputKeyboardButtonRequestPeer{
						Text:        btn.Text,
						ButtonID:    btn.RequestUsers.RequestID,
						MaxQuantity: maxQ,
						PeerType:    reqUser,
					}
					if btn.RequestUsers.RequestName {
						btnReq.SetNameRequested(true)
					}
					if btn.RequestUsers.RequestUsername {
						btnReq.SetUsernameRequested(true)
					}
					if btn.RequestUsers.RequestPhoto {
						btnReq.SetPhotoRequested(true)
					}
					buttons = append(buttons, btnReq)
				} else if btn.RequestChat != nil {
					var peerType tg.RequestPeerTypeClass
					if btn.RequestChat.ChatIsChannel {
						bc := &tg.RequestPeerTypeBroadcast{}
						if btn.RequestChat.ChatIsCreated {
							bc.SetCreator(true)
						}
						if btn.RequestChat.ChatHasUsername != nil {
							bc.SetHasUsername(*btn.RequestChat.ChatHasUsername)
						}
						peerType = bc
					} else {
						ch := &tg.RequestPeerTypeChat{}
						if btn.RequestChat.ChatIsCreated {
							ch.SetCreator(true)
						}
						if btn.RequestChat.ChatHasUsername != nil {
							ch.SetHasUsername(*btn.RequestChat.ChatHasUsername)
						}
						if btn.RequestChat.ChatIsForum != nil {
							ch.SetForum(*btn.RequestChat.ChatIsForum)
						}
						peerType = ch
					}
					btnReq := &tg.InputKeyboardButtonRequestPeer{
						Text:        btn.Text,
						ButtonID:    btn.RequestChat.RequestID,
						MaxQuantity: 1,
						PeerType:    peerType,
					}
					if btn.RequestChat.RequestTitle {
						btnReq.SetNameRequested(true)
					}
					if btn.RequestChat.RequestUsername {
						btnReq.SetUsernameRequested(true)
					}
					if btn.RequestChat.RequestPhoto {
						btnReq.SetPhotoRequested(true)
					}
					buttons = append(buttons, btnReq)
				} else if btn.WebApp != nil && btn.WebApp.URL != "" {
					buttons = append(buttons, &tg.KeyboardButtonSimpleWebView{
						Text: btn.Text,
						URL:  btn.WebApp.URL,
					})
				} else {
					buttons = append(buttons, &tg.KeyboardButton{
						Text: btn.Text,
					})
				}
			}
			rows = append(rows, tg.KeyboardButtonRow{Buttons: buttons})
		}
		res := &tg.ReplyKeyboardMarkup{
			Resize:    reply.ResizeKeyboard,
			SingleUse: reply.OneTimeKeyboard,
			Selective: reply.Selective,
			Rows:      rows,
		}
		if reply.IsPersistent {
			res.SetPersistent(true)
		}
		if reply.InputFieldPlaceholder != "" {
			res.SetPlaceholder(reply.InputFieldPlaceholder)
		}
		return res, nil
	}

	return nil, nil
}
