package converter

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/fileid"
	"github.com/gotd/td/tg"
)

// MTProtoConverter converts raw MTProto objects into Bot API models.
type MTProtoConverter struct {
	selfUserID int64
}

// NewMTProtoConverter creates a converter instance.
func NewMTProtoConverter() *MTProtoConverter {
	return &MTProtoConverter{}
}

// SetSelfUserID lets the converter distinguish my_chat_member from chat_member updates.
func (c *MTProtoConverter) SetSelfUserID(id int64) { c.selfUserID = id }

// ConvertUpdate transforms an MTProto update into a Bot API Update struct.
func (c *MTProtoConverter) ConvertUpdate(updateID int, u tg.UpdateClass, entities *EntityContext) (*Update, error) {
	switch upd := u.(type) {
	case *tg.UpdateNewMessage:
		msg, err := c.ConvertMessage(upd.Message, entities)
		if err != nil {
			return nil, err
		}
		if msg == nil {
			return nil, nil
		}
		return &Update{
			UpdateID: updateID,
			Message:  msg,
		}, nil

	case *tg.UpdateEditMessage:
		msg, err := c.ConvertMessage(upd.Message, entities)
		if err != nil {
			return nil, err
		}
		if msg == nil {
			return nil, nil
		}
		return &Update{
			UpdateID:      updateID,
			EditedMessage: msg,
		}, nil

	case *tg.UpdateNewChannelMessage:
		msg, err := c.ConvertMessage(upd.Message, entities)
		if err != nil {
			return nil, err
		}
		if msg == nil {
			return nil, nil
		}
		result := &Update{UpdateID: updateID}
		if msg.Chat.Type == "channel" {
			result.ChannelPost = msg
		} else {
			result.Message = msg
		}
		return result, nil

	case *tg.UpdateEditChannelMessage:
		msg, err := c.ConvertMessage(upd.Message, entities)
		if err != nil {
			return nil, err
		}
		if msg == nil {
			return nil, nil
		}
		result := &Update{UpdateID: updateID}
		if msg.Chat.Type == "channel" {
			result.EditedChannelPost = msg
		} else {
			result.EditedMessage = msg
		}
		return result, nil

	case *tg.UpdateBotCallbackQuery:
		cbQuery := &CallbackQuery{
			ID:           strconv.FormatInt(upd.QueryID, 10),
			ChatInstance: strconv.FormatInt(upd.ChatInstance, 10),
			Data:         string(upd.Data),
		}
		if entities != nil {
			if u := entities.GetUser(upd.UserID); u != nil {
				cbQuery.From = *u
			}
		}
		if cbQuery.From.ID == 0 {
			cbQuery.From = User{
				ID:        upd.UserID,
				IsBot:     false,
				FirstName: "User",
			}
		}

		var chat Chat
		var chatID int64
		switch p := upd.Peer.(type) {
		case *tg.PeerUser:
			chatID = p.UserID
			chat.Type = "private"
			chat.FirstName = cbQuery.From.FirstName
			chat.LastName = cbQuery.From.LastName
			chat.Username = cbQuery.From.Username
		case *tg.PeerChat:
			chatID = -p.ChatID
			chat.Type = "group"
		case *tg.PeerChannel:
			chatID = -1000000000000 - p.ChannelID
			chat.Type = "supergroup"
		}
		chat.ID = chatID
		if entities != nil {
			if c := entities.GetChat(chatID); c != nil {
				chat = *c
			}
		}

		cbQuery.Message = &Message{
			MessageID: int64(upd.MsgID),
			From:      &cbQuery.From,
			Chat:      chat,
			Date:      int(time.Now().Unix()),
		}

		return &Update{
			UpdateID:      updateID,
			CallbackQuery: cbQuery,
		}, nil

	case *tg.UpdateInlineBotCallbackQuery:
		cbQuery := &CallbackQuery{
			ID:           strconv.FormatInt(upd.QueryID, 10),
			ChatInstance: strconv.FormatInt(upd.ChatInstance, 10),
			Data:         string(upd.Data),
		}
		if entities != nil {
			if u := entities.GetUser(upd.UserID); u != nil {
				cbQuery.From = *u
			}
		}
		if cbQuery.From.ID == 0 {
			cbQuery.From = User{
				ID:        upd.UserID,
				IsBot:     false,
				FirstName: "User",
			}
		}
		cbQuery.InlineMsgID, _ = encodeInlineMessageID(upd.MsgID)
		return &Update{
			UpdateID:      updateID,
			CallbackQuery: cbQuery,
		}, nil

	case *tg.UpdateBotInlineQuery:
		return &Update{UpdateID: updateID, InlineQuery: &InlineQuery{
			ID:       strconv.FormatInt(upd.QueryID, 10),
			From:     userFromContext(upd.UserID, entities),
			Query:    upd.Query,
			Offset:   upd.Offset,
			ChatType: inlineQueryChatType(upd.PeerType),
			Location: convertLocation(upd.Geo),
		}}, nil

	case *tg.UpdateBotInlineSend:
		chosen := &ChosenInlineResult{
			ResultID: upd.ID,
			From:     userFromContext(upd.UserID, entities),
			Location: convertLocation(upd.Geo),
			Query:    upd.Query,
		}
		chosen.InlineMessageID, _ = encodeInlineMessageID(upd.MsgID)
		return &Update{UpdateID: updateID, ChosenInlineResult: chosen}, nil

	case *tg.UpdateBotShippingQuery:
		return &Update{UpdateID: updateID, ShippingQuery: &ShippingQuery{
			ID:              strconv.FormatInt(upd.QueryID, 10),
			From:            userFromContext(upd.UserID, entities),
			InvoicePayload:  string(upd.Payload),
			ShippingAddress: convertShippingAddress(upd.ShippingAddress),
		}}, nil

	case *tg.UpdateBotPrecheckoutQuery:
		query := &PreCheckoutQuery{
			ID:               strconv.FormatInt(upd.QueryID, 10),
			From:             userFromContext(upd.UserID, entities),
			Currency:         upd.Currency,
			TotalAmount:      upd.TotalAmount,
			InvoicePayload:   string(upd.Payload),
			ShippingOptionID: upd.ShippingOptionID,
		}
		if _, ok := upd.GetInfo(); ok {
			query.OrderInfo = convertOrderInfo(upd.Info)
		}
		return &Update{UpdateID: updateID, PreCheckoutQuery: query}, nil

	case *tg.UpdateMessagePoll:
		poll, ok := upd.GetPoll()
		if !ok {
			return nil, nil
		}
		return &Update{UpdateID: updateID, Poll: convertPoll(poll, upd.Results)}, nil

	case *tg.UpdateMessagePollVote:
		answer := &PollAnswer{PollID: strconv.FormatInt(upd.PollID, 10)}
		for _, option := range upd.Options {
			if len(option) != 0 {
				answer.OptionIDs = append(answer.OptionIDs, int(option[0]))
			}
		}
		switch peer := upd.Peer.(type) {
		case *tg.PeerUser:
			user := userFromContext(peer.UserID, entities)
			answer.User = &user
		default:
			chat := chatFromPeer(upd.Peer, entities)
			answer.VoterChat = &chat
		}
		return &Update{UpdateID: updateID, PollAnswer: answer}, nil

	case *tg.UpdateBotChatInviteRequester:
		request := &ChatJoinRequest{
			Chat:       chatFromPeer(upd.Peer, entities),
			From:       userFromContext(upd.UserID, entities),
			UserChatID: upd.UserID,
			Date:       upd.Date,
			Bio:        upd.About,
		}
		if invite, ok := upd.Invite.(*tg.ChatInviteExported); ok {
			request.InviteLink = convertInviteLink(invite, entities)
		}
		return &Update{UpdateID: updateID, ChatJoinRequest: request}, nil

	case *tg.UpdateChatParticipant:
		change := &ChatMemberUpdated{
			Chat:          chatFromPeer(&tg.PeerChat{ChatID: upd.ChatID}, entities),
			From:          userFromContext(upd.ActorID, entities),
			Date:          upd.Date,
			OldChatMember: convertBasicParticipant(upd.PrevParticipant, upd.UserID, entities),
			NewChatMember: convertBasicParticipant(upd.NewParticipant, upd.UserID, entities),
		}
		if invite, ok := upd.Invite.(*tg.ChatInviteExported); ok {
			change.InviteLink = convertInviteLink(invite, entities)
			change.ViaJoinRequest = invite.RequestNeeded
		}
		return c.memberUpdate(updateID, upd.UserID, change), nil

	case *tg.UpdateChannelParticipant:
		change := &ChatMemberUpdated{
			Chat:                    chatFromPeer(&tg.PeerChannel{ChannelID: upd.ChannelID}, entities),
			From:                    userFromContext(upd.ActorID, entities),
			Date:                    upd.Date,
			OldChatMember:           ConvertChannelParticipant(upd.PrevParticipant, upd.UserID, entities),
			NewChatMember:           ConvertChannelParticipant(upd.NewParticipant, upd.UserID, entities),
			ViaChatFolderInviteLink: upd.ViaChatlist,
		}
		if invite, ok := upd.Invite.(*tg.ChatInviteExported); ok {
			change.InviteLink = convertInviteLink(invite, entities)
			change.ViaJoinRequest = invite.RequestNeeded
		}
		return c.memberUpdate(updateID, upd.UserID, change), nil

	case *tg.UpdateBotChatBoost:
		return &Update{UpdateID: updateID, ChatBoost: &ChatBoostUpdated{
			Chat: chatFromPeer(upd.Peer, entities), Boost: convertBoost(upd.Boost, entities),
		}}, nil

	case *tg.UpdateBotMessageReaction:
		reaction := &MessageReactionUpdated{
			Chat:        chatFromPeer(upd.Peer, entities),
			MessageID:   int64(upd.MsgID),
			Date:        upd.Date,
			OldReaction: convertReactions(upd.OldReactions),
			NewReaction: convertReactions(upd.NewReactions),
		}
		switch actor := upd.Actor.(type) {
		case *tg.PeerUser:
			user := userFromContext(actor.UserID, entities)
			reaction.User = &user
		default:
			chat := chatFromPeer(upd.Actor, entities)
			reaction.ActorChat = &chat
		}
		return &Update{UpdateID: updateID, MessageReaction: reaction}, nil

	case *tg.UpdateBotMessageReactions:
		counts := make([]ReactionCount, 0, len(upd.Reactions))
		for _, count := range upd.Reactions {
			if reaction, ok := convertReaction(count.Reaction); ok {
				counts = append(counts, ReactionCount{Type: reaction, TotalCount: count.Count})
			}
		}
		return &Update{UpdateID: updateID, MessageReactionCount: &MessageReactionCountUpdated{
			Chat: chatFromPeer(upd.Peer, entities), MessageID: int64(upd.MsgID), Date: upd.Date, Reactions: counts,
		}}, nil

	case *tg.UpdateBotPurchasedPaidMedia:
		return &Update{UpdateID: updateID, PurchasedPaidMedia: &PaidMediaPurchased{
			From: userFromContext(upd.UserID, entities), PaidMediaPayload: upd.Payload,
		}}, nil

	case *tg.UpdateBotBusinessConnect:
		conn := upd.Connection
		var u User
		if entities != nil {
			if usr := entities.GetUser(conn.UserID); usr != nil {
				u = *usr
			}
		}
		if u.ID == 0 {
			u = User{
				ID:        conn.UserID,
				IsBot:     false,
				FirstName: "User",
			}
		}
		return &Update{
			UpdateID: updateID,
			BusinessConnection: &BusinessConnection{
				ID:         conn.ConnectionID,
				User:       u,
				UserChatID: conn.UserID,
				Date:       conn.Date,
				CanReply:   conn.Rights.Reply,
				IsEnabled:  !conn.Disabled,
				Rights:     ConvertBusinessBotRights(conn.Rights),
			},
		}, nil

	case *tg.UpdateBotNewBusinessMessage:
		msg, err := c.ConvertMessage(upd.Message, entities)
		if err != nil {
			return nil, err
		}
		if msg == nil {
			return nil, nil
		}
		msg.BusinessConnectionID = upd.ConnectionID
		return &Update{
			UpdateID:        updateID,
			BusinessMessage: msg,
		}, nil

	case *tg.UpdateBotEditBusinessMessage:
		msg, err := c.ConvertMessage(upd.Message, entities)
		if err != nil {
			return nil, err
		}
		if msg == nil {
			return nil, nil
		}
		msg.BusinessConnectionID = upd.ConnectionID
		return &Update{
			UpdateID:              updateID,
			EditedBusinessMessage: msg,
		}, nil

	case *tg.UpdateBotDeleteBusinessMessage:
		var chat Chat
		var chatID int64
		switch p := upd.Peer.(type) {
		case *tg.PeerUser:
			chatID = p.UserID
			chat.Type = "private"
		case *tg.PeerChat:
			chatID = -p.ChatID
			chat.Type = "group"
		case *tg.PeerChannel:
			chatID = -1000000000000 - p.ChannelID
			chat.Type = "channel"
		}
		chat.ID = chatID
		if entities != nil {
			if c := entities.GetChat(chatID); c != nil {
				chat = *c
			}
		}
		msgIDs := make([]int64, len(upd.Messages))
		for i, id := range upd.Messages {
			msgIDs[i] = int64(id)
		}
		return &Update{
			UpdateID: updateID,
			DeletedBusinessMessages: &BusinessMessagesDeleted{
				BusinessConnectionID: upd.ConnectionID,
				Chat:                 chat,
				MessageIDs:           msgIDs,
			},
		}, nil
	}

	return nil, nil
}

