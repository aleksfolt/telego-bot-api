package converter

import "encoding/json"

// ResponseParameters describes why a request was unsuccessful.
type ResponseParameters struct {
	MigrateToChatID int64 `json:"migrate_to_chat_id,omitempty"`
	RetryAfter      int   `json:"retry_after,omitempty"`
}

// ApiResponse is the standard Telegram Bot API response wrapper.
type ApiResponse struct {
	OK          bool                `json:"ok"`
	Result      json.RawMessage     `json:"result,omitempty"`
	ErrorCode   int                 `json:"error_code,omitempty"`
	Description string              `json:"description,omitempty"`
	Parameters  *ResponseParameters `json:"parameters,omitempty"`
}

// User represents a Telegram user or bot.
type User struct {
	ID                      int64  `json:"id"`
	IsBot                   bool   `json:"is_bot"`
	FirstName               string `json:"first_name"`
	LastName                string `json:"last_name,omitempty"`
	Username                string `json:"username,omitempty"`
	LanguageCode            string `json:"language_code,omitempty"`
	IsPremium               bool   `json:"is_premium,omitempty"`
	CanJoinGroups           bool   `json:"can_join_groups,omitempty"`
	CanReadAllGroupMessages bool   `json:"can_read_all_group_messages,omitempty"`
	SupportsInlineQueries   bool   `json:"supports_inline_queries,omitempty"`
	CanConnectToBusiness    *bool  `json:"can_connect_to_business,omitempty"`
	HasMainWebApp           bool   `json:"has_main_web_app,omitempty"`
	CanManageBots           *bool  `json:"can_manage_bots,omitempty"`
}

// BoolPtr returns a pointer to the given bool.
func BoolPtr(b bool) *bool {
	return &b
}

// Chat represents a Telegram chat (private, group, supergroup, or channel).
type Chat struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"` // "private", "group", "supergroup", "channel"
	Title     string `json:"title,omitempty"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	IsForum   bool   `json:"is_forum,omitempty"`
}

// MessageEntity represents an entity in a message (e.g. bold, link, mention).
type MessageEntity struct {
	Type          string `json:"type"`
	Offset        int    `json:"offset"`
	Length        int    `json:"length"`
	URL           string `json:"url,omitempty"`
	User          *User  `json:"user,omitempty"`
	Language      string `json:"language,omitempty"`
	CustomEmojiID string `json:"custom_emoji_id,omitempty"`
}

// PhotoSize represents one size of a photo or a file / sticker thumbnail.
type PhotoSize struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	FileSize     int    `json:"file_size,omitempty"`
}

type Document struct {
	FileID       string     `json:"file_id"`
	FileUniqueID string     `json:"file_unique_id"`
	Thumbnail    *PhotoSize `json:"thumbnail,omitempty"`
	FileName     string     `json:"file_name,omitempty"`
	MimeType     string     `json:"mime_type,omitempty"`
	FileSize     int64      `json:"file_size,omitempty"`
}

type Audio struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Duration     int    `json:"duration"`
	Performer    string `json:"performer,omitempty"`
	Title        string `json:"title,omitempty"`
	FileName     string `json:"file_name,omitempty"`
	MimeType     string `json:"mime_type,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
}

type Video struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Duration     int    `json:"duration"`
	FileName     string `json:"file_name,omitempty"`
	MimeType     string `json:"mime_type,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
}

type Animation Video

type Voice struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Duration     int    `json:"duration"`
	MimeType     string `json:"mime_type,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
}

type VideoNote struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Length       int    `json:"length"`
	Duration     int    `json:"duration"`
	FileSize     int64  `json:"file_size,omitempty"`
}

type Contact struct {
	PhoneNumber string `json:"phone_number"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name,omitempty"`
	UserID      int64  `json:"user_id,omitempty"`
	VCard       string `json:"vcard,omitempty"`
}

type Venue struct {
	Location        Location `json:"location"`
	Title           string   `json:"title"`
	Address         string   `json:"address"`
	FoursquareID    string   `json:"foursquare_id,omitempty"`
	FoursquareType  string   `json:"foursquare_type,omitempty"`
	GooglePlaceID   string   `json:"google_place_id,omitempty"`
	GooglePlaceType string   `json:"google_place_type,omitempty"`
}

type Dice struct {
	Emoji string `json:"emoji"`
	Value int    `json:"value"`
}

type MessageAutoDeleteTimerChanged struct {
	MessageAutoDeleteTime int `json:"message_auto_delete_time"`
}

