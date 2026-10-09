package botmanager

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"telego-bot-api/internal/converter"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

// invokeMaybeBusiness sends query on behalf of a business account when connectionID is set,
// otherwise on behalf of the bot itself.
func (b *BotInstance) invokeMaybeBusiness(ctx context.Context, connectionID string, query bin.Object, output bin.Decoder) error {
	if connectionID != "" {
		return b.invokeBusiness(ctx, connectionID, query, output)
	}
	return b.mtClient().Invoke(ctx, query, output)
}

// parseOwnedGiftID converts a Bot API owned_gift_id to an MTProto saved gift reference.
// Formats follow TDLib: "<message_id>" for gifts of users, "<chat_id>_<saved_id>" for gifts
// of chats, and "@<slug>" for unique gifts.
func (b *BotInstance) parseOwnedGiftID(ownedGiftID string) (tg.InputSavedStarGiftClass, error) {
	invalid := fmt.Errorf("Invalid owned gift identifier specified")
	if ownedGiftID == "" {
		return nil, invalid
	}
	if strings.HasPrefix(ownedGiftID, "@") {
		if len(ownedGiftID) == 1 {
			return nil, invalid
		}
		return &tg.InputSavedStarGiftSlug{Slug: ownedGiftID[1:]}, nil
	}
	if chatPart, savedPart, ok := strings.Cut(ownedGiftID, "_"); ok {
		chatID, err := strconv.ParseInt(chatPart, 10, 64)
		if err != nil {
			return nil, invalid
		}
		savedID, err := strconv.ParseInt(savedPart, 10, 64)
		if err != nil || savedID == 0 {
			return nil, invalid
		}
		peer, err := b.resolvePeer(chatID)
		if err != nil {
			return nil, err
		}
		return &tg.InputSavedStarGiftChat{Peer: peer, SavedID: savedID}, nil
	}
	msgID, err := strconv.Atoi(ownedGiftID)
	if err != nil || msgID <= 0 {
		return nil, invalid
	}
	return &tg.InputSavedStarGiftUser{MsgID: msgID}, nil
}

// payStars pays a Stars invoice the way TDLib does: it fetches the payment form for
// formInvoice, checks the price against starCount and submits sendInvoice. With exact
// set the price must match starCount, otherwise it must not exceed it.
func (b *BotInstance) payStars(ctx context.Context, connectionID string, formInvoice, sendInvoice tg.InputInvoiceClass, starCount int64, exact bool, what string) error {
	var formBox tg.PaymentsPaymentFormBox
	if err := b.invokeMaybeBusiness(ctx, connectionID, &tg.PaymentsGetPaymentFormRequest{Invoice: formInvoice}, &formBox); err != nil {
		return err
	}
	var formID int64
	var invoice tg.Invoice
	switch form := formBox.PaymentForm.(type) {
	case *tg.PaymentsPaymentFormStars:
		formID, invoice = form.FormID, form.Invoice
	case *tg.PaymentsPaymentFormStarGift:
		formID, invoice = form.FormID, form.Invoice
	default:
		return fmt.Errorf("Unsupported payment form")
	}
	if len(invoice.Prices) != 1 {
		return fmt.Errorf("Wrong %s price specified", what)
	}
	price := invoice.Prices[0].Amount
	if price > starCount || (exact && price != starCount) {
		return fmt.Errorf("Wrong %s price specified", what)
	}
	var result tg.PaymentsPaymentResultBox
	if err := b.invokeMaybeBusiness(ctx, connectionID, &tg.PaymentsSendStarsFormRequest{FormID: formID, Invoice: sendInvoice}, &result); err != nil {
		return err
	}
	if _, ok := result.PaymentResult.(*tg.PaymentsPaymentVerificationNeeded); ok {
		return fmt.Errorf("Payment verification is required")
	}
	return nil
}

// ConvertGiftToStars converts an owned star gift to Telegram Stars.
func (b *BotInstance) ConvertGiftToStars(ctx context.Context, req *converter.ConvertGiftToStarsRequest) (bool, error) {
	gift, err := b.parseOwnedGiftID(req.OwnedGiftID)
	if err != nil {
		return false, err
	}
	var res tg.BoolBox
	if err := b.invokeMaybeBusiness(ctx, req.BusinessConnectionID, &tg.PaymentsConvertStarGiftRequest{Stargift: gift}, &res); err != nil {
		return false, err
	}
	return true, nil
}

