package converter

import "encoding/json"

// ChatFullInfo represents full information about a chat.
type ChatFullInfo struct {
	ID                    int64            `json:"id"`
	Type                  string           `json:"type"`
	Title                 string           `json:"title,omitempty"`
	Username              string           `json:"username,omitempty"`
	FirstName             string           `json:"first_name,omitempty"`
	LastName              string           `json:"last_name,omitempty"`
	IsForum               bool             `json:"is_forum,omitempty"`
	AccentColorID         int              `json:"accent_color_id,omitempty"`
	MaxReactionCount      int              `json:"max_reaction_count,omitempty"`
	Bio                   string           `json:"bio,omitempty"`
	Description           string           `json:"description,omitempty"`
	InviteLink            string           `json:"invite_link,omitempty"`
	PinnedMessage         *Message         `json:"pinned_message,omitempty"`
	Permissions           *ChatPermissions `json:"permissions,omitempty"`
	SlowModeDelay         int              `json:"slow_mode_delay,omitempty"`
	MessageAutoDeleteTime int              `json:"message_auto_delete_time,omitempty"`
	HasAggressiveAntiSpam bool             `json:"has_aggressive_anti_spam_enabled,omitempty"`
	HasHiddenMembers      bool             `json:"has_hidden_members,omitempty"`
	HasProtectedContent   bool             `json:"has_protected_content,omitempty"`
	HasVisibleHistory     bool             `json:"has_visible_history,omitempty"`
	CanSetStickerSet      bool             `json:"can_set_sticker_set,omitempty"`
	CustomEmojiStickerSet string           `json:"custom_emoji_sticker_set_name,omitempty"`
}

// ChatPermissions describes actions that a non-administrator user is allowed to take in a chat.
type ChatPermissions struct {
	CanSendMessages       bool `json:"can_send_messages,omitempty"`
	CanSendAudios         bool `json:"can_send_audios,omitempty"`
	CanSendDocuments      bool `json:"can_send_documents,omitempty"`
	CanSendPhotos         bool `json:"can_send_photos,omitempty"`
	CanSendVideos         bool `json:"can_send_videos,omitempty"`
	CanSendVideoNotes     bool `json:"can_send_video_notes,omitempty"`
	CanSendVoiceNotes     bool `json:"can_send_voice_notes,omitempty"`
	CanSendPolls          bool `json:"can_send_polls,omitempty"`
	CanSendOtherMessages  bool `json:"can_send_other_messages,omitempty"`
	CanAddWebPagePreviews bool `json:"can_add_web_page_previews,omitempty"`
	CanChangeInfo         bool `json:"can_change_info,omitempty"`
	CanInviteUsers        bool `json:"can_invite_users,omitempty"`
	CanPinMessages        bool `json:"can_pin_messages,omitempty"`
	CanManageTopics       bool `json:"can_manage_topics,omitempty"`
}

// ChatMember contains information about one member of a chat.
type ChatMember struct {
	Status                string `json:"status"` // "creator", "administrator", "member", "restricted", "left", "kicked"
	User                  *User  `json:"user"`
	IsAnonymous           bool   `json:"is_anonymous,omitempty"`
	CustomTitle           string `json:"custom_title,omitempty"`
	CanBeEdited           bool   `json:"can_be_edited,omitempty"`
	CanManageChat         bool   `json:"can_manage_chat,omitempty"`
	CanDeleteMessages     bool   `json:"can_delete_messages,omitempty"`
	CanManageVideoChats   bool   `json:"can_manage_video_chats,omitempty"`
	CanRestrictMembers    bool   `json:"can_restrict_members,omitempty"`
	CanPromoteMembers     bool   `json:"can_promote_members,omitempty"`
	CanChangeInfo         bool   `json:"can_change_info,omitempty"`
	CanInviteUsers        bool   `json:"can_invite_users,omitempty"`
	CanPostStories        bool   `json:"can_post_stories,omitempty"`
	CanEditStories        bool   `json:"can_edit_stories,omitempty"`
	CanDeleteStories      bool   `json:"can_delete_stories,omitempty"`
	CanPostMessages       bool   `json:"can_post_messages,omitempty"`
	CanEditMessages       bool   `json:"can_edit_messages,omitempty"`
	CanPinMessages        bool   `json:"can_pin_messages,omitempty"`
	CanManageTopics       bool   `json:"can_manage_topics,omitempty"`
	IsMember              bool   `json:"is_member,omitempty"`
	CanSendMessages       bool   `json:"can_send_messages,omitempty"`
	CanSendAudios         bool   `json:"can_send_audios,omitempty"`
	CanSendDocuments      bool   `json:"can_send_documents,omitempty"`
	CanSendPhotos         bool   `json:"can_send_photos,omitempty"`
	CanSendVideos         bool   `json:"can_send_videos,omitempty"`
	CanSendVideoNotes     bool   `json:"can_send_video_notes,omitempty"`
	CanSendVoiceNotes     bool   `json:"can_send_voice_notes,omitempty"`
	CanSendPolls          bool   `json:"can_send_polls,omitempty"`
	CanSendOtherMessages  bool   `json:"can_send_other_messages,omitempty"`
	CanAddWebPagePreviews bool   `json:"can_add_web_page_previews,omitempty"`
	UntilDate             int    `json:"until_date,omitempty"`
}