// Message represents a Telegram message.
type Message struct {
	MessageID                     int64                          `json:"message_id"`
	MessageThreadID               int64                          `json:"message_thread_id,omitempty"`
	IsTopicMessage                bool                           `json:"is_topic_message,omitempty"`
	BusinessConnectionID          string                         `json:"business_connection_id,omitempty"`
	From                          *User                          `json:"from,omitempty"`
	Chat                          Chat                           `json:"chat"`
	Date                          int                            `json:"date"`
	Text                          string                         `json:"text,omitempty"`
	Caption                       string                         `json:"caption,omitempty"`
	Entities                      []MessageEntity                `json:"entities,omitempty"`
	CaptionEntities               []MessageEntity                `json:"caption_entities,omitempty"`
	Photo                         []PhotoSize                    `json:"photo,omitempty"`
	Document                      *Document                      `json:"document,omitempty"`
	Audio                         *Audio                         `json:"audio,omitempty"`
	Video                         *Video                         `json:"video,omitempty"`
	Animation                     *Animation                     `json:"animation,omitempty"`
	Voice                         *Voice                         `json:"voice,omitempty"`
	VideoNote                     *VideoNote                     `json:"video_note,omitempty"`
	Sticker                       *Sticker                       `json:"sticker,omitempty"`
	Location                      *Location                      `json:"location,omitempty"`
	Venue                         *Venue                         `json:"venue,omitempty"`
	Contact                       *Contact                       `json:"contact,omitempty"`
	Poll                          *Poll                          `json:"poll,omitempty"`
	Dice                          *Dice                          `json:"dice,omitempty"`
	MediaGroupID                  string                         `json:"media_group_id,omitempty"`
	AuthorSignature               string                         `json:"author_signature,omitempty"`
	EditDate                      int                            `json:"edit_date,omitempty"`
	HasProtectedContent           bool                           `json:"has_protected_content,omitempty"`
	ShowCaptionAboveMedia         bool                           `json:"show_caption_above_media,omitempty"`
	HasMediaSpoiler               bool                           `json:"has_media_spoiler,omitempty"`
	ViaBot                        *User                          `json:"via_bot,omitempty"`
	NewChatMembers                []User                         `json:"new_chat_members,omitempty"`
	LeftChatMember                *User                          `json:"left_chat_member,omitempty"`
	NewChatTitle                  string                         `json:"new_chat_title,omitempty"`
	NewChatPhoto                  []PhotoSize                    `json:"new_chat_photo,omitempty"`
	DeleteChatPhoto               bool                           `json:"delete_chat_photo,omitempty"`
	GroupChatCreated              bool                           `json:"group_chat_created,omitempty"`
	SupergroupChatCreated         bool                           `json:"supergroup_chat_created,omitempty"`
	ChannelChatCreated            bool                           `json:"channel_chat_created,omitempty"`
	MigrateToChatID               int64                          `json:"migrate_to_chat_id,omitempty"`
	MigrateFromChatID             int64                          `json:"migrate_from_chat_id,omitempty"`
	PinnedMessage                 *Message                       `json:"pinned_message,omitempty"`
	MessageAutoDeleteTimerChanged *MessageAutoDeleteTimerChanged `json:"message_auto_delete_timer_changed,omitempty"`
	ReplyToMessage                *Message                       `json:"reply_to_message,omitempty"`
}

// MessageID represents response with a message ID (e.g. copyMessage).
type MessageID struct {
	MessageID int64 `json:"message_id"`
}

// BusinessBotRights describes the permissions granted to a connected business bot.
type BusinessBotRights struct {
	CanReply                   bool `json:"can_reply,omitempty"`
	CanReadMessages            bool `json:"can_read_messages,omitempty"`
	CanDeleteSentMessages      bool `json:"can_delete_sent_messages,omitempty"`
	CanDeleteOutgoingMessages  bool `json:"can_delete_outgoing_messages,omitempty"`
	CanDeleteAllMessages       bool `json:"can_delete_all_messages,omitempty"`
	CanEditName                bool `json:"can_edit_name,omitempty"`
	CanEditBio                 bool `json:"can_edit_bio,omitempty"`
	CanEditProfilePhoto        bool `json:"can_edit_profile_photo,omitempty"`
	CanEditUsername            bool `json:"can_edit_username,omitempty"`
	CanChangeGiftSettings      bool `json:"can_change_gift_settings,omitempty"`
	CanViewGiftsAndStars       bool `json:"can_view_gifts_and_stars,omitempty"`
	CanConvertGiftsToStars     bool `json:"can_convert_gifts_to_stars,omitempty"`
	CanTransferAndUpgradeGifts bool `json:"can_transfer_and_upgrade_gifts,omitempty"`
	CanTransferStars           bool `json:"can_transfer_stars,omitempty"`
	CanManageStories           bool `json:"can_manage_stories,omitempty"`
}

// BusinessConnection describes the connection of the bot with a business account.
type BusinessConnection struct {
	ID         string             `json:"id"`
	User       User               `json:"user"`
	UserChatID int64              `json:"user_chat_id"`
	Date       int                `json:"date"`
	CanReply   bool               `json:"can_reply"`
	IsEnabled  bool               `json:"is_enabled"`
	Rights     *BusinessBotRights `json:"rights,omitempty"`
}

// BusinessMessagesDeleted represents service messages about the deletion of messages in a connected business account.
type BusinessMessagesDeleted struct {
	BusinessConnectionID string  `json:"business_connection_id"`
	Chat                 Chat    `json:"chat"`
	MessageIDs           []int64 `json:"message_ids"`
}