// UpgradeGift upgrades an owned regular gift to a unique one, paying for the upgrade
// with up to star_count Stars when it was not prepaid.
func (b *BotInstance) UpgradeGift(ctx context.Context, req *converter.UpgradeGiftRequest) (bool, error) {
	if req.StarCount < 0 || req.StarCount > 1000000 {
		return false, fmt.Errorf("star_count is invalid")
	}
	gift, err := b.parseOwnedGiftID(req.OwnedGiftID)
	if err != nil {
		return false, err
	}
	if req.StarCount == 0 {
		var updates tg.UpdatesBox
		err := b.invokeMaybeBusiness(ctx, req.BusinessConnectionID, &tg.PaymentsUpgradeStarGiftRequest{
			Stargift: gift, KeepOriginalDetails: req.KeepOriginalDetails,
		}, &updates)
		return err == nil, err
	}
	invoice := &tg.InputInvoiceStarGiftUpgrade{Stargift: gift, KeepOriginalDetails: req.KeepOriginalDetails}
	if err := b.payStars(ctx, req.BusinessConnectionID, invoice, invoice, req.StarCount, false, "upgrade"); err != nil {
		return false, err
	}
	return true, nil
}

// TransferGift transfers an owned unique gift to another user or chat, paying
// exactly star_count Stars when the transfer is not free.
func (b *BotInstance) TransferGift(ctx context.Context, req *converter.TransferGiftRequest) (bool, error) {
	if req.StarCount < 0 || req.StarCount > 1000000 {
		return false, fmt.Errorf("star_count is invalid")
	}
	gift, err := b.parseOwnedGiftID(req.OwnedGiftID)
	if err != nil {
		return false, err
	}
	toPeer, err := b.resolvePeer(req.NewOwnerChatID)
	if err != nil {
		return false, err
	}
	if req.StarCount == 0 {
		var updates tg.UpdatesBox
		err := b.invokeMaybeBusiness(ctx, req.BusinessConnectionID, &tg.PaymentsTransferStarGiftRequest{
			Stargift: gift, ToID: toPeer,
		}, &updates)
		return err == nil, err
	}
	invoice := &tg.InputInvoiceStarGiftTransfer{Stargift: gift, ToID: toPeer}
	if err := b.payStars(ctx, req.BusinessConnectionID, invoice, invoice, req.StarCount, true, "transfer"); err != nil {
		return false, err
	}
	return true, nil
}

// TransferBusinessAccountStars transfers Stars from a business account to the bot.
func (b *BotInstance) TransferBusinessAccountStars(ctx context.Context, req *converter.TransferBusinessAccountStarsRequest) (bool, error) {
	if req.BusinessConnectionID == "" {
		return false, fmt.Errorf("business_connection_id is required")
	}
	if req.StarCount <= 0 || req.StarCount > 1000000000 {
		return false, fmt.Errorf("Invalid amount of Telegram Stars to transfer specified")
	}
	invoice := &tg.InputInvoiceBusinessBotTransferStars{Bot: &tg.InputUserSelf{}, Stars: req.StarCount}
	if err := b.payStars(ctx, req.BusinessConnectionID, invoice, invoice, req.StarCount, true, "transfer"); err != nil {
		return false, err
	}
	return true, nil
}

// GiftPremiumSubscription gifts Telegram Premium to a user, paid with the bot's Stars.
func (b *BotInstance) GiftPremiumSubscription(ctx context.Context, req *converter.GiftPremiumSubscriptionRequest) (bool, error) {
	if req.StarCount <= 0 {
		return false, fmt.Errorf("star_count is invalid")
	}
	user := b.inputUser(req.UserID)
	formInvoice := &tg.InputInvoicePremiumGiftStars{UserID: user, Months: req.MonthCount}
	sendInvoice := &tg.InputInvoicePremiumGiftStars{UserID: user, Months: req.MonthCount}
	if req.Text != "" {
		text, entities, err := inlineMessageEntities(req.Text, req.TextParseMode, req.TextEntities)
		if err != nil {
			return false, err
		}
		sendInvoice.SetMessage(tg.TextWithEntities{Text: text, Entities: entities})
	}
	if err := b.payStars(ctx, "", formInvoice, sendInvoice, req.StarCount, true, "Premium subscription"); err != nil {
		return false, err
	}
	return true, nil
}