func userFromContext(id int64, entities *EntityContext) User {
	if entities != nil {
		if user := entities.GetUser(id); user != nil {
			return *user
		}
	}
	return User{ID: id, FirstName: "User"}
}

func chatFromPeer(peer tg.PeerClass, entities *EntityContext) Chat {
	var chat Chat
	switch peer := peer.(type) {
	case *tg.PeerUser:
		chat = Chat{ID: peer.UserID, Type: "private"}
		if user := userFromContext(peer.UserID, entities); user.ID != 0 {
			chat.FirstName, chat.LastName, chat.Username = user.FirstName, user.LastName, user.Username
		}
	case *tg.PeerChat:
		chat = Chat{ID: -peer.ChatID, Type: "group"}
	case *tg.PeerChannel:
		chat = Chat{ID: -1000000000000 - peer.ChannelID, Type: "channel"}
	}
	if entities != nil {
		if resolved := entities.GetChat(chat.ID); resolved != nil {
			return *resolved
		}
	}
	return chat
}

func convertLocation(value tg.GeoPointClass) *Location {
	point, ok := value.(*tg.GeoPoint)
	if !ok {
		return nil
	}
	return &Location{Longitude: point.Long, Latitude: point.Lat, HorizontalAccuracy: float64(point.AccuracyRadius)}
}

