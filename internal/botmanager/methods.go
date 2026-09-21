package botmanager

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"telego-bot-api/internal/converter"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/fileid"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

func (b *BotInstance) inputUser(userID int64) tg.InputUserClass {
	peer, err := b.resolvePeer(userID)
	if err == nil {
		if user, ok := peer.(*tg.InputPeerUser); ok {
			return &tg.InputUser{UserID: user.UserID, AccessHash: user.AccessHash}
		}
	}
	return &tg.InputUser{UserID: userID}
}

func inputChannel(peer tg.InputPeerClass) (*tg.InputChannel, error) {
	channel, ok := peer.(*tg.InputPeerChannel)
	if !ok {
		return nil, fmt.Errorf("method is only available for supergroups and channels")
	}
	return &tg.InputChannel{ChannelID: channel.ChannelID, AccessHash: channel.AccessHash}, nil
}

func convertAdminRights(rights *converter.ChatAdministratorRights) tg.ChatAdminRights {
	if rights == nil {
		return tg.ChatAdminRights{}
	}
	return tg.ChatAdminRights{
		ChangeInfo: rights.CanChangeInfo, PostMessages: rights.CanPostMessages,
		EditMessages: rights.CanEditMessages, DeleteMessages: rights.CanDeleteMessages,
		BanUsers: rights.CanRestrictMembers, InviteUsers: rights.CanInviteUsers,
		PinMessages: rights.CanPinMessages, AddAdmins: rights.CanPromoteMembers,
		Anonymous: rights.IsAnonymous, ManageCall: rights.CanManageVideoChats,
		Other: rights.CanManageChat, ManageTopics: rights.CanManageTopics,
		PostStories: rights.CanPostStories, EditStories: rights.CanEditStories,
		DeleteStories: rights.CanDeleteStories,
	}
}

func convertBotAdminRights(rights tg.ChatAdminRights) *converter.ChatAdministratorRights {
	return &converter.ChatAdministratorRights{
		IsAnonymous: rights.Anonymous, CanManageChat: rights.Other,
		CanDeleteMessages: rights.DeleteMessages, CanManageVideoChats: rights.ManageCall,
		CanRestrictMembers: rights.BanUsers, CanPromoteMembers: rights.AddAdmins,
		CanChangeInfo: rights.ChangeInfo, CanInviteUsers: rights.InviteUsers,
		CanPostStories: rights.PostStories, CanEditStories: rights.EditStories,
		CanDeleteStories: rights.DeleteStories, CanPostMessages: rights.PostMessages,
		CanEditMessages: rights.EditMessages, CanPinMessages: rights.PinMessages,
		CanManageTopics: rights.ManageTopics,
	}
}

func (b *BotInstance) convertInviteLink(invite *tg.ChatInviteExported) *converter.ChatInviteLink {
	result := &converter.ChatInviteLink{
		InviteLink: invite.Link, Creator: b.GetMe(), CreatesJoinRequest: invite.RequestNeeded,
		IsPrimary: invite.Permanent, IsRevoked: invite.Revoked, Name: invite.Title,
		ExpireDate: invite.ExpireDate, MemberLimit: invite.UsageLimit,
		PendingMemberCount: invite.Requested,
	}
	if invite.SubscriptionPricing.Amount > 0 {
		result.SubscriptionPeriod = invite.SubscriptionPricing.Period
		result.SubscriptionPrice = int(invite.SubscriptionPricing.Amount)
	}
	return result
}

func (b *BotInstance) hideChatJoinRequest(ctx context.Context, chatID, userID int64, approved bool) (bool, error) {
	peer, err := b.resolvePeer(chatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}
	_, err = b.raw.MessagesHideChatJoinRequest(ctx, &tg.MessagesHideChatJoinRequestRequest{
		Approved: approved, Peer: peer, UserID: b.inputUser(userID),
	})
	return err == nil, err
}

func (b *BotInstance) editGeneralForumTopic(ctx context.Context, chatID int64, configure func(*tg.MessagesEditForumTopicRequest)) (bool, error) {
	peer, err := b.resolvePeer(chatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}
	request := &tg.MessagesEditForumTopicRequest{Peer: peer, TopicID: 1}
	configure(request)
	_, err = b.raw.MessagesEditForumTopic(ctx, request)
	return err == nil, err
}

func convertStickerDocument(document *tg.Document) (converter.Sticker, error) {
	encoded, err := fileid.EncodeFileID(fileid.FromDocument(document))
	if err != nil {
		return converter.Sticker{}, err
	}
	sticker := converter.Sticker{FileID: encoded, FileUniqueID: strconv.FormatInt(document.ID, 10),
		Type: "regular", FileSize: int(document.Size)}
	if document.MimeType == "application/x-tgsticker" {
		sticker.IsAnimated = true
	}
	if document.MimeType == "video/webm" {
		sticker.IsVideo = true
	}
	for _, attribute := range document.Attributes {
		switch value := attribute.(type) {
		case *tg.DocumentAttributeImageSize:
			sticker.Width, sticker.Height = value.W, value.H
		case *tg.DocumentAttributeVideo:
			sticker.Width, sticker.Height = value.W, value.H
		case *tg.DocumentAttributeSticker:
			sticker.Emoji = value.Alt
			if value.Mask {
				sticker.Type = "mask"
			}
			if set, ok := value.Stickerset.(*tg.InputStickerSetShortName); ok {
				sticker.SetName = set.ShortName
			}
		case *tg.DocumentAttributeCustomEmoji:
			sticker.Type = "custom_emoji"
			sticker.CustomEmojiID = strconv.FormatInt(document.ID, 10)
			sticker.Emoji = value.Alt
			sticker.NeedsRepainting = value.TextColor
			if set, ok := value.Stickerset.(*tg.InputStickerSetShortName); ok {
				sticker.SetName = set.ShortName
			}
		}
	}
	return sticker, nil
}

func convertStickerSetDocuments(set tg.MessagesStickerSetClass) ([]converter.Sticker, error) {
	value, ok := set.(*tg.MessagesStickerSet)
	if !ok {
		return []converter.Sticker{}, nil
	}
	stickers := make([]converter.Sticker, 0, len(value.Documents))
	for _, documentClass := range value.Documents {
		document, ok := documentClass.(*tg.Document)
		if !ok {
			continue
		}
		sticker, err := convertStickerDocument(document)
		if err != nil {
			return nil, err
		}
		stickers = append(stickers, sticker)
	}
	return stickers, nil
}

func inputDocumentFromFileID(encoded string) (*tg.InputDocument, error) {
	decoded, err := fileid.DecodeFileID(encoded)
	if err != nil {
		return nil, fmt.Errorf("invalid sticker file_id: %w", err)
	}
	return &tg.InputDocument{ID: decoded.ID, AccessHash: decoded.AccessHash, FileReference: decoded.FileReference}, nil
}

func parseMaskCoords(raw json.RawMessage) (tg.MaskCoords, bool, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return tg.MaskCoords{}, false, nil
	}
	var value struct {
		Point  string  `json:"point"`
		XShift float64 `json:"x_shift"`
		YShift float64 `json:"y_shift"`
		Scale  float64 `json:"scale"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return tg.MaskCoords{}, false, err
	}
	points := map[string]int{"forehead": 0, "eyes": 1, "mouth": 2, "chin": 3}
	point, ok := points[value.Point]
	if !ok {
		return tg.MaskCoords{}, false, fmt.Errorf("invalid mask point %q", value.Point)
	}
	return tg.MaskCoords{N: point, X: value.XShift, Y: value.YShift, Zoom: value.Scale}, true, nil
}

func parseInputSticker(raw json.RawMessage) (tg.InputStickerSetItem, error) {
	var value struct {
		Sticker      string          `json:"sticker"`
		EmojiList    []string        `json:"emoji_list"`
		MaskPosition json.RawMessage `json:"mask_position"`
		Keywords     []string        `json:"keywords"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return tg.InputStickerSetItem{}, err
	}
	document, err := inputDocumentFromFileID(value.Sticker)
	if err != nil {
		return tg.InputStickerSetItem{}, err
	}
	item := tg.InputStickerSetItem{Document: document, Emoji: strings.Join(value.EmojiList, "")}
	if item.Emoji == "" {
		item.Emoji = "🙂"
	}
	if coords, ok, err := parseMaskCoords(value.MaskPosition); err != nil {
		return tg.InputStickerSetItem{}, err
	} else if ok {
		item.SetMaskCoords(coords)
	}
	if len(value.Keywords) != 0 {
		item.SetKeywords(strings.Join(value.Keywords, ","))
	}
	return item, nil
}

func (b *BotInstance) uploadStickerDocument(ctx context.Context, data []byte, fileName, format string) (*tg.Document, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("sticker file is required")
	}
	if fileName == "" {
		fileName = "sticker.webp"
	}
	mimeType := "image/webp"
	switch format {
	case "animated":
		mimeType = "application/x-tgsticker"
	case "video":
		mimeType = "video/webm"
	}
	input, err := uploader.NewUploader(b.raw).FromBytes(ctx, fileName, data)
	if err != nil {
		return nil, err
	}
	media := &tg.InputMediaUploadedDocument{File: input, MimeType: mimeType, ForceFile: true,
		Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeFilename{FileName: fileName}}}
	result, err := b.raw.MessagesUploadMedia(ctx, &tg.MessagesUploadMediaRequest{Peer: &tg.InputPeerSelf{}, Media: media})
	if err != nil {
		return nil, err
	}
	documentMedia, ok := result.(*tg.MessageMediaDocument)
	if !ok {
		return nil, fmt.Errorf("unexpected uploaded sticker media %T", result)
	}
	document, ok := documentMedia.Document.(*tg.Document)
	if !ok {
		return nil, fmt.Errorf("uploaded sticker has no document")
	}
	return document, nil
}

type inlineResultJSON struct {
	Type                string                    `json:"type"`
	ID                  string                    `json:"id"`
	Title               string                    `json:"title"`
	Description         string                    `json:"description"`
	URL                 string                    `json:"url"`
	GameShortName       string                    `json:"game_short_name"`
	PhotoFileID         string                    `json:"photo_file_id"`
	GIFFileID           string                    `json:"gif_file_id"`
	MPEG4FileID         string                    `json:"mpeg4_file_id"`
	VideoFileID         string                    `json:"video_file_id"`
	AudioFileID         string                    `json:"audio_file_id"`
	VoiceFileID         string                    `json:"voice_file_id"`
	DocumentFileID      string                    `json:"document_file_id"`
	StickerFileID       string                    `json:"sticker_file_id"`
	PhotoURL            string                    `json:"photo_url"`
	GIFURL              string                    `json:"gif_url"`
	MPEG4URL            string                    `json:"mpeg4_url"`
	VideoURL            string                    `json:"video_url"`
	AudioURL            string                    `json:"audio_url"`
	VoiceURL            string                    `json:"voice_url"`
	DocumentURL         string                    `json:"document_url"`
	ThumbnailURL        string                    `json:"thumbnail_url"`
	MimeType            string                    `json:"mime_type"`
	Width               int                       `json:"width"`
	Height              int                       `json:"height"`
	Duration            int                       `json:"duration"`
	Caption             string                    `json:"caption"`
	ParseMode           string                    `json:"parse_mode"`
	CaptionEntities     []converter.MessageEntity `json:"caption_entities"`
	InputMessageContent json.RawMessage           `json:"input_message_content"`
	ReplyMarkup         json.RawMessage           `json:"reply_markup"`
}

type inlineMessageContentJSON struct {
	MessageText           string                    `json:"message_text"`
	ParseMode             string                    `json:"parse_mode"`
	Entities              []converter.MessageEntity `json:"entities"`
	DisableWebPagePreview bool                      `json:"disable_web_page_preview"`
	Latitude              float64                   `json:"latitude"`
	Longitude             float64                   `json:"longitude"`
	HorizontalAccuracy    float64                   `json:"horizontal_accuracy"`
	LivePeriod            int                       `json:"live_period"`
	Heading               int                       `json:"heading"`
	ProximityAlertRadius  int                       `json:"proximity_alert_radius"`
	Title                 string                    `json:"title"`
	Address               string                    `json:"address"`
	FoursquareID          string                    `json:"foursquare_id"`
	FoursquareType        string                    `json:"foursquare_type"`
	GooglePlaceID         string                    `json:"google_place_id"`
	GooglePlaceType       string                    `json:"google_place_type"`
	PhoneNumber           string                    `json:"phone_number"`
	FirstName             string                    `json:"first_name"`
	LastName              string                    `json:"last_name"`
	VCard                 string                    `json:"vcard"`
}

func inlineMessageEntities(text, parseMode string, entities []converter.MessageEntity) (string, []tg.MessageEntityClass, error) {
	if len(entities) != 0 {
		return text, converter.ConvertEntities(entities), nil
	}
	if parseMode != "" {
		return converter.ParseTextFormatting(text, parseMode)
	}
	return text, nil, nil
}

func buildInlineMessage(result inlineResultJSON) (tg.InputBotInlineMessageClass, error) {
	markup, err := converter.ParseReplyMarkup(result.ReplyMarkup)
	if err != nil {
		return nil, fmt.Errorf("invalid reply_markup: %w", err)
	}
	if len(result.InputMessageContent) != 0 && string(result.InputMessageContent) != "null" {
		var content inlineMessageContentJSON
		if err := json.Unmarshal(result.InputMessageContent, &content); err != nil {
			return nil, fmt.Errorf("invalid input_message_content: %w", err)
		}
		switch {
		case content.MessageText != "":
			text, entities, err := inlineMessageEntities(content.MessageText, content.ParseMode, content.Entities)
			if err != nil {
				return nil, err
			}
			return &tg.InputBotInlineMessageText{Message: text, Entities: entities,
				NoWebpage: content.DisableWebPagePreview, ReplyMarkup: markup}, nil
		case content.PhoneNumber != "":
			return &tg.InputBotInlineMessageMediaContact{PhoneNumber: content.PhoneNumber, FirstName: content.FirstName,
				LastName: content.LastName, Vcard: content.VCard, ReplyMarkup: markup}, nil
		case content.Title != "" && content.Address != "":
			provider, venueID, venueType := "foursquare", content.FoursquareID, content.FoursquareType
			if content.GooglePlaceID != "" {
				provider, venueID, venueType = "gplaces", content.GooglePlaceID, content.GooglePlaceType
			}
			return &tg.InputBotInlineMessageMediaVenue{GeoPoint: &tg.InputGeoPoint{Lat: content.Latitude, Long: content.Longitude,
				AccuracyRadius: int(content.HorizontalAccuracy)}, Title: content.Title, Address: content.Address,
				Provider: provider, VenueID: venueID, VenueType: venueType, ReplyMarkup: markup}, nil
		default:
			return &tg.InputBotInlineMessageMediaGeo{GeoPoint: &tg.InputGeoPoint{Lat: content.Latitude, Long: content.Longitude,
				AccuracyRadius: int(content.HorizontalAccuracy)}, Period: content.LivePeriod, Heading: content.Heading,
				ProximityNotificationRadius: content.ProximityAlertRadius, ReplyMarkup: markup}, nil
		}
	}
	if result.Type == "game" {
		return &tg.InputBotInlineMessageGame{ReplyMarkup: markup}, nil
	}
	caption, entities, err := inlineMessageEntities(result.Caption, result.ParseMode, result.CaptionEntities)
	if err != nil {
		return nil, err
	}
	if result.Type == "article" {
		if caption == "" {
			caption = result.Title
		}
		return &tg.InputBotInlineMessageText{Message: caption, Entities: entities, ReplyMarkup: markup}, nil
	}
	return &tg.InputBotInlineMessageMediaAuto{Message: caption, Entities: entities, ReplyMarkup: markup}, nil
}

func cachedInlineFileID(result inlineResultJSON) string {
	for _, value := range []string{result.PhotoFileID, result.GIFFileID, result.MPEG4FileID, result.VideoFileID,
		result.AudioFileID, result.VoiceFileID, result.DocumentFileID, result.StickerFileID} {
		if value != "" {
			return value
		}
	}
	return ""
}

func buildInlineResult(raw json.RawMessage) (tg.InputBotInlineResultClass, error) {
	var result inlineResultJSON
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	if result.ID == "" || result.Type == "" {
		return nil, fmt.Errorf("inline result type and id are required")
	}
	message, err := buildInlineMessage(result)
	if err != nil {
		return nil, err
	}
	if result.Type == "game" {
		return &tg.InputBotInlineResultGame{ID: result.ID, ShortName: result.GameShortName, SendMessage: message}, nil
	}
	if encoded := cachedInlineFileID(result); encoded != "" {
		decoded, err := fileid.DecodeFileID(encoded)
		if err != nil {
			return nil, fmt.Errorf("invalid cached file_id: %w", err)
		}
		if result.PhotoFileID != "" && (decoded.Type == fileid.Photo || decoded.Type == fileid.Thumbnail) {
			return &tg.InputBotInlineResultPhoto{ID: result.ID, Type: result.Type, Photo: &tg.InputPhoto{
				ID: decoded.ID, AccessHash: decoded.AccessHash, FileReference: decoded.FileReference}, SendMessage: message}, nil
		}
		return &tg.InputBotInlineResultDocument{ID: result.ID, Type: result.Type, Title: result.Title,
			Description: result.Description, Document: &tg.InputDocument{ID: decoded.ID, AccessHash: decoded.AccessHash,
				FileReference: decoded.FileReference}, SendMessage: message}, nil
	}
	value := &tg.InputBotInlineResult{ID: result.ID, Type: result.Type, Title: result.Title,
		Description: result.Description, URL: result.URL, SendMessage: message}
	mediaURL, mimeType := "", result.MimeType
	switch result.Type {
	case "photo":
		mediaURL, mimeType = result.PhotoURL, "image/jpeg"
	case "gif":
		mediaURL, mimeType = result.GIFURL, "image/gif"
	case "mpeg4_gif":
		mediaURL, mimeType = result.MPEG4URL, "video/mp4"
	case "video":
		mediaURL = result.VideoURL
	case "audio":
		mediaURL = result.AudioURL
	case "voice":
		mediaURL = result.VoiceURL
	case "document":
		mediaURL = result.DocumentURL
	}
	if mediaURL != "" {
		attributes := []tg.DocumentAttributeClass{}
		if result.Width != 0 || result.Height != 0 {
			attributes = append(attributes, &tg.DocumentAttributeImageSize{W: result.Width, H: result.Height})
		}
		value.SetContent(tg.InputWebDocument{URL: mediaURL, MimeType: mimeType, Attributes: attributes})
	}
	if result.ThumbnailURL != "" {
		value.SetThumb(tg.InputWebDocument{URL: result.ThumbnailURL, MimeType: "image/jpeg", Attributes: []tg.DocumentAttributeClass{}})
	}
	return value, nil
}