// GetChatGifts returns the gifts owned by a chat.
func (b *BotInstance) GetChatGifts(ctx context.Context, req *converter.GetChatGiftsRequest) (*converter.UserGifts, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, err
	}
	request := &tg.PaymentsGetSavedStarGiftsRequest{
		Peer:                peer,
		ExcludeUnsaved:      req.ExcludeUnsaved,
		ExcludeSaved:        req.ExcludeSaved,
		ExcludeUnlimited:    req.ExcludeUnlimited,
		ExcludeUpgradable:   req.ExcludeLimitedUpgradable,
		ExcludeUnupgradable: req.ExcludeLimitedNonUpgradable,
		ExcludeUnique:       req.ExcludeUnique,
		ExcludeHosted:       req.ExcludeFromBlockchain,
		SortByValue:         req.SortByPrice,
		Offset:              req.Offset,
		Limit:               giftsLimit(req.Limit),
	}
	return b.getSavedGifts(ctx, "", request, req.ChatID, req.ChatID > 0, false)
}

// GetUserGifts returns the gifts owned and hosted by a user.
func (b *BotInstance) GetUserGifts(ctx context.Context, req *converter.GetUserGiftsRequest) (*converter.UserGifts, error) {
	peer, err := b.resolvePeer(req.UserID)
	if err != nil {
		return nil, err
	}
	request := &tg.PaymentsGetSavedStarGiftsRequest{
		Peer:                peer,
		ExcludeUnsaved:      true,
		ExcludeUnlimited:    req.ExcludeUnlimited,
		ExcludeUpgradable:   req.ExcludeLimitedUpgradable,
		ExcludeUnupgradable: req.ExcludeLimitedNonUpgradable,
		ExcludeUnique:       req.ExcludeUnique,
		ExcludeHosted:       req.ExcludeFromBlockchain,
		SortByValue:         req.SortByPrice,
		Offset:              req.Offset,
		Limit:               giftsLimit(req.Limit),
	}
	return b.getSavedGifts(ctx, "", request, req.UserID, true, false)
}

// GetBusinessAccountGifts returns the gifts received and owned by a managed business account.
func (b *BotInstance) GetBusinessAccountGifts(ctx context.Context, req *converter.GetBusinessAccountGiftsRequest) (*converter.UserGifts, error) {
	connection, err := b.GetBusinessConnection(ctx, req.BusinessConnectionID)
	if err != nil {
		return nil, err
	}
	excludeUpgradable, excludeUnupgradable := req.ExcludeLimitedUpgradable, req.ExcludeLimitedNonUpgradable
	if req.ExcludeLimited {
		excludeUpgradable, excludeUnupgradable = true, true
	}
	request := &tg.PaymentsGetSavedStarGiftsRequest{
		Peer:                &tg.InputPeerSelf{},
		ExcludeUnsaved:      req.ExcludeUnsaved,
		ExcludeSaved:        req.ExcludeSaved,
		ExcludeUnlimited:    req.ExcludeUnlimited,
		ExcludeUpgradable:   excludeUpgradable,
		ExcludeUnupgradable: excludeUnupgradable,
		ExcludeUnique:       req.ExcludeUnique,
		ExcludeHosted:       req.ExcludeFromBlockchain,
		SortByValue:         req.SortByPrice,
		Offset:              req.Offset,
		Limit:               giftsLimit(req.Limit),
	}
	return b.getSavedGifts(ctx, req.BusinessConnectionID, request, connection.UserChatID, true, true)
}

func giftsLimit(limit int) int {
	if limit <= 0 || limit > 100 {
		return 100
	}
	return limit
}

// getSavedGifts loads saved gifts of ownerID and converts them to Bot API OwnedGifts.
// Owned gift identifiers are exposed only when the gifts can be managed by the bot.
func (b *BotInstance) getSavedGifts(ctx context.Context, connectionID string, request *tg.PaymentsGetSavedStarGiftsRequest, ownerID int64, ownerIsUser, canBeManaged bool) (*converter.UserGifts, error) {
	var res tg.PaymentsSavedStarGifts
	if err := b.invokeMaybeBusiness(ctx, connectionID, request, &res); err != nil {
		return nil, err
	}
	b.peers.IngestPeers(res.Users, res.Chats)
	entities := converter.NewEntityContext(res.Users, res.Chats)
	result := &converter.UserGifts{TotalCount: res.Count, Gifts: []converter.OwnedGift{}, NextOffset: res.NextOffset}
	for _, saved := range res.Gifts {
		owned, err := convertSavedGift(saved, ownerID, ownerIsUser, canBeManaged, entities)
		if err != nil {
			return nil, err
		}
		if owned != nil {
			result.Gifts = append(result.Gifts, *owned)
		}
	}
	return result, nil
}