func inlineQueryChatType(value tg.InlineQueryPeerTypeClass) string {
	switch value.(type) {
	case *tg.InlineQueryPeerTypeSameBotPM, *tg.InlineQueryPeerTypePM, *tg.InlineQueryPeerTypeBotPM:
		return "sender"
	case *tg.InlineQueryPeerTypeChat:
		return "group"
	case *tg.InlineQueryPeerTypeMegagroup:
		return "supergroup"
	case *tg.InlineQueryPeerTypeBroadcast:
		return "channel"
	default:
		return ""
	}
}

func convertShippingAddress(value tg.PostAddress) ShippingAddress {
	return ShippingAddress{CountryCode: value.CountryISO2, State: value.State, City: value.City,
		StreetLine1: value.StreetLine1, StreetLine2: value.StreetLine2, PostCode: value.PostCode}
}

func convertOrderInfo(value tg.PaymentRequestedInfo) *OrderInfo {
	info := &OrderInfo{Name: value.Name, PhoneNumber: value.Phone, Email: value.Email}
	if _, ok := value.GetShippingAddress(); ok {
		address := convertShippingAddress(value.ShippingAddress)
		info.ShippingAddress = &address
	}
	return info
}

func convertPoll(value tg.Poll, results tg.PollResults) *Poll {
	poll := &Poll{ID: strconv.FormatInt(value.ID, 10), Question: value.Question.Text,
		TotalVoterCount: results.TotalVoters, IsClosed: value.Closed, IsAnonymous: !value.PublicVoters,
		Type: "regular", AllowsMultipleAnswers: value.MultipleChoice, Explanation: results.Solution,
		ExplanationEntities: ConvertMTProtoEntities(results.SolutionEntities), CloseDate: value.CloseDate,
		OpenPeriod: value.ClosePeriod}
	if value.Quiz {
		poll.Type = "quiz"
	}
	for index, answerClass := range value.Answers {
		answer, ok := answerClass.(*tg.PollAnswer)
		if !ok {
			continue
		}
		option := PollOption{Text: answer.Text.Text}
		for _, voters := range results.Results {
			if bytes.Equal(voters.Option, answer.Option) {
				option.VoterCount = voters.Voters
				if voters.Correct {
					correct := index
					poll.CorrectOptionID = &correct
				}
				break
			}
		}
		poll.Options = append(poll.Options, option)
	}
	return poll
}