// CallbackQuery represents an incoming callback query from an inline keyboard.
type CallbackQuery struct {
	ID           string   `json:"id"`
	From         User     `json:"from"`
	Message      *Message `json:"message,omitempty"`
	InlineMsgID  string   `json:"inline_message_id,omitempty"`
	ChatInstance string   `json:"chat_instance"`
	Data         string   `json:"data,omitempty"`
}

type Location struct {
	Longitude            float64 `json:"longitude"`
	Latitude             float64 `json:"latitude"`
	HorizontalAccuracy   float64 `json:"horizontal_accuracy,omitempty"`
	LivePeriod           int     `json:"live_period,omitempty"`
	Heading              int     `json:"heading,omitempty"`
	ProximityAlertRadius int     `json:"proximity_alert_radius,omitempty"`
}

type InlineQuery struct {
	ID       string    `json:"id"`
	From     User      `json:"from"`
	Query    string    `json:"query"`
	Offset   string    `json:"offset"`
	ChatType string    `json:"chat_type,omitempty"`
	Location *Location `json:"location,omitempty"`
}

type ChosenInlineResult struct {
	ResultID        string    `json:"result_id"`
	From            User      `json:"from"`
	Location        *Location `json:"location,omitempty"`
	InlineMessageID string    `json:"inline_message_id,omitempty"`
	Query           string    `json:"query"`
}

type ShippingAddress struct {
	CountryCode string `json:"country_code"`
	State       string `json:"state"`
	City        string `json:"city"`
	StreetLine1 string `json:"street_line1"`
	StreetLine2 string `json:"street_line2"`
	PostCode    string `json:"post_code"`
}

type OrderInfo struct {
	Name            string           `json:"name,omitempty"`
	PhoneNumber     string           `json:"phone_number,omitempty"`
	Email           string           `json:"email,omitempty"`
	ShippingAddress *ShippingAddress `json:"shipping_address,omitempty"`
}

type ShippingQuery struct {
	ID              string          `json:"id"`
	From            User            `json:"from"`
	InvoicePayload  string          `json:"invoice_payload"`
	ShippingAddress ShippingAddress `json:"shipping_address"`
}

type PreCheckoutQuery struct {
	ID               string     `json:"id"`
	From             User       `json:"from"`
	Currency         string     `json:"currency"`
	TotalAmount      int64      `json:"total_amount"`
	InvoicePayload   string     `json:"invoice_payload"`
	ShippingOptionID string     `json:"shipping_option_id,omitempty"`
	OrderInfo        *OrderInfo `json:"order_info,omitempty"`
}

type PollAnswer struct {
	PollID    string `json:"poll_id"`
	VoterChat *Chat  `json:"voter_chat,omitempty"`
	User      *User  `json:"user,omitempty"`
	OptionIDs []int  `json:"option_ids"`
}

type ChatInviteLinkInfo struct {
	InviteLink         string `json:"invite_link"`
	Creator            User   `json:"creator"`
	CreatesJoinRequest bool   `json:"creates_join_request"`
	IsPrimary          bool   `json:"is_primary"`
	IsRevoked          bool   `json:"is_revoked"`
	Name               string `json:"name,omitempty"`
	ExpireDate         int    `json:"expire_date,omitempty"`
	MemberLimit        int    `json:"member_limit,omitempty"`
}

type ChatJoinRequest struct {
	Chat       Chat                `json:"chat"`
	From       User                `json:"from"`
	UserChatID int64               `json:"user_chat_id"`
	Date       int                 `json:"date"`
	Bio        string              `json:"bio,omitempty"`
	InviteLink *ChatInviteLinkInfo `json:"invite_link,omitempty"`
}

type ChatMemberUpdated struct {
	Chat                    Chat                `json:"chat"`
	From                    User                `json:"from"`
	Date                    int                 `json:"date"`
	OldChatMember           ChatMember          `json:"old_chat_member"`
	NewChatMember           ChatMember          `json:"new_chat_member"`
	InviteLink              *ChatInviteLinkInfo `json:"invite_link,omitempty"`
	ViaJoinRequest          bool                `json:"via_join_request,omitempty"`
	ViaChatFolderInviteLink bool                `json:"via_chat_folder_invite_link,omitempty"`
}

type ChatBoostUpdated struct {
	Chat  Chat      `json:"chat"`
	Boost ChatBoost `json:"boost"`
}

type ChatBoostRemoved struct {
	Chat       Chat            `json:"chat"`
	BoostID    string          `json:"boost_id"`
	RemoveDate int             `json:"remove_date"`
	Source     ChatBoostSource `json:"source"`
}

type MessageReactionUpdated struct {
	Chat        Chat           `json:"chat"`
	MessageID   int64          `json:"message_id"`
	User        *User          `json:"user,omitempty"`
	ActorChat   *Chat          `json:"actor_chat,omitempty"`
	Date        int            `json:"date"`
	OldReaction []ReactionType `json:"old_reaction"`
	NewReaction []ReactionType `json:"new_reaction"`
}