func buildInlineResults(raw json.RawMessage) ([]tg.InputBotInlineResultClass, bool, error) {
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, false, fmt.Errorf("invalid results: %w", err)
	}
	results := make([]tg.InputBotInlineResultClass, 0, len(values))
	gallery := len(values) != 0
	for _, value := range values {
		result, err := buildInlineResult(value)
		if err != nil {
			return nil, false, err
		}
		results = append(results, result)
		var header struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(value, &header)
		if header.Type != "photo" && header.Type != "gif" && header.Type != "mpeg4_gif" && header.Type != "video" {
			gallery = false
		}
	}
	return results, gallery, nil
}

func encodeInlineMessageID(value tg.InputBotInlineMessageIDClass) (string, error) {
	buffer := &bin.Buffer{}
	switch id := value.(type) {
	case *tg.InputBotInlineMessageID:
		if err := id.EncodeBare(buffer); err != nil {
			return "", err
		}
	case *tg.InputBotInlineMessageID64:
		if err := id.EncodeBare(buffer); err != nil {
			return "", err
		}
	case nil:
		return "", nil
	default:
		return "", fmt.Errorf("unexpected inline message id type %T", value)
	}
	return base64.RawURLEncoding.EncodeToString(buffer.Buf), nil
}

func decodeInlineMessageID(encoded string) (tg.InputBotInlineMessageIDClass, error) {
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("invalid inline_message_id: %w", err)
	}
	buffer := &bin.Buffer{Buf: data}
	switch len(data) {
	case 20:
		value := &tg.InputBotInlineMessageID{}
		if err := value.DecodeBare(buffer); err != nil {
			return nil, err
		}
		return value, nil
	case 24:
		value := &tg.InputBotInlineMessageID64{}
		if err := value.DecodeBare(buffer); err != nil {
			return nil, err
		}
		return value, nil
	default:
		return nil, fmt.Errorf("invalid inline_message_id length")
	}
}

func (b *BotInstance) sendMediaMessage(ctx context.Context, peer tg.InputPeerClass, connectionID string,
	media tg.InputMediaClass, message string, entities []tg.MessageEntityClass, silent, protect bool,
	threadID int, reply *converter.ReplyParameters, markupRaw json.RawMessage) (tg.UpdatesClass, error) {
	randomID, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	request := &tg.MessagesSendMediaRequest{Peer: peer, Media: media, Message: message, Entities: entities,
		RandomID: randomID.Int64(), Silent: silent, Noforwards: protect}
	if threadID != 0 || reply != nil {
		replyTo := &tg.InputReplyToMessage{}
		if threadID != 0 {
			replyTo.SetTopMsgID(threadID)
		}
		if reply != nil {
			replyTo.ReplyToMsgID = int(reply.MessageID)
		}
		request.SetReplyTo(replyTo)
	}
	if len(markupRaw) != 0 {
		markup, err := converter.ParseReplyMarkup(markupRaw)
		if err != nil {
			return nil, err
		}
		request.SetReplyMarkup(markup)
	}
	if connectionID != "" {
		var box tg.UpdatesBox
		if err := b.invokeBusiness(ctx, connectionID, request, &box); err != nil {
			return nil, err
		}
		return box.Updates, nil
	}
	return b.raw.MessagesSendMedia(ctx, request)
}

func (b *BotInstance) editMessageMedia(ctx context.Context, connectionID string, peer tg.InputPeerClass, messageID int64,
	inlineMessageID string, media tg.InputMediaClass, markupRaw json.RawMessage) error {
	var markup tg.ReplyMarkupClass
	var err error
	if len(markupRaw) != 0 {
		markup, err = converter.ParseReplyMarkup(markupRaw)
		if err != nil {
			return err
		}
	}
	if inlineMessageID != "" {
		id, err := decodeInlineMessageID(inlineMessageID)
		if err != nil {
			return err
		}
		request := &tg.MessagesEditInlineBotMessageRequest{ID: id}
		request.SetMedia(media)
		if markup != nil {
			request.SetReplyMarkup(markup)
		}
		_, err = b.raw.MessagesEditInlineBotMessage(ctx, request)
		return err
	}
	request := &tg.MessagesEditMessageRequest{Peer: peer, ID: int(messageID)}
	request.SetMedia(media)
	if markup != nil {
		request.SetReplyMarkup(markup)
	}
	if connectionID != "" {
		var box tg.UpdatesBox
		return b.invokeBusiness(ctx, connectionID, request, &box)
	}
	_, err = b.raw.MessagesEditMessage(ctx, request)
	return err
}

type invoiceParameters struct {
	Title, Description, Payload, ProviderToken, Currency, StartParameter, ProviderData, PhotoURL string
	Prices                                                                                       json.RawMessage
	SubscriptionPeriod, MaxTipAmount, PhotoSize, PhotoWidth, PhotoHeight                         int
	SuggestedTipAmounts                                                                          []int
	NeedName, NeedPhoneNumber, NeedEmail, NeedShippingAddress                                    bool
	SendPhoneNumberToProvider, SendEmailToProvider, IsFlexible                                   bool
}

func buildInvoiceMedia(value invoiceParameters) (*tg.InputMediaInvoice, error) {
	var rawPrices []struct {
		Label  string `json:"label"`
		Amount int64  `json:"amount"`
	}
	if err := json.Unmarshal(value.Prices, &rawPrices); err != nil {
		return nil, fmt.Errorf("invalid prices: %w", err)
	}
	prices := make([]tg.LabeledPrice, 0, len(rawPrices))
	for _, price := range rawPrices {
		prices = append(prices, tg.LabeledPrice{Label: price.Label, Amount: price.Amount})
	}
	tips := make([]int64, len(value.SuggestedTipAmounts))
	for index, amount := range value.SuggestedTipAmounts {
		tips[index] = int64(amount)
	}
	invoice := tg.Invoice{Currency: value.Currency, Prices: prices, NameRequested: value.NeedName,
		PhoneRequested: value.NeedPhoneNumber, EmailRequested: value.NeedEmail,
		ShippingAddressRequested: value.NeedShippingAddress, Flexible: value.IsFlexible,
		PhoneToProvider: value.SendPhoneNumberToProvider, EmailToProvider: value.SendEmailToProvider}
	if value.MaxTipAmount != 0 {
		invoice.SetMaxTipAmount(int64(value.MaxTipAmount))
	}
	if len(tips) != 0 {
		invoice.SetSuggestedTipAmounts(tips)
	}
	if value.SubscriptionPeriod != 0 {
		invoice.SetSubscriptionPeriod(value.SubscriptionPeriod)
	}
	providerData := value.ProviderData
	if providerData == "" {
		providerData = "{}"
	}
	media := &tg.InputMediaInvoice{Title: value.Title, Description: value.Description, Invoice: invoice,
		Payload: []byte(value.Payload), ProviderData: tg.DataJSON{Data: providerData}}
	if value.ProviderToken != "" {
		media.SetProvider(value.ProviderToken)
	}
	if value.StartParameter != "" {
		media.SetStartParam(value.StartParameter)
	}
	if value.PhotoURL != "" {
		attributes := []tg.DocumentAttributeClass{}
		if value.PhotoWidth != 0 || value.PhotoHeight != 0 {
			attributes = append(attributes,
				&tg.DocumentAttributeImageSize{W: value.PhotoWidth, H: value.PhotoHeight})
		}
		media.SetPhoto(tg.InputWebDocument{URL: value.PhotoURL, Size: value.PhotoSize, MimeType: "image/jpeg", Attributes: attributes})
	}
	return media, nil
}

// ------------------------------------------------------------------------------------------------
// Chat & Administration Methods
// ------------------------------------------------------------------------------------------------

// GetChat returns up to date information about the chat.
func (b *BotInstance) GetChat(ctx context.Context, chatID int64) (*converter.ChatFullInfo, error) {
	peer, err := b.resolvePeer(chatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}
	switch cp := peer.(type) {
	case *tg.InputPeerChannel:
		res, err := b.raw.ChannelsGetFullChannel(ctx, &tg.InputChannel{ChannelID: cp.ChannelID, AccessHash: cp.AccessHash})
		if err != nil {
			return nil, err
		}
		info := &converter.ChatFullInfo{ID: chatID, Type: "channel"}
		for _, value := range res.Chats {
			if chat, ok := value.(*tg.Channel); ok && chat.ID == cp.ChannelID {
				info.Title, info.Username = chat.Title, chat.Username
				if chat.Megagroup {
					info.Type = "supergroup"
				}
				break
			}
		}
		if full, ok := res.FullChat.(*tg.ChannelFull); ok {
			info.Description = full.About
		}
		return info, nil
	case *tg.InputPeerUser:
		res, err := b.raw.UsersGetFullUser(ctx, &tg.InputUser{UserID: cp.UserID, AccessHash: cp.AccessHash})
		if err != nil {
			return nil, err
		}
		info := &converter.ChatFullInfo{ID: chatID, Type: "private", Bio: res.FullUser.About}
		for _, value := range res.Users {
			if user, ok := value.(*tg.User); ok && user.ID == cp.UserID {
				info.FirstName, info.LastName, info.Username = user.FirstName, user.LastName, user.Username
				break
			}
		}
		return info, nil
	case *tg.InputPeerChat:
		res, err := b.raw.MessagesGetFullChat(ctx, cp.ChatID)
		if err != nil {
			return nil, err
		}
		info := &converter.ChatFullInfo{ID: chatID, Type: "group"}
		for _, value := range res.Chats {
			if chat, ok := value.(*tg.Chat); ok && chat.ID == cp.ChatID {
				info.Title = chat.Title
				break
			}
		}
		if full, ok := res.FullChat.(*tg.ChatFull); ok {
			info.Description = full.About
		}
		return info, nil
	default:
		return nil, fmt.Errorf("unsupported chat peer %T", peer)
	}
}

// ForwardMessage forwards a message from one chat to another.
func (b *BotInstance) ForwardMessage(ctx context.Context, req *converter.ForwardMessageRequest) (*converter.Message, error) {
	toPeer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve to peer: %w", err)
	}
	fromPeer, err := b.resolvePeer(req.FromChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve from peer: %w", err)
	}

	randomID, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	sendReq := &tg.MessagesForwardMessagesRequest{
		ToPeer:     toPeer,
		FromPeer:   fromPeer,
		ID:         []int{int(req.MessageID)},
		RandomID:   []int64{randomID.Int64()},
		Silent:     req.DisableNotification,
		Noforwards: req.ProtectContent,
	}

	updates, err := b.raw.MessagesForwardMessages(ctx, sendReq)
	if err != nil {
		return nil, fmt.Errorf("mtproto forward message: %w", err)
	}

	now := int(time.Now().Unix())
	switch u := updates.(type) {
	case *tg.Updates:
		for _, upd := range u.Updates {
			if msgUpd, ok := upd.(*tg.UpdateNewMessage); ok {
				if m, ok := msgUpd.Message.(*tg.Message); ok {
					return &converter.Message{
						MessageID: int64(m.ID),
						From:      b.GetMe(),
						Chat:      converter.Chat{ID: req.ChatID},
						Date:      now,
					}, nil
				}
			}
		}
	}

	messageID := extractSentMessageID(updates)
	if messageID == 0 {
		return nil, fmt.Errorf("forward succeeded without a message id")
	}
	return &converter.Message{MessageID: messageID, From: b.GetMe(), Chat: converter.Chat{ID: req.ChatID}, Date: now}, nil
}

// ForwardMessages forwards multiple messages.
func (b *BotInstance) ForwardMessages(ctx context.Context, req *converter.ForwardMessagesRequest) ([]int64, error) {
	toPeer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve to peer: %w", err)
	}
	fromPeer, err := b.resolvePeer(req.FromChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve from peer: %w", err)
	}

	var ids []int
	var randomIDs []int64
	for _, id := range req.MessageIDs {
		ids = append(ids, int(id))
		rnd, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
		randomIDs = append(randomIDs, rnd.Int64())
	}

	sendReq := &tg.MessagesForwardMessagesRequest{
		ToPeer:     toPeer,
		FromPeer:   fromPeer,
		ID:         ids,
		RandomID:   randomIDs,
		Silent:     req.DisableNotification,
		Noforwards: req.ProtectContent,
	}

	updates, err := b.raw.MessagesForwardMessages(ctx, sendReq)
	if err != nil {
		return nil, fmt.Errorf("mtproto forward messages: %w", err)
	}

	var result []int64
	switch u := updates.(type) {
	case *tg.Updates:
		for _, upd := range u.Updates {
			if msgUpd, ok := upd.(*tg.UpdateNewMessage); ok {
				if m, ok := msgUpd.Message.(*tg.Message); ok {
					result = append(result, int64(m.ID))
				}
			}
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("forward succeeded without message ids")
	}
	return result, nil
}

// CopyMessages copies multiple messages.
func (b *BotInstance) CopyMessages(ctx context.Context, req *converter.CopyMessagesRequest) ([]int64, error) {
	toPeer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve to peer: %w", err)
	}
	fromPeer, err := b.resolvePeer(req.FromChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve from peer: %w", err)
	}

	var ids []int
	var randomIDs []int64
	for _, id := range req.MessageIDs {
		ids = append(ids, int(id))
		rnd, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
		randomIDs = append(randomIDs, rnd.Int64())
	}

	sendReq := &tg.MessagesForwardMessagesRequest{
		ToPeer:     toPeer,
		FromPeer:   fromPeer,
		ID:         ids,
		RandomID:   randomIDs,
		DropAuthor: true,
		Silent:     req.DisableNotification,
		Noforwards: req.ProtectContent,
	}

	updates, err := b.raw.MessagesForwardMessages(ctx, sendReq)
	if err != nil {
		return nil, fmt.Errorf("mtproto copy messages: %w", err)
	}

	var result []int64
	switch u := updates.(type) {
	case *tg.Updates:
		for _, upd := range u.Updates {
			if msgUpd, ok := upd.(*tg.UpdateNewMessage); ok {
				if m, ok := msgUpd.Message.(*tg.Message); ok {
					result = append(result, int64(m.ID))
				}
			}
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("copy succeeded without message ids")
	}
	return result, nil
}

// DeleteMessages deletes multiple messages.
func (b *BotInstance) DeleteMessages(ctx context.Context, chatID int64, messageIDs []int64) (bool, error) {
	peer, err := b.resolvePeer(chatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}

	var ids []int
	for _, id := range messageIDs {
		ids = append(ids, int(id))
	}

	if cp, ok := peer.(*tg.InputPeerChannel); ok {
		_, err := b.raw.ChannelsDeleteMessages(ctx, &tg.ChannelsDeleteMessagesRequest{
			Channel: &tg.InputChannel{ChannelID: cp.ChannelID, AccessHash: cp.AccessHash},
			ID:      ids,
		})
		if err != nil {
			return false, fmt.Errorf("mtproto channels delete messages: %w", err)
		}
	} else {
		_, err := b.raw.MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{
			Revoke: true,
			ID:     ids,
		})
		if err != nil {
			return false, fmt.Errorf("mtproto messages delete messages: %w", err)
		}
	}
	return true, nil
}

// SetMessageReaction sets reaction on a message.
func (b *BotInstance) SetMessageReaction(ctx context.Context, req *converter.SetMessageReactionRequest) (bool, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}

	var reactions []tg.ReactionClass
	for _, r := range req.Reaction {
		if r.Type == "custom_emoji" && r.CustomEmojiID != "" {
			id, err := strconv.ParseInt(r.CustomEmojiID, 10, 64)
			if err != nil {
				return false, err
			}
			reactions = append(reactions, &tg.ReactionCustomEmoji{DocumentID: id})
		} else if r.Type == "paid" {
			reactions = append(reactions, &tg.ReactionPaid{})
		} else if r.Emoji != "" {
			reactions = append(reactions, &tg.ReactionEmoji{Emoticon: r.Emoji})
		}
	}

	_, err = b.raw.MessagesSendReaction(ctx, &tg.MessagesSendReactionRequest{
		Peer:     peer,
		MsgID:    int(req.MessageID),
		Reaction: reactions,
		Big:      req.IsBig,
	})
	return err == nil, err
}

// SendLocation sends a point on the map.
func (b *BotInstance) SendLocation(ctx context.Context, req *converter.SendLocationRequest) (*converter.Message, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}

	var media tg.InputMediaClass
	if req.LivePeriod > 0 {
		media = &tg.InputMediaGeoLive{
			GeoPoint: &tg.InputGeoPoint{
				Lat:            req.Latitude,
				Long:           req.Longitude,
				AccuracyRadius: int(req.HorizontalAccuracy),
			},
			Period:  req.LivePeriod,
			Heading: req.Heading,
		}
	} else {
		media = &tg.InputMediaGeoPoint{
			GeoPoint: &tg.InputGeoPoint{
				Lat:            req.Latitude,
				Long:           req.Longitude,
				AccuracyRadius: int(req.HorizontalAccuracy),
			},
		}
	}

	updates, err := b.sendMediaMessage(ctx, peer, req.BusinessConnectionID, media, "", nil,
		req.DisableNotification, req.ProtectContent, req.MessageThreadID, req.ReplyParameters, req.ReplyMarkup)
	if err != nil {
		return nil, err
	}
	return &converter.Message{MessageID: extractSentMessageID(updates), From: b.GetMe(), Chat: converter.Chat{ID: req.ChatID},
		Date: int(time.Now().Unix()), BusinessConnectionID: req.BusinessConnectionID}, nil
}

// EditMessageLiveLocation edits a live location message.
func (b *BotInstance) EditMessageLiveLocation(ctx context.Context, req *converter.EditMessageLiveLocationRequest) (*converter.Message, error) {
	var peer tg.InputPeerClass
	var err error
	if req.InlineMessageID == "" {
		peer, err = b.resolvePeer(req.ChatID)
		if err != nil {
			return nil, err
		}
	}
	media := &tg.InputMediaGeoLive{GeoPoint: &tg.InputGeoPoint{Lat: req.Latitude, Long: req.Longitude,
		AccuracyRadius: int(req.HorizontalAccuracy)}, Heading: req.Heading,
		ProximityNotificationRadius: req.ProximityAlertRadius}
	if err := b.editMessageMedia(ctx, req.BusinessConnectionID, peer, req.MessageID, req.InlineMessageID, media, req.ReplyMarkup); err != nil {
		return nil, err
	}
	return &converter.Message{MessageID: req.MessageID, From: b.GetMe(), Chat: converter.Chat{ID: req.ChatID},
		Date: int(time.Now().Unix()), BusinessConnectionID: req.BusinessConnectionID}, nil
}

// StopMessageLiveLocation stops updating a live location message.
func (b *BotInstance) StopMessageLiveLocation(ctx context.Context, req *converter.StopMessageLiveLocationRequest) (*converter.Message, error) {
	var peer tg.InputPeerClass
	var err error
	if req.InlineMessageID == "" {
		peer, err = b.resolvePeer(req.ChatID)
		if err != nil {
			return nil, err
		}
	}
	media := &tg.InputMediaGeoLive{Stopped: true, GeoPoint: &tg.InputGeoPointEmpty{}}
	if err := b.editMessageMedia(ctx, req.BusinessConnectionID, peer, req.MessageID, req.InlineMessageID, media, req.ReplyMarkup); err != nil {
		return nil, err
	}
	return &converter.Message{MessageID: req.MessageID, From: b.GetMe(), Chat: converter.Chat{ID: req.ChatID},
		Date: int(time.Now().Unix()), BusinessConnectionID: req.BusinessConnectionID}, nil
}