// ConvertPoll transforms an MTProto poll and its current results into Bot API form.
func ConvertPoll(value tg.Poll, results tg.PollResults) *Poll {
	return convertPoll(value, results)
}

func convertReaction(value tg.ReactionClass) (ReactionType, bool) {
	switch value := value.(type) {
	case *tg.ReactionEmoji:
		return ReactionType{Type: "emoji", Emoji: value.Emoticon}, true
	case *tg.ReactionCustomEmoji:
		return ReactionType{Type: "custom_emoji", CustomEmojiID: strconv.FormatInt(value.DocumentID, 10)}, true
	case *tg.ReactionPaid:
		return ReactionType{Type: "paid"}, true
	default:
		return ReactionType{}, false
	}
}

func convertReactions(values []tg.ReactionClass) []ReactionType {
	result := make([]ReactionType, 0, len(values))
	for _, value := range values {
		if reaction, ok := convertReaction(value); ok {
			result = append(result, reaction)
		}
	}
	return result
}

func convertInviteLink(value *tg.ChatInviteExported, entities *EntityContext) *ChatInviteLinkInfo {
	return &ChatInviteLinkInfo{InviteLink: value.Link, Creator: userFromContext(value.AdminID, entities),
		CreatesJoinRequest: value.RequestNeeded, IsPrimary: value.Permanent, IsRevoked: value.Revoked,
		Name: value.Title, ExpireDate: value.ExpireDate, MemberLimit: value.UsageLimit}
}

func encodeInlineMessageID(value tg.InputBotInlineMessageIDClass) (string, error) {
	if value == nil {
		return "", nil
	}
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
	default:
		return "", fmt.Errorf("unexpected inline message id type %T", value)
	}
	return base64.RawURLEncoding.EncodeToString(buffer.Buf), nil
}

func (c *MTProtoConverter) memberUpdate(updateID int, affectedUserID int64, value *ChatMemberUpdated) *Update {
	result := &Update{UpdateID: updateID}
	if affectedUserID == c.selfUserID && c.selfUserID != 0 {
		result.MyChatMember = value
	} else {
		result.ChatMember = value
	}
	return result
}

func participantUser(id int64, entities *EntityContext) *User {
	user := userFromContext(id, entities)
	return &user
}

func convertBasicParticipant(value tg.ChatParticipantClass, userID int64, entities *EntityContext) ChatMember {
	member := ChatMember{Status: "left", User: participantUser(userID, entities)}
	switch participant := value.(type) {
	case *tg.ChatParticipant:
		member.Status = "member"
		member.User = participantUser(participant.UserID, entities)
	case *tg.ChatParticipantAdmin:
		member.Status = "administrator"
		member.User = participantUser(participant.UserID, entities)
		member.CanManageChat = true
	case *tg.ChatParticipantCreator:
		member.Status = "creator"
		member.User = participantUser(participant.UserID, entities)
		member.IsAnonymous = false
	}
	return member
}

// ConvertBasicParticipant converts a legacy basic-group participant into Bot API form.
func ConvertBasicParticipant(value tg.ChatParticipantClass, userID int64, entities *EntityContext) ChatMember {
	return convertBasicParticipant(value, userID, entities)
}

func applyAdminRights(member *ChatMember, rights tg.ChatAdminRights) {
	member.IsAnonymous = rights.Anonymous
	member.CanManageChat = true
	member.CanDeleteMessages = rights.DeleteMessages
	member.CanManageVideoChats = rights.ManageCall
	member.CanRestrictMembers = rights.BanUsers
	member.CanPromoteMembers = rights.AddAdmins
	member.CanChangeInfo = rights.ChangeInfo
	member.CanInviteUsers = rights.InviteUsers
	member.CanPostStories = rights.PostStories
	member.CanEditStories = rights.EditStories
	member.CanDeleteStories = rights.DeleteStories
	member.CanPostMessages = rights.PostMessages
	member.CanEditMessages = rights.EditMessages
	member.CanPinMessages = rights.PinMessages
	member.CanManageTopics = rights.ManageTopics
}