// ChatInviteLink represents an invite link for a chat.
type ChatInviteLink struct {
	InviteLink         string `json:"invite_link"`
	Creator            *User  `json:"creator"`
	CreatesJoinRequest bool   `json:"creates_join_request"`
	IsPrimary          bool   `json:"is_primary"`
	IsRevoked          bool   `json:"is_revoked"`
	Name               string `json:"name,omitempty"`
	ExpireDate         int    `json:"expire_date,omitempty"`
	MemberLimit        int    `json:"member_limit,omitempty"`
	PendingMemberCount int    `json:"pending_member_count,omitempty"`
	SubscriptionPeriod int    `json:"subscription_period,omitempty"`
	SubscriptionPrice  int    `json:"subscription_price,omitempty"`
}

// ChatAdministratorRights represents the rights of an administrator in a chat.
type ChatAdministratorRights struct {
	IsAnonymous         bool `json:"is_anonymous"`
	CanManageChat       bool `json:"can_manage_chat"`
	CanDeleteMessages   bool `json:"can_delete_messages"`
	CanManageVideoChats bool `json:"can_manage_video_chats"`
	CanRestrictMembers  bool `json:"can_restrict_members"`
	CanPromoteMembers   bool `json:"can_promote_members"`
	CanChangeInfo       bool `json:"can_change_info"`
	CanInviteUsers      bool `json:"can_invite_users"`
	CanPostStories      bool `json:"can_post_stories,omitempty"`
	CanEditStories      bool `json:"can_edit_stories,omitempty"`
	CanDeleteStories    bool `json:"can_delete_stories,omitempty"`
	CanPostMessages     bool `json:"can_post_messages,omitempty"`
	CanEditMessages     bool `json:"can_edit_messages,omitempty"`
	CanPinMessages      bool `json:"can_pin_messages,omitempty"`
	CanManageTopics     bool `json:"can_manage_topics,omitempty"`
}

// MenuButton describes the bot's menu button in a private chat.
type MenuButton struct {
	Type   string      `json:"type"` // "commands", "web_app", "default"
	Text   string      `json:"text,omitempty"`
	WebApp *WebAppInfo `json:"web_app,omitempty"`
}

// WebAppInfo describes a Web App.
type WebAppInfo struct {
	URL string `json:"url"`
}

// BotName represents the bot's name.
type BotName struct {
	Name string `json:"name"`
}

// BotDescription represents the bot's description.
type BotDescription struct {
	Description string `json:"description"`
}

// BotShortDescription represents the bot's short description.
type BotShortDescription struct {
	ShortDescription string `json:"short_description"`
}

// ForumTopic represents a forum topic.
type ForumTopic struct {
	MessageThreadID   int    `json:"message_thread_id"`
	Name              string `json:"name"`
	IconColor         int    `json:"icon_color"`
	IconCustomEmojiID string `json:"icon_custom_emoji_id,omitempty"`
}

// PollOption contains information about one answer option in a poll.
type PollOption struct {
	Text       string `json:"text"`
	VoterCount int    `json:"voter_count"`
}

// Poll contains information about a poll.
type Poll struct {
	ID                    string          `json:"id"`
	Question              string          `json:"question"`
	Options               []PollOption    `json:"options"`
	TotalVoterCount       int             `json:"total_voter_count"`
	IsClosed              bool            `json:"is_closed"`
	IsAnonymous           bool            `json:"is_anonymous"`
	Type                  string          `json:"type"`
	AllowsMultipleAnswers bool            `json:"allows_multiple_answers"`
	CorrectOptionID       *int            `json:"correct_option_id,omitempty"`
	Explanation           string          `json:"explanation,omitempty"`
	ExplanationEntities   []MessageEntity `json:"explanation_entities,omitempty"`
	OpenPeriod            int             `json:"open_period,omitempty"`
	CloseDate             int             `json:"close_date,omitempty"`
}

// ReactionType represents a reaction type.
type ReactionType struct {
	Type          string `json:"type"` // "emoji" or "custom_emoji"
	Emoji         string `json:"emoji,omitempty"`
	CustomEmojiID string `json:"custom_emoji_id,omitempty"`
}

// Sticker represents a sticker.
type Sticker struct {
	FileID          string     `json:"file_id"`
	FileUniqueID    string     `json:"file_unique_id"`
	Type            string     `json:"type"` // "regular", "mask", "custom_emoji"
	Width           int        `json:"width"`
	Height          int        `json:"height"`
	IsAnimated      bool       `json:"is_animated"`
	IsVideo         bool       `json:"is_video"`
	Thumbnail       *PhotoSize `json:"thumbnail,omitempty"`
	Emoji           string     `json:"emoji,omitempty"`
	SetName         string     `json:"set_name,omitempty"`
	FileSize        int        `json:"file_size,omitempty"`
	CustomEmojiID   string     `json:"custom_emoji_id,omitempty"`
	NeedsRepainting bool       `json:"needs_repainting,omitempty"`
}

// StickerSet represents a sticker set.
type StickerSet struct {
	Name        string     `json:"name"`
	Title       string     `json:"title"`
	StickerType string     `json:"sticker_type"` // "regular", "mask", "custom_emoji"
	Stickers    []Sticker  `json:"stickers"`
	Thumbnail   *PhotoSize `json:"thumbnail,omitempty"`
}

// SentWebAppMessage contains information about an inline message sent by a Web App user.
type SentWebAppMessage struct {
	InlineMessageID string `json:"inline_message_id,omitempty"`
}

// PreparedInlineMessage describes an inline message to be sent by and on behalf of a user.
type PreparedInlineMessage struct {
	ID             string `json:"id"`
	ExpirationDate int    `json:"expiration_date"`
}