// SendVenue sends information about a venue.
func (b *BotInstance) SendVenue(ctx context.Context, req *converter.SendVenueRequest) (*converter.Message, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}

	media := &tg.InputMediaVenue{
		GeoPoint: &tg.InputGeoPoint{
			Lat:  req.Latitude,
			Long: req.Longitude,
		},
		Title:     req.Title,
		Address:   req.Address,
		Provider:  "foursquare",
		VenueID:   req.FoursquareID,
		VenueType: req.FoursquareType,
	}

	if req.GooglePlaceID != "" {
		media.Provider, media.VenueID, media.VenueType = "gplaces", req.GooglePlaceID, req.GooglePlaceType
	}
	updates, err := b.sendMediaMessage(ctx, peer, req.BusinessConnectionID, media, "", nil, req.DisableNotification,
		req.ProtectContent, req.MessageThreadID, req.ReplyParameters, req.ReplyMarkup)
	if err != nil {
		return nil, err
	}
	return &converter.Message{MessageID: extractSentMessageID(updates), From: b.GetMe(), Chat: converter.Chat{ID: req.ChatID},
		Date: int(time.Now().Unix()), BusinessConnectionID: req.BusinessConnectionID}, nil
}

// SendContact sends phone contacts.
func (b *BotInstance) SendContact(ctx context.Context, req *converter.SendContactRequest) (*converter.Message, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}

	media := &tg.InputMediaContact{
		PhoneNumber: req.PhoneNumber,
		FirstName:   req.FirstName,
		LastName:    req.LastName,
		Vcard:       req.VCard,
	}

	updates, err := b.sendMediaMessage(ctx, peer, req.BusinessConnectionID, media, "", nil, req.DisableNotification,
		req.ProtectContent, req.MessageThreadID, req.ReplyParameters, req.ReplyMarkup)
	if err != nil {
		return nil, err
	}
	return &converter.Message{MessageID: extractSentMessageID(updates), From: b.GetMe(), Chat: converter.Chat{ID: req.ChatID},
		Date: int(time.Now().Unix()), BusinessConnectionID: req.BusinessConnectionID}, nil
}

// SendPoll sends a native poll.
func (b *BotInstance) SendPoll(ctx context.Context, req *converter.SendPollRequest) (*converter.Message, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, err
	}
	question, questionEntities, err := inlineMessageEntities(req.Question, req.QuestionParseMode, nil)
	if err != nil {
		return nil, err
	}
	var rawOptions []json.RawMessage
	if err := json.Unmarshal(req.Options, &rawOptions); err != nil {
		return nil, err
	}
	answers := make([]tg.PollAnswerClass, 0, len(rawOptions))
	for index, raw := range rawOptions {
		var text string
		var option struct {
			Text          string                    `json:"text"`
			TextParseMode string                    `json:"text_parse_mode"`
			TextEntities  []converter.MessageEntity `json:"text_entities"`
		}
		if err := json.Unmarshal(raw, &text); err != nil {
			if err := json.Unmarshal(raw, &option); err != nil {
				return nil, err
			}
			text = option.Text
		}
		clean, entities, err := inlineMessageEntities(text, option.TextParseMode, option.TextEntities)
		if err != nil {
			return nil, err
		}
		answers = append(answers, &tg.PollAnswer{Text: tg.TextWithEntities{Text: clean, Entities: entities}, Option: []byte{byte(index)}})
	}
	anonymous := true
	if req.IsAnonymous != nil {
		anonymous = *req.IsAnonymous
	}
	poll := tg.Poll{PublicVoters: !anonymous, MultipleChoice: req.AllowsMultipleAnswers, Quiz: req.Type == "quiz",
		Closed: req.IsClosed, Question: tg.TextWithEntities{Text: question, Entities: questionEntities}, Answers: answers,
		ClosePeriod: req.OpenPeriod, CloseDate: req.CloseDate}
	media := &tg.InputMediaPoll{Poll: poll}
	if req.Type == "quiz" {
		media.SetCorrectAnswers([]int{req.CorrectOptionID})
	}
	if req.Explanation != "" {
		clean, entities, err := inlineMessageEntities(req.Explanation, req.ExplanationParseMode, nil)
		if err != nil {
			return nil, err
		}
		media.SetSolution(clean)
		media.SetSolutionEntities(entities)
	}
	updates, err := b.sendMediaMessage(ctx, peer, req.BusinessConnectionID, media, "", nil, req.DisableNotification,
		req.ProtectContent, req.MessageThreadID, req.ReplyParameters, req.ReplyMarkup)
	if err != nil {
		return nil, err
	}
	return &converter.Message{MessageID: extractSentMessageID(updates), From: b.GetMe(), Chat: converter.Chat{ID: req.ChatID},
		Date: int(time.Now().Unix()), BusinessConnectionID: req.BusinessConnectionID}, nil
}

// StopPoll stops a poll.
func (b *BotInstance) StopPoll(ctx context.Context, req *converter.StopPollRequest) (*converter.Poll, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, err
	}
	if req.BusinessConnectionID != "" {
		// Business messages can't be fetched through the bot's ordinary message
		// history. Telegram accepts an empty closed poll as the edit marker and
		// returns the complete stopped poll in the business edit update.
		poll := tg.Poll{Closed: true, Question: tg.TextWithEntities{}, Answers: []tg.PollAnswerClass{}}
		edit := &tg.MessagesEditMessageRequest{Peer: peer, ID: int(req.MessageID)}
		edit.SetMedia(&tg.InputMediaPoll{Poll: poll})
		if len(req.ReplyMarkup) != 0 {
			markup, err := converter.ParseReplyMarkup(req.ReplyMarkup)
			if err != nil {
				return nil, err
			}
			edit.SetReplyMarkup(markup)
		}
		var box tg.UpdatesBox
		if err := b.invokeBusiness(ctx, req.BusinessConnectionID, edit, &box); err != nil {
			return nil, err
		}
		if stopped := extractPollFromUpdates(box.Updates); stopped != nil {
			return stopped, nil
		}
		return nil, fmt.Errorf("business stop poll response did not contain a poll")
	}

	ids := []tg.InputMessageClass{&tg.InputMessageID{ID: int(req.MessageID)}}
	var history tg.MessagesMessagesClass
	if channelPeer, ok := peer.(*tg.InputPeerChannel); ok {
		history, err = b.raw.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{Channel: &tg.InputChannel{ChannelID: channelPeer.ChannelID, AccessHash: channelPeer.AccessHash}, ID: ids})
	} else {
		history, err = b.raw.MessagesGetMessages(ctx, ids)
	}
	if err != nil {
		return nil, err
	}
	var messages []tg.MessageClass
	switch value := history.(type) {
	case *tg.MessagesMessages:
		messages = value.Messages
	case *tg.MessagesMessagesSlice:
		messages = value.Messages
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("poll message not found")
	}
	message, ok := messages[0].(*tg.Message)
	if !ok {
		return nil, fmt.Errorf("message is not a poll")
	}
	pollMedia, ok := message.Media.(*tg.MessageMediaPoll)
	if !ok {
		return nil, fmt.Errorf("message is not a poll")
	}
	poll := pollMedia.Poll
	poll.Closed = true
	inputMedia := &tg.InputMediaPoll{Poll: poll}
	edit := &tg.MessagesEditMessageRequest{Peer: peer, ID: int(req.MessageID)}
	edit.SetMedia(inputMedia)
	if len(req.ReplyMarkup) != 0 {
		markup, err := converter.ParseReplyMarkup(req.ReplyMarkup)
		if err != nil {
			return nil, err
		}
		edit.SetReplyMarkup(markup)
	}
	_, err = b.raw.MessagesEditMessage(ctx, edit)
	if err != nil {
		return nil, err
	}
	return converter.ConvertPoll(poll, pollMedia.Results), nil
}

// SendDice sends an animated emoji that displays a random value.
func (b *BotInstance) SendDice(ctx context.Context, req *converter.SendDiceRequest) (*converter.Message, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}

	emoji := req.Emoji
	if emoji == "" {
		emoji = "🎲"
	}

	media := &tg.InputMediaDice{
		Emoticon: emoji,
	}

	updates, err := b.sendMediaMessage(ctx, peer, req.BusinessConnectionID, media, "", nil, req.DisableNotification,
		req.ProtectContent, req.MessageThreadID, req.ReplyParameters, req.ReplyMarkup)
	if err != nil {
		return nil, err
	}
	return &converter.Message{MessageID: extractSentMessageID(updates), From: b.GetMe(), Chat: converter.Chat{ID: req.ChatID},
		Date: int(time.Now().Unix()), BusinessConnectionID: req.BusinessConnectionID}, nil
}

// PinChatMessage pins a message in a chat.
func (b *BotInstance) PinChatMessage(ctx context.Context, req *converter.PinChatMessageRequest) (bool, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}

	pinReq := &tg.MessagesUpdatePinnedMessageRequest{
		Peer:   peer,
		ID:     int(req.MessageID),
		Silent: req.DisableNotification,
	}

	if req.BusinessConnectionID != "" {
		var res tg.UpdatesBox
		err = b.invokeBusiness(ctx, req.BusinessConnectionID, pinReq, &res)
	} else {
		_, err = b.raw.MessagesUpdatePinnedMessage(ctx, pinReq)
	}
	return err == nil, err
}

// UnpinChatMessage unpins a message in a chat.
func (b *BotInstance) UnpinChatMessage(ctx context.Context, req *converter.UnpinChatMessageRequest) (bool, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}

	unpinReq := &tg.MessagesUpdatePinnedMessageRequest{
		Peer:  peer,
		ID:    int(req.MessageID),
		Unpin: true,
	}

	if req.BusinessConnectionID != "" {
		var res tg.UpdatesBox
		err = b.invokeBusiness(ctx, req.BusinessConnectionID, unpinReq, &res)
	} else {
		_, err = b.raw.MessagesUpdatePinnedMessage(ctx, unpinReq)
	}
	return err == nil, err
}

// UnpinAllChatMessages clears the list of pinned messages in a chat.
func (b *BotInstance) UnpinAllChatMessages(ctx context.Context, chatID int64) (bool, error) {
	peer, err := b.resolvePeer(chatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}

	_, err = b.raw.MessagesUnpinAllMessages(ctx, &tg.MessagesUnpinAllMessagesRequest{
		Peer: peer,
	})
	return err == nil, err
}

// LeaveChat removes bot from a chat.
func (b *BotInstance) LeaveChat(ctx context.Context, chatID int64) (bool, error) {
	peer, err := b.resolvePeer(chatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}

	if cp, ok := peer.(*tg.InputPeerChannel); ok {
		_, err = b.raw.ChannelsLeaveChannel(ctx, &tg.InputChannel{ChannelID: cp.ChannelID, AccessHash: cp.AccessHash})
		if err != nil {
			return false, err
		}
	} else if cp, ok := peer.(*tg.InputPeerChat); ok {
		_, err = b.raw.MessagesDeleteChatUser(ctx, &tg.MessagesDeleteChatUserRequest{
			ChatID: cp.ChatID,
			UserID: &tg.InputUserSelf{},
		})
		if err != nil {
			return false, err
		}
	} else {
		return false, fmt.Errorf("can't leave a private chat")
	}
	return true, nil
}

// GetChatAdministrators returns a list of administrators in a chat.
func (b *BotInstance) GetChatAdministrators(ctx context.Context, chatID int64) ([]*converter.ChatMember, error) {
	peer, err := b.resolvePeer(chatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}
	switch cp := peer.(type) {
	case *tg.InputPeerChannel:
		res, err := b.raw.ChannelsGetParticipants(ctx, &tg.ChannelsGetParticipantsRequest{
			Channel: &tg.InputChannel{ChannelID: cp.ChannelID, AccessHash: cp.AccessHash},
			Filter:  &tg.ChannelParticipantsAdmins{}, Limit: 200,
		})
		if err != nil {
			return nil, err
		}
		participants, ok := res.(*tg.ChannelsChannelParticipants)
		if !ok {
			return nil, fmt.Errorf("unexpected participants response %T", res)
		}
		entities := converter.NewEntityContext(participants.Users, participants.Chats)
		list := make([]*converter.ChatMember, 0, len(participants.Participants))
		for _, participant := range participants.Participants {
			member := converter.ConvertChannelParticipant(participant, 0, entities)
			if member.Status == "creator" || member.Status == "administrator" {
				copy := member
				list = append(list, &copy)
			}
		}
		return list, nil
	case *tg.InputPeerChat:
		full, err := b.raw.MessagesGetFullChat(ctx, cp.ChatID)
		if err != nil {
			return nil, err
		}
		chatFull, ok := full.FullChat.(*tg.ChatFull)
		if !ok {
			return nil, fmt.Errorf("unexpected full chat response %T", full.FullChat)
		}
		participants, ok := chatFull.Participants.(*tg.ChatParticipants)
		if !ok {
			return []*converter.ChatMember{}, nil
		}
		entities := converter.NewEntityContext(full.Users, full.Chats)
		list := make([]*converter.ChatMember, 0)
		for _, participant := range participants.Participants {
			member := converter.ConvertBasicParticipant(participant, 0, entities)
			if member.Status == "creator" || member.Status == "administrator" {
				copy := member
				list = append(list, &copy)
			}
		}
		return list, nil
	default:
		return nil, fmt.Errorf("chat administrators are unavailable for private chats")
	}
}

// GetChatMemberCount returns the number of members in a chat.
func (b *BotInstance) GetChatMemberCount(ctx context.Context, chatID int64) (int, error) {
	peer, err := b.resolvePeer(chatID)
	if err != nil {
		return 0, fmt.Errorf("resolve peer: %w", err)
	}
	switch cp := peer.(type) {
	case *tg.InputPeerChannel:
		res, err := b.raw.ChannelsGetFullChannel(ctx, &tg.InputChannel{ChannelID: cp.ChannelID, AccessHash: cp.AccessHash})
		if err != nil {
			return 0, err
		}
		full, ok := res.FullChat.(*tg.ChannelFull)
		if !ok {
			return 0, fmt.Errorf("unexpected full channel response %T", res.FullChat)
		}
		return full.ParticipantsCount, nil
	case *tg.InputPeerChat:
		res, err := b.raw.MessagesGetFullChat(ctx, cp.ChatID)
		if err != nil {
			return 0, err
		}
		full, ok := res.FullChat.(*tg.ChatFull)
		if !ok {
			return 0, fmt.Errorf("unexpected full chat response %T", res.FullChat)
		}
		participants, ok := full.Participants.(*tg.ChatParticipants)
		if !ok {
			return 0, nil
		}
		return len(participants.Participants), nil
	default:
		return 1, nil
	}
}

// BanChatMember bans a user from a chat.
func (b *BotInstance) BanChatMember(ctx context.Context, req *converter.BanChatMemberRequest) (bool, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}

	if cp, ok := peer.(*tg.InputPeerChannel); ok {
		userPeer, _ := b.resolvePeer(req.UserID)
		var inputUser tg.InputUserClass
		if up, ok := userPeer.(*tg.InputPeerUser); ok {
			inputUser = &tg.InputUser{UserID: up.UserID, AccessHash: up.AccessHash}
		} else {
			inputUser = &tg.InputUser{UserID: req.UserID}
		}
		_, err = b.raw.ChannelsEditBanned(ctx, &tg.ChannelsEditBannedRequest{
			Channel:     &tg.InputChannel{ChannelID: cp.ChannelID, AccessHash: cp.AccessHash},
			Participant: &tg.InputPeerUser{UserID: req.UserID},
			BannedRights: tg.ChatBannedRights{
				ViewMessages: true,
				UntilDate:    req.UntilDate,
			},
		})
		_ = inputUser
		return err == nil, err
	}
	return false, fmt.Errorf("banChatMember is only supported for supergroups and channels")
}

// UnbanChatMember unbans a user in a chat.
func (b *BotInstance) UnbanChatMember(ctx context.Context, req *converter.UnbanChatMemberRequest) (bool, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}

	if cp, ok := peer.(*tg.InputPeerChannel); ok {
		_, err = b.raw.ChannelsEditBanned(ctx, &tg.ChannelsEditBannedRequest{
			Channel:      &tg.InputChannel{ChannelID: cp.ChannelID, AccessHash: cp.AccessHash},
			Participant:  &tg.InputPeerUser{UserID: req.UserID},
			BannedRights: tg.ChatBannedRights{},
		})
		return err == nil, err
	}
	return false, fmt.Errorf("unbanChatMember is only supported for supergroups and channels")
}

// RestrictChatMember restricts a user in a supergroup.
func (b *BotInstance) RestrictChatMember(ctx context.Context, req *converter.RestrictChatMemberRequest) (bool, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}

	if cp, ok := peer.(*tg.InputPeerChannel); ok {
		_, err = b.raw.ChannelsEditBanned(ctx, &tg.ChannelsEditBannedRequest{
			Channel:     &tg.InputChannel{ChannelID: cp.ChannelID, AccessHash: cp.AccessHash},
			Participant: &tg.InputPeerUser{UserID: req.UserID},
			BannedRights: tg.ChatBannedRights{
				SendMessages: !req.Permissions.CanSendMessages,
				SendMedia:    !req.Permissions.CanSendPhotos && !req.Permissions.CanSendVideos,
				SendPolls:    !req.Permissions.CanSendPolls,
				InviteUsers:  !req.Permissions.CanInviteUsers,
				PinMessages:  !req.Permissions.CanPinMessages,
				ChangeInfo:   !req.Permissions.CanChangeInfo,
				UntilDate:    req.UntilDate,
			},
		})
		return err == nil, err
	}
	return false, fmt.Errorf("restrictChatMember is only supported for supergroups")
}

// PromoteChatMember promotes or demotes a user in a chat.
func (b *BotInstance) PromoteChatMember(ctx context.Context, req *converter.PromoteChatMemberRequest) (bool, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}

	if cp, ok := peer.(*tg.InputPeerChannel); ok {
		_, err = b.raw.ChannelsEditAdmin(ctx, &tg.ChannelsEditAdminRequest{
			Channel: &tg.InputChannel{ChannelID: cp.ChannelID, AccessHash: cp.AccessHash},
			UserID:  &tg.InputUser{UserID: req.UserID},
			AdminRights: tg.ChatAdminRights{
				ChangeInfo:     req.CanChangeInfo,
				PostMessages:   req.CanPostMessages,
				EditMessages:   req.CanEditMessages,
				DeleteMessages: req.CanDeleteMessages,
				BanUsers:       req.CanRestrictMembers,
				InviteUsers:    req.CanInviteUsers,
				PinMessages:    req.CanPinMessages,
				AddAdmins:      req.CanPromoteMembers,
				ManageCall:     req.CanManageVideoChats,
				ManageTopics:   req.CanManageTopics,
				Anonymous:      req.IsAnonymous,
			},
			Rank: "",
		})
		return err == nil, err
	}
	return false, fmt.Errorf("promoteChatMember is only supported for supergroups and channels")
}