// ConvertChannelParticipant converts an MTProto channel participant into Bot API form.
func ConvertChannelParticipant(value tg.ChannelParticipantClass, userID int64, entities *EntityContext) ChatMember {
	member := ChatMember{Status: "left", User: participantUser(userID, entities)}
	switch participant := value.(type) {
	case *tg.ChannelParticipant:
		member.Status = "member"
		member.User = participantUser(participant.UserID, entities)
	case *tg.ChannelParticipantSelf:
		member.Status = "member"
		member.User = participantUser(participant.UserID, entities)
	case *tg.ChannelParticipantCreator:
		member.Status = "creator"
		member.User = participantUser(participant.UserID, entities)
		member.CustomTitle = participant.Rank
		applyAdminRights(&member, participant.AdminRights)
	case *tg.ChannelParticipantAdmin:
		member.Status = "administrator"
		member.User = participantUser(participant.UserID, entities)
		member.CustomTitle = participant.Rank
		member.CanBeEdited = participant.CanEdit
		applyAdminRights(&member, participant.AdminRights)
	case *tg.ChannelParticipantBanned:
		member.User = participantUser(userID, entities)
		if peer, ok := participant.Peer.(*tg.PeerUser); ok {
			member.User = participantUser(peer.UserID, entities)
		}
		member.UntilDate = participant.BannedRights.UntilDate
		if participant.BannedRights.ViewMessages {
			member.Status = "kicked"
		} else {
			member.Status = "restricted"
			member.IsMember = !participant.Left
			member.CanSendMessages = !participant.BannedRights.SendMessages && !participant.BannedRights.SendPlain
			member.CanSendAudios = !participant.BannedRights.SendMedia && !participant.BannedRights.SendAudios
			member.CanSendDocuments = !participant.BannedRights.SendMedia && !participant.BannedRights.SendDocs
			member.CanSendPhotos = !participant.BannedRights.SendMedia && !participant.BannedRights.SendPhotos
			member.CanSendVideos = !participant.BannedRights.SendMedia && !participant.BannedRights.SendVideos
			member.CanSendVideoNotes = !participant.BannedRights.SendMedia && !participant.BannedRights.SendRoundvideos
			member.CanSendVoiceNotes = !participant.BannedRights.SendMedia && !participant.BannedRights.SendVoices
			member.CanSendPolls = !participant.BannedRights.SendPolls
			member.CanSendOtherMessages = !participant.BannedRights.SendStickers && !participant.BannedRights.SendGifs && !participant.BannedRights.SendGames && !participant.BannedRights.SendInline
			member.CanAddWebPagePreviews = !participant.BannedRights.EmbedLinks
			member.CanChangeInfo = !participant.BannedRights.ChangeInfo
			member.CanInviteUsers = !participant.BannedRights.InviteUsers
			member.CanPinMessages = !participant.BannedRights.PinMessages
			member.CanManageTopics = !participant.BannedRights.ManageTopics
		}
	case *tg.ChannelParticipantLeft:
		if peer, ok := participant.Peer.(*tg.PeerUser); ok {
			member.User = participantUser(peer.UserID, entities)
		}
	}
	return member
}

func convertBoost(value tg.Boost, entities *EntityContext) ChatBoost {
	source := "premium"
	if value.Giveaway {
		source = "giveaway"
	} else if value.Gift {
		source = "gift_code"
	}
	return ChatBoost{
		BoostID: value.ID, AddDate: value.Date, ExpirationDate: value.Expires,
		Source: ChatBoostSource{Source: source, User: userFromContext(value.UserID, entities),
			GiveawayMessageID: value.GiveawayMsgID, IsUnclaimed: value.Unclaimed, PrizeStarCount: value.Stars},
	}
}

// EntityContext holds users and chats from an updates container for fast resolution.
type EntityContext struct {
	users map[int64]*User
	chats map[int64]*Chat
}

// NewEntityContext creates an EntityContext from MTProto entities.
func NewEntityContext(users []tg.UserClass, chats []tg.ChatClass) *EntityContext {
	ctx := &EntityContext{
		users: make(map[int64]*User),
		chats: make(map[int64]*Chat),
	}

	for _, u := range users {
		if user, ok := u.(*tg.User); ok {
			ctx.users[user.ID] = &User{
				ID:           user.ID,
				IsBot:        user.Bot,
				FirstName:    user.FirstName,
				LastName:     user.LastName,
				Username:     user.Username,
				LanguageCode: user.LangCode,
				IsPremium:    user.Premium,
			}
		}
	}

	for _, c := range chats {
		switch chat := c.(type) {
		case *tg.Channel:
			chatType := "channel"
			if chat.Megagroup {
				chatType = "supergroup"
			}
			botApiChatID := -1000000000000 - chat.ID
			ctx.chats[botApiChatID] = &Chat{
				ID:       botApiChatID,
				Type:     chatType,
				Title:    chat.Title,
				Username: chat.Username,
				IsForum:  chat.Forum,
			}
		case *tg.Chat:
			botApiChatID := -chat.ID
			ctx.chats[botApiChatID] = &Chat{
				ID:    botApiChatID,
				Type:  "group",
				Title: chat.Title,
			}
		}
	}

	return ctx
}

// GetUser returns user by ID.
func (e *EntityContext) GetUser(id int64) *User {
	if e == nil {
		return nil
	}
	return e.users[id]
}