// StarTransactions contains a list of Telegram Star transactions.
type StarTransactions struct {
	Transactions []interface{} `json:"transactions"`
}

// GameHighScore represents one row of the high scores table for a game.
type GameHighScore struct {
	Position int   `json:"position"`
	User     User  `json:"user"`
	Score    int64 `json:"score"`
}

// UserChatBoosts represents a list of boosts added to a chat by a user.
type UserChatBoosts struct {
	Boosts []ChatBoost `json:"boosts"`
}

type ChatBoost struct {
	BoostID        string          `json:"boost_id"`
	AddDate        int             `json:"add_date"`
	ExpirationDate int             `json:"expiration_date"`
	Source         ChatBoostSource `json:"source"`
}

type ChatBoostSource struct {
	Source            string `json:"source"`
	User              User   `json:"user"`
	GiveawayMessageID int    `json:"giveaway_message_id,omitempty"`
	IsUnclaimed       bool   `json:"is_unclaimed,omitempty"`
	PrizeStarCount    int64  `json:"prize_star_count,omitempty"`
}

// -------------------------------------------------------------
// Request structs for the newly added Bot API methods:
// -------------------------------------------------------------

type GetChatRequest struct {
	ChatID int64 `json:"chat_id"`
}

type ForwardMessageRequest struct {
	ChatID              int64 `json:"chat_id"`
	MessageThreadID     int   `json:"message_thread_id,omitempty"`
	FromChatID          int64 `json:"from_chat_id"`
	DisableNotification bool  `json:"disable_notification,omitempty"`
	ProtectContent      bool  `json:"protect_content,omitempty"`
	VideoStartTimestamp int   `json:"video_start_timestamp,omitempty"`
	MessageID           int64 `json:"message_id"`
}

type ForwardMessagesRequest struct {
	ChatID                int64   `json:"chat_id"`
	MessageThreadID       int     `json:"message_thread_id,omitempty"`
	DirectMessagesTopicID int     `json:"direct_messages_topic_id,omitempty"`
	FromChatID            int64   `json:"from_chat_id"`
	MessageIDs            []int64 `json:"message_ids"`
	DisableNotification   bool    `json:"disable_notification,omitempty"`
	ProtectContent        bool    `json:"protect_content,omitempty"`
	MessageEffectID       string  `json:"message_effect_id,omitempty"`
}

type CopyMessagesRequest struct {
	ChatID                int64   `json:"chat_id"`
	MessageThreadID       int     `json:"message_thread_id,omitempty"`
	DirectMessagesTopicID int     `json:"direct_messages_topic_id,omitempty"`
	FromChatID            int64   `json:"from_chat_id"`
	MessageIDs            []int64 `json:"message_ids"`
	DisableNotification   bool    `json:"disable_notification,omitempty"`
	ProtectContent        bool    `json:"protect_content,omitempty"`
	RemoveCaption         bool    `json:"remove_caption,omitempty"`
	MessageEffectID       string  `json:"message_effect_id,omitempty"`
}

type DeleteMessagesRequest struct {
	ChatID     int64   `json:"chat_id"`
	MessageIDs []int64 `json:"message_ids"`
}

type SetMessageReactionRequest struct {
	ChatID    int64          `json:"chat_id"`
	MessageID int64          `json:"message_id"`
	Reaction  []ReactionType `json:"reaction,omitempty"`
	IsBig     bool           `json:"is_big,omitempty"`
}

type SendLocationRequest struct {
	BusinessConnectionID string           `json:"business_connection_id,omitempty"`
	ChatID               int64            `json:"chat_id"`
	MessageThreadID      int              `json:"message_thread_id,omitempty"`
	Latitude             float64          `json:"latitude"`
	Longitude            float64          `json:"longitude"`
	HorizontalAccuracy   float64          `json:"horizontal_accuracy,omitempty"`
	LivePeriod           int              `json:"live_period,omitempty"`
	Heading              int              `json:"heading,omitempty"`
	ProximityAlertRadius int              `json:"proximity_alert_radius,omitempty"`
	DisableNotification  bool             `json:"disable_notification,omitempty"`
	ProtectContent       bool             `json:"protect_content,omitempty"`
	ReplyParameters      *ReplyParameters `json:"reply_parameters,omitempty"`
	ReplyMarkup          json.RawMessage  `json:"reply_markup,omitempty"`
}

type EditMessageLiveLocationRequest struct {
	BusinessConnectionID string          `json:"business_connection_id,omitempty"`
	ChatID               int64           `json:"chat_id,omitempty"`
	MessageID            int64           `json:"message_id,omitempty"`
	InlineMessageID      string          `json:"inline_message_id,omitempty"`
	Latitude             float64         `json:"latitude"`
	Longitude            float64         `json:"longitude"`
	HorizontalAccuracy   float64         `json:"horizontal_accuracy,omitempty"`
	Heading              int             `json:"heading,omitempty"`
	ProximityAlertRadius int             `json:"proximity_alert_radius,omitempty"`
	LivePeriod           int             `json:"live_period,omitempty"`
	ReplyMarkup          json.RawMessage `json:"reply_markup,omitempty"`
}

type StopMessageLiveLocationRequest struct {
	BusinessConnectionID string          `json:"business_connection_id,omitempty"`
	ChatID               int64           `json:"chat_id,omitempty"`
	MessageID            int64           `json:"message_id,omitempty"`
	InlineMessageID      string          `json:"inline_message_id,omitempty"`
	LivePeriod           int             `json:"live_period,omitempty"`
	Heading              int             `json:"heading,omitempty"`
	ProximityAlertRadius int             `json:"proximity_alert_radius,omitempty"`
	ReplyMarkup          json.RawMessage `json:"reply_markup,omitempty"`
}