// SetChatAdministratorCustomTitle sets custom title for an admin.
func (b *BotInstance) SetChatAdministratorCustomTitle(ctx context.Context, req *converter.SetChatAdministratorCustomTitleRequest) (bool, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}

	if cp, ok := peer.(*tg.InputPeerChannel); ok {
		participant, err := b.raw.ChannelsGetParticipant(ctx, &tg.ChannelsGetParticipantRequest{
			Channel: &tg.InputChannel{ChannelID: cp.ChannelID, AccessHash: cp.AccessHash}, Participant: &tg.InputPeerUser{UserID: req.UserID},
		})
		if err != nil {
			return false, err
		}
		admin, ok := participant.Participant.(*tg.ChannelParticipantAdmin)
		if !ok {
			return false, fmt.Errorf("user is not an administrator")
		}
		_, err = b.raw.ChannelsEditAdmin(ctx, &tg.ChannelsEditAdminRequest{
			Channel:     &tg.InputChannel{ChannelID: cp.ChannelID, AccessHash: cp.AccessHash},
			UserID:      &tg.InputUser{UserID: req.UserID},
			AdminRights: admin.AdminRights,
			Rank:        req.CustomTitle,
		})
		return err == nil, err
	}
	return false, fmt.Errorf("custom administrator titles are only supported for supergroups and channels")
}

// BanChatSenderChat bans a channel chat in a supergroup.
func (b *BotInstance) BanChatSenderChat(ctx context.Context, chatID, senderChatID int64) (bool, error) {
	peer, err := b.resolvePeer(chatID)
	if err != nil {
		return false, fmt.Errorf("resolve chat: %w", err)
	}
	channel, err := inputChannel(peer)
	if err != nil {
		return false, err
	}
	sender, err := b.resolvePeer(senderChatID)
	if err != nil {
		return false, fmt.Errorf("resolve sender chat: %w", err)
	}
	_, err = b.raw.ChannelsEditBanned(ctx, &tg.ChannelsEditBannedRequest{
		Channel: channel, Participant: sender,
		BannedRights: tg.ChatBannedRights{ViewMessages: true, SendMessages: true, UntilDate: 0},
	})
	return err == nil, err
}

// UnbanChatSenderChat unbans a channel chat in a supergroup.
func (b *BotInstance) UnbanChatSenderChat(ctx context.Context, chatID, senderChatID int64) (bool, error) {
	peer, err := b.resolvePeer(chatID)
	if err != nil {
		return false, fmt.Errorf("resolve chat: %w", err)
	}
	channel, err := inputChannel(peer)
	if err != nil {
		return false, err
	}
	sender, err := b.resolvePeer(senderChatID)
	if err != nil {
		return false, fmt.Errorf("resolve sender chat: %w", err)
	}
	_, err = b.raw.ChannelsEditBanned(ctx, &tg.ChannelsEditBannedRequest{
		Channel: channel, Participant: sender, BannedRights: tg.ChatBannedRights{},
	})
	return err == nil, err
}

// SetChatPermissions sets default chat permissions for all members.
func (b *BotInstance) SetChatPermissions(ctx context.Context, req *converter.SetChatPermissionsRequest) (bool, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}

	_, err = b.raw.MessagesEditChatDefaultBannedRights(ctx, &tg.MessagesEditChatDefaultBannedRightsRequest{
		Peer: peer,
		BannedRights: tg.ChatBannedRights{
			SendMessages: !req.Permissions.CanSendMessages,
			SendMedia:    !req.Permissions.CanSendPhotos,
			SendPolls:    !req.Permissions.CanSendPolls,
			InviteUsers:  !req.Permissions.CanInviteUsers,
			PinMessages:  !req.Permissions.CanPinMessages,
			ChangeInfo:   !req.Permissions.CanChangeInfo,
		},
	})
	return err == nil, err
}

// ExportChatInviteLink generates a new primary invite link for a chat.
func (b *BotInstance) ExportChatInviteLink(ctx context.Context, chatID int64) (string, error) {
	peer, err := b.resolvePeer(chatID)
	if err != nil {
		return "", fmt.Errorf("resolve peer: %w", err)
	}

	res, err := b.raw.MessagesExportChatInvite(ctx, &tg.MessagesExportChatInviteRequest{
		Peer: peer, LegacyRevokePermanent: true,
	})
	if err != nil {
		return "", err
	}
	if inv, ok := res.(*tg.ChatInviteExported); ok {
		return inv.Link, nil
	}
	return "", fmt.Errorf("unexpected invite type %T", res)
}

// CreateChatInviteLink creates an additional invite link for a chat.
func (b *BotInstance) CreateChatInviteLink(ctx context.Context, req *converter.CreateChatInviteLinkRequest) (*converter.ChatInviteLink, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}
	invite, err := b.raw.MessagesExportChatInvite(ctx, &tg.MessagesExportChatInviteRequest{
		Peer: peer, ExpireDate: req.ExpireDate, UsageLimit: req.MemberLimit,
		RequestNeeded: req.CreatesJoinRequest, Title: req.Name,
	})
	if err != nil {
		return nil, err
	}
	exported, ok := invite.(*tg.ChatInviteExported)
	if !ok {
		return nil, fmt.Errorf("unexpected invite type %T", invite)
	}
	return b.convertInviteLink(exported), nil
}

// CreateChatSubscriptionInviteLink creates a paid subscription invite link.
func (b *BotInstance) CreateChatSubscriptionInviteLink(ctx context.Context, req *converter.CreateChatSubscriptionInviteLinkRequest) (*converter.ChatInviteLink, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}
	request := &tg.MessagesExportChatInviteRequest{Peer: peer, Title: req.Name}
	request.SetSubscriptionPricing(tg.StarsSubscriptionPricing{Period: req.SubscriptionPeriod, Amount: req.SubscriptionPrice})
	invite, err := b.raw.MessagesExportChatInvite(ctx, request)
	if err != nil {
		return nil, err
	}
	exported, ok := invite.(*tg.ChatInviteExported)
	if !ok {
		return nil, fmt.Errorf("unexpected invite type %T", invite)
	}
	return b.convertInviteLink(exported), nil
}

// EditChatSubscriptionInviteLink edits the administrator-visible name of a paid invite link.
func (b *BotInstance) EditChatSubscriptionInviteLink(ctx context.Context, req *converter.EditChatSubscriptionInviteLinkRequest) (*converter.ChatInviteLink, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}
	request := &tg.MessagesEditExportedChatInviteRequest{Peer: peer, Link: req.InviteLink}
	request.SetTitle(req.Name)
	result, err := b.raw.MessagesEditExportedChatInvite(ctx, request)
	if err != nil {
		return nil, err
	}
	edited, ok := result.(*tg.MessagesExportedChatInvite)
	if !ok {
		return nil, fmt.Errorf("unexpected invite result %T", result)
	}
	exported, ok := edited.Invite.(*tg.ChatInviteExported)
	if !ok {
		return nil, fmt.Errorf("unexpected invite type %T", edited.Invite)
	}
	return b.convertInviteLink(exported), nil
}

// EditChatInviteLink edits a non-primary invite link created by the bot.
func (b *BotInstance) EditChatInviteLink(ctx context.Context, req *converter.EditChatInviteLinkRequest) (*converter.ChatInviteLink, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}
	result, err := b.raw.MessagesEditExportedChatInvite(ctx, &tg.MessagesEditExportedChatInviteRequest{
		Peer: peer, Link: req.InviteLink, ExpireDate: req.ExpireDate,
		UsageLimit: req.MemberLimit, RequestNeeded: req.CreatesJoinRequest, Title: req.Name,
	})
	if err != nil {
		return nil, err
	}
	var invite tg.ExportedChatInviteClass
	switch value := result.(type) {
	case *tg.MessagesExportedChatInvite:
		invite = value.Invite
	case *tg.MessagesExportedChatInviteReplaced:
		invite = value.NewInvite
	default:
		return nil, fmt.Errorf("unexpected edit invite result %T", result)
	}
	exported, ok := invite.(*tg.ChatInviteExported)
	if !ok {
		return nil, fmt.Errorf("unexpected invite type %T", invite)
	}
	return b.convertInviteLink(exported), nil
}

// RevokeChatInviteLink revokes an invite link.
func (b *BotInstance) RevokeChatInviteLink(ctx context.Context, req *converter.RevokeChatInviteLinkRequest) (*converter.ChatInviteLink, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}
	result, err := b.raw.MessagesEditExportedChatInvite(ctx, &tg.MessagesEditExportedChatInviteRequest{
		Peer: peer, Link: req.InviteLink, Revoked: true,
	})
	if err != nil {
		return nil, err
	}
	var invite tg.ExportedChatInviteClass
	switch value := result.(type) {
	case *tg.MessagesExportedChatInvite:
		invite = value.Invite
	case *tg.MessagesExportedChatInviteReplaced:
		invite = value.Invite
	default:
		return nil, fmt.Errorf("unexpected revoke invite result %T", result)
	}
	exported, ok := invite.(*tg.ChatInviteExported)
	if !ok {
		return nil, fmt.Errorf("unexpected invite type %T", invite)
	}
	return b.convertInviteLink(exported), nil
}

// ApproveChatJoinRequest approves a chat join request.
func (b *BotInstance) ApproveChatJoinRequest(ctx context.Context, chatID, userID int64) (bool, error) {
	return b.hideChatJoinRequest(ctx, chatID, userID, true)
}

// DeclineChatJoinRequest declines a chat join request.
func (b *BotInstance) DeclineChatJoinRequest(ctx context.Context, chatID, userID int64) (bool, error) {
	return b.hideChatJoinRequest(ctx, chatID, userID, false)
}

// SetChatPhoto sets a new profile photo for the chat.
func (b *BotInstance) SetChatPhoto(ctx context.Context, req *converter.SetChatPhotoRequest) (bool, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}
	channel, err := inputChannel(peer)
	if err != nil {
		return false, err
	}
	data := req.PhotoData
	if len(data) == 0 && isMediaURL(req.Photo) {
		// Stream from URL directly into Telegram — no intermediate buffer.
		inputFile, _, _, err := b.uploadFromURL(ctx, req.Photo)
		if err != nil {
			return false, fmt.Errorf("streaming upload chat photo: %w", err)
		}
		_, err = b.raw.ChannelsEditPhoto(ctx, &tg.ChannelsEditPhotoRequest{
			Channel: channel, Photo: &tg.InputChatUploadedPhoto{File: inputFile},
		})
		return err == nil, err
	}
	if len(data) == 0 {
		return false, fmt.Errorf("photo upload is required")
	}
	file, err := uploader.NewUploader(b.raw).FromBytes(ctx, "chat-photo.jpg", data)
	if err != nil {
		return false, fmt.Errorf("upload chat photo: %w", err)
	}
	_, err = b.raw.ChannelsEditPhoto(ctx, &tg.ChannelsEditPhotoRequest{
		Channel: channel, Photo: &tg.InputChatUploadedPhoto{File: file},
	})
	return err == nil, err
}

// DeleteChatPhoto deletes a chat photo.
func (b *BotInstance) DeleteChatPhoto(ctx context.Context, chatID int64) (bool, error) {
	peer, err := b.resolvePeer(chatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}
	channel, err := inputChannel(peer)
	if err != nil {
		return false, err
	}
	_, err = b.raw.ChannelsEditPhoto(ctx, &tg.ChannelsEditPhotoRequest{
		Channel: channel, Photo: &tg.InputChatPhotoEmpty{},
	})
	return err == nil, err
}

// SetChatTitle changes the title of a chat.
func (b *BotInstance) SetChatTitle(ctx context.Context, chatID int64, title string) (bool, error) {
	peer, err := b.resolvePeer(chatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}

	if cp, ok := peer.(*tg.InputPeerChannel); ok {
		_, err = b.raw.ChannelsEditTitle(ctx, &tg.ChannelsEditTitleRequest{
			Channel: &tg.InputChannel{ChannelID: cp.ChannelID, AccessHash: cp.AccessHash},
			Title:   title,
		})
		return err == nil, err
	}
	if chat, ok := peer.(*tg.InputPeerChat); ok {
		_, err = b.raw.MessagesEditChatTitle(ctx, &tg.MessagesEditChatTitleRequest{ChatID: chat.ChatID, Title: title})
		return err == nil, err
	}
	return false, fmt.Errorf("chat title can't be changed for private chats")
}

// SetChatDescription changes the description of a chat.
func (b *BotInstance) SetChatDescription(ctx context.Context, chatID int64, description string) (bool, error) {
	peer, err := b.resolvePeer(chatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}

	result, err := b.raw.MessagesEditChatAbout(ctx, &tg.MessagesEditChatAboutRequest{
		Peer:  peer,
		About: description,
	})
	return result, err
}

// SetChatMenuButton changes the bot's menu button.
func (b *BotInstance) SetChatMenuButton(ctx context.Context, req *converter.SetChatMenuButtonRequest) (bool, error) {
	var user tg.InputUserClass = &tg.InputUserEmpty{}
	if req.ChatID != 0 {
		user = b.inputUser(req.ChatID)
	}
	var button tg.BotMenuButtonClass = &tg.BotMenuButtonDefault{}
	if req.MenuButton != nil {
		switch req.MenuButton.Type {
		case "commands":
			button = &tg.BotMenuButtonCommands{}
		case "web_app":
			if req.MenuButton.WebApp == nil || req.MenuButton.WebApp.URL == "" {
				return false, fmt.Errorf("web_app URL is required")
			}
			button = &tg.BotMenuButton{Text: req.MenuButton.Text, URL: req.MenuButton.WebApp.URL}
		case "default", "":
		default:
			return false, fmt.Errorf("unsupported menu button type %q", req.MenuButton.Type)
		}
	}
	return b.raw.BotsSetBotMenuButton(ctx, &tg.BotsSetBotMenuButtonRequest{UserID: user, Button: button})
}

// GetChatMenuButton gets the current value of the bot's menu button.
func (b *BotInstance) GetChatMenuButton(ctx context.Context, chatID int64) (*converter.MenuButton, error) {
	var user tg.InputUserClass = &tg.InputUserEmpty{}
	if chatID != 0 {
		user = b.inputUser(chatID)
	}
	button, err := b.raw.BotsGetBotMenuButton(ctx, user)
	if err != nil {
		return nil, err
	}
	switch value := button.(type) {
	case *tg.BotMenuButtonDefault:
		return &converter.MenuButton{Type: "default"}, nil
	case *tg.BotMenuButtonCommands:
		return &converter.MenuButton{Type: "commands"}, nil
	case *tg.BotMenuButton:
		return &converter.MenuButton{Type: "web_app", Text: value.Text, WebApp: &converter.WebAppInfo{URL: value.URL}}, nil
	default:
		return nil, fmt.Errorf("unexpected menu button type %T", button)
	}
}

// SetMyDefaultAdministratorRights changes default administrator rights.
func (b *BotInstance) SetMyDefaultAdministratorRights(ctx context.Context, req *converter.SetMyDefaultAdministratorRightsRequest) (bool, error) {
	rights := convertAdminRights(req.Rights)
	if req.ForChannels {
		return b.raw.BotsSetBotBroadcastDefaultAdminRights(ctx, rights)
	}
	return b.raw.BotsSetBotGroupDefaultAdminRights(ctx, rights)
}

// GetMyDefaultAdministratorRights gets default administrator rights.
func (b *BotInstance) GetMyDefaultAdministratorRights(ctx context.Context, forChannels bool) (*converter.ChatAdministratorRights, error) {
	full, err := b.raw.UsersGetFullUser(ctx, &tg.InputUserSelf{})
	if err != nil {
		return nil, err
	}
	if forChannels {
		rights, _ := full.FullUser.GetBotBroadcastAdminRights()
		return convertBotAdminRights(rights), nil
	}
	rights, _ := full.FullUser.GetBotGroupAdminRights()
	return convertBotAdminRights(rights), nil
}

// GetMyCommands gets the current list of the bot's commands.
func (b *BotInstance) GetMyCommands(ctx context.Context, req *converter.GetMyCommandsRequest) ([]converter.BotCommand, error) {
	res, err := b.raw.BotsGetBotCommands(ctx, &tg.BotsGetBotCommandsRequest{
		Scope:    &tg.BotCommandScopeDefault{},
		LangCode: req.LanguageCode,
	})
	if err != nil {
		return nil, err
	}

	var list []converter.BotCommand
	for _, c := range res {
		list = append(list, converter.BotCommand{
			Command:     c.Command,
			Description: c.Description,
		})
	}
	return list, nil
}

// DeleteMyCommands deletes the list of the bot's commands.
func (b *BotInstance) DeleteMyCommands(ctx context.Context, req *converter.DeleteMyCommandsRequest) (bool, error) {
	_, err := b.raw.BotsResetBotCommands(ctx, &tg.BotsResetBotCommandsRequest{
		Scope:    &tg.BotCommandScopeDefault{},
		LangCode: req.LanguageCode,
	})
	return err == nil, err
}

// SetMyName changes the bot's name.
func (b *BotInstance) SetMyName(ctx context.Context, name, langCode string) (bool, error) {
	_, err := b.raw.BotsSetBotInfo(ctx, &tg.BotsSetBotInfoRequest{
		Name:     name,
		LangCode: langCode,
	})
	return err == nil, err
}

// GetMyName gets the current bot name.
func (b *BotInstance) GetMyName(ctx context.Context, langCode string) (*converter.BotName, error) {
	info, err := b.raw.BotsGetBotInfo(ctx, &tg.BotsGetBotInfoRequest{
		LangCode: langCode,
	})
	if err != nil {
		return nil, err
	}
	return &converter.BotName{Name: info.Name}, nil
}

// SetMyDescription changes the bot's description.
func (b *BotInstance) SetMyDescription(ctx context.Context, description, langCode string) (bool, error) {
	_, err := b.raw.BotsSetBotInfo(ctx, &tg.BotsSetBotInfoRequest{
		About:    description,
		LangCode: langCode,
	})
	return err == nil, err
}

// GetMyDescription gets the bot's description.
func (b *BotInstance) GetMyDescription(ctx context.Context, langCode string) (*converter.BotDescription, error) {
	info, err := b.raw.BotsGetBotInfo(ctx, &tg.BotsGetBotInfoRequest{
		LangCode: langCode,
	})
	if err != nil {
		return nil, err
	}
	return &converter.BotDescription{Description: info.About}, nil
}

// SetMyShortDescription changes the bot's short description.
func (b *BotInstance) SetMyShortDescription(ctx context.Context, shortDescription, langCode string) (bool, error) {
	_, err := b.raw.BotsSetBotInfo(ctx, &tg.BotsSetBotInfoRequest{
		Description: shortDescription,
		LangCode:    langCode,
	})
	return err == nil, err
}

// GetMyShortDescription gets the bot's short description.
func (b *BotInstance) GetMyShortDescription(ctx context.Context, langCode string) (*converter.BotShortDescription, error) {
	info, err := b.raw.BotsGetBotInfo(ctx, &tg.BotsGetBotInfoRequest{
		LangCode: langCode,
	})
	if err != nil {
		return nil, err
	}
	return &converter.BotShortDescription{ShortDescription: info.Description}, nil
}