type ReactionCount struct {
	Type       ReactionType `json:"type"`
	TotalCount int          `json:"total_count"`
}

type MessageReactionCountUpdated struct {
	Chat      Chat            `json:"chat"`
	MessageID int64           `json:"message_id"`
	Date      int             `json:"date"`
	Reactions []ReactionCount `json:"reactions"`
}

type PaidMediaPurchased struct {
	From             User   `json:"from"`
	PaidMediaPayload string `json:"paid_media_payload"`
}

// Update represents an incoming update for grammY.
type Update struct {
	UpdateID                int                          `json:"update_id"`
	Message                 *Message                     `json:"message,omitempty"`
	EditedMessage           *Message                     `json:"edited_message,omitempty"`
	ChannelPost             *Message                     `json:"channel_post,omitempty"`
	EditedChannelPost       *Message                     `json:"edited_channel_post,omitempty"`
	BusinessConnection      *BusinessConnection          `json:"business_connection,omitempty"`
	BusinessMessage         *Message                     `json:"business_message,omitempty"`
	EditedBusinessMessage   *Message                     `json:"edited_business_message,omitempty"`
	DeletedBusinessMessages *BusinessMessagesDeleted     `json:"deleted_business_messages,omitempty"`
	CallbackQuery           *CallbackQuery               `json:"callback_query,omitempty"`
	InlineQuery             *InlineQuery                 `json:"inline_query,omitempty"`
	ChosenInlineResult      *ChosenInlineResult          `json:"chosen_inline_result,omitempty"`
	ShippingQuery           *ShippingQuery               `json:"shipping_query,omitempty"`
	PreCheckoutQuery        *PreCheckoutQuery            `json:"pre_checkout_query,omitempty"`
	Poll                    *Poll                        `json:"poll,omitempty"`
	PollAnswer              *PollAnswer                  `json:"poll_answer,omitempty"`
	ChatJoinRequest         *ChatJoinRequest             `json:"chat_join_request,omitempty"`
	MyChatMember            *ChatMemberUpdated           `json:"my_chat_member,omitempty"`
	ChatMember              *ChatMemberUpdated           `json:"chat_member,omitempty"`
	ChatBoost               *ChatBoostUpdated            `json:"chat_boost,omitempty"`
	RemovedChatBoost        *ChatBoostRemoved            `json:"removed_chat_boost,omitempty"`
	MessageReaction         *MessageReactionUpdated      `json:"message_reaction,omitempty"`
	MessageReactionCount    *MessageReactionCountUpdated `json:"message_reaction_count,omitempty"`
	PurchasedPaidMedia      *PaidMediaPurchased          `json:"purchased_paid_media,omitempty"`
}

// ReplyParameters describes reply parameters for messages.
type ReplyParameters struct {
	MessageID int64 `json:"message_id"`
	ChatID    int64 `json:"chat_id,omitempty"`
}

// LinkPreviewOptions options for link preview generation.
type LinkPreviewOptions struct {
	IsDisabled bool `json:"is_disabled,omitempty"`
}

// SendMessageRequest represents parameters for the sendMessage method.
type SendMessageRequest struct {
	BusinessConnectionID  string              `json:"business_connection_id,omitempty"`
	ChatID                int64               `json:"chat_id"`
	MessageThreadID       int                 `json:"message_thread_id,omitempty"`
	Text                  string              `json:"text"`
	ParseMode             string              `json:"parse_mode,omitempty"`
	Entities              []MessageEntity     `json:"entities,omitempty"`
	LinkPreviewOptions    *LinkPreviewOptions `json:"link_preview_options,omitempty"`
	DisableWebPagePreview bool                `json:"disable_web_page_preview,omitempty"`
	ProtectContent        bool                `json:"protect_content,omitempty"`
	ReplyToMessageID      int64               `json:"reply_to_message_id,omitempty"`
	ReplyParameters       *ReplyParameters    `json:"reply_parameters,omitempty"`
	ReplyMarkup           json.RawMessage     `json:"reply_markup,omitempty"`
}

// EditMessageTextRequest represents parameters for editMessageText.
type EditMessageTextRequest struct {
	BusinessConnectionID string              `json:"business_connection_id,omitempty"`
	ChatID               int64               `json:"chat_id,omitempty"`
	MessageID            int64               `json:"message_id,omitempty"`
	InlineMsgID          string              `json:"inline_message_id,omitempty"`
	Text                 string              `json:"text"`
	ParseMode            string              `json:"parse_mode,omitempty"`
	Entities             []MessageEntity     `json:"entities,omitempty"`
	LinkPreviewOptions   *LinkPreviewOptions `json:"link_preview_options,omitempty"`
	ReplyMarkup          json.RawMessage     `json:"reply_markup,omitempty"`
}