type SendVenueRequest struct {
	BusinessConnectionID string           `json:"business_connection_id,omitempty"`
	ChatID               int64            `json:"chat_id"`
	MessageThreadID      int              `json:"message_thread_id,omitempty"`
	Latitude             float64          `json:"latitude"`
	Longitude            float64          `json:"longitude"`
	Title                string           `json:"title"`
	Address              string           `json:"address"`
	FoursquareID         string           `json:"foursquare_id,omitempty"`
	FoursquareType       string           `json:"foursquare_type,omitempty"`
	GooglePlaceID        string           `json:"google_place_id,omitempty"`
	GooglePlaceType      string           `json:"google_place_type,omitempty"`
	DisableNotification  bool             `json:"disable_notification,omitempty"`
	ProtectContent       bool             `json:"protect_content,omitempty"`
	ReplyParameters      *ReplyParameters `json:"reply_parameters,omitempty"`
	ReplyMarkup          json.RawMessage  `json:"reply_markup,omitempty"`
}

type SendContactRequest struct {
	BusinessConnectionID string           `json:"business_connection_id,omitempty"`
	ChatID               int64            `json:"chat_id"`
	MessageThreadID      int              `json:"message_thread_id,omitempty"`
	PhoneNumber          string           `json:"phone_number"`
	FirstName            string           `json:"first_name"`
	LastName             string           `json:"last_name,omitempty"`
	VCard                string           `json:"vcard,omitempty"`
	DisableNotification  bool             `json:"disable_notification,omitempty"`
	ProtectContent       bool             `json:"protect_content,omitempty"`
	ReplyParameters      *ReplyParameters `json:"reply_parameters,omitempty"`
	ReplyMarkup          json.RawMessage  `json:"reply_markup,omitempty"`
}

type SendPollRequest struct {
	BusinessConnectionID  string           `json:"business_connection_id,omitempty"`
	ChatID                int64            `json:"chat_id"`
	MessageThreadID       int              `json:"message_thread_id,omitempty"`
	Question              string           `json:"question"`
	QuestionParseMode     string           `json:"question_parse_mode,omitempty"`
	QuestionEntities      []MessageEntity  `json:"question_entities,omitempty"`
	Options               json.RawMessage  `json:"options"`
	IsAnonymous           *bool            `json:"is_anonymous,omitempty"`
	Type                  string           `json:"type,omitempty"`
	AllowsMultipleAnswers bool             `json:"allows_multiple_answers,omitempty"`
	CorrectOptionID       int              `json:"correct_option_id,omitempty"`
	Explanation           string           `json:"explanation,omitempty"`
	ExplanationParseMode  string           `json:"explanation_parse_mode,omitempty"`
	ExplanationEntities   []MessageEntity  `json:"explanation_entities,omitempty"`
	OpenPeriod            int              `json:"open_period,omitempty"`
	CloseDate             int              `json:"close_date,omitempty"`
	IsClosed              bool             `json:"is_closed,omitempty"`
	AllowAddingOptions    bool             `json:"allow_adding_options,omitempty"`
	HideResultsUntilClose bool             `json:"hide_results_until_closes,omitempty"`
	DisableNotification   bool             `json:"disable_notification,omitempty"`
	ProtectContent        bool             `json:"protect_content,omitempty"`
	ReplyParameters       *ReplyParameters `json:"reply_parameters,omitempty"`
	ReplyMarkup           json.RawMessage  `json:"reply_markup,omitempty"`
}

type StopPollRequest struct {
	BusinessConnectionID string          `json:"business_connection_id,omitempty"`
	ChatID               int64           `json:"chat_id"`
	MessageID            int64           `json:"message_id"`
	ReplyMarkup          json.RawMessage `json:"reply_markup,omitempty"`
}

type SendDiceRequest struct {
	BusinessConnectionID string           `json:"business_connection_id,omitempty"`
	ChatID               int64            `json:"chat_id"`
	MessageThreadID      int              `json:"message_thread_id,omitempty"`
	Emoji                string           `json:"emoji,omitempty"`
	DisableNotification  bool             `json:"disable_notification,omitempty"`
	ProtectContent       bool             `json:"protect_content,omitempty"`
	ReplyParameters      *ReplyParameters `json:"reply_parameters,omitempty"`
	ReplyMarkup          json.RawMessage  `json:"reply_markup,omitempty"`
}

type SetUserEmojiStatusRequest struct {
	UserID                    int64  `json:"user_id"`
	CustomEmojiID             string `json:"custom_emoji_id,omitempty"`
	EmojiStatusCustomEmojiID  string `json:"emoji_status_custom_emoji_id,omitempty"`
	EmojiStatusExpirationDate int    `json:"emoji_status_expiration_date,omitempty"`
}

type BanChatMemberRequest struct {
	ChatID         int64 `json:"chat_id"`
	UserID         int64 `json:"user_id"`
	UntilDate      int   `json:"until_date,omitempty"`
	RevokeMessages bool  `json:"revoke_messages,omitempty"`
}

type UnbanChatMemberRequest struct {
	ChatID       int64 `json:"chat_id"`
	UserID       int64 `json:"user_id"`
	OnlyIfBanned bool  `json:"only_if_banned,omitempty"`
}