// SetUserEmojiStatus changes the emoji status for a given user.
func (b *BotInstance) SetUserEmojiStatus(ctx context.Context, req *converter.SetUserEmojiStatusRequest) (bool, error) {
	var status tg.EmojiStatusClass = &tg.EmojiStatusEmpty{}
	if req.CustomEmojiID != "" {
		documentID, err := strconv.ParseInt(req.CustomEmojiID, 10, 64)
		if err != nil {
			return false, fmt.Errorf("invalid custom_emoji_id: %w", err)
		}
		emojiStatus := &tg.EmojiStatus{DocumentID: documentID}
		if req.EmojiStatusExpirationDate != 0 {
			emojiStatus.SetUntil(req.EmojiStatusExpirationDate)
		}
		status = emojiStatus
	}
	return b.raw.BotsUpdateUserEmojiStatus(ctx, &tg.BotsUpdateUserEmojiStatusRequest{
		UserID: b.inputUser(req.UserID), EmojiStatus: status,
	})
}

// CreateForumTopic creates a topic in a forum supergroup chat.
func (b *BotInstance) CreateForumTopic(ctx context.Context, req *converter.CreateForumTopicRequest) (*converter.ForumTopic, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}

	randomID, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	request := &tg.MessagesCreateForumTopicRequest{
		Peer:      peer,
		Title:     req.Name,
		IconColor: req.IconColor,
		RandomID:  randomID.Int64(),
	}
	if req.IconCustomEmojiID != "" {
		iconID, err := strconv.ParseInt(req.IconCustomEmojiID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid icon_custom_emoji_id: %w", err)
		}
		request.SetIconEmojiID(iconID)
	}
	res, err := b.raw.MessagesCreateForumTopic(ctx, request)
	if err != nil {
		return nil, err
	}

	threadID := int(extractSentMessageID(res))
	if threadID == 0 {
		return nil, fmt.Errorf("forum topic created without message id")
	}
	return &converter.ForumTopic{
		MessageThreadID: threadID, Name: req.Name, IconColor: req.IconColor,
		IconCustomEmojiID: req.IconCustomEmojiID,
	}, nil
}

// EditForumTopic edits name and icon of a topic.
func (b *BotInstance) EditForumTopic(ctx context.Context, req *converter.EditForumTopicRequest) (bool, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}

	edit := &tg.MessagesEditForumTopicRequest{Peer: peer, TopicID: req.MessageThreadID}
	if req.Name != "" {
		edit.SetTitle(req.Name)
	}
	if req.IconCustomEmojiID != "" {
		iconID, err := strconv.ParseInt(req.IconCustomEmojiID, 10, 64)
		if err != nil {
			return false, fmt.Errorf("invalid icon_custom_emoji_id: %w", err)
		}
		edit.SetIconEmojiID(iconID)
	}
	_, err = b.raw.MessagesEditForumTopic(ctx, edit)
	return err == nil, err
}

// CloseForumTopic closes an open topic in a forum supergroup chat.
func (b *BotInstance) CloseForumTopic(ctx context.Context, chatID int64, threadID int) (bool, error) {
	peer, err := b.resolvePeer(chatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}

	edit := &tg.MessagesEditForumTopicRequest{Peer: peer, TopicID: threadID}
	edit.SetClosed(true)
	_, err = b.raw.MessagesEditForumTopic(ctx, edit)
	return err == nil, err
}

// ReopenForumTopic reopens a closed topic in a forum supergroup chat.
func (b *BotInstance) ReopenForumTopic(ctx context.Context, chatID int64, threadID int) (bool, error) {
	peer, err := b.resolvePeer(chatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}

	edit := &tg.MessagesEditForumTopicRequest{Peer: peer, TopicID: threadID}
	edit.SetClosed(false)
	_, err = b.raw.MessagesEditForumTopic(ctx, edit)
	return err == nil, err
}

// DeleteForumTopic deletes a forum topic along with all its messages.
func (b *BotInstance) DeleteForumTopic(ctx context.Context, chatID int64, threadID int) (bool, error) {
	peer, err := b.resolvePeer(chatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}

	_, err = b.raw.MessagesDeleteTopicHistory(ctx, &tg.MessagesDeleteTopicHistoryRequest{
		Peer:     peer,
		TopMsgID: threadID,
	})
	return err == nil, err
}

// UnpinAllForumTopicMessages clears the list of pinned messages in a forum topic.
func (b *BotInstance) UnpinAllForumTopicMessages(ctx context.Context, chatID int64, threadID int) (bool, error) {
	peer, err := b.resolvePeer(chatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}
	req := &tg.MessagesUnpinAllMessagesRequest{Peer: peer}
	req.SetTopMsgID(threadID)
	_, err = b.raw.MessagesUnpinAllMessages(ctx, req)
	return err == nil, err
}

// EditGeneralForumTopic edits the name of the General topic in a forum supergroup chat.
func (b *BotInstance) EditGeneralForumTopic(ctx context.Context, chatID int64, name string) (bool, error) {
	return b.editGeneralForumTopic(ctx, chatID, func(req *tg.MessagesEditForumTopicRequest) { req.SetTitle(name) })
}

// CloseGeneralForumTopic closes an open 'General' topic in a forum supergroup chat.
func (b *BotInstance) CloseGeneralForumTopic(ctx context.Context, chatID int64) (bool, error) {
	return b.editGeneralForumTopic(ctx, chatID, func(req *tg.MessagesEditForumTopicRequest) { req.SetClosed(true) })
}

// ReopenGeneralForumTopic reopens a closed 'General' topic.
func (b *BotInstance) ReopenGeneralForumTopic(ctx context.Context, chatID int64) (bool, error) {
	return b.editGeneralForumTopic(ctx, chatID, func(req *tg.MessagesEditForumTopicRequest) { req.SetClosed(false) })
}

// HideGeneralForumTopic hides the 'General' topic.
func (b *BotInstance) HideGeneralForumTopic(ctx context.Context, chatID int64) (bool, error) {
	return b.editGeneralForumTopic(ctx, chatID, func(req *tg.MessagesEditForumTopicRequest) { req.SetHidden(true) })
}

// UnhideGeneralForumTopic unhides the 'General' topic.
func (b *BotInstance) UnhideGeneralForumTopic(ctx context.Context, chatID int64) (bool, error) {
	return b.editGeneralForumTopic(ctx, chatID, func(req *tg.MessagesEditForumTopicRequest) { req.SetHidden(false) })
}

// UnpinAllGeneralForumTopicMessages clears the list of pinned messages in the General topic.
func (b *BotInstance) UnpinAllGeneralForumTopicMessages(ctx context.Context, chatID int64) (bool, error) {
	return b.UnpinAllForumTopicMessages(ctx, chatID, 1)
}

// GetForumTopicIconStickers gets custom emoji stickers for forum topics.
func (b *BotInstance) GetForumTopicIconStickers(ctx context.Context) ([]converter.Sticker, error) {
	set, err := b.raw.MessagesGetStickerSet(ctx, &tg.MessagesGetStickerSetRequest{
		Stickerset: &tg.InputStickerSetEmojiDefaultTopicIcons{}, Hash: 0,
	})
	if err != nil {
		return nil, err
	}
	return convertStickerSetDocuments(set)
}

// SetChatStickerSet associates a sticker set with a supergroup.
func (b *BotInstance) SetChatStickerSet(ctx context.Context, req *converter.SetChatStickerSetRequest) (bool, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}
	channel, err := inputChannel(peer)
	if err != nil {
		return false, err
	}
	return b.raw.ChannelsSetStickers(ctx, &tg.ChannelsSetStickersRequest{
		Channel: channel, Stickerset: &tg.InputStickerSetShortName{ShortName: req.StickerSetName},
	})
}

// DeleteChatStickerSet removes the sticker set associated with a supergroup.
func (b *BotInstance) DeleteChatStickerSet(ctx context.Context, req *converter.DeleteChatStickerSetRequest) (bool, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}
	channel, err := inputChannel(peer)
	if err != nil {
		return false, err
	}
	return b.raw.ChannelsSetStickers(ctx, &tg.ChannelsSetStickersRequest{
		Channel: channel, Stickerset: &tg.InputStickerSetEmpty{},
	})
}

// AnswerInlineQuery sends answers to an inline query.
func (b *BotInstance) AnswerInlineQuery(ctx context.Context, req *converter.AnswerInlineQueryRequest) (bool, error) {
	queryID, err := strconv.ParseInt(req.InlineQueryID, 10, 64)
	if err != nil {
		return false, fmt.Errorf("invalid inline_query_id: %w", err)
	}
	results, gallery, err := buildInlineResults(req.Results)
	if err != nil {
		return false, err
	}
	request := &tg.MessagesSetInlineBotResultsRequest{QueryID: queryID, Results: results, CacheTime: req.CacheTime,
		Private: req.IsPersonal, Gallery: gallery}
	if req.NextOffset != "" {
		request.SetNextOffset(req.NextOffset)
	}
	if len(req.Button) != 0 && string(req.Button) != "null" {
		var button struct {
			Text           string `json:"text"`
			StartParameter string `json:"start_parameter"`
			WebApp         *struct {
				URL string `json:"url"`
			} `json:"web_app"`
		}
		if err := json.Unmarshal(req.Button, &button); err != nil {
			return false, fmt.Errorf("invalid button: %w", err)
		}
		if button.WebApp != nil {
			request.SetSwitchWebview(tg.InlineBotWebView{Text: button.Text, URL: button.WebApp.URL})
		} else {
			request.SetSwitchPm(tg.InlineBotSwitchPM{Text: button.Text, StartParam: button.StartParameter})
		}
	}
	return b.raw.MessagesSetInlineBotResults(ctx, request)
}

// AnswerWebAppQuery sets the result of an interaction with a Web App.
func (b *BotInstance) AnswerWebAppQuery(ctx context.Context, req *converter.AnswerWebAppQueryRequest) (*converter.SentWebAppMessage, error) {
	result, err := buildInlineResult(req.Result)
	if err != nil {
		return nil, err
	}
	response, err := b.raw.MessagesSendWebViewResultMessage(ctx, &tg.MessagesSendWebViewResultMessageRequest{
		BotQueryID: req.WebAppQueryID, Result: result,
	})
	if err != nil {
		return nil, err
	}
	messageID, err := encodeInlineMessageID(response.MsgID)
	if err != nil {
		return nil, err
	}
	return &converter.SentWebAppMessage{InlineMessageID: messageID}, nil
}

// SavePreparedInlineMessage stores a message that can be sent by a user of a Mini App.
func (b *BotInstance) SavePreparedInlineMessage(ctx context.Context, req *converter.SavePreparedInlineMessageRequest) (*converter.PreparedInlineMessage, error) {
	result, err := buildInlineResult(req.Result)
	if err != nil {
		return nil, err
	}
	peerTypes := make([]tg.InlineQueryPeerTypeClass, 0, 4)
	if req.AllowUserChats {
		peerTypes = append(peerTypes, &tg.InlineQueryPeerTypePM{})
	}
	if req.AllowBotChats {
		peerTypes = append(peerTypes, &tg.InlineQueryPeerTypeBotPM{})
	}
	if req.AllowGroupChats {
		peerTypes = append(peerTypes, &tg.InlineQueryPeerTypeChat{}, &tg.InlineQueryPeerTypeMegagroup{})
	}
	if req.AllowChannelChats {
		peerTypes = append(peerTypes, &tg.InlineQueryPeerTypeBroadcast{})
	}
	request := &tg.MessagesSavePreparedInlineMessageRequest{Result: result, UserID: b.inputUser(req.UserID)}
	if len(peerTypes) != 0 {
		request.SetPeerTypes(peerTypes)
	}
	response, err := b.raw.MessagesSavePreparedInlineMessage(ctx, request)
	if err != nil {
		return nil, err
	}
	return &converter.PreparedInlineMessage{ID: response.ID, ExpirationDate: response.ExpireDate}, nil
}

// SendInvoice sends an invoice message.
func (b *BotInstance) SendInvoice(ctx context.Context, req *converter.SendInvoiceRequest) (*converter.Message, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, err
	}
	media, err := buildInvoiceMedia(invoiceParameters{Title: req.Title, Description: req.Description, Payload: req.Payload,
		ProviderToken: req.ProviderToken, Currency: req.Currency, Prices: req.Prices, MaxTipAmount: req.MaxTipAmount,
		SuggestedTipAmounts: req.SuggestedTipAmounts, StartParameter: req.StartParameter, ProviderData: req.ProviderData,
		PhotoURL: req.PhotoURL, PhotoSize: req.PhotoSize, PhotoWidth: req.PhotoWidth, PhotoHeight: req.PhotoHeight,
		NeedName: req.NeedName, NeedPhoneNumber: req.NeedPhoneNumber, NeedEmail: req.NeedEmail,
		NeedShippingAddress: req.NeedShippingAddress, SendPhoneNumberToProvider: req.SendPhoneNumberToProvider,
		SendEmailToProvider: req.SendEmailToProvider, IsFlexible: req.IsFlexible})
	if err != nil {
		return nil, err
	}
	randomID, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	send := &tg.MessagesSendMediaRequest{Peer: peer, Media: media, RandomID: randomID.Int64(),
		Silent: req.DisableNotification, Noforwards: req.ProtectContent}
	if req.MessageThreadID != 0 || req.ReplyParameters != nil {
		reply := &tg.InputReplyToMessage{}
		if req.MessageThreadID != 0 {
			reply.SetTopMsgID(req.MessageThreadID)
		}
		if req.ReplyParameters != nil {
			reply.ReplyToMsgID = int(req.ReplyParameters.MessageID)
		}
		send.SetReplyTo(reply)
	}
	if len(req.ReplyMarkup) != 0 {
		markup, err := converter.ParseReplyMarkup(req.ReplyMarkup)
		if err != nil {
			return nil, err
		}
		send.SetReplyMarkup(markup)
	}
	updates, err := b.raw.MessagesSendMedia(ctx, send)
	if err != nil {
		return nil, err
	}
	return &converter.Message{MessageID: extractSentMessageID(updates), From: b.GetMe(),
		Chat: converter.Chat{ID: req.ChatID}, Date: int(time.Now().Unix())}, nil
}

// CreateInvoiceLink creates a link for an invoice.
func (b *BotInstance) CreateInvoiceLink(ctx context.Context, req *converter.CreateInvoiceLinkRequest) (string, error) {
	media, err := buildInvoiceMedia(invoiceParameters{Title: req.Title, Description: req.Description, Payload: req.Payload,
		ProviderToken: req.ProviderToken, Currency: req.Currency, Prices: req.Prices, SubscriptionPeriod: req.SubscriptionPeriod,
		MaxTipAmount: req.MaxTipAmount, SuggestedTipAmounts: req.SuggestedTipAmounts, StartParameter: req.StartParameter,
		ProviderData: req.ProviderData, PhotoURL: req.PhotoURL, PhotoSize: req.PhotoSize, PhotoWidth: req.PhotoWidth,
		PhotoHeight: req.PhotoHeight, NeedName: req.NeedName, NeedPhoneNumber: req.NeedPhoneNumber,
		NeedEmail: req.NeedEmail, NeedShippingAddress: req.NeedShippingAddress,
		SendPhoneNumberToProvider: req.SendPhoneNumberToProvider, SendEmailToProvider: req.SendEmailToProvider,
		IsFlexible: req.IsFlexible})
	if err != nil {
		return "", err
	}
	if req.BusinessConnectionID != "" {
		var result tg.PaymentsExportedInvoice
		if err := b.invokeBusiness(ctx, req.BusinessConnectionID, &tg.PaymentsExportInvoiceRequest{InvoiceMedia: media}, &result); err != nil {
			return "", err
		}
		return result.URL, nil
	}
	result, err := b.raw.PaymentsExportInvoice(ctx, media)
	if err != nil {
		return "", err
	}
	return result.URL, nil
}

// AnswerShippingQuery replies to shipping queries.
func (b *BotInstance) AnswerShippingQuery(ctx context.Context, req *converter.AnswerShippingQueryRequest) (bool, error) {
	queryID, err := strconv.ParseInt(req.ShippingQueryID, 10, 64)
	if err != nil {
		return false, fmt.Errorf("invalid shipping_query_id: %w", err)
	}
	request := &tg.MessagesSetBotShippingResultsRequest{QueryID: queryID}
	if req.OK {
		var options []struct {
			ID     string `json:"id"`
			Title  string `json:"title"`
			Prices []struct {
				Label  string `json:"label"`
				Amount int64  `json:"amount"`
			} `json:"prices"`
		}
		if err := json.Unmarshal(req.ShippingOptions, &options); err != nil {
			return false, fmt.Errorf("invalid shipping_options: %w", err)
		}
		shippingOptions := make([]tg.ShippingOption, 0, len(options))
		for _, option := range options {
			prices := make([]tg.LabeledPrice, 0, len(option.Prices))
			for _, price := range option.Prices {
				prices = append(prices, tg.LabeledPrice{Label: price.Label, Amount: price.Amount})
			}
			shippingOptions = append(shippingOptions, tg.ShippingOption{ID: option.ID, Title: option.Title, Prices: prices})
		}
		request.SetShippingOptions(shippingOptions)
	} else {
		request.SetError(req.ErrorMessage)
	}
	return b.raw.MessagesSetBotShippingResults(ctx, request)
}

// SendPaidMedia sends paid media.
func (b *BotInstance) SendPaidMedia(ctx context.Context, req *converter.SendPaidMediaRequest) (*converter.Message, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, err
	}
	var items []converter.InputMediaItem
	if err := json.Unmarshal(req.Media, &items); err != nil {
		return nil, fmt.Errorf("invalid media: %w", err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("media must not be empty")
	}
	media := make([]tg.InputMediaClass, 0, len(items))
	for _, item := range items {
		if item.Type != "photo" && item.Type != "video" {
			return nil, fmt.Errorf("paid media type %q is not supported", item.Type)
		}
		resolved, err := b.resolveInputSingleMedia(ctx, peer, req.BusinessConnectionID, item, req.Files, req.FileNames)
		if err != nil {
			return nil, err
		}
		media = append(media, resolved)
	}
	paid := &tg.InputMediaPaidMedia{StarsAmount: int64(req.StarCount), ExtendedMedia: media}
	if req.Payload != "" {
		paid.SetPayload(req.Payload)
	}
	caption, entities, err := inlineMessageEntities(req.Caption, req.ParseMode, req.CaptionEntities)
	if err != nil {
		return nil, err
	}
	randomID, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	send := &tg.MessagesSendMediaRequest{Peer: peer, Media: paid, Message: caption, Entities: entities,
		RandomID: randomID.Int64(), Silent: req.DisableNotification, Noforwards: req.ProtectContent,
		InvertMedia: req.ShowCaptionAboveMedia}
	if req.ReplyParameters != nil {
		send.SetReplyTo(&tg.InputReplyToMessage{ReplyToMsgID: int(req.ReplyParameters.MessageID)})
	}
	if len(req.ReplyMarkup) != 0 {
		markup, err := converter.ParseReplyMarkup(req.ReplyMarkup)
		if err != nil {
			return nil, err
		}
		send.SetReplyMarkup(markup)
	}
	var updates tg.UpdatesClass
	if req.BusinessConnectionID != "" {
		var box tg.UpdatesBox
		err = b.invokeBusiness(ctx, req.BusinessConnectionID, send, &box)
		updates = box.Updates
	} else {
		updates, err = b.raw.MessagesSendMedia(ctx, send)
	}
	if err != nil {
		return nil, err
	}
	return &converter.Message{MessageID: extractSentMessageID(updates), From: b.GetMe(),
		Chat: converter.Chat{ID: req.ChatID}, Date: int(time.Now().Unix()), Caption: caption,
		BusinessConnectionID: req.BusinessConnectionID}, nil
}