// GetChat returns chat by Bot API ID.
func (e *EntityContext) GetChat(id int64) *Chat {
	if e == nil {
		return nil
	}
	return e.chats[id]
}

// ConvertMessage transforms an MTProto MessageClass into a Bot API Message.
func (c *MTProtoConverter) ConvertMessage(m tg.MessageClass, entities *EntityContext) (*Message, error) {
	if service, ok := m.(*tg.MessageService); ok {
		return convertServiceMessage(service, entities), nil
	}
	msg, ok := m.(*tg.Message)
	if !ok {
		return nil, nil
	}

	var chat Chat
	var from *User

	// 1. Resolve Peer
	switch peer := msg.PeerID.(type) {
	case *tg.PeerUser:
		chat = Chat{
			ID:   peer.UserID,
			Type: "private",
		}
		if entities != nil {
			if u := entities.GetUser(peer.UserID); u != nil {
				chat.FirstName = u.FirstName
				chat.LastName = u.LastName
				chat.Username = u.Username
			}
		}
	case *tg.PeerChannel:
		botApiChatID := -1000000000000 - peer.ChannelID
		chat = Chat{
			ID:   botApiChatID,
			Type: "channel",
		}
		if entities != nil {
			if ch := entities.GetChat(botApiChatID); ch != nil {
				chat = *ch
			}
		}
	case *tg.PeerChat:
		botApiChatID := -peer.ChatID
		chat = Chat{
			ID:   botApiChatID,
			Type: "group",
		}
		if entities != nil {
			if ch := entities.GetChat(botApiChatID); ch != nil {
				chat = *ch
			}
		}
	default:
		return nil, fmt.Errorf("unsupported peer type: %T", msg.PeerID)
	}

	// 2. Resolve From
	if msg.FromID != nil {
		if peerUser, ok := msg.FromID.(*tg.PeerUser); ok {
			if entities != nil {
				from = entities.GetUser(peerUser.UserID)
			}
			if from == nil {
				from = &User{ID: peerUser.UserID}
			}
		}
	} else if chat.Type == "private" {
		from = &User{
			ID:        chat.ID,
			FirstName: chat.FirstName,
			LastName:  chat.LastName,
			Username:  chat.Username,
		}
	}

	var replyToMsg *Message
	if msg.ReplyTo != nil {
		if header, ok := msg.ReplyTo.(*tg.MessageReplyHeader); ok && header.ReplyToMsgID != 0 {
			replyToMsg = &Message{
				MessageID: int64(header.ReplyToMsgID),
			}
		}
	}

	var msgEntities []MessageEntity
	if len(msg.Entities) > 0 {
		msgEntities = ConvertMTProtoEntities(msg.Entities)
	}

	result := &Message{
		MessageID:      int64(msg.ID),
		From:           from,
		Chat:           chat,
		Date:           msg.Date,
		ReplyToMessage: replyToMsg,
	}
	if msg.GroupedID != 0 {
		result.MediaGroupID = strconv.FormatInt(msg.GroupedID, 10)
	}
	if msg.EditDate != 0 {
		result.EditDate = msg.EditDate
	}
	if msg.PostAuthor != "" {
		result.AuthorSignature = msg.PostAuthor
	}
	if msg.Noforwards {
		result.HasProtectedContent = true
	}
	if msg.InvertMedia {
		result.ShowCaptionAboveMedia = true
	}
	if msg.ViaBotID != 0 {
		result.ViaBot = &User{ID: msg.ViaBotID, IsBot: true}
	}
	if header, ok := msg.ReplyTo.(*tg.MessageReplyHeader); ok {
		if header.ReplyToTopID != 0 {
			result.MessageThreadID = int64(header.ReplyToTopID)
		}
		if header.ForumTopic {
			result.IsTopicMessage = true
		}
	}
	if msg.Media == nil {
		result.Text = msg.Message
		result.Entities = msgEntities
	} else {
		result.Caption = msg.Message
		result.CaptionEntities = msgEntities
		applyMessageMedia(result, msg.Media)
	}
	if msg.ReplyMarkup != nil {
		result.ReplyMarkup = ConvertMTProtoReplyMarkup(msg.ReplyMarkup)
	}
	return result, nil
}