type RestrictChatMemberRequest struct {
	ChatID                        int64           `json:"chat_id"`
	UserID                        int64           `json:"user_id"`
	Permissions                   ChatPermissions `json:"permissions"`
	UseIndependentChatPermissions bool            `json:"use_independent_chat_permissions,omitempty"`
	UntilDate                     int             `json:"until_date,omitempty"`
}

type PromoteChatMemberRequest struct {
	ChatID                   int64 `json:"chat_id"`
	UserID                   int64 `json:"user_id"`
	IsAnonymous              bool  `json:"is_anonymous,omitempty"`
	CanManageChat            bool  `json:"can_manage_chat,omitempty"`
	CanDeleteMessages        bool  `json:"can_delete_messages,omitempty"`
	CanManageVideoChats      bool  `json:"can_manage_video_chats,omitempty"`
	CanManageVoiceChats      bool  `json:"can_manage_voice_chats,omitempty"`
	CanRestrictMembers       bool  `json:"can_restrict_members,omitempty"`
	CanPromoteMembers        bool  `json:"can_promote_members,omitempty"`
	CanChangeInfo            bool  `json:"can_change_info,omitempty"`
	CanInviteUsers           bool  `json:"can_invite_users,omitempty"`
	CanPostStories           bool  `json:"can_post_stories,omitempty"`
	CanEditStories           bool  `json:"can_edit_stories,omitempty"`
	CanDeleteStories         bool  `json:"can_delete_stories,omitempty"`
	CanPostMessages          bool  `json:"can_post_messages,omitempty"`
	CanEditMessages          bool  `json:"can_edit_messages,omitempty"`
	CanPinMessages           bool  `json:"can_pin_messages,omitempty"`
	CanManageTopics          bool  `json:"can_manage_topics,omitempty"`
	CanManageTags            bool  `json:"can_manage_tags,omitempty"`
	CanManageDirectMessages  bool  `json:"can_manage_direct_messages,omitempty"`
	CanSendWelcomeMessages   bool  `json:"can_send_welcome_messages,omitempty"`
}

type SetChatAdministratorCustomTitleRequest struct {
	ChatID      int64  `json:"chat_id"`
	UserID      int64  `json:"user_id"`
	CustomTitle string `json:"custom_title"`
}

type BanChatSenderChatRequest struct {
	ChatID       int64 `json:"chat_id"`
	SenderChatID int64 `json:"sender_chat_id"`
	UntilDate    int   `json:"until_date,omitempty"`
}

type UnbanChatSenderChatRequest struct {
	ChatID       int64 `json:"chat_id"`
	SenderChatID int64 `json:"sender_chat_id"`
}

type SetChatPermissionsRequest struct {
	ChatID                        int64           `json:"chat_id"`
	Permissions                   ChatPermissions `json:"permissions"`
	UseIndependentChatPermissions bool            `json:"use_independent_chat_permissions,omitempty"`
}

type ExportChatInviteLinkRequest struct {
	ChatID int64 `json:"chat_id"`
}

type CreateChatInviteLinkRequest struct {
	ChatID             int64  `json:"chat_id"`
	Name               string `json:"name,omitempty"`
	ExpireDate         int    `json:"expire_date,omitempty"`
	MemberLimit        int    `json:"member_limit,omitempty"`
	CreatesJoinRequest bool   `json:"creates_join_request,omitempty"`
}

type EditChatInviteLinkRequest struct {
	ChatID             int64  `json:"chat_id"`
	InviteLink         string `json:"invite_link"`
	Name               string `json:"name,omitempty"`
	ExpireDate         int    `json:"expire_date,omitempty"`
	MemberLimit        int    `json:"member_limit,omitempty"`
	CreatesJoinRequest bool   `json:"creates_join_request,omitempty"`
}

type CreateChatSubscriptionInviteLinkRequest struct {
	ChatID             int64  `json:"chat_id"`
	Name               string `json:"name,omitempty"`
	SubscriptionPeriod int    `json:"subscription_period"`
	SubscriptionPrice  int64  `json:"subscription_price"`
}

type EditChatSubscriptionInviteLinkRequest struct {
	ChatID     int64  `json:"chat_id"`
	InviteLink string `json:"invite_link"`
	Name       string `json:"name,omitempty"`
}

type SetChatStickerSetRequest struct {
	ChatID         int64  `json:"chat_id"`
	StickerSetName string `json:"sticker_set_name"`
}

type DeleteChatStickerSetRequest struct {
	ChatID int64 `json:"chat_id"`
}

type RevokeChatInviteLinkRequest struct {
	ChatID     int64  `json:"chat_id"`
	InviteLink string `json:"invite_link"`
}

type ApproveChatJoinRequestRequest struct {
	ChatID int64 `json:"chat_id"`
	UserID int64 `json:"user_id"`
}

type DeclineChatJoinRequestRequest struct {
	ChatID int64 `json:"chat_id"`
	UserID int64 `json:"user_id"`
}

type SetChatPhotoRequest struct {
	ChatID    int64  `json:"chat_id"`
	Photo     string `json:"photo,omitempty"`
	PhotoData []byte `json:"-"`
}

type DeleteChatPhotoRequest struct {
	ChatID int64 `json:"chat_id"`
}

type SetChatTitleRequest struct {
	ChatID int64  `json:"chat_id"`
	Title  string `json:"title"`
}

type SetChatDescriptionRequest struct {
	ChatID      int64  `json:"chat_id"`
	Description string `json:"description,omitempty"`
}