// RefundStarPayment refunds a Telegram Star payment.
func (b *BotInstance) RefundStarPayment(ctx context.Context, req *converter.RefundStarPaymentRequest) (bool, error) {
	_, err := b.raw.PaymentsRefundStarsCharge(ctx, &tg.PaymentsRefundStarsChargeRequest{
		UserID: b.inputUser(req.UserID), ChargeID: req.TelegramPaymentChargeID,
	})
	return err == nil, err
}

// GetStarTransactions returns the bot's Telegram Star transactions.
func (b *BotInstance) GetStarTransactions(ctx context.Context, req *converter.GetStarTransactionsRequest) (*converter.StarTransactions, error) {
	limit := req.Limit
	if limit == 0 {
		limit = 100
	}
	result, err := b.raw.PaymentsGetStarsTransactions(ctx, &tg.PaymentsGetStarsTransactionsRequest{
		Peer: &tg.InputPeerSelf{}, Offset: strconv.Itoa(req.Offset), Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	users := make(map[int64]converter.User)
	for _, class := range result.Users {
		if user, ok := class.(*tg.User); ok {
			users[user.ID] = converter.User{ID: user.ID, IsBot: user.Bot, FirstName: user.FirstName, LastName: user.LastName, Username: user.Username}
		}
	}
	transactions := make([]interface{}, 0, len(result.History))
	for _, transaction := range result.History {
		transactions = append(transactions, convertStarTransaction(transaction, users))
	}
	return &converter.StarTransactions{Transactions: transactions}, nil
}

func convertStarTransaction(transaction tg.StarsTransaction, users map[int64]converter.User) map[string]interface{} {
	amount, nanos := int64(0), 0
	if value, ok := transaction.Amount.(*tg.StarsAmount); ok {
		amount, nanos = value.Amount, value.Nanos
	}
	item := map[string]interface{}{"id": transaction.ID, "amount": amount, "date": transaction.Date}
	if nanos != 0 {
		item["nanostar_amount"] = nanos
	}

	peer, ok := transaction.Peer.(*tg.StarsTransactionPeer)
	if !ok {
		return item
	}
	userPeer, ok := peer.Peer.(*tg.PeerUser)
	if !ok {
		return item
	}

	transactionType := "invoice_payment"
	switch {
	case transaction.BusinessTransfer:
		transactionType = "business_account_transfer"
	case transaction.PremiumGiftMonths > 0:
		transactionType = "premium_purchase"
	case transaction.Gift:
		transactionType = "gift_purchase"
	case len(transaction.ExtendedMedia) > 0:
		transactionType = "paid_media_payment"
	}
	partner := map[string]interface{}{
		"type":             "user",
		"transaction_type": transactionType,
		"user":             users[userPeer.UserID],
	}
	if transaction.BotPayload != nil {
		if transactionType == "paid_media_payment" {
			partner["paid_media_payload"] = string(transaction.BotPayload)
		} else if transactionType == "invoice_payment" {
			partner["invoice_payload"] = string(transaction.BotPayload)
		}
	}
	if transaction.SubscriptionPeriod != 0 && transactionType == "invoice_payment" {
		partner["subscription_period"] = transaction.SubscriptionPeriod
	}
	if transaction.PremiumGiftMonths != 0 && transactionType == "premium_purchase" {
		partner["premium_subscription_duration"] = transaction.PremiumGiftMonths
	}

	if amount >= 0 && !transaction.Refund {
		item["source"] = partner
	} else {
		item["receiver"] = partner
	}
	return item
}

// GetAvailableGifts returns available gifts.
func (b *BotInstance) GetAvailableGifts(ctx context.Context) (interface{}, error) {
	result, err := b.raw.PaymentsGetStarGifts(ctx, 0)
	if err != nil {
		return nil, err
	}
	value, ok := result.(*tg.PaymentsStarGifts)
	if !ok {
		return nil, fmt.Errorf("getAvailableGifts returned unexpected response %T", result)
	}
	gifts := make([]interface{}, 0, len(value.Gifts))
	for _, class := range value.Gifts {
		gift, ok := class.(*tg.StarGift)
		if !ok {
			continue
		}
		item := map[string]interface{}{"id": strconv.FormatInt(gift.ID, 10), "star_count": gift.Stars}
		if document, ok := gift.Sticker.(*tg.Document); ok {
			sticker, err := convertStickerDocument(document)
			if err != nil {
				return nil, err
			}
			item["sticker"] = sticker
		}
		if gift.UpgradeStars != 0 {
			item["upgrade_star_count"] = gift.UpgradeStars
		}
		if gift.Limited {
			item["total_count"], item["remaining_count"] = gift.AvailabilityTotal, gift.AvailabilityRemains
		}
		gifts = append(gifts, item)
	}
	return map[string]interface{}{"gifts": gifts}, nil
}

// SendGift sends a gift to the given user.
func (b *BotInstance) SendGift(ctx context.Context, req *converter.SendGiftRequest) (bool, error) {
	peer, err := b.resolvePeer(req.UserID)
	if err != nil {
		return false, err
	}
	giftID, err := strconv.ParseInt(req.GiftID, 10, 64)
	if err != nil {
		return false, fmt.Errorf("invalid gift_id: %w", err)
	}
	invoice := &tg.InputInvoiceStarGift{Peer: peer, GiftID: giftID, IncludeUpgrade: req.PayForUpgrade}
	if req.Text != "" {
		text, entities, err := inlineMessageEntities(req.Text, req.TextParseMode, nil)
		if err != nil {
			return false, err
		}
		invoice.SetMessage(tg.TextWithEntities{Text: text, Entities: entities})
	}
	form, err := b.raw.PaymentsGetPaymentForm(ctx, &tg.PaymentsGetPaymentFormRequest{Invoice: invoice})
	if err != nil {
		return false, err
	}
	_, err = b.raw.PaymentsSendStarsForm(ctx, &tg.PaymentsSendStarsFormRequest{FormID: form.GetFormID(), Invoice: invoice})
	return err == nil, err
}

// VerifyUser verifies a user on behalf of the organization.
func (b *BotInstance) VerifyUser(ctx context.Context, req *converter.VerifyUserRequest) (bool, error) {
	peer, err := b.resolvePeer(req.UserID)
	if err != nil {
		return false, err
	}
	request := &tg.BotsSetCustomVerificationRequest{Enabled: true, Peer: peer}
	if req.CustomDescription != "" {
		request.SetCustomDescription(req.CustomDescription)
	}
	return b.raw.BotsSetCustomVerification(ctx, request)
}

// VerifyChat verifies a chat on behalf of the organization.
func (b *BotInstance) VerifyChat(ctx context.Context, req *converter.VerifyChatRequest) (bool, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return false, err
	}
	request := &tg.BotsSetCustomVerificationRequest{Enabled: true, Peer: peer}
	if req.CustomDescription != "" {
		request.SetCustomDescription(req.CustomDescription)
	}
	return b.raw.BotsSetCustomVerification(ctx, request)
}

// RemoveUserVerification removes verification from a user.
func (b *BotInstance) RemoveUserVerification(ctx context.Context, req *converter.RemoveUserVerificationRequest) (bool, error) {
	peer, err := b.resolvePeer(req.UserID)
	if err != nil {
		return false, err
	}
	return b.raw.BotsSetCustomVerification(ctx, &tg.BotsSetCustomVerificationRequest{Peer: peer})
}

// RemoveChatVerification removes verification from a chat.
func (b *BotInstance) RemoveChatVerification(ctx context.Context, req *converter.RemoveChatVerificationRequest) (bool, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return false, err
	}
	return b.raw.BotsSetCustomVerification(ctx, &tg.BotsSetCustomVerificationRequest{Peer: peer})
}

// GetStickerSet returns a sticker set.
func (b *BotInstance) GetStickerSet(ctx context.Context, req *converter.GetStickerSetRequest) (*converter.StickerSet, error) {
	result, err := b.raw.MessagesGetStickerSet(ctx, &tg.MessagesGetStickerSetRequest{Stickerset: &tg.InputStickerSetShortName{ShortName: req.Name}})
	if err != nil {
		return nil, err
	}
	value, ok := result.(*tg.MessagesStickerSet)
	if !ok {
		return nil, fmt.Errorf("unexpected sticker set type %T", result)
	}
	stickers, err := convertStickerSetDocuments(result)
	if err != nil {
		return nil, err
	}
	stickerType := "regular"
	if value.Set.Masks {
		stickerType = "mask"
	} else if value.Set.Emojis {
		stickerType = "custom_emoji"
	}
	return &converter.StickerSet{Name: value.Set.ShortName, Title: value.Set.Title, StickerType: stickerType, Stickers: stickers}, nil
}

// GetCustomEmojiStickers returns information about custom emoji stickers.
func (b *BotInstance) GetCustomEmojiStickers(ctx context.Context, req *converter.GetCustomEmojiStickersRequest) ([]converter.Sticker, error) {
	ids := make([]int64, 0, len(req.CustomEmojiIDs))
	for _, value := range req.CustomEmojiIDs {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	documents, err := b.raw.MessagesGetCustomEmojiDocuments(ctx, ids)
	if err != nil {
		return nil, err
	}
	stickers := make([]converter.Sticker, 0, len(documents))
	for _, documentClass := range documents {
		if document, ok := documentClass.(*tg.Document); ok {
			sticker, err := convertStickerDocument(document)
			if err != nil {
				return nil, err
			}
			stickers = append(stickers, sticker)
		}
	}
	return stickers, nil
}

// UploadStickerFile uploads a file with a sticker for later use.
func (b *BotInstance) UploadStickerFile(ctx context.Context, req *converter.UploadStickerFileRequest) (*converter.File, error) {
	document, err := b.uploadStickerDocument(ctx, req.StickerData, "sticker", req.StickerFormat)
	if err != nil {
		return nil, err
	}
	encoded, err := fileid.EncodeFileID(fileid.FromDocument(document))
	if err != nil {
		return nil, err
	}
	return &converter.File{FileID: encoded, FileUniqueID: strconv.FormatInt(document.ID, 10), FileSize: document.Size}, nil
}

// CreateNewStickerSet creates a new sticker set.
func (b *BotInstance) CreateNewStickerSet(ctx context.Context, req *converter.CreateNewStickerSetRequest) (bool, error) {
	var rawItems []json.RawMessage
	if err := json.Unmarshal(req.Stickers, &rawItems); err != nil {
		return false, err
	}
	items := make([]tg.InputStickerSetItem, 0, len(rawItems))
	for _, raw := range rawItems {
		item, err := parseInputSticker(raw)
		if err != nil {
			return false, err
		}
		items = append(items, item)
	}
	request := &tg.StickersCreateStickerSetRequest{UserID: b.inputUser(req.UserID), Title: req.Title,
		ShortName: req.Name, Stickers: items, TextColor: req.NeedsRepainting}
	switch req.StickerType {
	case "mask":
		request.Masks = true
	case "custom_emoji":
		request.Emojis = true
	}
	_, err := b.raw.StickersCreateStickerSet(ctx, request)
	return err == nil, err
}

// AddStickerToSet adds a new sticker to a set.
func (b *BotInstance) AddStickerToSet(ctx context.Context, req *converter.AddStickerToSetRequest) (bool, error) {
	item, err := parseInputSticker(req.Sticker)
	if err != nil {
		return false, err
	}
	_, err = b.raw.StickersAddStickerToSet(ctx, &tg.StickersAddStickerToSetRequest{Stickerset: &tg.InputStickerSetShortName{ShortName: req.Name}, Sticker: item})
	return err == nil, err
}

// SetStickerPositionInSet moves a sticker in a set to a specific position.
func (b *BotInstance) SetStickerPositionInSet(ctx context.Context, req *converter.SetStickerPositionInSetRequest) (bool, error) {
	document, err := inputDocumentFromFileID(req.Sticker)
	if err != nil {
		return false, err
	}
	_, err = b.raw.StickersChangeStickerPosition(ctx, &tg.StickersChangeStickerPositionRequest{Sticker: document, Position: req.Position})
	return err == nil, err
}

// DeleteStickerFromSet deletes a sticker from a set.
func (b *BotInstance) DeleteStickerFromSet(ctx context.Context, req *converter.DeleteStickerFromSetRequest) (bool, error) {
	document, err := inputDocumentFromFileID(req.Sticker)
	if err != nil {
		return false, err
	}
	_, err = b.raw.StickersRemoveStickerFromSet(ctx, document)
	return err == nil, err
}

// ReplaceStickerInSet replaces an existing sticker in a set.
func (b *BotInstance) ReplaceStickerInSet(ctx context.Context, req *converter.ReplaceStickerInSetRequest) (bool, error) {
	oldDocument, err := inputDocumentFromFileID(req.OldSticker)
	if err != nil {
		return false, err
	}
	item, err := parseInputSticker(req.Sticker)
	if err != nil {
		return false, err
	}
	_, err = b.raw.StickersReplaceSticker(ctx, &tg.StickersReplaceStickerRequest{Sticker: oldDocument, NewSticker: item})
	return err == nil, err
}

// SetStickerEmojiList changes the list of emoji associated with a sticker.
func (b *BotInstance) SetStickerEmojiList(ctx context.Context, req *converter.SetStickerEmojiListRequest) (bool, error) {
	document, err := inputDocumentFromFileID(req.Sticker)
	if err != nil {
		return false, err
	}
	request := &tg.StickersChangeStickerRequest{Sticker: document}
	request.SetEmoji(strings.Join(req.EmojiList, ""))
	_, err = b.raw.StickersChangeSticker(ctx, request)
	return err == nil, err
}

// SetStickerKeywords changes search keywords for a sticker.
func (b *BotInstance) SetStickerKeywords(ctx context.Context, req *converter.SetStickerKeywordsRequest) (bool, error) {
	document, err := inputDocumentFromFileID(req.Sticker)
	if err != nil {
		return false, err
	}
	request := &tg.StickersChangeStickerRequest{Sticker: document}
	request.SetKeywords(strings.Join(req.Keywords, ","))
	_, err = b.raw.StickersChangeSticker(ctx, request)
	return err == nil, err
}

// SetStickerMaskPosition changes the mask position of a mask sticker.
func (b *BotInstance) SetStickerMaskPosition(ctx context.Context, req *converter.SetStickerMaskPositionRequest) (bool, error) {
	document, err := inputDocumentFromFileID(req.Sticker)
	if err != nil {
		return false, err
	}
	coords, ok, err := parseMaskCoords(req.MaskPosition)
	if err != nil {
		return false, err
	}
	request := &tg.StickersChangeStickerRequest{Sticker: document}
	if ok {
		request.SetMaskCoords(coords)
	} else {
		request.SetMaskCoords(tg.MaskCoords{})
	}
	_, err = b.raw.StickersChangeSticker(ctx, request)
	return err == nil, err
}

// SetStickerSetTitle sets the title of a created sticker set.
func (b *BotInstance) SetStickerSetTitle(ctx context.Context, req *converter.SetStickerSetTitleRequest) (bool, error) {
	_, err := b.raw.StickersRenameStickerSet(ctx, &tg.StickersRenameStickerSetRequest{Stickerset: &tg.InputStickerSetShortName{ShortName: req.Name}, Title: req.Title})
	return err == nil, err
}

// SetStickerSetThumbnail sets the thumbnail of a regular or mask sticker set.
func (b *BotInstance) SetStickerSetThumbnail(ctx context.Context, req *converter.SetStickerSetThumbnailRequest) (bool, error) {
	request := &tg.StickersSetStickerSetThumbRequest{Stickerset: &tg.InputStickerSetShortName{ShortName: req.Name}}
	if len(req.ThumbnailData) != 0 {
		document, err := b.uploadStickerDocument(ctx, req.ThumbnailData, "thumbnail", req.Format)
		if err != nil {
			return false, err
		}
		request.SetThumb(&tg.InputDocument{ID: document.ID, AccessHash: document.AccessHash, FileReference: document.FileReference})
	} else if req.Thumbnail != "" {
		document, err := inputDocumentFromFileID(req.Thumbnail)
		if err != nil {
			return false, err
		}
		request.SetThumb(document)
	}
	_, err := b.raw.StickersSetStickerSetThumb(ctx, request)
	return err == nil, err
}

// SetCustomEmojiStickerSetThumbnail sets the thumbnail of a custom emoji sticker set.
func (b *BotInstance) SetCustomEmojiStickerSetThumbnail(ctx context.Context, req *converter.SetCustomEmojiStickerSetThumbnailRequest) (bool, error) {
	request := &tg.StickersSetStickerSetThumbRequest{Stickerset: &tg.InputStickerSetShortName{ShortName: req.Name}}
	id := int64(0)
	if req.CustomEmojiID != "" {
		var err error
		id, err = strconv.ParseInt(req.CustomEmojiID, 10, 64)
		if err != nil {
			return false, err
		}
	}
	request.SetThumbDocumentID(id)
	_, err := b.raw.StickersSetStickerSetThumb(ctx, request)
	return err == nil, err
}

// DeleteStickerSet deletes a sticker set that was created by the bot.
func (b *BotInstance) DeleteStickerSet(ctx context.Context, req *converter.DeleteStickerSetRequest) (bool, error) {
	return b.raw.StickersDeleteStickerSet(ctx, &tg.InputStickerSetShortName{ShortName: req.Name})
}

// SendGame sends a game.
func (b *BotInstance) SendGame(ctx context.Context, req *converter.SendGameRequest) (*converter.Message, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, err
	}
	send := &tg.MessagesSendMediaRequest{Peer: peer, Media: &tg.InputMediaGame{ID: &tg.InputGameShortName{
		BotID: &tg.InputUserSelf{}, ShortName: req.GameShortName,
	}}, Silent: req.DisableNotification, Noforwards: req.ProtectContent}
	if req.ReplyParameters != nil || req.MessageThreadID != 0 {
		reply := &tg.InputReplyToMessage{}
		if req.ReplyParameters != nil {
			reply.ReplyToMsgID = int(req.ReplyParameters.MessageID)
		}
		if req.MessageThreadID != 0 {
			reply.SetTopMsgID(req.MessageThreadID)
		}
		send.SetReplyTo(reply)
	}
	randomID, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	send.RandomID = randomID.Int64()
	if len(req.ReplyMarkup) != 0 {
		markup, err := converter.ParseReplyMarkup(req.ReplyMarkup)
		if err != nil {
			return nil, err
		}
		send.SetReplyMarkup(markup)
	}
	updates, err := b.sendMedia(ctx, req.BusinessConnectionID, send)
	if err != nil {
		return nil, err
	}
	return &converter.Message{MessageID: extractSentMessageID(updates), From: b.GetMe(),
		Chat: converter.Chat{ID: req.ChatID}, Date: int(time.Now().Unix()),
		BusinessConnectionID: req.BusinessConnectionID}, nil
}