// EditMessageCaptionRequest represents parameters for editMessageCaption.
type EditMessageCaptionRequest struct {
	BusinessConnectionID string          `json:"business_connection_id,omitempty"`
	ChatID               int64           `json:"chat_id,omitempty"`
	MessageID            int64           `json:"message_id,omitempty"`
	InlineMsgID          string          `json:"inline_message_id,omitempty"`
	Caption              string          `json:"caption,omitempty"`
	ParseMode            string          `json:"parse_mode,omitempty"`
	CaptionEntities      []MessageEntity `json:"caption_entities,omitempty"`
	ReplyMarkup          json.RawMessage `json:"reply_markup,omitempty"`
}

// EditMessageReplyMarkupRequest represents parameters for editMessageReplyMarkup.
type EditMessageReplyMarkupRequest struct {
	BusinessConnectionID string          `json:"business_connection_id,omitempty"`
	ChatID               int64           `json:"chat_id,omitempty"`
	MessageID            int64           `json:"message_id,omitempty"`
	InlineMsgID          string          `json:"inline_message_id,omitempty"`
	ReplyMarkup          json.RawMessage `json:"reply_markup,omitempty"`
}

// EditMessageMediaRequest represents parameters for editMessageMedia.
type EditMessageMediaRequest struct {
	BusinessConnectionID string          `json:"business_connection_id,omitempty"`
	ChatID               int64           `json:"chat_id,omitempty"`
	MessageID            int64           `json:"message_id,omitempty"`
	InlineMsgID          string          `json:"inline_message_id,omitempty"`
	Media                json.RawMessage `json:"media"`
	ReplyMarkup          json.RawMessage `json:"reply_markup,omitempty"`

	Files     map[string][]byte `json:"-"`
	FileNames map[string]string `json:"-"`
}

// DeleteMessageRequest represents parameters for deleteMessage.
type DeleteMessageRequest struct {
	BusinessConnectionID string `json:"business_connection_id,omitempty"`
	ChatID               int64  `json:"chat_id"`
	MessageID            int64  `json:"message_id"`
}

// SendChatActionRequest represents parameters for sendChatAction.
type SendChatActionRequest struct {
	ChatID          int64  `json:"chat_id"`
	MessageThreadID int    `json:"message_thread_id,omitempty"`
	Action          string `json:"action"`
}

// CopyMessageRequest represents parameters for copyMessage.
type CopyMessageRequest struct {
	ChatID           int64            `json:"chat_id"`
	MessageThreadID  int              `json:"message_thread_id,omitempty"`
	FromChatID       int64            `json:"from_chat_id"`
	MessageID        int64            `json:"message_id"`
	Caption          string           `json:"caption,omitempty"`
	ParseMode        string           `json:"parse_mode,omitempty"`
	CaptionEntities  []MessageEntity  `json:"caption_entities,omitempty"`
	ProtectContent   bool             `json:"protect_content,omitempty"`
	ReplyToMessageID int64            `json:"reply_to_message_id,omitempty"`
	ReplyParameters  *ReplyParameters `json:"reply_parameters,omitempty"`
	ReplyMarkup      json.RawMessage  `json:"reply_markup,omitempty"`
}

// AnswerCallbackQueryRequest represents parameters for answerCallbackQuery.
type AnswerCallbackQueryRequest struct {
	CallbackQueryID string `json:"callback_query_id"`
	Text            string `json:"text,omitempty"`
	ShowAlert       bool   `json:"show_alert,omitempty"`
	URL             string `json:"url,omitempty"`
	CacheTime       int    `json:"cache_time,omitempty"`
}

// SetWebhookRequest represents parameters for setWebhook.
type SetWebhookRequest struct {
	URL                string   `json:"url"`
	IPAddress          string   `json:"ip_address,omitempty"`
	MaxConnections     int      `json:"max_connections,omitempty"`
	AllowedUpdates     []string `json:"allowed_updates,omitempty"`
	DropPendingUpdates bool     `json:"drop_pending_updates,omitempty"`
	SecretToken        string   `json:"secret_token,omitempty"`
}

// DeleteWebhookRequest represents parameters for deleteWebhook.
type DeleteWebhookRequest struct {
	DropPendingUpdates bool `json:"drop_pending_updates,omitempty"`
}

// GetUpdatesRequest represents parameters for getUpdates.
type GetUpdatesRequest struct {
	Offset         int      `json:"offset,omitempty"`
	Limit          int      `json:"limit,omitempty"`
	Timeout        int      `json:"timeout,omitempty"`
	AllowedUpdates []string `json:"allowed_updates,omitempty"`
}

// WebhookInfo represents response for getWebhookInfo.
type WebhookInfo struct {
	URL                  string   `json:"url"`
	HasCustomCertificate bool     `json:"has_custom_certificate"`
	PendingUpdateCount   int      `json:"pending_update_count"`
	IPAddress            string   `json:"ip_address,omitempty"`
	LastErrorDate        int      `json:"last_error_date,omitempty"`
	LastErrorMessage     string   `json:"last_error_message,omitempty"`
	MaxConnections       int      `json:"max_connections,omitempty"`
	AllowedUpdates       []string `json:"allowed_updates,omitempty"`
}