type PinChatMessageRequest struct {
	BusinessConnectionID string `json:"business_connection_id,omitempty"`
	ChatID               int64  `json:"chat_id"`
	MessageID            int64  `json:"message_id"`
	DisableNotification  bool   `json:"disable_notification,omitempty"`
}

type UnpinChatMessageRequest struct {
	BusinessConnectionID string `json:"business_connection_id,omitempty"`
	ChatID               int64  `json:"chat_id"`
	MessageID            int64  `json:"message_id,omitempty"`
}

type UnpinAllChatMessagesRequest struct {
	ChatID int64 `json:"chat_id"`
}

type LeaveChatRequest struct {
	ChatID int64 `json:"chat_id"`
}

type GetChatAdministratorsRequest struct {
	ChatID int64 `json:"chat_id"`
}

type GetChatMemberCountRequest struct {
	ChatID int64 `json:"chat_id"`
}

type SetChatMenuButtonRequest struct {
	ChatID     int64       `json:"chat_id,omitempty"`
	UserID     int64       `json:"user_id,omitempty"`
	MenuButton *MenuButton `json:"menu_button,omitempty"`
}

type GetChatMenuButtonRequest struct {
	ChatID int64 `json:"chat_id,omitempty"`
	UserID int64 `json:"user_id,omitempty"`
}

type SetMyDefaultAdministratorRightsRequest struct {
	Rights      *ChatAdministratorRights `json:"rights,omitempty"`
	ForChannels bool                     `json:"for_channels,omitempty"`
}

type GetMyDefaultAdministratorRightsRequest struct {
	ForChannels bool `json:"for_channels,omitempty"`
}

type GetMyCommandsRequest struct {
	Scope        json.RawMessage `json:"scope,omitempty"`
	LanguageCode string          `json:"language_code,omitempty"`
}

type DeleteMyCommandsRequest struct {
	Scope        json.RawMessage `json:"scope,omitempty"`
	LanguageCode string          `json:"language_code,omitempty"`
}

type SetMyNameRequest struct {
	Name         string `json:"name,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
}

type GetMyNameRequest struct {
	LanguageCode string `json:"language_code,omitempty"`
}

type SetMyDescriptionRequest struct {
	Description  string `json:"description,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
}

type GetMyDescriptionRequest struct {
	LanguageCode string `json:"language_code,omitempty"`
}

type SetMyShortDescriptionRequest struct {
	ShortDescription string `json:"short_description,omitempty"`
	LanguageCode     string `json:"language_code,omitempty"`
}

type GetMyShortDescriptionRequest struct {
	LanguageCode string `json:"language_code,omitempty"`
}

type CreateForumTopicRequest struct {
	ChatID            int64  `json:"chat_id"`
	Name              string `json:"name"`
	IconColor         int    `json:"icon_color,omitempty"`
	IconCustomEmojiID string `json:"icon_custom_emoji_id,omitempty"`
}

type EditForumTopicRequest struct {
	ChatID            int64  `json:"chat_id"`
	MessageThreadID   int    `json:"message_thread_id"`
	Name              string `json:"name,omitempty"`
	IconCustomEmojiID string `json:"icon_custom_emoji_id,omitempty"`
}

type CloseForumTopicRequest struct {
	ChatID          int64 `json:"chat_id"`
	MessageThreadID int   `json:"message_thread_id"`
}

type ReopenForumTopicRequest struct {
	ChatID          int64 `json:"chat_id"`
	MessageThreadID int   `json:"message_thread_id"`
}

type DeleteForumTopicRequest struct {
	ChatID          int64 `json:"chat_id"`
	MessageThreadID int   `json:"message_thread_id"`
}

type UnpinAllForumTopicMessagesRequest struct {
	ChatID          int64 `json:"chat_id"`
	MessageThreadID int   `json:"message_thread_id"`
}

type EditGeneralForumTopicRequest struct {
	ChatID int64  `json:"chat_id"`
	Name   string `json:"name"`
}

type CloseGeneralForumTopicRequest struct {
	ChatID int64 `json:"chat_id"`
}

type ReopenGeneralForumTopicRequest struct {
	ChatID int64 `json:"chat_id"`
}

type HideGeneralForumTopicRequest struct {
	ChatID int64 `json:"chat_id"`
}

type UnhideGeneralForumTopicRequest struct {
	ChatID int64 `json:"chat_id"`
}

type UnpinAllGeneralForumTopicMessagesRequest struct {
	ChatID int64 `json:"chat_id"`
}

type AnswerInlineQueryRequest struct {
	InlineQueryID     string          `json:"inline_query_id"`
	Results           json.RawMessage `json:"results"`
	CacheTime         int             `json:"cache_time,omitempty"`
	IsPersonal        bool            `json:"is_personal,omitempty"`
	NextOffset        string          `json:"next_offset,omitempty"`
	Button            json.RawMessage `json:"button,omitempty"`
	SwitchPmText      string          `json:"switch_pm_text,omitempty"`
	SwitchPmParameter string          `json:"switch_pm_parameter,omitempty"`
}

type AnswerWebAppQueryRequest struct {
	WebAppQueryID string          `json:"web_app_query_id"`
	Result        json.RawMessage `json:"result"`
}

type SavePreparedInlineMessageRequest struct {
	UserID            int64           `json:"user_id"`
	Result            json.RawMessage `json:"result"`
	AllowUserChats    bool            `json:"allow_user_chats,omitempty"`
	AllowBotChats     bool            `json:"allow_bot_chats,omitempty"`
	AllowGroupChats   bool            `json:"allow_group_chats,omitempty"`
	AllowChannelChats bool            `json:"allow_channel_chats,omitempty"`
}