func convertSavedGift(saved tg.SavedStarGift, ownerID int64, ownerIsUser, canBeManaged bool, entities *converter.EntityContext) (*converter.OwnedGift, error) {
	owned := &converter.OwnedGift{
		SendDate:          saved.Date,
		Text:              saved.Message.Text,
		Entities:          converter.ConvertMTProtoEntities(saved.Message.Entities),
		IsPrivate:         saved.NameHidden,
		IsSaved:           !saved.Unsaved,
		CanBeUpgraded:     saved.CanUpgrade,
		WasRefunded:       saved.Refunded,
		ConvertStarCount:  saved.ConvertStars,
		UniqueGiftNumber:  saved.GiftNum,
		IsUpgradeSeparate: saved.UpgradeStars > 0 && saved.UpgradeSeparate,
	}
	if value, ok := saved.GetUpgradeStars(); ok {
		owned.PrepaidUpgradeStarCount = value
	}
	if value, ok := saved.GetTransferStars(); ok {
		owned.CanBeTransferred = true
		owned.TransferStarCount = value
	}
	if saved.CanTransferAt > 0 {
		owned.NextTransferDate = saved.CanTransferAt
	}
	if canBeManaged {
		if ownerIsUser {
			if saved.MsgID != 0 {
				owned.OwnedGiftID = strconv.Itoa(saved.MsgID)
			}
		} else if saved.SavedID != 0 {
			owned.OwnedGiftID = fmt.Sprintf("%d_%d", ownerID, saved.SavedID)
		}
	}
	if from, ok := saved.GetFromID(); ok {
		switch peer := from.(type) {
		case *tg.PeerUser:
			user := entities.GetUser(peer.UserID)
			if user == nil {
				user = &converter.User{ID: peer.UserID, FirstName: "User"}
			}
			owned.SenderUser = user
		default:
			chat := giftChat(from, entities)
			owned.SenderChat = &chat
		}
	}

	switch gift := saved.Gift.(type) {
	case *tg.StarGift:
		value, err := convertRegularGift(gift, entities)
		if err != nil {
			return nil, err
		}
		owned.Type, owned.Gift = "regular", value
	case *tg.StarGiftUnique:
		value, err := convertUniqueGift(gift, entities)
		if err != nil {
			return nil, err
		}
		owned.Type, owned.Gift = "unique", value
	default:
		return nil, nil
	}
	return owned, nil
}

func convertRegularGift(gift *tg.StarGift, entities *converter.EntityContext) (*converter.Gift, error) {
	value := &converter.Gift{
		ID:                     strconv.FormatInt(gift.ID, 10),
		StarCount:              gift.Stars,
		IsPremium:              gift.RequirePremium,
		HasColors:              gift.PeerColorAvailable,
		UniqueGiftVariantCount: gift.UpgradeVariants,
	}
	if document, ok := gift.Sticker.(*tg.Document); ok {
		sticker, err := convertStickerDocument(document)
		if err != nil {
			return nil, err
		}
		value.Sticker = &sticker
	}
	if upgrade, ok := gift.GetUpgradeStars(); ok {
		value.UpgradeStarCount = upgrade
	}
	if gift.Limited && gift.AvailabilityTotal > 0 {
		remaining, total := gift.AvailabilityRemains, gift.AvailabilityTotal
		value.RemainingCount, value.TotalCount = &remaining, &total
	}
	if gift.LimitedPerUser && gift.PerUserTotal > 0 {
		remaining, total := gift.PerUserRemains, gift.PerUserTotal
		value.PersonalRemainingCount, value.PersonalTotalCount = &remaining, &total
	}
	if publisher, ok := gift.GetReleasedBy(); ok {
		chat := giftChat(publisher, entities)
		value.PublisherChat = &chat
	}
	if background, ok := gift.GetBackground(); ok {
		value.Background = &converter.GiftBackground{
			CenterColor: background.CenterColor, EdgeColor: background.EdgeColor, TextColor: background.TextColor,
		}
	}
	return value, nil
}