// SendPhotoRequest represents parameters for sendPhoto.
type SendPhotoRequest struct {
	BusinessConnectionID  string           `json:"business_connection_id,omitempty"`
	ChatID                int64            `json:"chat_id"`
	MessageThreadID       int              `json:"message_thread_id,omitempty"`
	Photo                 string           `json:"photo"`
	Caption               string           `json:"caption,omitempty"`
	ParseMode             string           `json:"parse_mode,omitempty"`
	CaptionEntities       []MessageEntity  `json:"caption_entities,omitempty"`
	ShowCaptionAboveMedia bool             `json:"show_caption_above_media,omitempty"`
	HasSpoiler            bool             `json:"has_spoiler,omitempty"`
	DisableNotification   bool             `json:"disable_notification,omitempty"`
	ProtectContent        bool             `json:"protect_content,omitempty"`
	ReplyToMessageID      int64            `json:"reply_to_message_id,omitempty"`
	ReplyParameters       *ReplyParameters `json:"reply_parameters,omitempty"`
	ReplyMarkup           json.RawMessage  `json:"reply_markup,omitempty"`

	PhotoData     []byte `json:"-"`
	PhotoFileName string `json:"-"`
}

// SendVideoRequest represents parameters for sendVideo.
type SendVideoRequest struct {
	BusinessConnectionID string           `json:"business_connection_id,omitempty"`
	ChatID               int64            `json:"chat_id"`
	MessageThreadID      int              `json:"message_thread_id,omitempty"`
	Video                string           `json:"video"`
	Thumbnail            string           `json:"thumbnail,omitempty"`
	Duration             int              `json:"duration,omitempty"`
	Width                int              `json:"width,omitempty"`
	Height               int              `json:"height,omitempty"`
	Caption              string           `json:"caption,omitempty"`
	ParseMode            string           `json:"parse_mode,omitempty"`
	CaptionEntities      []MessageEntity  `json:"caption_entities,omitempty"`
	HasSpoiler           bool             `json:"has_spoiler,omitempty"`
	SupportsStreaming    bool             `json:"supports_streaming,omitempty"`
	DisableNotification  bool             `json:"disable_notification,omitempty"`
	ProtectContent       bool             `json:"protect_content,omitempty"`
	ReplyToMessageID     int64            `json:"reply_to_message_id,omitempty"`
	ReplyParameters      *ReplyParameters `json:"reply_parameters,omitempty"`
	ReplyMarkup          json.RawMessage  `json:"reply_markup,omitempty"`

	ThumbnailData []byte `json:"-"`
	VideoData     []byte `json:"-"`
	VideoFileName string `json:"-"`
}

// SendDocumentRequest represents parameters for sendDocument.
type SendDocumentRequest struct {
	BusinessConnectionID        string           `json:"business_connection_id,omitempty"`
	ChatID                      int64            `json:"chat_id"`
	MessageThreadID             int              `json:"message_thread_id,omitempty"`
	Document                    string           `json:"document"`
	Thumbnail                   string           `json:"thumbnail,omitempty"`
	Caption                     string           `json:"caption,omitempty"`
	ParseMode                   string           `json:"parse_mode,omitempty"`
	CaptionEntities             []MessageEntity  `json:"caption_entities,omitempty"`
	DisableContentTypeDetection bool             `json:"disable_content_type_detection,omitempty"`
	DisableNotification         bool             `json:"disable_notification,omitempty"`
	ProtectContent              bool             `json:"protect_content,omitempty"`
	ReplyToMessageID            int64            `json:"reply_to_message_id,omitempty"`
	ReplyParameters             *ReplyParameters `json:"reply_parameters,omitempty"`
	ReplyMarkup                 json.RawMessage  `json:"reply_markup,omitempty"`

	ThumbnailData    []byte `json:"-"`
	DocumentData     []byte `json:"-"`
	DocumentFileName string `json:"-"`
}

// SendVoiceRequest represents parameters for sendVoice.
type SendVoiceRequest struct {
	BusinessConnectionID string           `json:"business_connection_id,omitempty"`
	ChatID               int64            `json:"chat_id"`
	MessageThreadID      int              `json:"message_thread_id,omitempty"`
	Voice                string           `json:"voice"`
	Caption              string           `json:"caption,omitempty"`
	ParseMode            string           `json:"parse_mode,omitempty"`
	CaptionEntities      []MessageEntity  `json:"caption_entities,omitempty"`
	Duration             int              `json:"duration,omitempty"`
	DisableNotification  bool             `json:"disable_notification,omitempty"`
	ProtectContent       bool             `json:"protect_content,omitempty"`
	ReplyToMessageID     int64            `json:"reply_to_message_id,omitempty"`
	ReplyParameters      *ReplyParameters `json:"reply_parameters,omitempty"`
	ReplyMarkup          json.RawMessage  `json:"reply_markup,omitempty"`

	VoiceData     []byte `json:"-"`
	VoiceFileName string `json:"-"`
}