type SendInvoiceRequest struct {
	ChatID                    int64            `json:"chat_id"`
	MessageThreadID           int              `json:"message_thread_id,omitempty"`
	Title                     string           `json:"title"`
	Description               string           `json:"description"`
	Payload                   string           `json:"payload"`
	ProviderToken             string           `json:"provider_token,omitempty"`
	Currency                  string           `json:"currency"`
	Prices                    json.RawMessage  `json:"prices"`
	MaxTipAmount              int              `json:"max_tip_amount,omitempty"`
	SuggestedTipAmounts       []int            `json:"suggested_tip_amounts,omitempty"`
	StartParameter            string           `json:"start_parameter,omitempty"`
	ProviderData              string           `json:"provider_data,omitempty"`
	PhotoURL                  string           `json:"photo_url,omitempty"`
	PhotoSize                 int              `json:"photo_size,omitempty"`
	PhotoWidth                int              `json:"photo_width,omitempty"`
	PhotoHeight               int              `json:"photo_height,omitempty"`
	NeedName                  bool             `json:"need_name,omitempty"`
	NeedPhoneNumber           bool             `json:"need_phone_number,omitempty"`
	NeedEmail                 bool             `json:"need_email,omitempty"`
	NeedShippingAddress       bool             `json:"need_shipping_address,omitempty"`
	SendPhoneNumberToProvider bool             `json:"send_phone_number_to_provider,omitempty"`
	SendEmailToProvider       bool             `json:"send_email_to_provider,omitempty"`
	IsFlexible                bool             `json:"is_flexible,omitempty"`
	DisableNotification       bool             `json:"disable_notification,omitempty"`
	ProtectContent            bool             `json:"protect_content,omitempty"`
	ReplyParameters           *ReplyParameters `json:"reply_parameters,omitempty"`
	ReplyMarkup               json.RawMessage  `json:"reply_markup,omitempty"`
}

type CreateInvoiceLinkRequest struct {
	BusinessConnectionID      string          `json:"business_connection_id,omitempty"`
	Title                     string          `json:"title"`
	Description               string          `json:"description"`
	Payload                   string          `json:"payload"`
	ProviderToken             string          `json:"provider_token,omitempty"`
	Currency                  string          `json:"currency"`
	Prices                    json.RawMessage `json:"prices"`
	SubscriptionPeriod        int             `json:"subscription_period,omitempty"`
	MaxTipAmount              int             `json:"max_tip_amount,omitempty"`
	SuggestedTipAmounts       []int           `json:"suggested_tip_amounts,omitempty"`
	StartParameter            string          `json:"start_parameter,omitempty"`
	ProviderData              string          `json:"provider_data,omitempty"`
	PhotoURL                  string          `json:"photo_url,omitempty"`
	PhotoSize                 int             `json:"photo_size,omitempty"`
	PhotoWidth                int             `json:"photo_width,omitempty"`
	PhotoHeight               int             `json:"photo_height,omitempty"`
	NeedName                  bool            `json:"need_name,omitempty"`
	NeedPhoneNumber           bool            `json:"need_phone_number,omitempty"`
	NeedEmail                 bool            `json:"need_email,omitempty"`
	NeedShippingAddress       bool            `json:"need_shipping_address,omitempty"`
	SendPhoneNumberToProvider bool            `json:"send_phone_number_to_provider,omitempty"`
	SendEmailToProvider       bool            `json:"send_email_to_provider,omitempty"`
	IsFlexible                bool            `json:"is_flexible,omitempty"`
}

type AnswerShippingQueryRequest struct {
	ShippingQueryID string          `json:"shipping_query_id"`
	OK              bool            `json:"ok"`
	ShippingOptions json.RawMessage `json:"shipping_options,omitempty"`
	ErrorMessage    string          `json:"error_message,omitempty"`
}

type SendPaidMediaRequest struct {
	BusinessConnectionID  string            `json:"business_connection_id,omitempty"`
	ChatID                int64             `json:"chat_id"`
	StarCount             int               `json:"star_count"`
	Media                 json.RawMessage   `json:"media"`
	Payload               string            `json:"payload,omitempty"`
	Caption               string            `json:"caption,omitempty"`
	ParseMode             string            `json:"parse_mode,omitempty"`
	CaptionEntities       []MessageEntity   `json:"caption_entities,omitempty"`
	ShowCaptionAboveMedia bool              `json:"show_caption_above_media,omitempty"`
	DisableNotification   bool              `json:"disable_notification,omitempty"`
	ProtectContent        bool              `json:"protect_content,omitempty"`
	ReplyParameters       *ReplyParameters  `json:"reply_parameters,omitempty"`
	ReplyMarkup           json.RawMessage   `json:"reply_markup,omitempty"`
	Files                 map[string][]byte `json:"-"`
	FileNames             map[string]string `json:"-"`
}

type RefundStarPaymentRequest struct {
	UserID                  int64  `json:"user_id"`
	TelegramPaymentChargeID string `json:"telegram_payment_charge_id"`
}

type GetStarTransactionsRequest struct {
	Offset int `json:"offset,omitempty"`
	Limit  int `json:"limit,omitempty"`
}

type SendGiftRequest struct {
	UserID        int64  `json:"user_id,omitempty"`
	ChatID        int64  `json:"chat_id,omitempty"`
	GiftID        string `json:"gift_id"`
	PayForUpgrade bool   `json:"pay_for_upgrade,omitempty"`
	Text          string `json:"text,omitempty"`
	TextParseMode string `json:"text_parse_mode,omitempty"`
}