// SetGameScore sets the score of the specified user in a game.
func (b *BotInstance) SetGameScore(ctx context.Context, req *converter.SetGameScoreRequest) (interface{}, error) {
	if req.InlineMessageID != "" {
		id, err := decodeInlineMessageID(req.InlineMessageID)
		if err != nil {
			return nil, err
		}
		ok, err := b.raw.MessagesSetInlineGameScore(ctx, &tg.MessagesSetInlineGameScoreRequest{
			EditMessage: !req.DisableEditMessage, Force: req.Force, ID: id,
			UserID: b.inputUser(req.UserID), Score: int(req.Score),
		})
		if err != nil {
			return nil, err
		}
		return ok, nil
	}
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, err
	}
	updates, err := b.raw.MessagesSetGameScore(ctx, &tg.MessagesSetGameScoreRequest{EditMessage: !req.DisableEditMessage,
		Force: req.Force, Peer: peer, ID: int(req.MessageID), UserID: b.inputUser(req.UserID), Score: int(req.Score)})
	if err != nil {
		return nil, err
	}
	messageID := extractSentMessageID(updates)
	if messageID == 0 {
		messageID = req.MessageID
	}
	return &converter.Message{MessageID: messageID, From: b.GetMe(), Chat: converter.Chat{ID: req.ChatID},
		Date: int(time.Now().Unix())}, nil
}

// GetGameHighScores gets data for high score tables.
func (b *BotInstance) GetGameHighScores(ctx context.Context, req *converter.GetGameHighScoresRequest) ([]converter.GameHighScore, error) {
	var result *tg.MessagesHighScores
	var err error
	if req.InlineMessageID != "" {
		id, decodeErr := decodeInlineMessageID(req.InlineMessageID)
		if decodeErr != nil {
			return nil, decodeErr
		}
		result, err = b.raw.MessagesGetInlineGameHighScores(ctx, &tg.MessagesGetInlineGameHighScoresRequest{
			ID: id, UserID: b.inputUser(req.UserID),
		})
	} else {
		peer, resolveErr := b.resolvePeer(req.ChatID)
		if resolveErr != nil {
			return nil, resolveErr
		}
		result, err = b.raw.MessagesGetGameHighScores(ctx, &tg.MessagesGetGameHighScoresRequest{
			Peer: peer, ID: int(req.MessageID), UserID: b.inputUser(req.UserID),
		})
	}
	if err != nil {
		return nil, err
	}
	users := make(map[int64]converter.User, len(result.Users))
	for _, userClass := range result.Users {
		if user, ok := userClass.(*tg.User); ok {
			users[user.ID] = converter.User{ID: user.ID, IsBot: user.Bot, FirstName: user.FirstName,
				LastName: user.LastName, Username: user.Username}
		}
	}
	scores := make([]converter.GameHighScore, 0, len(result.Scores))
	for _, score := range result.Scores {
		scores = append(scores, converter.GameHighScore{Position: score.Pos, User: users[score.UserID], Score: int64(score.Score)})
	}
	return scores, nil
}

// SetPassportDataErrors reports errors related to Telegram Passport data.
func (b *BotInstance) SetPassportDataErrors(ctx context.Context, req *converter.SetPassportDataErrorsRequest) (bool, error) {
	var values []struct {
		Source     string   `json:"source"`
		Type       string   `json:"type"`
		Message    string   `json:"message"`
		FieldName  string   `json:"field_name"`
		DataHash   string   `json:"data_hash"`
		FileHash   string   `json:"file_hash"`
		FileHashes []string `json:"file_hashes"`
	}
	if err := json.Unmarshal(req.Errors, &values); err != nil {
		return false, err
	}
	errors := make([]tg.SecureValueErrorClass, 0, len(values))
	for _, value := range values {
		var valueType tg.SecureValueTypeClass
		switch value.Type {
		case "personal_details":
			valueType = &tg.SecureValueTypePersonalDetails{}
		case "passport":
			valueType = &tg.SecureValueTypePassport{}
		case "driver_license":
			valueType = &tg.SecureValueTypeDriverLicense{}
		case "identity_card":
			valueType = &tg.SecureValueTypeIdentityCard{}
		case "internal_passport":
			valueType = &tg.SecureValueTypeInternalPassport{}
		case "address":
			valueType = &tg.SecureValueTypeAddress{}
		case "utility_bill":
			valueType = &tg.SecureValueTypeUtilityBill{}
		case "bank_statement":
			valueType = &tg.SecureValueTypeBankStatement{}
		case "rental_agreement":
			valueType = &tg.SecureValueTypeRentalAgreement{}
		case "passport_registration":
			valueType = &tg.SecureValueTypePassportRegistration{}
		case "temporary_registration":
			valueType = &tg.SecureValueTypeTemporaryRegistration{}
		case "phone_number":
			valueType = &tg.SecureValueTypePhone{}
		case "email":
			valueType = &tg.SecureValueTypeEmail{}
		default:
			return false, fmt.Errorf("unsupported passport element type %q", value.Type)
		}
		decodeHash := func(encoded string) ([]byte, error) {
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				return nil, fmt.Errorf("invalid base64 hash: %w", err)
			}
			return decoded, nil
		}
		switch value.Source {
		case "data":
			hash, err := decodeHash(value.DataHash)
			if err != nil {
				return false, err
			}
			errors = append(errors, &tg.SecureValueErrorData{Type: valueType, DataHash: hash, Field: value.FieldName, Text: value.Message})
		case "front_side":
			hash, err := decodeHash(value.FileHash)
			if err != nil {
				return false, err
			}
			errors = append(errors, &tg.SecureValueErrorFrontSide{Type: valueType, FileHash: hash, Text: value.Message})
		case "reverse_side":
			hash, err := decodeHash(value.FileHash)
			if err != nil {
				return false, err
			}
			errors = append(errors, &tg.SecureValueErrorReverseSide{Type: valueType, FileHash: hash, Text: value.Message})
		case "selfie":
			hash, err := decodeHash(value.FileHash)
			if err != nil {
				return false, err
			}
			errors = append(errors, &tg.SecureValueErrorSelfie{Type: valueType, FileHash: hash, Text: value.Message})
		case "file":
			hash, err := decodeHash(value.FileHash)
			if err != nil {
				return false, err
			}
			errors = append(errors, &tg.SecureValueErrorFile{Type: valueType, FileHash: hash, Text: value.Message})
		case "translation_file":
			hash, err := decodeHash(value.FileHash)
			if err != nil {
				return false, err
			}
			errors = append(errors, &tg.SecureValueErrorTranslationFile{Type: valueType, FileHash: hash, Text: value.Message})
		case "files", "translation_files":
			hashes := make([][]byte, 0, len(value.FileHashes))
			for _, encoded := range value.FileHashes {
				hash, err := decodeHash(encoded)
				if err != nil {
					return false, err
				}
				hashes = append(hashes, hash)
			}
			if value.Source == "files" {
				errors = append(errors, &tg.SecureValueErrorFiles{Type: valueType, FileHash: hashes, Text: value.Message})
			} else {
				errors = append(errors, &tg.SecureValueErrorTranslationFiles{Type: valueType, FileHash: hashes, Text: value.Message})
			}
		case "unspecified":
			hash, err := decodeHash(value.DataHash)
			if err != nil {
				return false, err
			}
			errors = append(errors, &tg.SecureValueError{Type: valueType, Hash: hash, Text: value.Message})
		default:
			return false, fmt.Errorf("unsupported passport error source %q", value.Source)
		}
	}
	return b.raw.UsersSetSecureValueErrors(ctx, &tg.UsersSetSecureValueErrorsRequest{ID: b.inputUser(req.UserID), Errors: errors})
}

// GetUserChatBoosts gets the list of boosts added to a chat by a user.
func (b *BotInstance) GetUserChatBoosts(ctx context.Context, req *converter.GetUserChatBoostsRequest) (*converter.UserChatBoosts, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, err
	}
	result, err := b.raw.PremiumGetUserBoosts(ctx, &tg.PremiumGetUserBoostsRequest{Peer: peer, UserID: b.inputUser(req.UserID)})
	if err != nil {
		return nil, err
	}
	users := make(map[int64]converter.User, len(result.Users))
	for _, userClass := range result.Users {
		if user, ok := userClass.(*tg.User); ok {
			users[user.ID] = converter.User{ID: user.ID, IsBot: user.Bot, FirstName: user.FirstName,
				LastName: user.LastName, Username: user.Username}
		}
	}
	boosts := make([]converter.ChatBoost, 0, len(result.Boosts))
	for _, boost := range result.Boosts {
		source := "premium"
		if boost.Gift {
			source = "gift_code"
		} else if boost.Giveaway {
			source = "giveaway"
		}
		boosts = append(boosts, converter.ChatBoost{BoostID: boost.ID, AddDate: boost.Date,
			ExpirationDate: boost.Expires, Source: converter.ChatBoostSource{Source: source, User: users[boost.UserID],
				GiveawayMessageID: boost.GiveawayMsgID, IsUnclaimed: boost.Unclaimed, PrizeStarCount: boost.Stars}})
	}
	return &converter.UserChatBoosts{Boosts: boosts}, nil
}

// ReadBusinessConnection acknowledges reading business connection updates.
func (b *BotInstance) ReadBusinessConnection(ctx context.Context, connectionID string) (bool, error) {
	_, err := b.raw.AccountGetBotBusinessConnection(ctx, connectionID)
	return err == nil, err
}

// LogOut logs out from the cloud Bot API server before launching the bot locally.
func (b *BotInstance) LogOut(ctx context.Context) (bool, error) {
	_, err := b.raw.AuthLogOut(ctx)
	return err == nil, err
}

// SetMyProfilePhoto sets profile photo for the current bot.
func (b *BotInstance) SetMyProfilePhoto(ctx context.Context, data []byte) (bool, error) {
	if len(data) == 0 {
		return false, fmt.Errorf("photo upload is required")
	}
	file, err := uploader.NewUploader(b.raw).FromBytes(ctx, "profile-photo.jpg", data)
	if err != nil {
		return false, fmt.Errorf("upload profile photo: %w", err)
	}
	_, err = b.raw.PhotosUploadProfilePhoto(ctx, &tg.PhotosUploadProfilePhotoRequest{
		File: file,
	})
	return err == nil, err
}

// RemoveMyProfilePhoto removes profile photo of the current bot.
func (b *BotInstance) RemoveMyProfilePhoto(ctx context.Context) (bool, error) {
	_, err := b.raw.PhotosUpdateProfilePhoto(ctx, &tg.PhotosUpdateProfilePhotoRequest{
		ID: &tg.InputPhotoEmpty{},
	})
	return err == nil, err
}

// GetMyStarBalance returns current bot's Star balance.
func (b *BotInstance) GetMyStarBalance(ctx context.Context) (*converter.StarAmount, error) {
	status, err := b.raw.PaymentsGetStarsStatus(ctx, &tg.PaymentsGetStarsStatusRequest{
		Peer: &tg.InputPeerSelf{},
	})
	if err != nil {
		return nil, err
	}
	var amount int64
	var nanos int
	if stars, ok := status.Balance.(*tg.StarsAmount); ok {
		amount = stars.Amount
		nanos = stars.Nanos
	}
	return &converter.StarAmount{
		Amount:         amount,
		NanostarAmount: nanos,
	}, nil
}

// DeleteMessageReaction deletes a reaction from a message.
func (b *BotInstance) DeleteMessageReaction(ctx context.Context, req *converter.DeleteMessageReactionRequest) (bool, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}
	_, err = b.raw.MessagesSendReaction(ctx, &tg.MessagesSendReactionRequest{
		Peer:     peer,
		MsgID:    int(req.MessageID),
		Reaction: nil,
	})
	return err == nil, err
}

// DeleteAllMessageReactions clears all reactions from a message.
func (b *BotInstance) DeleteAllMessageReactions(ctx context.Context, req *converter.DeleteAllMessageReactionsRequest) (bool, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}
	_, err = b.raw.MessagesSendReaction(ctx, &tg.MessagesSendReactionRequest{
		Peer:     peer,
		MsgID:    int(req.MessageID),
		Reaction: nil,
	})
	return err == nil, err
}

// SetChatMemberTag sets a custom tag/title for a member in a group/supergroup.
func (b *BotInstance) SetChatMemberTag(ctx context.Context, req *converter.SetChatMemberTagRequest) (bool, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}
	channel, err := inputChannel(peer)
	if err != nil {
		return false, err
	}
	_, err = b.raw.ChannelsEditAdmin(ctx, &tg.ChannelsEditAdminRequest{
		Channel: channel,
		UserID:  b.inputUser(req.UserID),
		AdminRights: tg.ChatAdminRights{
			Other: true,
		},
		Rank: req.Tag,
	})
	return err == nil, err
}

// GetUserProfileAudios returns profile audios for a user.
func (b *BotInstance) GetUserProfileAudios(ctx context.Context, userID int64, offset, limit int) (*converter.UserProfileAudios, error) {
	return nil, fmt.Errorf("getUserProfileAudios is not implemented")
}

// AnswerChatJoinRequestQuery answers a chat join request query.
func (b *BotInstance) AnswerChatJoinRequestQuery(ctx context.Context, req *converter.AnswerChatJoinRequestQueryRequest) (bool, error) {
	return false, fmt.Errorf("answerChatJoinRequestQuery is not implemented")
}

// SendChatJoinRequestWebApp sends a web app url for a chat join request query.
func (b *BotInstance) SendChatJoinRequestWebApp(ctx context.Context, req *converter.SendChatJoinRequestWebAppRequest) (bool, error) {
	return false, fmt.Errorf("sendChatJoinRequestWebApp is not implemented")
}

// AnswerCustomQuery answers a custom query.
func (b *BotInstance) AnswerCustomQuery(ctx context.Context, req *converter.AnswerCustomQueryRequest) (bool, error) {
	return false, fmt.Errorf("answerCustomQuery is not implemented")
}

// SendCustomRequest sends a custom MTProto request.
func (b *BotInstance) SendCustomRequest(ctx context.Context, req *converter.SendCustomRequestRequest) (interface{}, error) {
	return nil, fmt.Errorf("sendCustomRequest is not implemented")
}

// AnswerGuestQuery answers a guest query.
func (b *BotInstance) AnswerGuestQuery(ctx context.Context, req *converter.AnswerGuestQueryRequest) (bool, error) {
	return false, fmt.Errorf("answerGuestQuery is not implemented")
}

// ApproveSuggestedPost approves a suggested post in a channel.
func (b *BotInstance) ApproveSuggestedPost(ctx context.Context, req *converter.ApproveSuggestedPostRequest) (bool, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return false, err
	}
	appReq := &tg.MessagesToggleSuggestedPostApprovalRequest{
		Peer:  peer,
		MsgID: int(req.MessageID),
	}
	if req.SendDate != 0 {
		appReq.SetScheduleDate(req.SendDate)
	}
	_, err = b.raw.MessagesToggleSuggestedPostApproval(ctx, appReq)
	if err != nil {
		return false, err
	}
	return true, nil
}

// DeclineSuggestedPost declines a suggested post in a channel.
func (b *BotInstance) DeclineSuggestedPost(ctx context.Context, req *converter.DeclineSuggestedPostRequest) (bool, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return false, err
	}
	decReq := &tg.MessagesToggleSuggestedPostApprovalRequest{
		Reject: true,
		Peer:   peer,
		MsgID:  int(req.MessageID),
	}
	if req.Comment != "" {
		decReq.SetRejectComment(req.Comment)
	}
	_, err = b.raw.MessagesToggleSuggestedPostApproval(ctx, decReq)
	if err != nil {
		return false, err
	}
	return true, nil
}

// ConvertGiftToStars converts an owned star gift to Telegram Stars.
func (b *BotInstance) ConvertGiftToStars(ctx context.Context, req *converter.ConvertGiftToStarsRequest) (bool, error) {
	msgID, _ := strconv.Atoi(req.OwnedGiftID)
	var giftInput tg.InputSavedStarGiftClass = &tg.InputSavedStarGiftUser{MsgID: msgID}
	if req.OwnedGiftID != "" && msgID == 0 {
		giftInput = &tg.InputSavedStarGiftSlug{Slug: req.OwnedGiftID}
	}
	if req.BusinessConnectionID != "" {
		var res tg.BoolBox
		err := b.invokeBusiness(ctx, req.BusinessConnectionID, &tg.PaymentsConvertStarGiftRequest{Stargift: giftInput}, &res)
		return err == nil, err
	}
	res, err := b.raw.PaymentsConvertStarGift(ctx, giftInput)
	return res, err
}

// UpgradeGift upgrades an owned star gift.
func (b *BotInstance) UpgradeGift(ctx context.Context, req *converter.UpgradeGiftRequest) (interface{}, error) {
	msgID, _ := strconv.Atoi(req.OwnedGiftID)
	var giftInput tg.InputSavedStarGiftClass = &tg.InputSavedStarGiftUser{MsgID: msgID}
	if req.OwnedGiftID != "" && msgID == 0 {
		giftInput = &tg.InputSavedStarGiftSlug{Slug: req.OwnedGiftID}
	}
	upgradeReq := &tg.PaymentsUpgradeStarGiftRequest{
		Stargift:            giftInput,
		KeepOriginalDetails: req.KeepOriginalDetails,
	}
	if req.StarCount > 0 {
		return nil, fmt.Errorf("paid gift upgrades are not implemented; refusing to report a false success")
	}
	if req.BusinessConnectionID != "" {
		var updates tg.UpdatesBox
		if err := b.invokeBusiness(ctx, req.BusinessConnectionID, upgradeReq, &updates); err != nil {
			return nil, err
		}
	} else {
		if _, err := b.raw.PaymentsUpgradeStarGift(ctx, upgradeReq); err != nil {
			return nil, err
		}
	}
	return map[string]interface{}{"ok": true}, nil
}