func convertUniqueGift(gift *tg.StarGiftUnique, entities *converter.EntityContext) (*converter.UniqueGift, error) {
	value := &converter.UniqueGift{
		GiftID:    strconv.FormatInt(gift.GiftID, 10),
		BaseName:  gift.Title,
		Name:      gift.Slug,
		Number:    gift.Num,
		IsPremium: gift.RequirePremium,
		IsBurned:  gift.Burned,
	}
	for _, attribute := range gift.Attributes {
		switch attr := attribute.(type) {
		case *tg.StarGiftAttributeModel:
			value.Model.Name = attr.Name
			value.Model.Rarity, value.Model.RarityPerMille = giftRarity(attr.Rarity)
			if document, ok := attr.Document.(*tg.Document); ok {
				sticker, err := convertStickerDocument(document)
				if err != nil {
					return nil, err
				}
				value.Model.Sticker = sticker
			}
		case *tg.StarGiftAttributePattern:
			value.Symbol.Name = attr.Name
			value.Symbol.Rarity, value.Symbol.RarityPerMille = giftRarity(attr.Rarity)
			if document, ok := attr.Document.(*tg.Document); ok {
				sticker, err := convertStickerDocument(document)
				if err != nil {
					return nil, err
				}
				value.Symbol.Sticker = sticker
			}
		case *tg.StarGiftAttributeBackdrop:
			value.Backdrop.Name = attr.Name
			value.Backdrop.Rarity, value.Backdrop.RarityPerMille = giftRarity(attr.Rarity)
			value.Backdrop.Colors = converter.UniqueGiftBackdropColors{
				CenterColor: attr.CenterColor, EdgeColor: attr.EdgeColor,
				SymbolColor: attr.PatternColor, TextColor: attr.TextColor,
			}
		}
	}
	if publisher, ok := gift.GetReleasedBy(); ok {
		chat := giftChat(publisher, entities)
		value.PublisherChat = &chat
	}
	if color, ok := gift.GetPeerColor(); ok {
		if collectible, ok := color.(*tg.PeerColorCollectible); ok {
			value.Colors = &converter.UniqueGiftColors{
				ModelCustomEmojiID:    strconv.FormatInt(collectible.GiftEmojiID, 10),
				SymbolCustomEmojiID:   strconv.FormatInt(collectible.BackgroundEmojiID, 10),
				LightThemeMainColor:   collectible.AccentColor,
				LightThemeOtherColors: nonNilInts(collectible.Colors),
				DarkThemeMainColor:    collectible.DarkAccentColor,
				DarkThemeOtherColors:  nonNilInts(collectible.DarkColors),
			}
		}
	}
	if _, ok := gift.GetHostID(); ok {
		value.IsFromBlockchain = true
	}
	return value, nil
}

// giftRarity returns the Bot API rarity name and per-mille value; named rarities have per-mille 0.
func giftRarity(rarity tg.StarGiftAttributeRarityClass) (string, int) {
	switch value := rarity.(type) {
	case *tg.StarGiftAttributeRarity:
		return "", value.Permille
	case *tg.StarGiftAttributeRarityUncommon:
		return "uncommon", 0
	case *tg.StarGiftAttributeRarityRare:
		return "rare", 0
	case *tg.StarGiftAttributeRarityEpic:
		return "epic", 0
	case *tg.StarGiftAttributeRarityLegendary:
		return "legendary", 0
	}
	return "", 0
}

func giftChat(peer tg.PeerClass, entities *converter.EntityContext) converter.Chat {
	var chatID int64
	chatType := "private"
	switch value := peer.(type) {
	case *tg.PeerUser:
		chatID = value.UserID
	case *tg.PeerChat:
		chatID, chatType = -value.ChatID, "group"
	case *tg.PeerChannel:
		chatID, chatType = -1000000000000-value.ChannelID, "channel"
	}
	if chat := entities.GetChat(chatID); chat != nil {
		return *chat
	}
	chat := converter.Chat{ID: chatID, Type: chatType}
	if user := entities.GetUser(chatID); chatType == "private" && user != nil {
		chat.FirstName, chat.LastName, chat.Username = user.FirstName, user.LastName, user.Username
	}
	return chat
}

func nonNilInts(values []int) []int {
	if values == nil {
		return []int{}
	}
	return values
}