// SendAudioRequest represents parameters for sendAudio.
type SendAudioRequest struct {
	BusinessConnectionID string           `json:"business_connection_id,omitempty"`
	ChatID               int64            `json:"chat_id"`
	MessageThreadID      int              `json:"message_thread_id,omitempty"`
	Audio                string           `json:"audio"`
	Thumbnail            string           `json:"thumbnail,omitempty"`
	Caption              string           `json:"caption,omitempty"`
	ParseMode            string           `json:"parse_mode,omitempty"`
	CaptionEntities      []MessageEntity  `json:"caption_entities,omitempty"`
	Duration             int              `json:"duration,omitempty"`
	Performer            string           `json:"performer,omitempty"`
	Title                string           `json:"title,omitempty"`
	DisableNotification  bool             `json:"disable_notification,omitempty"`
	ProtectContent       bool             `json:"protect_content,omitempty"`
	ReplyToMessageID     int64            `json:"reply_to_message_id,omitempty"`
	ReplyParameters      *ReplyParameters `json:"reply_parameters,omitempty"`
	ReplyMarkup          json.RawMessage  `json:"reply_markup,omitempty"`

	ThumbnailData []byte `json:"-"`
	AudioData     []byte `json:"-"`
	AudioFileName string `json:"-"`
}

// SendAnimationRequest represents parameters for sendAnimation.
type SendAnimationRequest struct {
	BusinessConnectionID string           `json:"business_connection_id,omitempty"`
	ChatID               int64            `json:"chat_id"`
	MessageThreadID      int              `json:"message_thread_id,omitempty"`
	Animation            string           `json:"animation"`
	Thumbnail            string           `json:"thumbnail,omitempty"`
	Duration             int              `json:"duration,omitempty"`
	Width                int              `json:"width,omitempty"`
	Height               int              `json:"height,omitempty"`
	Caption              string           `json:"caption,omitempty"`
	ParseMode            string           `json:"parse_mode,omitempty"`
	CaptionEntities      []MessageEntity  `json:"caption_entities,omitempty"`
	HasSpoiler           bool             `json:"has_spoiler,omitempty"`
	DisableNotification  bool             `json:"disable_notification,omitempty"`
	ProtectContent       bool             `json:"protect_content,omitempty"`
	ReplyToMessageID     int64            `json:"reply_to_message_id,omitempty"`
	ReplyParameters      *ReplyParameters `json:"reply_parameters,omitempty"`
	ReplyMarkup          json.RawMessage  `json:"reply_markup,omitempty"`

	ThumbnailData     []byte `json:"-"`
	AnimationData     []byte `json:"-"`
	AnimationFileName string `json:"-"`
}

// SendVideoNoteRequest represents parameters for sendVideoNote.
type SendVideoNoteRequest struct {
	BusinessConnectionID string           `json:"business_connection_id,omitempty"`
	ChatID               int64            `json:"chat_id"`
	MessageThreadID      int              `json:"message_thread_id,omitempty"`
	VideoNote            string           `json:"video_note"`
	Thumbnail            string           `json:"thumbnail,omitempty"`
	Duration             int              `json:"duration,omitempty"`
	Length               int              `json:"length,omitempty"`
	DisableNotification  bool             `json:"disable_notification,omitempty"`
	ProtectContent       bool             `json:"protect_content,omitempty"`
	ReplyToMessageID     int64            `json:"reply_to_message_id,omitempty"`
	ReplyParameters      *ReplyParameters `json:"reply_parameters,omitempty"`
	ReplyMarkup          json.RawMessage  `json:"reply_markup,omitempty"`

	ThumbnailData     []byte `json:"-"`
	VideoNoteData     []byte `json:"-"`
	VideoNoteFileName string `json:"-"`
}

// SendStickerRequest represents parameters for sendSticker.
type SendStickerRequest struct {
	BusinessConnectionID string           `json:"business_connection_id,omitempty"`
	ChatID               int64            `json:"chat_id"`
	MessageThreadID      int              `json:"message_thread_id,omitempty"`
	Sticker              string           `json:"sticker"`
	Emoji                string           `json:"emoji,omitempty"`
	DisableNotification  bool             `json:"disable_notification,omitempty"`
	ProtectContent       bool             `json:"protect_content,omitempty"`
	ReplyToMessageID     int64            `json:"reply_to_message_id,omitempty"`
	ReplyParameters      *ReplyParameters `json:"reply_parameters,omitempty"`
	ReplyMarkup          json.RawMessage  `json:"reply_markup,omitempty"`

	StickerData     []byte `json:"-"`
	StickerFileName string `json:"-"`
}

// AnswerPreCheckoutQueryRequest represents parameters for answerPreCheckoutQuery.
type AnswerPreCheckoutQueryRequest struct {
	PreCheckoutQueryID string `json:"pre_checkout_query_id"`
	OK                 bool   `json:"ok"`
	ErrorMessage       string `json:"error_message,omitempty"`
}