type DeleteMessageReactionRequest struct {
	ChatID      int64  `json:"chat_id"`
	MessageID   int64  `json:"message_id"`
	UserID      int64  `json:"user_id,omitempty"`
	ActorChatID string `json:"actor_chat_id,omitempty"`
}

type DeleteAllMessageReactionsRequest struct {
	ChatID      int64  `json:"chat_id"`
	MessageID   int64  `json:"message_id,omitempty"`
	ActorChatID string `json:"actor_chat_id,omitempty"`
	UserID      int64  `json:"user_id,omitempty"`
}

type VerifyUserRequest struct {
	UserID            int64  `json:"user_id"`
	CustomDescription string `json:"custom_description,omitempty"`
}

type VerifyChatRequest struct {
	ChatID            int64  `json:"chat_id"`
	CustomDescription string `json:"custom_description,omitempty"`
}

type RemoveUserVerificationRequest struct {
	UserID int64 `json:"user_id"`
}

type RemoveChatVerificationRequest struct {
	ChatID int64 `json:"chat_id"`
}

type GetStickerSetRequest struct {
	Name string `json:"name"`
}

type GetCustomEmojiStickersRequest struct {
	CustomEmojiIDs []string `json:"custom_emoji_ids"`
}

type UploadStickerFileRequest struct {
	UserID        int64  `json:"user_id"`
	Sticker       string `json:"sticker,omitempty"`
	PngSticker    string `json:"png_sticker,omitempty"`
	StickerFormat string `json:"sticker_format"`
	StickerData   []byte `json:"-"`
}

type CreateNewStickerSetRequest struct {
	UserID          int64           `json:"user_id"`
	Name            string          `json:"name"`
	Title           string          `json:"title"`
	Stickers        json.RawMessage `json:"stickers"`
	StickerType     string          `json:"sticker_type,omitempty"`
	NeedsRepainting bool            `json:"needs_repainting,omitempty"`
	ContainsMasks   bool            `json:"contains_masks,omitempty"`
}

type AddStickerToSetRequest struct {
	UserID  int64           `json:"user_id"`
	Name    string          `json:"name"`
	Sticker json.RawMessage `json:"sticker"`
}

type SetStickerPositionInSetRequest struct {
	Sticker  string `json:"sticker"`
	Position int    `json:"position"`
}

type DeleteStickerFromSetRequest struct {
	Sticker string `json:"sticker"`
}

type ReplaceStickerInSetRequest struct {
	UserID     int64           `json:"user_id"`
	Name       string          `json:"name"`
	OldSticker string          `json:"old_sticker"`
	Sticker    json.RawMessage `json:"sticker"`
}

type SetStickerEmojiListRequest struct {
	Sticker   string   `json:"sticker"`
	EmojiList []string `json:"emoji_list"`
}

type SetStickerKeywordsRequest struct {
	Sticker  string   `json:"sticker"`
	Keywords []string `json:"keywords,omitempty"`
}

type SetStickerMaskPositionRequest struct {
	Sticker      string          `json:"sticker"`
	MaskPosition json.RawMessage `json:"mask_position,omitempty"`
}

type SetStickerSetTitleRequest struct {
	Name  string `json:"name"`
	Title string `json:"title"`
}

type SetStickerSetThumbnailRequest struct {
	Name          string `json:"name"`
	UserID        int64  `json:"user_id"`
	Thumbnail     string `json:"thumbnail,omitempty"`
	Format        string `json:"format,omitempty"`
	ThumbnailData []byte `json:"-"`
}

type SetCustomEmojiStickerSetThumbnailRequest struct {
	Name          string `json:"name"`
	CustomEmojiID string `json:"custom_emoji_id,omitempty"`
}

type DeleteStickerSetRequest struct {
	Name string `json:"name"`
}

type SendGameRequest struct {
	BusinessConnectionID string           `json:"business_connection_id,omitempty"`
	ChatID               int64            `json:"chat_id"`
	MessageThreadID      int              `json:"message_thread_id,omitempty"`
	GameShortName        string           `json:"game_short_name"`
	DisableNotification  bool             `json:"disable_notification,omitempty"`
	ProtectContent       bool             `json:"protect_content,omitempty"`
	ReplyParameters      *ReplyParameters `json:"reply_parameters,omitempty"`
	ReplyMarkup          json.RawMessage  `json:"reply_markup,omitempty"`
}

type SetGameScoreRequest struct {
	UserID             int64  `json:"user_id"`
	Score              int64  `json:"score"`
	Force              bool   `json:"force,omitempty"`
	DisableEditMessage bool   `json:"disable_edit_message,omitempty"`
	EditMessage        *bool  `json:"edit_message,omitempty"`
	ChatID             int64  `json:"chat_id,omitempty"`
	MessageID          int64  `json:"message_id,omitempty"`
	InlineMessageID    string `json:"inline_message_id,omitempty"`
}

type GetGameHighScoresRequest struct {
	UserID          int64  `json:"user_id"`
	ChatID          int64  `json:"chat_id,omitempty"`
	MessageID       int64  `json:"message_id,omitempty"`
	InlineMessageID string `json:"inline_message_id,omitempty"`
}

type SetPassportDataErrorsRequest struct {
	UserID int64           `json:"user_id"`
	Errors json.RawMessage `json:"errors"`
}

type GetUserChatBoostsRequest struct {
	ChatID int64 `json:"chat_id"`
	UserID int64 `json:"user_id"`
}