func convertServiceMessage(service *tg.MessageService, entities *EntityContext) *Message {
	message := &Message{MessageID: int64(service.ID), Chat: chatFromPeer(service.PeerID, entities), Date: service.Date}
	if peer, ok := service.FromID.(*tg.PeerUser); ok {
		user := userFromContext(peer.UserID, entities)
		message.From = &user
	}
	if message.From == nil && message.Chat.Type == "private" {
		user := userFromContext(message.Chat.ID, entities)
		message.From = &user
	}
	switch action := service.Action.(type) {
	case *tg.MessageActionChatCreate:
		message.GroupChatCreated = true
		message.NewChatTitle = action.Title
		for _, id := range action.Users {
			message.NewChatMembers = append(message.NewChatMembers, userFromContext(id, entities))
		}
	case *tg.MessageActionChatAddUser:
		for _, id := range action.Users {
			message.NewChatMembers = append(message.NewChatMembers, userFromContext(id, entities))
		}
	case *tg.MessageActionChatDeleteUser:
		user := userFromContext(action.UserID, entities)
		message.LeftChatMember = &user
	case *tg.MessageActionChatEditTitle:
		message.NewChatTitle = action.Title
	case *tg.MessageActionChatEditPhoto:
		if photo, ok := action.Photo.(*tg.Photo); ok {
			message.NewChatPhoto = photoSizes(photo)
		}
	case *tg.MessageActionChatDeletePhoto:
		message.DeleteChatPhoto = true
	case *tg.MessageActionChannelCreate:
		message.NewChatTitle = action.Title
		if message.Chat.Type == "supergroup" {
			message.SupergroupChatCreated = true
		} else {
			message.ChannelChatCreated = true
		}
	case *tg.MessageActionChatMigrateTo:
		message.MigrateToChatID = -1000000000000 - action.ChannelID
	case *tg.MessageActionChannelMigrateFrom:
		message.MigrateFromChatID = -action.ChatID
	case *tg.MessageActionPinMessage:
		if header, ok := service.ReplyTo.(*tg.MessageReplyHeader); ok {
			message.PinnedMessage = &Message{MessageID: int64(header.ReplyToMsgID), Chat: message.Chat}
		}
	case *tg.MessageActionSetMessagesTTL:
		message.MessageAutoDeleteTimerChanged = &MessageAutoDeleteTimerChanged{MessageAutoDeleteTime: action.Period}
	}
	if header, ok := service.ReplyTo.(*tg.MessageReplyHeader); ok && header.ReplyToMsgID != 0 {
		message.ReplyToMessage = &Message{MessageID: int64(header.ReplyToMsgID), Chat: message.Chat}
	}
	return message
}

func photoSizes(photo *tg.Photo) []PhotoSize {
	result := make([]PhotoSize, 0, len(photo.Sizes))
	for _, class := range photo.Sizes {
		var kind string
		var width, height, size int
		switch value := class.(type) {
		case *tg.PhotoSize:
			kind, width, height, size = value.Type, value.W, value.H, value.Size
		case *tg.PhotoSizeProgressive:
			kind, width, height = value.Type, value.W, value.H
			if len(value.Sizes) != 0 {
				size = value.Sizes[len(value.Sizes)-1]
			}
		case *tg.PhotoCachedSize:
			kind, width, height, size = value.Type, value.W, value.H, len(value.Bytes)
		default:
			continue
		}
		if kind == "" {
			continue
		}
		encoded, err := fileid.EncodeFileID(fileid.FromPhoto(photo, []rune(kind)[0]))
		if err != nil {
			continue
		}
		result = append(result, PhotoSize{FileID: encoded, FileUniqueID: fmt.Sprintf("%d_%s", photo.ID, kind),
			Width: width, Height: height, FileSize: size})
	}
	return result
}

func documentFile(document *tg.Document) (string, string) {
	encoded, err := fileid.EncodeFileID(fileid.FromDocument(document))
	if err != nil {
		return "", strconv.FormatInt(document.ID, 10)
	}
	return encoded, strconv.FormatInt(document.ID, 10)
}

func applyDocumentMedia(message *Message, document *tg.Document, media *tg.MessageMediaDocument) {
	fileID, uniqueID := documentFile(document)
	base := Document{FileID: fileID, FileUniqueID: uniqueID, MimeType: document.MimeType, FileSize: document.Size}
	var audio *tg.DocumentAttributeAudio
	var video *tg.DocumentAttributeVideo
	var sticker *tg.DocumentAttributeSticker
	isAnimated := false
	isCustomEmoji := false
	for _, class := range document.Attributes {
		switch value := class.(type) {
		case *tg.DocumentAttributeFilename:
			base.FileName = value.FileName
		case *tg.DocumentAttributeAudio:
			audio = value
		case *tg.DocumentAttributeVideo:
			video = value
		case *tg.DocumentAttributeSticker:
			sticker = value
		case *tg.DocumentAttributeAnimated:
			isAnimated = true
		case *tg.DocumentAttributeCustomEmoji:
			isCustomEmoji = true
		}
	}
	if sticker != nil || isCustomEmoji {
		value := &Sticker{FileID: fileID, FileUniqueID: uniqueID, Type: "regular", FileSize: int(document.Size),
			IsAnimated: document.MimeType == "application/x-tgsticker", IsVideo: document.MimeType == "video/webm"}
		if sticker != nil {
			value.Emoji = sticker.Alt
			if set, ok := sticker.Stickerset.(*tg.InputStickerSetShortName); ok {
				value.SetName = set.ShortName
			}
		}
		if isCustomEmoji {
			value.Type = "custom_emoji"
			value.CustomEmojiID = uniqueID
		}
		if video != nil {
			value.Width, value.Height = video.W, video.H
		}
		message.Sticker = value
		return
	}
	if video != nil {
		value := Video{FileID: fileID, FileUniqueID: uniqueID, Width: video.W, Height: video.H,
			Duration: int(video.Duration), FileName: base.FileName, MimeType: base.MimeType, FileSize: base.FileSize}
		if media.Round || video.RoundMessage {
			message.VideoNote = &VideoNote{FileID: fileID, FileUniqueID: uniqueID, Length: video.W,
				Duration: int(video.Duration), FileSize: document.Size}
		} else if isAnimated || document.MimeType == "image/gif" {
			animation := Animation(value)
			message.Animation = &animation
		} else {
			message.Video = &value
		}
		return
	}
	if audio != nil {
		if media.Voice || audio.Voice {
			message.Voice = &Voice{FileID: fileID, FileUniqueID: uniqueID, Duration: audio.Duration,
				MimeType: document.MimeType, FileSize: document.Size}
		} else {
			message.Audio = &Audio{FileID: fileID, FileUniqueID: uniqueID, Duration: audio.Duration,
				Performer: audio.Performer, Title: audio.Title, FileName: base.FileName,
				MimeType: document.MimeType, FileSize: document.Size}
		}
		return
	}
	message.Document = &base
}