// GetChatMemberRequest represents parameters for getChatMember.
type GetChatMemberRequest struct {
	ChatID int64 `json:"chat_id"`
	UserID int64 `json:"user_id"`
}

// BotCommand represents a Telegram bot command.
type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// SetMyCommandsRequest represents parameters for setMyCommands.
type SetMyCommandsRequest struct {
	Commands     []BotCommand    `json:"commands"`
	Scope        json.RawMessage `json:"scope,omitempty"`
	LanguageCode string          `json:"language_code,omitempty"`
}

// File represents a Telegram file ready for download.
type File struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	FileSize     int64  `json:"file_size,omitempty"`
	FilePath     string `json:"file_path,omitempty"`
}

// GetFileRequest represents parameters for getFile.
type GetFileRequest struct {
	FileID string `json:"file_id"`
}

// UserProfilePhotos represents a user's profile pictures.
type UserProfilePhotos struct {
	TotalCount int           `json:"total_count"`
	Photos     [][]PhotoSize `json:"photos"`
}

// GetUserProfilePhotosRequest represents parameters for getUserProfilePhotos.
type GetUserProfilePhotosRequest struct {
	UserID int64 `json:"user_id"`
	Offset int   `json:"offset,omitempty"`
	Limit  int   `json:"limit,omitempty"`
}

// InputMediaItem represents a media item in sendMediaGroup.
type InputMediaItem struct {
	Type                  string          `json:"type"` // "photo", "video", "document", "audio"
	Media                 string          `json:"media"`
	Caption               string          `json:"caption,omitempty"`
	ParseMode             string          `json:"parse_mode,omitempty"`
	CaptionEntities       []MessageEntity `json:"caption_entities,omitempty"`
	Duration              int             `json:"duration,omitempty"`
	Width                 int             `json:"width,omitempty"`
	Height                int             `json:"height,omitempty"`
	SupportsStreaming     bool            `json:"supports_streaming,omitempty"`
	ShowCaptionAboveMedia bool            `json:"show_caption_above_media,omitempty"`
	HasSpoiler            bool            `json:"has_spoiler,omitempty"`
	Thumbnail             string          `json:"thumbnail,omitempty"`
}

// SendMediaGroupRequest represents parameters for sendMediaGroup.
type SendMediaGroupRequest struct {
	BusinessConnectionID string            `json:"business_connection_id,omitempty"`
	ChatID               int64             `json:"chat_id"`
	MessageThreadID      int               `json:"message_thread_id,omitempty"`
	Media                json.RawMessage   `json:"media"`
	DisableNotification  bool              `json:"disable_notification,omitempty"`
	ProtectContent       bool              `json:"protect_content,omitempty"`
	ReplyParameters      *ReplyParameters  `json:"reply_parameters,omitempty"`
	Files                map[string][]byte `json:"-"`
	FileNames            map[string]string `json:"-"`
}

// SetBusinessAccountBioRequest represents parameters for setBusinessAccountBio.
type SetBusinessAccountBioRequest struct {
	BusinessConnectionID string `json:"business_connection_id"`
	Bio                  string `json:"bio"`
}

// SetBusinessAccountNameRequest represents parameters for setBusinessAccountName.
type SetBusinessAccountNameRequest struct {
	BusinessConnectionID string `json:"business_connection_id"`
	FirstName            string `json:"first_name"`
	LastName             string `json:"last_name,omitempty"`
}

type SetBusinessAccountUsernameRequest struct {
	BusinessConnectionID string `json:"business_connection_id"`
	Username             string `json:"username"`
}

type RemoveBusinessAccountProfilePhotoRequest struct {
	BusinessConnectionID string `json:"business_connection_id"`
	IsPublic             bool   `json:"is_public,omitempty"`
}

// PostStoryRequest represents parameters for postStory.
type PostStoryRequest struct {
	BusinessConnectionID string            `json:"business_connection_id,omitempty"`
	ChatID               int64             `json:"chat_id"`
	Content              json.RawMessage   `json:"content"`
	Caption              string            `json:"caption,omitempty"`
	ParseMode            string            `json:"parse_mode,omitempty"`
	CaptionEntities      []MessageEntity   `json:"caption_entities,omitempty"`
	ActivePeriod         int               `json:"active_period,omitempty"`
	PostToChatPage       bool              `json:"post_to_chat_page,omitempty"`
	ProtectContent       bool              `json:"protect_content,omitempty"`
	Files                map[string][]byte `json:"-"`
	FileNames            map[string]string `json:"-"`
}

type DeleteStoryRequest struct {
	BusinessConnectionID string `json:"business_connection_id"`
	StoryID              int    `json:"story_id"`
}

type ReadBusinessMessageRequest struct {
	BusinessConnectionID string `json:"business_connection_id"`
	ChatID               int64  `json:"chat_id"`
	MessageID            int64  `json:"message_id"`
}
