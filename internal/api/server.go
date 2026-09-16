package api

import (
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"

	"telego-bot-api/internal/botmanager"
	"telego-bot-api/internal/converter"
	"telego-bot-api/internal/metrics"
	"telego-bot-api/internal/webhook"

	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

// Server implements high-performance FastHTTP routing for the Telegram Bot API gateway.
type Server struct {
	addr       string
	botManager *botmanager.Manager
	dispatcher *webhook.Dispatcher
	logger     *zap.Logger
	fastServer *fasthttp.Server
}

// NewServer creates a new API HTTP server.
func NewServer(addr string, botManager *botmanager.Manager, dispatcher *webhook.Dispatcher, logger *zap.Logger) *Server {
	s := &Server{
		addr:       addr,
		botManager: botManager,
		dispatcher: dispatcher,
		logger:     logger,
	}
	s.fastServer = &fasthttp.Server{
		Handler:            s.HandleRequest,
		Name:               "telego-bot-api",
		ReadTimeout:        60 * time.Second,
		WriteTimeout:       60 * time.Second,
		IdleTimeout:        15 * time.Second,
		MaxRequestBodySize: 2048 * 1024 * 1024, // 2GB for large file uploads
	}
	return s
}

// Start runs the HTTP server.
func (s *Server) Start() error {
	s.logger.Info("Starting telego-bot-api HTTP server", zap.String("addr", s.addr))
	return s.fastServer.ListenAndServe(s.addr)
}

// Shutdown gracefully shuts down the HTTP server without dropping active requests.
func (s *Server) Shutdown() error {
	s.logger.Info("Shutting down telego-bot-api HTTP server...")
	return s.fastServer.Shutdown()
}

// ListenAndServe runs the HTTP server (alias to Start).
func (s *Server) ListenAndServe() error {
	return s.Start()
}

// HandleRequest routes incoming Bot API HTTP requests: /bot<token>/<method> or /file/bot<token>/<file_path>
func (s *Server) HandleRequest(ctx *fasthttp.RequestCtx) {
	path := string(ctx.Path())

	// Handle file download: /file/bot<token>/<file_path>
	if strings.HasPrefix(path, "/file/bot") {
		s.handleDownloadFile(ctx)
		return
	}

	// Prometheus metrics endpoint for monitoring & Grafana
	if path == "/metrics" {
		ctx.SetContentType("text/plain; version=0.0.4; charset=utf-8")
		active, hibernated := s.botManager.GetBotCounts()
		queueSize := 0
		if s.dispatcher != nil {
			queueSize = s.dispatcher.QueueSize()
		}
		metrics.DefaultRegistry.WritePrometheus(ctx, active, hibernated, queueSize)
		return
	}

	// Deep Health & status endpoint for observability
	if path == "/status" || path == "/health" {
		ctx.SetContentType("application/json")
		status := s.botManager.GetBotsStatus()
		active, hibernated := s.botManager.GetBotCounts()

		redisStatus := "connected"
		pingCtx, pingCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		if err := s.botManager.PingRedis(pingCtx); err != nil {
			redisStatus = "error: " + err.Error()
		}
		pingCancel()

		queueSize := 0
		if s.dispatcher != nil {
			queueSize = s.dispatcher.QueueSize()
		}

		body, _ := json.Marshal(map[string]any{
			"ok":              true,
			"status":          "healthy",
			"redis":           redisStatus,
			"total_bots":      len(status),
			"active_bots":     active,
			"hibernated_bots": hibernated,
			"webhook_queue":   queueSize,
			"bots":            status,
		})
		ctx.SetBody(body)
		return
	}

	// Validate path prefix: must start with /bot
	if !strings.HasPrefix(path, "/bot") {
		s.respondError(ctx, 404, "Not Found")
		return
	}

	remaining := strings.TrimPrefix(path, "/bot")
	parts := strings.SplitN(remaining, "/", 2)
	if len(parts) != 2 {
		s.respondError(ctx, 400, "Bad Request: invalid URL path format")
		return
	}

	token := parts[0]
	method := strings.ToLower(parts[1])

	start := time.Now()
	defer func() {
		status := ctx.Response.StatusCode()
		latency := time.Since(start)
		metrics.DefaultRegistry.IncRequests(method, status)
		metrics.DefaultRegistry.ObserveDuration(method, latency)

		fields := []zap.Field{
			zap.String("method", method),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.String("client_ip", ctx.RemoteIP().String()),
		}
		if status >= 400 {
			s.logger.Warn("API Request", fields...)
		} else {
			s.logger.Info("API Request", fields...)
		}
	}()

	// Ensure bot instance is loaded/active
	bot, err := s.botManager.GetOrCreate(context.Background(), token)
	if err != nil {
		s.logger.Error("Failed to resolve bot session", zap.Error(err))
		s.respondError(ctx, 401, "Unauthorized: "+err.Error())
		return
	}

	switch method {
	// Bot config & webhooks & auth
	case "getme":
		s.handleGetMe(ctx, bot)
	case "logout":
		s.handleLogOut(ctx, bot)
	case "close":
		s.handleClose(ctx, token)
	case "setwebhook":
		s.handleSetWebhook(ctx, bot)
	case "deletewebhook":
		s.handleDeleteWebhook(ctx, bot)
	case "getwebhookinfo":
		s.handleGetWebhookInfo(ctx, bot)
	case "getupdates":
		s.handleGetUpdates(ctx, bot)

	// Messages, Forwarding & Actions
	case "sendmessage":
		s.handleSendMessage(ctx, bot)
	case "forwardmessage":
		s.handleForwardMessage(ctx, bot)
	case "forwardmessages":
		s.handleForwardMessages(ctx, bot)
	case "copymessage":
		s.handleCopyMessage(ctx, bot)
	case "copymessages":
		s.handleCopyMessages(ctx, bot)
	case "editmessagetext":
		s.handleEditMessageText(ctx, bot)
	case "deletemessage":
		s.handleDeleteMessage(ctx, bot)
	case "deletemessages":
		s.handleDeleteMessages(ctx, bot)
	case "editmessagemedia":
		s.handleEditMessageMedia(ctx, bot)
	case "sendchataction":
		s.handleSendChatAction(ctx, bot)
	case "setmessagereaction":
		s.handleSetMessageReaction(ctx, bot)
	case "editmessagecaption":
		s.handleEditMessageCaption(ctx, bot)
	case "editmessagereplymarkup":
		s.handleEditMessageReplyMarkup(ctx, bot)

	// Location, Contacts, Polls, Dice
	case "sendlocation":
		s.handleSendLocation(ctx, bot)
	case "editmessagelivelocation":
		s.handleEditMessageLiveLocation(ctx, bot)
	case "stopmessagelivelocation":
		s.handleStopMessageLiveLocation(ctx, bot)
	case "sendvenue":
		s.handleSendVenue(ctx, bot)
	case "sendcontact":
		s.handleSendContact(ctx, bot)
	case "sendpoll":
		s.handleSendPoll(ctx, bot)
	case "stoppoll":
		s.handleStopPoll(ctx, bot)
	case "senddice":
		s.handleSendDice(ctx, bot)

	// Media & Files
	case "sendphoto":
		s.handleSendPhoto(ctx, bot)
	case "sendvideo":
		s.handleSendVideo(ctx, bot)
	case "senddocument":
		s.handleSendDocument(ctx, bot)
	case "sendvoice":
		s.handleSendVoice(ctx, bot)
	case "sendvideonote":
		s.handleSendVideoNote(ctx, bot)
	case "sendaudio":
		s.handleSendAudio(ctx, bot)
	case "sendsticker":
		s.handleSendSticker(ctx, bot)
	case "sendanimation":
		s.handleSendAnimation(ctx, bot)
	case "sendmediagroup":
		s.handleSendMediaGroup(ctx, bot)
	case "sendpaidmedia":
		s.handleSendPaidMedia(ctx, bot)
	case "getfile":
		s.handleGetFile(ctx, bot)
	case "getuserprofilephotos":
		s.handleGetUserProfilePhotos(ctx, bot)
	case "setuseremojistatus":
		s.handleSetUserEmojiStatus(ctx, bot)

	// Chat Management & Settings
	case "getchat":
		s.handleGetChat(ctx, bot)
	case "getchatadministrators":
		s.handleGetChatAdministrators(ctx, bot)
	case "getchatmembercount", "getchatmemberscount":
		s.handleGetChatMemberCount(ctx, bot)
	case "getchatmember":
		s.handleGetChatMember(ctx, bot)
	case "banchatmember", "kickchatmember":
		s.handleBanChatMember(ctx, bot)
	case "unbanchatmember":
		s.handleUnbanChatMember(ctx, bot)
	case "restrictchatmember":
		s.handleRestrictChatMember(ctx, bot)
	case "promotechatmember":
		s.handlePromoteChatMember(ctx, bot)
	case "setchatadministratorcustomtitle":
		s.handleSetChatAdministratorCustomTitle(ctx, bot)
	case "banchatsenderchat":
		s.handleBanChatSenderChat(ctx, bot)
	case "unbanchatsenderchat":
		s.handleUnbanChatSenderChat(ctx, bot)
	case "setchatpermissions":
		s.handleSetChatPermissions(ctx, bot)
	case "exportchatinvitelink":
		s.handleExportChatInviteLink(ctx, bot)
	case "createchatinvitelink":
		s.handleCreateChatInviteLink(ctx, bot)
	case "editchatinvitelink":
		s.handleEditChatInviteLink(ctx, bot)
	case "createchatsubscriptioninvitelink":
		s.handleCreateChatSubscriptionInviteLink(ctx, bot)
	case "editchatsubscriptioninvitelink":
		s.handleEditChatSubscriptionInviteLink(ctx, bot)
	case "revokechatinvitelink":
		s.handleRevokeChatInviteLink(ctx, bot)
	case "approvechatjoinrequest":
		s.handleApproveChatJoinRequest(ctx, bot)
	case "declinechatjoinrequest":
		s.handleDeclineChatJoinRequest(ctx, bot)
	case "setchatphoto":
		s.handleSetChatPhoto(ctx, bot)
	case "deletechatphoto":
		s.handleDeleteChatPhoto(ctx, bot)
	case "setchatstickerset":
		s.handleSetChatStickerSet(ctx, bot)
	case "deletechatstickerset":
		s.handleDeleteChatStickerSet(ctx, bot)
	case "setchattitle":
		s.handleSetChatTitle(ctx, bot)
	case "setchatdescription":
		s.handleSetChatDescription(ctx, bot)
	case "pinchatmessage":
		s.handlePinChatMessage(ctx, bot)
	case "unpinchatmessage":
		s.handleUnpinChatMessage(ctx, bot)
	case "unpinallchatmessages":
		s.handleUnpinAllChatMessages(ctx, bot)
	case "leavechat":
		s.handleLeaveChat(ctx, bot)
	case "setchatmenubutton":
		s.handleSetChatMenuButton(ctx, bot)
	case "getchatmenubutton":
		s.handleGetChatMenuButton(ctx, bot)
	case "setmydefaultadministratorrights":
		s.handleSetMyDefaultAdministratorRights(ctx, bot)
	case "getmydefaultadministratorrights":
		s.handleGetMyDefaultAdministratorRights(ctx, bot)

	// Bot Commands & Info
	case "setmycommands":
		s.handleSetMyCommands(ctx, bot)
	case "getmycommands":
		s.handleGetMyCommands(ctx, bot)
	case "deletemycommands":
		s.handleDeleteMyCommands(ctx, bot)
	case "setmyname":
		s.handleSetMyName(ctx, bot)
	case "getmyname":
		s.handleGetMyName(ctx, bot)
	case "setmydescription":
		s.handleSetMyDescription(ctx, bot)
	case "getmydescription":
		s.handleGetMyDescription(ctx, bot)
	case "setmyshortdescription":
		s.handleSetMyShortDescription(ctx, bot)
	case "getmyshortdescription":
		s.handleGetMyShortDescription(ctx, bot)

	// Forum Topics
	case "createforumtopic":
		s.handleCreateForumTopic(ctx, bot)
	case "editforumtopic":
		s.handleEditForumTopic(ctx, bot)
	case "closeforumtopic":
		s.handleCloseForumTopic(ctx, bot)
	case "reopenforumtopic":
		s.handleReopenForumTopic(ctx, bot)
	case "deleteforumtopic":
		s.handleDeleteForumTopic(ctx, bot)
	case "unpinallforumtopicmessages":
		s.handleUnpinAllForumTopicMessages(ctx, bot)
	case "editgeneralforumtopic":
		s.handleEditGeneralForumTopic(ctx, bot)
	case "closegeneralforumtopic":
		s.handleCloseGeneralForumTopic(ctx, bot)
	case "reopengeneralforumtopic":
		s.handleReopenGeneralForumTopic(ctx, bot)
	case "hidegeneralforumtopic":
		s.handleHideGeneralForumTopic(ctx, bot)
	case "unhidegeneralforumtopic":
		s.handleUnhideGeneralForumTopic(ctx, bot)
	case "unpinallgeneralforumtopicmessages":
		s.handleUnpinAllGeneralForumTopicMessages(ctx, bot)
	case "getforumtopiciconstickers":
		s.handleGetForumTopicIconStickers(ctx, bot)

	// Inline Mode & Callbacks
	case "answercallbackquery":
		s.handleAnswerCallbackQuery(ctx, bot)
	case "answerinlinequery":
		s.handleAnswerInlineQuery(ctx, bot)
	case "answerwebappquery":
		s.handleAnswerWebAppQuery(ctx, bot)
	case "savepreparedinlinemessage":
		s.handleSavePreparedInlineMessage(ctx, bot)

	// Payments, Stars & Gifts
	case "sendinvoice":
		s.handleSendInvoice(ctx, bot)
	case "createinvoicelink":
		s.handleCreateInvoiceLink(ctx, bot)
	case "answershippingquery":
		s.handleAnswerShippingQuery(ctx, bot)
	case "answerprecheckoutquery":
		s.handleAnswerPreCheckoutQuery(ctx, bot)
	case "refundstarpayment":
		s.handleRefundStarPayment(ctx, bot)
	case "getstartransactions":
		s.handleGetStarTransactions(ctx, bot)
	case "getavailablegifts":
		s.handleGetAvailableGifts(ctx, bot)
	case "sendgift":
		s.handleSendGift(ctx, bot)
	case "verifyuser":
		s.handleVerifyUser(ctx, bot)
	case "verifychat":
		s.handleVerifyChat(ctx, bot)
	case "removeuserverification":
		s.handleRemoveUserVerification(ctx, bot)
	case "removechatverification":
		s.handleRemoveChatVerification(ctx, bot)

	// Stickers
	case "getstickerset":
		s.handleGetStickerSet(ctx, bot)
	case "getcustomemojistickers":
		s.handleGetCustomEmojiStickers(ctx, bot)
	case "uploadstickerfile":
		s.handleUploadStickerFile(ctx, bot)
	case "createnewstickerset":
		s.handleCreateNewStickerSet(ctx, bot)
	case "addstickertoset":
		s.handleAddStickerToSet(ctx, bot)
	case "setstickerpositioninset":
		s.handleSetStickerPositionInSet(ctx, bot)
	case "deletestickerfromset":
		s.handleDeleteStickerFromSet(ctx, bot)
	case "replacestickerinset":
		s.handleReplaceStickerInSet(ctx, bot)
	case "setstickeremojilist":
		s.handleSetStickerEmojiList(ctx, bot)
	case "setstickerkeywords":
		s.handleSetStickerKeywords(ctx, bot)
	case "setstickermaskposition":
		s.handleSetStickerMaskPosition(ctx, bot)
	case "setstickersettitle":
		s.handleSetStickerSetTitle(ctx, bot)
	case "setstickersetthumbnail":
		s.handleSetStickerSetThumbnail(ctx, bot)
	case "setcustomemojistickersetthumbnail":
		s.handleSetCustomEmojiStickerSetThumbnail(ctx, bot)
	case "deletestickerset":
		s.handleDeleteStickerSet(ctx, bot)

	// Games & Passport
	case "sendgame":
		s.handleSendGame(ctx, bot)
	case "setgamescore":
		s.handleSetGameScore(ctx, bot)
	case "getgamehighscores":
		s.handleGetGameHighScores(ctx, bot)
	case "setpassportdataerrors":
		s.handleSetPassportDataErrors(ctx, bot)

	// Boosts & Telegram Business
	case "getuserchatboosts":
		s.handleGetUserChatBoosts(ctx, bot)
	case "getbusinessconnection":
		s.handleGetBusinessConnection(ctx, bot)
	case "deletebusinessmessages":
		s.handleDeleteBusinessMessages(ctx, bot)
	case "readbusinessmessage":
		s.handleReadBusinessMessage(ctx, bot)
	case "setbusinessaccountbio":
		s.handleSetBusinessAccountBio(ctx, bot)
	case "setbusinessaccountname":
		s.handleSetBusinessAccountName(ctx, bot)
	case "setbusinessaccountusername":
		s.handleSetBusinessAccountUsername(ctx, bot)
	case "removebusinessaccountprofilephoto":
		s.handleRemoveBusinessAccountProfilePhoto(ctx, bot)
	case "poststory":
		s.handlePostStory(ctx, bot)
	case "deletestory":
		s.handleDeleteStory(ctx, bot)
	case "readbusinessconnection":
		s.handleReadBusinessConnection(ctx, bot)

	default:
		s.respondError(ctx, 404, "Not Found: method not supported yet in telego-bot-api: "+method)
	}
}

func (s *Server) handleGetMe(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	user := bot.GetOrRefreshMe(c)
	if user == nil {
		s.respondError(ctx, 500, "Bot profile not loaded yet")
		return
	}
	s.respondOK(ctx, user)
}

func (s *Server) handleSetWebhook(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetWebhookRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: invalid payload: "+err.Error())
		return
	}
	if req.URL == "" {
		s.respondError(ctx, 400, "Bad Request: url parameter is required")
		return
	}

	if err := bot.SetWebhook(context.Background(), req.URL, req.SecretToken, req.DropPendingUpdates); err != nil {
		s.respondError(ctx, 500, "Failed to save webhook: "+err.Error())
		return
	}
	s.respondOK(ctx, true)
}