// TransferGift transfers an owned star gift to another user or channel.
func (b *BotInstance) TransferGift(ctx context.Context, req *converter.TransferGiftRequest) (bool, error) {
	if req.StarCount > 0 {
		return false, fmt.Errorf("paid gift transfers are not implemented; refusing to report a false success")
	}
	msgID, _ := strconv.Atoi(req.OwnedGiftID)
	var giftInput tg.InputSavedStarGiftClass = &tg.InputSavedStarGiftUser{MsgID: msgID}
	if req.OwnedGiftID != "" && msgID == 0 {
		giftInput = &tg.InputSavedStarGiftSlug{Slug: req.OwnedGiftID}
	}
	toPeer, err := b.resolvePeer(req.NewOwnerChatID)
	if err != nil {
		return false, err
	}
	transferReq := &tg.PaymentsTransferStarGiftRequest{
		Stargift: giftInput,
		ToID:     toPeer,
	}
	if req.BusinessConnectionID != "" {
		var updates tg.UpdatesBox
		if err := b.invokeBusiness(ctx, req.BusinessConnectionID, transferReq, &updates); err != nil {
			return false, err
		}
		return true, nil
	}
	_, err = b.raw.PaymentsTransferStarGift(ctx, transferReq)
	return err == nil, err
}

// GetChatGifts returns the list of gifts for a chat.
func (b *BotInstance) GetChatGifts(ctx context.Context, req *converter.GetChatGiftsRequest) (*converter.UserGifts, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	res, err := b.raw.PaymentsGetSavedStarGifts(ctx, &tg.PaymentsGetSavedStarGiftsRequest{
		Peer:                peer,
		ExcludeUnsaved:      req.ExcludeUnsaved,
		ExcludeSaved:        req.ExcludeSaved,
		ExcludeUnlimited:    req.ExcludeUnlimited,
		ExcludeUnique:       req.ExcludeUnique,
		ExcludeUpgradable:   req.ExcludeLimitedUpgradable,
		ExcludeUnupgradable: req.ExcludeLimitedNonUpgradable,
		SortByValue:         req.SortByPrice,
		Offset:              req.Offset,
		Limit:               limit,
	})
	if err != nil {
		return nil, err
	}
	if res.Count > 0 {
		return nil, fmt.Errorf("getChatGifts returned %d gifts, but gift conversion is not implemented", res.Count)
	}
	return &converter.UserGifts{TotalCount: res.Count, Gifts: []converter.OwnedGift{}}, nil
}

// GetUserGifts returns the list of gifts for a user.
func (b *BotInstance) GetUserGifts(ctx context.Context, req *converter.GetUserGiftsRequest) (*converter.UserGifts, error) {
	peer, err := b.resolvePeer(req.UserID)
	if err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	res, err := b.raw.PaymentsGetSavedStarGifts(ctx, &tg.PaymentsGetSavedStarGiftsRequest{
		Peer:                peer,
		ExcludeUnlimited:    req.ExcludeUnlimited,
		ExcludeUnique:       req.ExcludeUnique,
		ExcludeUpgradable:   req.ExcludeLimitedUpgradable,
		ExcludeUnupgradable: req.ExcludeLimitedNonUpgradable,
		SortByValue:         req.SortByPrice,
		Offset:              req.Offset,
		Limit:               limit,
	})
	if err != nil {
		return nil, err
	}
	if res.Count > 0 {
		return nil, fmt.Errorf("getUserGifts returned %d gifts, but gift conversion is not implemented", res.Count)
	}
	return &converter.UserGifts{TotalCount: res.Count, Gifts: []converter.OwnedGift{}}, nil
}

// GetBusinessAccountGifts returns the list of gifts for a business account.
func (b *BotInstance) GetBusinessAccountGifts(ctx context.Context, req *converter.GetBusinessAccountGiftsRequest) (*converter.UserGifts, error) {
	connection, err := b.GetBusinessConnection(ctx, req.BusinessConnectionID)
	if err != nil {
		return nil, err
	}
	peer, err := b.resolvePeer(connection.UserChatID)
	if err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	var res tg.PaymentsSavedStarGifts
	err = b.invokeBusiness(ctx, req.BusinessConnectionID, &tg.PaymentsGetSavedStarGiftsRequest{
		Peer:                peer,
		ExcludeUnsaved:      req.ExcludeUnsaved,
		ExcludeSaved:        req.ExcludeSaved,
		ExcludeUnlimited:    req.ExcludeUnlimited,
		ExcludeUnique:       req.ExcludeUnique,
		ExcludeUpgradable:   req.ExcludeLimitedUpgradable,
		ExcludeUnupgradable: req.ExcludeLimitedNonUpgradable,
		SortByValue:         req.SortByPrice,
		Offset:              req.Offset,
		Limit:               limit,
	}, &res)
	if err != nil {
		return nil, err
	}
	if res.Count > 0 {
		return nil, fmt.Errorf("getBusinessAccountGifts returned %d gifts, but gift conversion is not implemented", res.Count)
	}
	return &converter.UserGifts{TotalCount: res.Count, Gifts: []converter.OwnedGift{}}, nil
}

// GetBusinessAccountStarBalance returns the Star balance of a connected business account.
func (b *BotInstance) GetBusinessAccountStarBalance(ctx context.Context, businessConnectionID string) (*converter.StarAmount, error) {
	connection, err := b.GetBusinessConnection(ctx, businessConnectionID)
	if err != nil {
		return nil, err
	}
	peer, err := b.resolvePeer(connection.UserChatID)
	if err != nil {
		return nil, err
	}
	var res tg.PaymentsStarsStatus
	err = b.invokeBusiness(ctx, businessConnectionID, &tg.PaymentsGetStarsStatusRequest{Peer: peer}, &res)
	if err != nil {
		return nil, err
	}
	var amount int64
	var nanos int
	if stars, ok := res.Balance.(*tg.StarsAmount); ok {
		amount = stars.Amount
		nanos = stars.Nanos
	}
	return &converter.StarAmount{
		Amount:         amount,
		NanostarAmount: nanos,
	}, nil
}

// TransferBusinessAccountStars transfers Stars from a business account.
func (b *BotInstance) TransferBusinessAccountStars(ctx context.Context, req *converter.TransferBusinessAccountStarsRequest) (bool, error) {
	if req.StarCount < 1 || req.StarCount > 10000 {
		return false, fmt.Errorf("star_count must be between 1 and 10000")
	}
	return false, fmt.Errorf("business Stars transfer is not implemented; refusing to report a false success")
}

// SetBusinessAccountGiftSettings updates gift settings for a business account.
func (b *BotInstance) SetBusinessAccountGiftSettings(ctx context.Context, req *converter.SetBusinessAccountGiftSettingsRequest) (bool, error) {
	var accepted converter.AcceptedGiftTypes
	if len(req.AcceptedGiftTypes) == 0 {
		return false, fmt.Errorf("accepted_gift_types is required")
	}
	if err := json.Unmarshal(req.AcceptedGiftTypes, &accepted); err != nil {
		return false, fmt.Errorf("invalid accepted_gift_types: %w", err)
	}
	disallowed := tg.DisallowedGiftsSettings{
		DisallowUnlimitedStargifts:    !accepted.UnlimitedGifts,
		DisallowLimitedStargifts:      !accepted.LimitedGifts,
		DisallowUniqueStargifts:       !accepted.UniqueGifts,
		DisallowPremiumGifts:          !accepted.PremiumSubscription,
		DisallowStargiftsFromChannels: !accepted.GiftsFromChannels,
	}
	settings := tg.GlobalPrivacySettings{}
	settings.SetDisplayGiftsButton(req.ShowGiftButton)
	settings.SetDisallowedGifts(disallowed)
	var result tg.GlobalPrivacySettings
	if err := b.invokeBusiness(ctx, req.BusinessConnectionID, &tg.AccountSetGlobalPrivacySettingsRequest{Settings: settings}, &result); err != nil {
		return false, err
	}
	return true, nil
}

// SetBusinessAccountProfilePhoto sets a profile photo for a business account.
func (b *BotInstance) SetBusinessAccountProfilePhoto(ctx context.Context, req *converter.SetBusinessAccountProfilePhotoRequest) (bool, error) {
	if len(req.PhotoData) == 0 {
		return false, fmt.Errorf("photo upload is required")
	}
	raw, err := b.businessRawClient(ctx, req.BusinessConnectionID)
	if err != nil {
		return false, err
	}
	fileName := req.PhotoFileName
	if fileName == "" {
		if req.PhotoType == "animated" {
			fileName = "profile-photo.mp4"
		} else {
			fileName = "profile-photo.jpg"
		}
	}
	file, err := uploader.NewUploader(raw).FromBytes(ctx, fileName, req.PhotoData)
	if err != nil {
		return false, fmt.Errorf("upload business profile photo: %w", err)
	}
	request := &tg.PhotosUploadProfilePhotoRequest{Fallback: req.IsPublic}
	switch req.PhotoType {
	case "static":
		request.File = file
	case "animated":
		request.Video = file
		request.VideoStartTs = req.MainFrameTimestamp
	default:
		return false, fmt.Errorf("profile photo type must be static or animated")
	}
	var result tg.PhotosPhoto
	if err := b.invokeBusiness(ctx, req.BusinessConnectionID, request, &result); err != nil {
		return false, err
	}
	return true, nil
}

// GetManagedBotToken returns the token of a managed bot.
func (b *BotInstance) GetManagedBotToken(ctx context.Context, userID int64) (string, error) {
	return "", fmt.Errorf("getManagedBotToken is not implemented")
}

// GetManagedBotAccessSettings returns access settings of a managed bot.
func (b *BotInstance) GetManagedBotAccessSettings(ctx context.Context, userID int64) (*converter.ManagedBotAccessSettings, error) {
	return nil, fmt.Errorf("getManagedBotAccessSettings is not implemented")
}

// SetManagedBotAccessSettings updates access settings of a managed bot.
func (b *BotInstance) SetManagedBotAccessSettings(ctx context.Context, req *converter.SetManagedBotAccessSettingsRequest) (bool, error) {
	return false, fmt.Errorf("setManagedBotAccessSettings is not implemented")
}

// ReplaceManagedBotToken replaces token of a managed bot.
func (b *BotInstance) ReplaceManagedBotToken(ctx context.Context, userID int64) (string, error) {
	return "", fmt.Errorf("replaceManagedBotToken is not implemented")
}

// GetUserPersonalChatMessages returns messages from personal chat with a user.
func (b *BotInstance) GetUserPersonalChatMessages(ctx context.Context, userID int64, limit int) ([]converter.Message, error) {
	peer, err := b.resolvePeer(userID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	res, err := b.raw.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:  peer,
		Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	var messages []converter.Message
	switch h := res.(type) {
	case *tg.MessagesMessages:
		entities := converter.NewEntityContext(h.Users, h.Chats)
		for _, m := range h.Messages {
			if conv, err := b.converter.ConvertMessage(m, entities); err == nil && conv != nil {
				messages = append(messages, *conv)
			}
		}
	case *tg.MessagesMessagesSlice:
		entities := converter.NewEntityContext(h.Users, h.Chats)
		for _, m := range h.Messages {
			if conv, err := b.converter.ConvertMessage(m, entities); err == nil && conv != nil {
				messages = append(messages, *conv)
			}
		}
	case *tg.MessagesChannelMessages:
		entities := converter.NewEntityContext(h.Users, h.Chats)
		for _, m := range h.Messages {
			if conv, err := b.converter.ConvertMessage(m, entities); err == nil && conv != nil {
				messages = append(messages, *conv)
			}
		}
	}
	return messages, nil
}

// GiftPremiumSubscription gifts Telegram Premium subscription to a user.
func (b *BotInstance) GiftPremiumSubscription(ctx context.Context, req *converter.GiftPremiumSubscriptionRequest) (bool, error) {
	return false, fmt.Errorf("giftPremiumSubscription is not implemented; refusing to report a false success")
}

// EditUserStarSubscription edits or cancels a star subscription.
func (b *BotInstance) EditUserStarSubscription(ctx context.Context, req *converter.EditUserStarSubscriptionRequest) (bool, error) {
	_, err := b.raw.PaymentsChangeStarsSubscription(ctx, &tg.PaymentsChangeStarsSubscriptionRequest{
		Peer:           &tg.InputPeerSelf{},
		SubscriptionID: req.TelegramPaymentChargeID,
		Canceled:       req.IsCanceled,
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

// RepostStory reposts a story on behalf of a business account or bot.
func (b *BotInstance) RepostStory(ctx context.Context, req *converter.RepostStoryRequest) (interface{}, error) {
	connection, _, err := b.businessConnection(ctx, req.BusinessConnectionID, true)
	if err != nil {
		return nil, err
	}
	peer, err := b.resolvePeer(connection.UserChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve business peer: %w", err)
	}
	fromPeer, err := b.resolvePeer(req.FromChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve from_chat_id: %w", err)
	}
	randomID, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	request := &tg.StoriesSendStoryRequest{
		Peer:         peer,
		Media:        &tg.InputMediaStory{Peer: fromPeer, ID: req.StoryID},
		PrivacyRules: []tg.InputPrivacyRuleClass{&tg.InputPrivacyValueAllowAll{}},
		RandomID:     randomID.Int64(),
		Pinned:       req.PostToChatPage,
		Noforwards:   req.ProtectContent,
	}
	request.SetFwdFromID(fromPeer)
	request.SetFwdFromStory(req.StoryID)
	if req.ActivePeriod != 0 {
		request.SetPeriod(req.ActivePeriod)
	}
	var box tg.UpdatesBox
	if err := b.invokeBusinessDirect(ctx, req.BusinessConnectionID, request, &box); err != nil {
		return nil, err
	}
	storyID := extractStoryID(box.Updates)
	if storyID == 0 {
		return nil, fmt.Errorf("story reposted without story id")
	}
	return map[string]interface{}{"id": storyID, "chat": converter.Chat{ID: connection.UserChatID}, "date": int(time.Now().Unix())}, nil
}

// EditStory edits a story previously posted through a business connection.
func (b *BotInstance) EditStory(ctx context.Context, req *converter.EditStoryRequest) (interface{}, error) {
	connection, err := b.GetBusinessConnection(ctx, req.BusinessConnectionID)
	if err != nil {
		return nil, err
	}
	peer, err := b.resolvePeer(connection.UserChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve business peer: %w", err)
	}
	request := &tg.StoriesEditStoryRequest{
		Peer: peer,
		ID:   req.StoryID,
	}
	if req.Caption != "" {
		caption, entities, err := inlineMessageEntities(req.Caption, req.ParseMode, req.CaptionEntities)
		if err == nil {
			request.SetCaption(caption)
			request.SetEntities(entities)
		}
	}
	var box tg.UpdatesBox
	if err := b.invokeBusinessDirect(ctx, req.BusinessConnectionID, request, &box); err != nil {
		return nil, err
	}
	return map[string]interface{}{"id": req.StoryID, "chat": converter.Chat{ID: connection.UserChatID}, "date": int(time.Now().Unix())}, nil
}

// SavePreparedKeyboardButton generates and saves a prepared keyboard button.
func (b *BotInstance) SavePreparedKeyboardButton(ctx context.Context, req *converter.SavePreparedKeyboardButtonRequest) (*converter.PreparedKeyboardButton, error) {
	return nil, fmt.Errorf("savePreparedKeyboardButton is not implemented")
}

// SendLivePhoto sends a live photo.
func (b *BotInstance) SendLivePhoto(ctx context.Context, req *converter.SendLivePhotoRequest) (*converter.Message, error) {
	sendPhotoReq := &converter.SendPhotoRequest{
		BusinessConnectionID:  req.BusinessConnectionID,
		ChatID:                req.ChatID,
		MessageThreadID:       req.MessageThreadID,
		Photo:                 req.Photo,
		Caption:               req.Caption,
		ParseMode:             req.ParseMode,
		CaptionEntities:       req.CaptionEntities,
		HasSpoiler:            req.HasSpoiler,
		ShowCaptionAboveMedia: req.ShowCaptionAboveMedia,
		DisableNotification:   req.DisableNotification,
		ProtectContent:        req.ProtectContent,
		ReplyParameters:       req.ReplyParameters,
		ReplyMarkup:           req.ReplyMarkup,
		PhotoData:             req.PhotoData,
	}
	return b.SendPhoto(ctx, sendPhotoReq)
}

// SendMessageDraft saves a message draft.
func (b *BotInstance) SendMessageDraft(ctx context.Context, req *converter.SendMessageDraftRequest) (bool, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return false, err
	}
	text, entities, _ := inlineMessageEntities(req.Text, req.ParseMode, req.Entities)
	draftReq := &tg.MessagesSaveDraftRequest{
		Peer:    peer,
		Message: text,
	}
	if len(entities) > 0 {
		draftReq.SetEntities(entities)
	}
	if req.MessageThreadID != 0 {
		draftReq.SetReplyTo(&tg.InputReplyToMessage{ReplyToMsgID: req.MessageThreadID})
	}
	res, err := b.raw.MessagesSaveDraft(ctx, draftReq)
	if err != nil {
		return false, err
	}
	return res, nil
}

// SendRichMessageDraft saves a rich message draft.
func (b *BotInstance) SendRichMessageDraft(ctx context.Context, req *converter.SendRichMessageDraftRequest) (bool, error) {
	return false, fmt.Errorf("sendRichMessageDraft is not implemented")
}

// SendRichMessage sends a rich message.
func (b *BotInstance) SendRichMessage(ctx context.Context, req *converter.SendRichMessageRequest) (*converter.Message, error) {
	return nil, fmt.Errorf("sendRichMessage is not implemented")
}

// SendChecklist sends a checklist message.
func (b *BotInstance) SendChecklist(ctx context.Context, req *converter.SendChecklistRequest) (*converter.Message, error) {
	return nil, fmt.Errorf("sendChecklist is not implemented")
}

// EditMessageChecklist edits a checklist message.
func (b *BotInstance) EditMessageChecklist(ctx context.Context, req *converter.EditMessageChecklistRequest) (interface{}, error) {
	return nil, fmt.Errorf("editMessageChecklist is not implemented")
}

// EditEphemeralMessageText edits text of an ephemeral message.
func (b *BotInstance) EditEphemeralMessageText(ctx context.Context, req *converter.EditEphemeralMessageTextRequest) (bool, error) {
	return false, fmt.Errorf("editEphemeralMessageText is not implemented")
}

// EditEphemeralMessageMedia edits media of an ephemeral message.
func (b *BotInstance) EditEphemeralMessageMedia(ctx context.Context, req *converter.EditEphemeralMessageMediaRequest) (bool, error) {
	return false, fmt.Errorf("editEphemeralMessageMedia is not implemented")
}

// EditEphemeralMessageCaption edits caption of an ephemeral message.
func (b *BotInstance) EditEphemeralMessageCaption(ctx context.Context, req *converter.EditEphemeralMessageCaptionRequest) (bool, error) {
	return false, fmt.Errorf("editEphemeralMessageCaption is not implemented")
}

// EditEphemeralMessageReplyMarkup edits reply markup of an ephemeral message.
func (b *BotInstance) EditEphemeralMessageReplyMarkup(ctx context.Context, req *converter.EditEphemeralMessageReplyMarkupRequest) (bool, error) {
	return false, fmt.Errorf("editEphemeralMessageReplyMarkup is not implemented")
}

// DeleteEphemeralMessage deletes an ephemeral message.
func (b *BotInstance) DeleteEphemeralMessage(ctx context.Context, req *converter.DeleteEphemeralMessageRequest) (bool, error) {
	return false, fmt.Errorf("deleteEphemeralMessage is not implemented")
}