func applyMessageMedia(message *Message, class tg.MessageMediaClass) {
	switch media := class.(type) {
	case *tg.MessageMediaPhoto:
		message.HasMediaSpoiler = media.Spoiler
		if photo, ok := media.Photo.(*tg.Photo); ok {
			message.Photo = photoSizes(photo)
		}
	case *tg.MessageMediaDocument:
		message.HasMediaSpoiler = media.Spoiler
		if document, ok := media.Document.(*tg.Document); ok {
			applyDocumentMedia(message, document, media)
		}
	case *tg.MessageMediaGeo:
		message.Location = convertLocation(media.Geo)
	case *tg.MessageMediaGeoLive:
		message.Location = convertLocation(media.Geo)
		if message.Location != nil {
			message.Location.LivePeriod = media.Period
			message.Location.Heading = media.Heading
			message.Location.ProximityAlertRadius = media.ProximityNotificationRadius
		}
	case *tg.MessageMediaVenue:
		if location := convertLocation(media.Geo); location != nil {
			venue := &Venue{Location: *location, Title: media.Title, Address: media.Address}
			if media.Provider == "gplaces" {
				venue.GooglePlaceID, venue.GooglePlaceType = media.VenueID, media.VenueType
			} else {
				venue.FoursquareID, venue.FoursquareType = media.VenueID, media.VenueType
			}
			message.Venue = venue
		}
	case *tg.MessageMediaContact:
		message.Contact = &Contact{PhoneNumber: media.PhoneNumber, FirstName: media.FirstName,
			LastName: media.LastName, UserID: media.UserID, VCard: media.Vcard}
	case *tg.MessageMediaPoll:
		message.Poll = convertPoll(media.Poll, media.Results)
	case *tg.MessageMediaDice:
		message.Dice = &Dice{Emoji: media.Emoticon, Value: media.Value}
	}
}

// ConvertShortMessage transforms a tg.UpdateShortMessage into a Bot API Update.
func (c *MTProtoConverter) ConvertShortMessage(updateID int, upd *tg.UpdateShortMessage) *Update {
	msg := &Message{
		MessageID: int64(upd.ID),
		Date:      upd.Date,
		Chat: Chat{
			ID:   upd.UserID,
			Type: "private",
		},
		From: &User{
			ID: upd.UserID,
		},
		Text: upd.Message,
	}
	if len(upd.Entities) > 0 {
		msg.Entities = ConvertMTProtoEntities(upd.Entities)
	}
	if upd.ReplyTo != nil {
		if header, ok := upd.ReplyTo.(*tg.MessageReplyHeader); ok && header.ReplyToMsgID != 0 {
			msg.ReplyToMessage = &Message{MessageID: int64(header.ReplyToMsgID)}
		}
	}
	return &Update{
		UpdateID: updateID,
		Message:  msg,
	}
}

// ConvertShortChatMessage transforms a tg.UpdateShortChatMessage into a Bot API Update.
func (c *MTProtoConverter) ConvertShortChatMessage(updateID int, upd *tg.UpdateShortChatMessage) *Update {
	msg := &Message{
		MessageID: int64(upd.ID),
		Date:      upd.Date,
		Chat: Chat{
			ID:   -upd.ChatID,
			Type: "group",
		},
		From: &User{
			ID: upd.FromID,
		},
		Text: upd.Message,
	}
	if len(upd.Entities) > 0 {
		msg.Entities = ConvertMTProtoEntities(upd.Entities)
	}
	if upd.ReplyTo != nil {
		if header, ok := upd.ReplyTo.(*tg.MessageReplyHeader); ok && header.ReplyToMsgID != 0 {
			msg.ReplyToMessage = &Message{MessageID: int64(header.ReplyToMsgID)}
		}
	}
	return &Update{
		UpdateID: updateID,
		Message:  msg,
	}
}

// ConvertBusinessBotRights converts MTProto tg.BusinessBotRights to Bot API BusinessBotRights.
func ConvertBusinessBotRights(r tg.BusinessBotRights) *BusinessBotRights {
	return &BusinessBotRights{
		CanReply:                   r.Reply,
		CanReadMessages:            r.ReadMessages,
		CanDeleteSentMessages:      r.DeleteSentMessages,
		CanDeleteOutgoingMessages:  r.DeleteSentMessages,
		CanDeleteAllMessages:       r.DeleteReceivedMessages,
		CanEditName:                r.EditName,
		CanEditBio:                 r.EditBio,
		CanEditProfilePhoto:        r.EditProfilePhoto,
		CanEditUsername:            r.EditUsername,
		CanChangeGiftSettings:      r.ChangeGiftSettings,
		CanViewGiftsAndStars:       r.ViewGifts,
		CanConvertGiftsToStars:     r.SellGifts,
		CanTransferAndUpgradeGifts: r.TransferAndUpgradeGifts,
		CanTransferStars:           r.TransferStars,
		CanManageStories:           r.ManageStories,
	}
}