func (s *Server) handleDeleteWebhook(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.DeleteWebhookRequest
	_ = bindRequest(ctx, &req)
	if err := bot.DeleteWebhook(context.Background(), req.DropPendingUpdates); err != nil {
		s.respondError(ctx, 500, "Failed to delete webhook: "+err.Error())
		return
	}
	s.respondOK(ctx, true)
}

func (s *Server) handleGetWebhookInfo(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	info, err := bot.GetWebhookInfo(context.Background())
	if err != nil {
		s.respondError(ctx, 500, "Failed to get webhook info: "+err.Error())
		return
	}
	s.respondOK(ctx, info)
}

func (s *Server) handleGetUpdates(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	// Telegram Bot API specification: getUpdates cannot be called while a webhook is active
	if bot.HasWebhook() {
		s.respondError(ctx, 409, "Conflict: can't use getUpdates method while webhook is active; use deleteWebhook to delete the webhook first")
		return
	}

	var req converter.GetUpdatesRequest
	_ = bindRequest(ctx, &req)

	timeout := req.Timeout
	if timeout == 0 && ctx.QueryArgs().Has("timeout") {
		timeout, _ = strconv.Atoi(string(ctx.QueryArgs().Peek("timeout")))
	}

	limit := req.Limit
	if limit == 0 && ctx.QueryArgs().Has("limit") {
		limit, _ = strconv.Atoi(string(ctx.QueryArgs().Peek("limit")))
	}

	offset := req.Offset
	if offset == 0 && ctx.QueryArgs().Has("offset") {
		offset, _ = strconv.Atoi(string(ctx.QueryArgs().Peek("offset")))
	}

	c, cancel := context.WithTimeout(context.Background(), time.Duration(timeout+5)*time.Second)
	defer cancel()

	updates, err := bot.GetUpdates(c, offset, limit, timeout, req.AllowedUpdates)
	if err != nil {
		s.respondError(ctx, 500, "Failed to get updates: "+err.Error())
		return
	}
	if updates == nil {
		updates = []*converter.Update{}
	}
	s.respondOK(ctx, updates)
}

