package api

import (
	"context"
	"io"
	"time"

	"telego-bot-api/internal/botmanager"
	"telego-bot-api/internal/converter"

	"github.com/valyala/fasthttp"
)

// ------------------------------------------------------------------------------------------------
// Chat & Member Handlers
// ------------------------------------------------------------------------------------------------

func (s *Server) handleGetChat(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.GetChatRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	if req.ChatID == 0 {
		s.respondError(ctx, 400, "Bad Request: chat_id is required")
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	chat, err := bot.GetChat(c, req.ChatID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, chat)
}

func (s *Server) handleGetChatAdministrators(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.GetChatAdministratorsRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	admins, err := bot.GetChatAdministrators(c, req.ChatID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, admins)
}

func (s *Server) handleGetChatMemberCount(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.GetChatMemberCountRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	count, err := bot.GetChatMemberCount(c, req.ChatID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, count)
}

func (s *Server) handleBanChatMember(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.BanChatMemberRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.BanChatMember(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleUnbanChatMember(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.UnbanChatMemberRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.UnbanChatMember(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleRestrictChatMember(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.RestrictChatMemberRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.RestrictChatMember(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handlePromoteChatMember(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.PromoteChatMemberRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.PromoteChatMember(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleSetChatAdministratorCustomTitle(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetChatAdministratorCustomTitleRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.SetChatAdministratorCustomTitle(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleBanChatSenderChat(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.BanChatSenderChatRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.BanChatSenderChat(c, req.ChatID, req.SenderChatID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleUnbanChatSenderChat(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.UnbanChatSenderChatRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.UnbanChatSenderChat(c, req.ChatID, req.SenderChatID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleSetChatPermissions(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetChatPermissionsRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.SetChatPermissions(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleExportChatInviteLink(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.ExportChatInviteLinkRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	link, err := bot.ExportChatInviteLink(c, req.ChatID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, link)
}

func (s *Server) handleCreateChatInviteLink(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.CreateChatInviteLinkRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	link, err := bot.CreateChatInviteLink(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, link)
}

func (s *Server) handleEditChatInviteLink(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.EditChatInviteLinkRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	link, err := bot.EditChatInviteLink(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, link)
}

func (s *Server) handleRevokeChatInviteLink(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.RevokeChatInviteLinkRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	link, err := bot.RevokeChatInviteLink(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, link)
}

func (s *Server) handleApproveChatJoinRequest(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.ApproveChatJoinRequestRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.ApproveChatJoinRequest(c, req.ChatID, req.UserID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleDeclineChatJoinRequest(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.DeclineChatJoinRequestRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.DeclineChatJoinRequest(c, req.ChatID, req.UserID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleSetChatPhoto(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetChatPhotoRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}

	if data, _ := ExtractFileFromRequest(ctx, "photo", req.Photo); len(data) > 0 {
		req.PhotoData = data
	}

	c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	ok, err := bot.SetChatPhoto(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleDeleteChatPhoto(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.DeleteChatPhotoRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.DeleteChatPhoto(c, req.ChatID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleSetChatTitle(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetChatTitleRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.SetChatTitle(c, req.ChatID, req.Title)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleSetChatDescription(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetChatDescriptionRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.SetChatDescription(c, req.ChatID, req.Description)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleLeaveChat(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.LeaveChatRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.LeaveChat(c, req.ChatID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleSetChatMenuButton(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetChatMenuButtonRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.SetChatMenuButton(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleGetChatMenuButton(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.GetChatMenuButtonRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	btn, err := bot.GetChatMenuButton(c, req.ChatID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, btn)
}

func (s *Server) handleSetMyDefaultAdministratorRights(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetMyDefaultAdministratorRightsRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.SetMyDefaultAdministratorRights(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleGetMyDefaultAdministratorRights(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.GetMyDefaultAdministratorRightsRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rights, err := bot.GetMyDefaultAdministratorRights(c, req.ForChannels)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, rights)
}

// ------------------------------------------------------------------------------------------------
// Messages, Forwarding & Reactions Handlers
// ------------------------------------------------------------------------------------------------

func (s *Server) handleForwardMessage(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.ForwardMessageRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	msg, err := bot.ForwardMessage(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

func (s *Server) handleForwardMessages(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.ForwardMessagesRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	ids, err := bot.ForwardMessages(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ids)
}

func (s *Server) handleCopyMessages(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.CopyMessagesRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	ids, err := bot.CopyMessages(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ids)
}

func (s *Server) handleDeleteMessages(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.DeleteMessagesRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.DeleteMessages(c, req.ChatID, req.MessageIDs)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleSetMessageReaction(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetMessageReactionRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.SetMessageReaction(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleSendLocation(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendLocationRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msg, err := bot.SendLocation(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

func (s *Server) handleEditMessageLiveLocation(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.EditMessageLiveLocationRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msg, err := bot.EditMessageLiveLocation(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

func (s *Server) handleStopMessageLiveLocation(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.StopMessageLiveLocationRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msg, err := bot.StopMessageLiveLocation(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

func (s *Server) handleSendVenue(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendVenueRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msg, err := bot.SendVenue(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

func (s *Server) handleSendContact(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendContactRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msg, err := bot.SendContact(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

func (s *Server) handleSendPoll(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendPollRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msg, err := bot.SendPoll(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

func (s *Server) handleStopPoll(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.StopPollRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	poll, err := bot.StopPoll(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, poll)
}

func (s *Server) handleSendDice(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendDiceRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msg, err := bot.SendDice(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

func (s *Server) handlePinChatMessage(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.PinChatMessageRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.PinChatMessage(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleUnpinChatMessage(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.UnpinChatMessageRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.UnpinChatMessage(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleUnpinAllChatMessages(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.UnpinAllChatMessagesRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.UnpinAllChatMessages(c, req.ChatID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

// ------------------------------------------------------------------------------------------------
// Bot Commands & Description Handlers
// ------------------------------------------------------------------------------------------------

func (s *Server) handleGetMyCommands(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.GetMyCommandsRequest
	_ = bindRequest(ctx, &req)
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmds, err := bot.GetMyCommands(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, cmds)
}

func (s *Server) handleDeleteMyCommands(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.DeleteMyCommandsRequest
	_ = bindRequest(ctx, &req)
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.DeleteMyCommands(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleSetMyName(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetMyNameRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.SetMyName(c, req.Name, req.LanguageCode)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleGetMyName(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.GetMyNameRequest
	_ = bindRequest(ctx, &req)
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	name, err := bot.GetMyName(c, req.LanguageCode)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, name)
}

func (s *Server) handleSetMyDescription(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetMyDescriptionRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.SetMyDescription(c, req.Description, req.LanguageCode)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleGetMyDescription(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.GetMyDescriptionRequest
	_ = bindRequest(ctx, &req)
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	desc, err := bot.GetMyDescription(c, req.LanguageCode)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, desc)
}

func (s *Server) handleSetMyShortDescription(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetMyShortDescriptionRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.SetMyShortDescription(c, req.ShortDescription, req.LanguageCode)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleGetMyShortDescription(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.GetMyShortDescriptionRequest
	_ = bindRequest(ctx, &req)
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	desc, err := bot.GetMyShortDescription(c, req.LanguageCode)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, desc)
}

func (s *Server) handleSetUserEmojiStatus(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetUserEmojiStatusRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.SetUserEmojiStatus(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

// ------------------------------------------------------------------------------------------------
// Forum Topics Handlers
// ------------------------------------------------------------------------------------------------

func (s *Server) handleCreateForumTopic(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.CreateForumTopicRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	topic, err := bot.CreateForumTopic(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, topic)
}

func (s *Server) handleEditForumTopic(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.EditForumTopicRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.EditForumTopic(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleCloseForumTopic(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.CloseForumTopicRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.CloseForumTopic(c, req.ChatID, req.MessageThreadID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleReopenForumTopic(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.ReopenForumTopicRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.ReopenForumTopic(c, req.ChatID, req.MessageThreadID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleDeleteForumTopic(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.DeleteForumTopicRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.DeleteForumTopic(c, req.ChatID, req.MessageThreadID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleUnpinAllForumTopicMessages(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.UnpinAllForumTopicMessagesRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.UnpinAllForumTopicMessages(c, req.ChatID, req.MessageThreadID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleEditGeneralForumTopic(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.EditGeneralForumTopicRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.EditGeneralForumTopic(c, req.ChatID, req.Name)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleCloseGeneralForumTopic(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.CloseGeneralForumTopicRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.CloseGeneralForumTopic(c, req.ChatID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleReopenGeneralForumTopic(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.ReopenGeneralForumTopicRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.ReopenGeneralForumTopic(c, req.ChatID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleHideGeneralForumTopic(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.HideGeneralForumTopicRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.HideGeneralForumTopic(c, req.ChatID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleUnhideGeneralForumTopic(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.UnhideGeneralForumTopicRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.UnhideGeneralForumTopic(c, req.ChatID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleUnpinAllGeneralForumTopicMessages(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.UnpinAllGeneralForumTopicMessagesRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.UnpinAllGeneralForumTopicMessages(c, req.ChatID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleGetForumTopicIconStickers(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stickers, err := bot.GetForumTopicIconStickers(c)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, stickers)
}

// ------------------------------------------------------------------------------------------------
// Inline Mode Handlers
// ------------------------------------------------------------------------------------------------

func (s *Server) handleAnswerInlineQuery(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.AnswerInlineQueryRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.AnswerInlineQuery(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleAnswerWebAppQuery(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.AnswerWebAppQueryRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msg, err := bot.AnswerWebAppQuery(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

func (s *Server) handleSavePreparedInlineMessage(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SavePreparedInlineMessageRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msg, err := bot.SavePreparedInlineMessage(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

// ------------------------------------------------------------------------------------------------
// Payments, Stars & Gifts Handlers
// ------------------------------------------------------------------------------------------------

func (s *Server) handleSendInvoice(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendInvoiceRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	msg, err := bot.SendInvoice(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

func (s *Server) handleCreateInvoiceLink(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.CreateInvoiceLinkRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	link, err := bot.CreateInvoiceLink(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, link)
}

func (s *Server) handleAnswerShippingQuery(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.AnswerShippingQueryRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.AnswerShippingQuery(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleSendPaidMedia(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendPaidMediaRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	req.Files, req.FileNames = ExtractAllFilesFromRequest(ctx)
	c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	msg, err := bot.SendPaidMedia(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

func (s *Server) handleRefundStarPayment(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.RefundStarPaymentRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.RefundStarPayment(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleGetStarTransactions(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.GetStarTransactionsRequest
	_ = bindRequest(ctx, &req)
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := bot.GetStarTransactions(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, res)
}

func (s *Server) handleGetAvailableGifts(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	gifts, err := bot.GetAvailableGifts(c)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, gifts)
}

func (s *Server) handleSendGift(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendGiftRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.SendGift(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleVerifyUser(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.VerifyUserRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.VerifyUser(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleVerifyChat(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.VerifyChatRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.VerifyChat(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleRemoveUserVerification(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.RemoveUserVerificationRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.RemoveUserVerification(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleRemoveChatVerification(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.RemoveChatVerificationRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.RemoveChatVerification(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

// ------------------------------------------------------------------------------------------------
// Sticker Set Handlers
// ------------------------------------------------------------------------------------------------

func (s *Server) handleGetStickerSet(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.GetStickerSetRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	set, err := bot.GetStickerSet(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, set)
}

func (s *Server) handleGetCustomEmojiStickers(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.GetCustomEmojiStickersRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stickers, err := bot.GetCustomEmojiStickers(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, stickers)
}

func (s *Server) handleUploadStickerFile(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.UploadStickerFileRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}

	if data, _ := ExtractFileFromRequest(ctx, "sticker", req.Sticker); len(data) > 0 {
		req.StickerData = data
	}

	c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	file, err := bot.UploadStickerFile(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, file)
}

func (s *Server) handleCreateNewStickerSet(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.CreateNewStickerSetRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	ok, err := bot.CreateNewStickerSet(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleAddStickerToSet(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.AddStickerToSetRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	ok, err := bot.AddStickerToSet(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleSetStickerPositionInSet(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetStickerPositionInSetRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.SetStickerPositionInSet(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleDeleteStickerFromSet(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.DeleteStickerFromSetRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.DeleteStickerFromSet(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleReplaceStickerInSet(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.ReplaceStickerInSetRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	ok, err := bot.ReplaceStickerInSet(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleSetStickerEmojiList(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetStickerEmojiListRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.SetStickerEmojiList(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleSetStickerKeywords(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetStickerKeywordsRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.SetStickerKeywords(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleSetStickerMaskPosition(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetStickerMaskPositionRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.SetStickerMaskPosition(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleSetStickerSetTitle(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetStickerSetTitleRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.SetStickerSetTitle(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleSetStickerSetThumbnail(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetStickerSetThumbnailRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}

	if data, _ := ExtractFileFromRequest(ctx, "thumbnail", req.Thumbnail); len(data) > 0 {
		req.ThumbnailData = data
	}

	c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	ok, err := bot.SetStickerSetThumbnail(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleSetCustomEmojiStickerSetThumbnail(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetCustomEmojiStickerSetThumbnailRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.SetCustomEmojiStickerSetThumbnail(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleDeleteStickerSet(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.DeleteStickerSetRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.DeleteStickerSet(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

// ------------------------------------------------------------------------------------------------
// Games & Passport Handlers
// ------------------------------------------------------------------------------------------------

func (s *Server) handleSendGame(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendGameRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msg, err := bot.SendGame(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

func (s *Server) handleSetGameScore(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetGameScoreRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msg, err := bot.SetGameScore(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

func (s *Server) handleGetGameHighScores(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.GetGameHighScoresRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	scores, err := bot.GetGameHighScores(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, scores)
}

func (s *Server) handleSetPassportDataErrors(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetPassportDataErrorsRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.SetPassportDataErrors(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

// ------------------------------------------------------------------------------------------------
// Boosts & Business & Auth Handlers
// ------------------------------------------------------------------------------------------------

func (s *Server) handleGetUserChatBoosts(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.GetUserChatBoostsRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	boosts, err := bot.GetUserChatBoosts(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, boosts)
}

func (s *Server) handleReadBusinessConnection(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req struct {
		BusinessConnectionID string `json:"business_connection_id"`
	}
	_ = bindRequest(ctx, &req)
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.ReadBusinessConnection(c, req.BusinessConnectionID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleLogOut(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.LogOut(c)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleCreateChatSubscriptionInviteLink(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.CreateChatSubscriptionInviteLinkRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := bot.CreateChatSubscriptionInviteLink(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, result)
}

func (s *Server) handleEditChatSubscriptionInviteLink(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.EditChatSubscriptionInviteLinkRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := bot.EditChatSubscriptionInviteLink(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, result)
}

func (s *Server) handleSetChatStickerSet(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetChatStickerSetRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := bot.SetChatStickerSet(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, result)
}

func (s *Server) handleDeleteChatStickerSet(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.DeleteChatStickerSetRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := bot.DeleteChatStickerSet(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, result)
}

func (s *Server) handleDeleteStory(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.DeleteStoryRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	result, err := bot.DeleteStory(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, result)
}

func (s *Server) handleReadBusinessMessage(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.ReadBusinessMessageRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := bot.ReadBusinessMessage(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, result)
}

func (s *Server) handleSetBusinessAccountUsername(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetBusinessAccountUsernameRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := bot.SetBusinessAccountUsername(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, result)
}

func (s *Server) handleRemoveBusinessAccountProfilePhoto(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.RemoveBusinessAccountProfilePhotoRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := bot.RemoveBusinessAccountProfilePhoto(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, result)
}

func (s *Server) handleSetMyProfilePhoto(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetMyProfilePhotoRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	if data, _ := ExtractFileFromRequest(ctx, "photo", req.Photo); len(data) > 0 {
		req.PhotoData = data
	}
	if len(req.PhotoData) == 0 {
		s.respondError(ctx, 400, "Bad Request: photo upload is required")
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ok, err := bot.SetMyProfilePhoto(c, req.PhotoData)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleRemoveMyProfilePhoto(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := bot.RemoveMyProfilePhoto(c)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleGetMyStarBalance(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	balance, err := bot.GetMyStarBalance(c)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, balance)
}

func (s *Server) handleDeleteMessageReaction(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.DeleteMessageReactionRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	if req.ChatID == 0 || req.MessageID == 0 {
		s.respondError(ctx, 400, "Bad Request: chat_id and message_id are required")
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := bot.DeleteMessageReaction(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleDeleteAllMessageReactions(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.DeleteAllMessageReactionsRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	if req.ChatID == 0 {
		s.respondError(ctx, 400, "Bad Request: chat_id is required")
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := bot.DeleteAllMessageReactions(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleSetChatMemberTag(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetChatMemberTagRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	if req.ChatID == 0 || req.UserID == 0 {
		s.respondError(ctx, 400, "Bad Request: chat_id and user_id are required")
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := bot.SetChatMemberTag(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleGetUserProfileAudios(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req struct {
		UserID int64 `json:"user_id"`
		Offset int   `json:"offset,omitempty"`
		Limit  int   `json:"limit,omitempty"`
	}
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	if req.UserID == 0 {
		s.respondError(ctx, 400, "Bad Request: user_id is required")
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := bot.GetUserProfileAudios(c, req.UserID, req.Offset, req.Limit)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, res)
}

func (s *Server) handleAnswerChatJoinRequestQuery(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.AnswerChatJoinRequestQueryRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := bot.AnswerChatJoinRequestQuery(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleSendChatJoinRequestWebApp(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendChatJoinRequestWebAppRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := bot.SendChatJoinRequestWebApp(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleAnswerCustomQuery(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.AnswerCustomQueryRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := bot.AnswerCustomQuery(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleSendCustomRequest(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendCustomRequestRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := bot.SendCustomRequest(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, res)
}

func (s *Server) handleAnswerGuestQuery(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.AnswerGuestQueryRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := bot.AnswerGuestQuery(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleApproveSuggestedPost(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.ApproveSuggestedPostRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := bot.ApproveSuggestedPost(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleDeclineSuggestedPost(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.DeclineSuggestedPostRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := bot.DeclineSuggestedPost(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleConvertGiftToStars(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.ConvertGiftToStarsRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := bot.ConvertGiftToStars(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleUpgradeGift(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.UpgradeGiftRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := bot.UpgradeGift(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, res)
}

func (s *Server) handleTransferGift(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.TransferGiftRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := bot.TransferGift(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleGetChatGifts(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.GetChatGiftsRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := bot.GetChatGifts(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, res)
}

func (s *Server) handleGetUserGifts(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.GetUserGiftsRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := bot.GetUserGifts(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, res)
}

func (s *Server) handleGetBusinessAccountGifts(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.GetBusinessAccountGiftsRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := bot.GetBusinessAccountGifts(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, res)
}

func (s *Server) handleGetBusinessAccountStarBalance(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.GetBusinessAccountStarBalanceRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := bot.GetBusinessAccountStarBalance(c, req.BusinessConnectionID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, res)
}

func (s *Server) handleTransferBusinessAccountStars(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.TransferBusinessAccountStarsRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := bot.TransferBusinessAccountStars(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleSetBusinessAccountGiftSettings(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetBusinessAccountGiftSettingsRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := bot.SetBusinessAccountGiftSettings(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleSetBusinessAccountProfilePhoto(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetBusinessAccountProfilePhotoRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	if mf, err := ctx.MultipartForm(); err == nil && mf != nil {
		if headers := mf.File["photo"]; len(headers) > 0 {
			if f, err := headers[0].Open(); err == nil {
				data, _ := io.ReadAll(f)
				_ = f.Close()
				req.PhotoData = data
			}
		}
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := bot.SetBusinessAccountProfilePhoto(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleGetManagedBotToken(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.GetManagedBotTokenRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := bot.GetManagedBotToken(c, req.UserID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, res)
}

func (s *Server) handleGetManagedBotAccessSettings(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.GetManagedBotAccessSettingsRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := bot.GetManagedBotAccessSettings(c, req.UserID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, res)
}

func (s *Server) handleSetManagedBotAccessSettings(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetManagedBotAccessSettingsRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := bot.SetManagedBotAccessSettings(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleReplaceManagedBotToken(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.ReplaceManagedBotTokenRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := bot.ReplaceManagedBotToken(c, req.UserID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, res)
}

func (s *Server) handleGetUserPersonalChatMessages(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.GetUserPersonalChatMessagesRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := bot.GetUserPersonalChatMessages(c, req.UserID, req.Limit)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, res)
}

func (s *Server) handleGiftPremiumSubscription(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.GiftPremiumSubscriptionRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := bot.GiftPremiumSubscription(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleEditUserStarSubscription(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.EditUserStarSubscriptionRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := bot.EditUserStarSubscription(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleRepostStory(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.RepostStoryRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := bot.RepostStory(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, res)
}

func (s *Server) handleEditStory(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.EditStoryRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	req.Files, req.FileNames = ExtractAllFilesFromRequest(ctx)
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := bot.EditStory(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, res)
}

func (s *Server) handleSavePreparedKeyboardButton(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SavePreparedKeyboardButtonRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := bot.SavePreparedKeyboardButton(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, res)
}

func (s *Server) handleSendLivePhoto(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendLivePhotoRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	if mf, err := ctx.MultipartForm(); err == nil && mf != nil {
		if headers := mf.File["photo"]; len(headers) > 0 {
			if f, err := headers[0].Open(); err == nil {
				data, _ := io.ReadAll(f)
				_ = f.Close()
				req.PhotoData = data
			}
		}
		if headers := mf.File["live_photo"]; len(headers) > 0 {
			if f, err := headers[0].Open(); err == nil {
				data, _ := io.ReadAll(f)
				_ = f.Close()
				req.LivePhotoData = data
			}
		}
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := bot.SendLivePhoto(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, res)
}

func (s *Server) handleSendMessageDraft(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendMessageDraftRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := bot.SendMessageDraft(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleSendRichMessageDraft(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendRichMessageDraftRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := bot.SendRichMessageDraft(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleSendRichMessage(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendRichMessageRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := bot.SendRichMessage(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, res)
}

func (s *Server) handleSendChecklist(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendChecklistRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := bot.SendChecklist(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, res)
}

func (s *Server) handleEditMessageChecklist(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.EditMessageChecklistRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := bot.EditMessageChecklist(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, res)
}

func (s *Server) handleEditEphemeralMessageText(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.EditEphemeralMessageTextRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := bot.EditEphemeralMessageText(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleEditEphemeralMessageMedia(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.EditEphemeralMessageMediaRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := bot.EditEphemeralMessageMedia(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleEditEphemeralMessageCaption(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.EditEphemeralMessageCaptionRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := bot.EditEphemeralMessageCaption(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleEditEphemeralMessageReplyMarkup(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.EditEphemeralMessageReplyMarkupRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := bot.EditEphemeralMessageReplyMarkup(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleDeleteEphemeralMessage(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.DeleteEphemeralMessageRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := bot.DeleteEphemeralMessage(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}