func (s *Server) handleSendMessage(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendMessageRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: invalid payload: "+err.Error())
		return
	}
	if req.ChatID == 0 || req.Text == "" {
		s.respondError(ctx, 400, "Bad Request: chat_id and text are required")
		return
	}

	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msg, err := bot.SendMessage(c, &req)
	if err != nil {
		s.logger.Error("SendMessage failed", zap.Error(err))
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

func (s *Server) handleEditMessageText(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.EditMessageTextRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: invalid payload: "+err.Error())
		return
	}

	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msg, err := bot.EditMessageText(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

func (s *Server) handleDeleteMessage(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.DeleteMessageRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: invalid payload: "+err.Error())
		return
	}

	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.DeleteMessage(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleEditMessageMedia(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.EditMessageMediaRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: invalid payload: "+err.Error())
		return
	}

	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msg, err := bot.EditMessageMedia(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

func (s *Server) handleSendChatAction(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendChatActionRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: invalid payload: "+err.Error())
		return
	}

	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.SendChatAction(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleCopyMessage(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.CopyMessageRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: invalid payload: "+err.Error())
		return
	}

	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msgID, err := bot.CopyMessage(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msgID)
}

func (s *Server) handleEditMessageCaption(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.EditMessageCaptionRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: invalid payload: "+err.Error())
		return
	}

	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msg, err := bot.EditMessageCaption(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

func (s *Server) handleEditMessageReplyMarkup(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.EditMessageReplyMarkupRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: invalid payload: "+err.Error())
		return
	}

	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msg, err := bot.EditMessageReplyMarkup(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

func (s *Server) handleAnswerCallbackQuery(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.AnswerCallbackQueryRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: invalid payload: "+err.Error())
		return
	}

	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.AnswerCallbackQuery(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleSendPhoto(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendPhotoRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: invalid payload: "+err.Error())
		return
	}

	if fh, err := ctx.FormFile("photo"); err == nil && fh != nil {
		f, err := fh.Open()
		if err == nil {
			data, err := io.ReadAll(f)
			_ = f.Close()
			if err == nil {
				req.PhotoData = data
				req.PhotoFileName = fh.Filename
			}
		}
	}

	if req.ChatID == 0 || (req.Photo == "" && len(req.PhotoData) == 0) {
		s.respondError(ctx, 400, "Bad Request: chat_id and photo are required")
		return
	}

	c, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	msg, err := bot.SendPhoto(c, &req)
	if err != nil {
		s.logger.Error("SendPhoto failed", zap.Error(err))
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

func (s *Server) handleSendVideo(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendVideoRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: invalid payload: "+err.Error())
		return
	}

	if fh, err := ctx.FormFile("video"); err == nil && fh != nil {
		f, err := fh.Open()
		if err == nil {
			data, err := io.ReadAll(f)
			_ = f.Close()
			if err == nil {
				req.VideoData = data
				req.VideoFileName = fh.Filename
			}
		}
	}

	if req.ChatID == 0 || (req.Video == "" && len(req.VideoData) == 0) {
		s.respondError(ctx, 400, "Bad Request: chat_id and video are required")
		return
	}

	c, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	msg, err := bot.SendVideo(c, &req)
	if err != nil {
		s.logger.Error("SendVideo failed", zap.Error(err))
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

func (s *Server) handleSendDocument(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendDocumentRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: invalid payload: "+err.Error())
		return
	}

	if fh, err := ctx.FormFile("document"); err == nil && fh != nil {
		f, err := fh.Open()
		if err == nil {
			data, err := io.ReadAll(f)
			_ = f.Close()
			if err == nil {
				req.DocumentData = data
				req.DocumentFileName = fh.Filename
			}
		}
	}

	if req.ChatID == 0 || (req.Document == "" && len(req.DocumentData) == 0) {
		s.respondError(ctx, 400, "Bad Request: chat_id and document are required")
		return
	}

	c, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	msg, err := bot.SendDocument(c, &req)
	if err != nil {
		s.logger.Error("SendDocument failed", zap.Error(err))
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

func (s *Server) handleSendVoice(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendVoiceRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: invalid payload: "+err.Error())
		return
	}

	if fh, err := ctx.FormFile("voice"); err == nil && fh != nil {
		f, err := fh.Open()
		if err == nil {
			data, err := io.ReadAll(f)
			_ = f.Close()
			if err == nil {
				req.VoiceData = data
				req.VoiceFileName = fh.Filename
			}
		}
	}

	if req.ChatID == 0 || (req.Voice == "" && len(req.VoiceData) == 0) {
		s.respondError(ctx, 400, "Bad Request: chat_id and voice are required")
		return
	}

	c, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	msg, err := bot.SendVoice(c, &req)
	if err != nil {
		s.logger.Error("SendVoice failed", zap.Error(err))
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

func (s *Server) handleSendVideoNote(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendVideoNoteRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: invalid payload: "+err.Error())
		return
	}

	if fh, err := ctx.FormFile("video_note"); err == nil && fh != nil {
		f, err := fh.Open()
		if err == nil {
			data, err := io.ReadAll(f)
			_ = f.Close()
			if err == nil {
				req.VideoNoteData = data
				req.VideoNoteFileName = fh.Filename
			}
		}
	}

	if req.ChatID == 0 || (req.VideoNote == "" && len(req.VideoNoteData) == 0) {
		s.respondError(ctx, 400, "Bad Request: chat_id and video_note are required")
		return
	}

	c, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	msg, err := bot.SendVideoNote(c, &req)
	if err != nil {
		s.logger.Error("SendVideoNote failed", zap.Error(err))
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

func (s *Server) handleSendAudio(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendAudioRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: invalid payload: "+err.Error())
		return
	}

	if fh, err := ctx.FormFile("audio"); err == nil && fh != nil {
		f, err := fh.Open()
		if err == nil {
			data, err := io.ReadAll(f)
			_ = f.Close()
			if err == nil {
				req.AudioData = data
				req.AudioFileName = fh.Filename
			}
		}
	}

	if req.ChatID == 0 || (req.Audio == "" && len(req.AudioData) == 0) {
		s.respondError(ctx, 400, "Bad Request: chat_id and audio are required")
		return
	}

	c, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	msg, err := bot.SendAudio(c, &req)
	if err != nil {
		s.logger.Error("SendAudio failed", zap.Error(err))
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

func (s *Server) handleSendSticker(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendStickerRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: invalid payload: "+err.Error())
		return
	}

	if fh, err := ctx.FormFile("sticker"); err == nil && fh != nil {
		f, err := fh.Open()
		if err == nil {
			data, err := io.ReadAll(f)
			_ = f.Close()
			if err == nil {
				req.StickerData = data
				req.StickerFileName = fh.Filename
			}
		}
	}

	if req.ChatID == 0 || (req.Sticker == "" && len(req.StickerData) == 0) {
		s.respondError(ctx, 400, "Bad Request: chat_id and sticker are required")
		return
	}

	c, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	msg, err := bot.SendSticker(c, &req)
	if err != nil {
		s.logger.Error("SendSticker failed", zap.Error(err))
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

func (s *Server) handleSendAnimation(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendAnimationRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: invalid payload: "+err.Error())
		return
	}

	if fh, err := ctx.FormFile("animation"); err == nil && fh != nil {
		f, err := fh.Open()
		if err == nil {
			data, err := io.ReadAll(f)
			_ = f.Close()
			if err == nil {
				req.AnimationData = data
				req.AnimationFileName = fh.Filename
			}
		}
	}

	if req.ChatID == 0 || (req.Animation == "" && len(req.AnimationData) == 0) {
		s.respondError(ctx, 400, "Bad Request: chat_id and animation are required")
		return
	}

	c, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	msg, err := bot.SendAnimation(c, &req)
	if err != nil {
		s.logger.Error("SendAnimation failed", zap.Error(err))
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msg)
}

func (s *Server) handleGetFile(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.GetFileRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: invalid payload: "+err.Error())
		return
	}
	if req.FileID == "" {
		s.respondError(ctx, 400, "Bad Request: file_id is required")
		return
	}

	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	file, err := bot.GetFile(c, req.FileID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, file)
}

func (s *Server) handleDownloadFile(ctx *fasthttp.RequestCtx) {
	path := string(ctx.Path())
	trimmed := strings.TrimPrefix(path, "/file/bot")
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) != 2 {
		s.respondError(ctx, 400, "Bad Request: invalid file download path")
		return
	}
	token := parts[0]
	filePath := parts[1]
	fileID := strings.TrimPrefix(filePath, "files/")

	bot, err := s.botManager.GetOrCreate(context.Background(), token)
	if err != nil {
		s.respondError(ctx, 401, "Unauthorized: "+err.Error())
		return
	}

	ctx.SetContentType("application/octet-stream")
	c, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if err := bot.DownloadFile(c, fileID, ctx); err != nil {
		s.logger.Error("DownloadFile failed", zap.Error(err))
		s.respondError(ctx, 404, "File not found: "+err.Error())
		return
	}
}

func (s *Server) handleAnswerPreCheckoutQuery(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.AnswerPreCheckoutQueryRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: invalid payload: "+err.Error())
		return
	}

	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.AnswerPreCheckoutQuery(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleGetChatMember(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.GetChatMemberRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: invalid payload: "+err.Error())
		return
	}

	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	member, err := bot.GetChatMember(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, member)
}

func (s *Server) handleSetMyCommands(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetMyCommandsRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: invalid payload: "+err.Error())
		return
	}

	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.SetMyCommands(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleGetBusinessConnection(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req struct {
		BusinessConnectionID string `json:"business_connection_id"`
	}
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	if req.BusinessConnectionID == "" {
		s.respondError(ctx, 400, "Bad Request: business_connection_id is required")
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := bot.GetBusinessConnection(c, req.BusinessConnectionID)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, conn)
}

func (s *Server) handleDeleteBusinessMessages(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	s.respondOK(ctx, true)
}

func (s *Server) handleGetUserProfilePhotos(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.GetUserProfilePhotosRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: invalid payload: "+err.Error())
		return
	}
	if req.UserID == 0 {
		s.respondError(ctx, 400, "Bad Request: user_id is required")
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	photos, err := bot.GetUserProfilePhotos(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, photos)
}

func (s *Server) handleSendMediaGroup(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SendMediaGroupRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: invalid payload: "+err.Error())
		return
	}

	if mf, err := ctx.MultipartForm(); err == nil && mf != nil {
		req.Files = make(map[string][]byte)
		req.FileNames = make(map[string]string)
		for name, fhs := range mf.File {
			if len(fhs) > 0 {
				fh := fhs[0]
				if f, err := fh.Open(); err == nil {
					if data, err := io.ReadAll(f); err == nil {
						req.Files[name] = data
						req.FileNames[name] = fh.Filename
					}
					_ = f.Close()
				}
			}
		}
	}

	if req.ChatID == 0 || len(req.Media) == 0 {
		s.respondError(ctx, 400, "Bad Request: chat_id and media are required")
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	msgs, err := bot.SendMediaGroup(c, &req)
	if err != nil {
		s.logger.Error("SendMediaGroup failed", zap.Error(err))
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, msgs)
}

func (s *Server) handleSetBusinessAccountBio(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetBusinessAccountBioRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	if req.BusinessConnectionID == "" {
		s.respondError(ctx, 400, "Bad Request: business_connection_id is required")
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.SetBusinessAccountBio(c, req.BusinessConnectionID, req.Bio)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handleSetBusinessAccountName(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.SetBusinessAccountNameRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	if req.BusinessConnectionID == "" {
		s.respondError(ctx, 400, "Bad Request: business_connection_id is required")
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := bot.SetBusinessAccountName(c, req.BusinessConnectionID, req.FirstName, req.LastName)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, ok)
}

func (s *Server) handlePostStory(ctx *fasthttp.RequestCtx, bot *botmanager.BotInstance) {
	var req converter.PostStoryRequest
	if err := bindRequest(ctx, &req); err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	if mf, err := ctx.MultipartForm(); err == nil && mf != nil {
		req.Files = make(map[string][]byte)
		req.FileNames = make(map[string]string)
		for name, fhs := range mf.File {
			if len(fhs) == 0 {
				continue
			}
			file, err := fhs[0].Open()
			if err != nil {
				continue
			}
			data, readErr := io.ReadAll(file)
			_ = file.Close()
			if readErr == nil {
				req.Files[name], req.FileNames[name] = data, fhs[0].Filename
			}
		}
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	story, err := bot.PostStory(c, &req)
	if err != nil {
		s.respondError(ctx, 400, "Bad Request: "+err.Error())
		return
	}
	s.respondOK(ctx, story)
}

func (s *Server) handleClose(ctx *fasthttp.RequestCtx, token string) {
	closed := s.botManager.CloseBot(token)
	s.respondOK(ctx, closed)
}

func (s *Server) respondOK(ctx *fasthttp.RequestCtx, result interface{}) {
	raw, _ := json.Marshal(result)
	resp := converter.ApiResponse{
		OK:     true,
		Result: raw,
	}
	ctx.SetContentType("application/json")
	ctx.SetStatusCode(200)
	_ = json.NewEncoder(ctx).Encode(resp)
}

func (s *Server) respondError(ctx *fasthttp.RequestCtx, code int, desc string) {
	// If generic 400 was provided, attempt intelligent mapping of RPC error messages
	if code == 400 {
		if mappedCode, mappedDesc, params := MapErrorString(desc); mappedCode != 400 || params != nil || mappedDesc != desc {
			s.respondErrorWithParams(ctx, mappedCode, mappedDesc, params)
			return
		}
	}
	s.respondErrorWithParams(ctx, code, desc, nil)
}

func (s *Server) respondErrorWithParams(ctx *fasthttp.RequestCtx, code int, desc string, params *converter.ResponseParameters) {
	s.logger.Warn("HTTP Error Response",
		zap.Int("code", code),
		zap.String("description", desc),
		zap.String("path", redactBotTokenInPath(string(ctx.Path()))),
	)
	resp := converter.ApiResponse{
		OK:          false,
		ErrorCode:   code,
		Description: desc,
		Parameters:  params,
	}
	ctx.SetContentType("application/json")
	ctx.SetStatusCode(code)
	_ = json.NewEncoder(ctx).Encode(resp)
}

func (s *Server) respondRpcError(ctx *fasthttp.RequestCtx, err error) {
	code, desc, params := MapRpcError(err)
	s.respondErrorWithParams(ctx, code, desc, params)
}

func redactBotTokenInPath(path string) string {
	const marker = "/bot"
	markerIndex := strings.Index(path, marker)
	if markerIndex < 0 {
		return path
	}

	tokenStart := markerIndex + len(marker)
	rest := path[tokenStart:]
	slashIndex := strings.IndexByte(rest, '/')
	if slashIndex < 0 {
		return path[:tokenStart] + "<redacted>"
	}
	return path[:tokenStart] + "<redacted>" + rest[slashIndex:]
}
