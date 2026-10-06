package botmanager

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"telego-bot-api/internal/converter"
	"telego-bot-api/internal/storage"
)

func TestAnswerChatJoinRequestQueryAcceptsStringAndNumberIDs(t *testing.T) {
	var got *tg.BotsSetJoinChatResultsRequest
	b := mediaTestBot(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		got = input.(*tg.BotsSetJoinChatResultsRequest)
		output.(*tg.BoolBox).Bool = &tg.BoolTrue{}
		return nil
	})

	for _, id := range []string{`"123"`, `123`} {
		var req converter.AnswerChatJoinRequestQueryRequest
		require.NoError(t, json.Unmarshal([]byte(`{"chat_join_request_query_id":`+id+`,"result":"Queue"}`), &req))
		ok, err := b.AnswerChatJoinRequestQuery(context.Background(), &req)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, int64(123), got.QueryID)
		require.IsType(t, &tg.JoinChatBotResultQueued{}, got.Result)
	}

	_, err := b.AnswerChatJoinRequestQuery(context.Background(), &converter.AnswerChatJoinRequestQueryRequest{
		ChatJoinRequestQueryID: "1", Result: "maybe",
	})
	require.EqualError(t, err, "Invalid query result specified")
}

func TestSendCustomRequestDecodesParametersString(t *testing.T) {
	b := mediaTestBot(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		request := input.(*tg.BotsSendCustomRequestRequest)
		require.Equal(t, "custom.method", request.CustomMethod)
		require.Equal(t, `{"a":1}`, request.Params.Data)
		output.(*tg.DataJSON).Data = `{"ok":true}`
		return nil
	})

	res, err := b.SendCustomRequest(context.Background(), &converter.SendCustomRequestRequest{
		Method: "custom.method", Parameters: json.RawMessage(`"{\"a\":1}"`),
	})
	require.NoError(t, err)
	require.Equal(t, json.RawMessage(`{"ok":true}`), res)
}

func starGiftForm(price int64) *tg.PaymentsPaymentFormStarGift {
	return &tg.PaymentsPaymentFormStarGift{FormID: 77, Invoice: tg.Invoice{Prices: []tg.LabeledPrice{{Amount: price}}}}
}

func TestUpgradeGiftPaysUpToStarCount(t *testing.T) {
	var sent bool
	b := mediaTestBot(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		switch query := businessQuery(t, input).(type) {
		case *tg.PaymentsGetPaymentFormRequest:
			invoice := query.Invoice.(*tg.InputInvoiceStarGiftUpgrade)
			require.Equal(t, &tg.InputSavedStarGiftUser{MsgID: 15}, invoice.Stargift)
			output.(*tg.PaymentsPaymentFormBox).PaymentForm = starGiftForm(20)
		case *tg.PaymentsSendStarsFormRequest:
			require.Equal(t, int64(77), query.FormID)
			sent = true
			output.(*tg.PaymentsPaymentResultBox).PaymentResult = &tg.PaymentsPaymentResult{Updates: &tg.Updates{}}
		default:
			t.Fatalf("unexpected query %T", query)
		}
		return nil
	})

	ok, err := b.UpgradeGift(context.Background(), &converter.UpgradeGiftRequest{
		BusinessConnectionID: "business-test", OwnedGiftID: "15", StarCount: 25,
	})
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, sent)

	_, err = b.UpgradeGift(context.Background(), &converter.UpgradeGiftRequest{
		BusinessConnectionID: "business-test", OwnedGiftID: "15", StarCount: 10,
	})
	require.EqualError(t, err, "Wrong upgrade price specified")
}

func TestTransferBusinessAccountStarsRequiresExactPrice(t *testing.T) {
	b := mediaTestBot(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		query := businessQuery(t, input).(*tg.PaymentsGetPaymentFormRequest)
		invoice := query.Invoice.(*tg.InputInvoiceBusinessBotTransferStars)
		require.Equal(t, int64(50), invoice.Stars)
		require.IsType(t, &tg.InputUserSelf{}, invoice.Bot)
		output.(*tg.PaymentsPaymentFormBox).PaymentForm = &tg.PaymentsPaymentFormStars{
			FormID: 1, Invoice: tg.Invoice{Prices: []tg.LabeledPrice{{Amount: 49}}},
		}
		return nil
	})

	_, err := b.TransferBusinessAccountStars(context.Background(), &converter.TransferBusinessAccountStarsRequest{
		BusinessConnectionID: "business-test", StarCount: 50,
	})
	require.EqualError(t, err, "Wrong transfer price specified")
}

func TestParseOwnedGiftIDFormats(t *testing.T) {
	b := mediaTestBot(nil)

	gift, err := b.parseOwnedGiftID("42")
	require.NoError(t, err)
	require.Equal(t, &tg.InputSavedStarGiftUser{MsgID: 42}, gift)

	gift, err = b.parseOwnedGiftID("@PlushPepe-1")
	require.NoError(t, err)
	require.Equal(t, &tg.InputSavedStarGiftSlug{Slug: "PlushPepe-1"}, gift)

	gift, err = b.parseOwnedGiftID("123_9")
	require.NoError(t, err)
	chatGift := gift.(*tg.InputSavedStarGiftChat)
	require.Equal(t, int64(9), chatGift.SavedID)

	for _, invalid := range []string{"", "@", "abc", "123_", "0"} {
		_, err := b.parseOwnedGiftID(invalid)
		require.Error(t, err, invalid)
	}
}

func TestGetBusinessAccountGiftsConvertsRegularAndUniqueGifts(t *testing.T) {
	b := mediaTestBot(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		query := businessQuery(t, input).(*tg.PaymentsGetSavedStarGiftsRequest)
		require.IsType(t, &tg.InputPeerSelf{}, query.Peer)
		require.True(t, query.ExcludeUpgradable)
		require.True(t, query.ExcludeUnupgradable)
		res := output.(*tg.PaymentsSavedStarGifts)
		res.Count = 2
		res.NextOffset = "next"
		res.Users = []tg.UserClass{&tg.User{ID: 456, FirstName: "Sender"}}
		regular := tg.SavedStarGift{
			FromID: &tg.PeerUser{UserID: 456}, Date: 1700000000, MsgID: 11, ConvertStars: 5, CanUpgrade: true,
			Gift: &tg.StarGift{ID: 1001, Stars: 50, Sticker: &tg.DocumentEmpty{}, Limited: true,
				AvailabilityRemains: 0, AvailabilityTotal: 100},
			Message: tg.TextWithEntities{Text: "hi"},
		}
		regular.SetFromID(regular.FromID)
		regular.SetUpgradeStars(25)
		unique := tg.SavedStarGift{
			Date: 1700000001, MsgID: 12, Unsaved: true,
			Gift: &tg.StarGiftUnique{GiftID: 1001, Title: "Pepe", Slug: "Pepe-7", Num: 7, Attributes: []tg.StarGiftAttributeClass{
				&tg.StarGiftAttributeModel{Name: "Gold", Document: &tg.DocumentEmpty{}, Rarity: &tg.StarGiftAttributeRarityRare{}},
				&tg.StarGiftAttributeBackdrop{Name: "Night", CenterColor: 1, EdgeColor: 2, PatternColor: 3, TextColor: 4,
					Rarity: &tg.StarGiftAttributeRarity{Permille: 15}},
			}},
		}
		unique.SetTransferStars(100)
		res.Gifts = []tg.SavedStarGift{regular, unique}
		return nil
	})

	gifts, err := b.GetBusinessAccountGifts(context.Background(), &converter.GetBusinessAccountGiftsRequest{
		BusinessConnectionID: "business-test", ExcludeLimited: true,
	})
	require.NoError(t, err)
	require.Equal(t, 2, gifts.TotalCount)
	require.Equal(t, "next", gifts.NextOffset)
	require.Len(t, gifts.Gifts, 2)

	regular := gifts.Gifts[0]
	require.Equal(t, "regular", regular.Type)
	require.Equal(t, "11", regular.OwnedGiftID)
	require.Equal(t, "Sender", regular.SenderUser.FirstName)
	require.True(t, regular.IsSaved)
	require.True(t, regular.CanBeUpgraded)
	require.Equal(t, int64(25), regular.PrepaidUpgradeStarCount)
	gift := regular.Gift.(*converter.Gift)
	require.Equal(t, "1001", gift.ID)
	require.Equal(t, 0, *gift.RemainingCount)
	require.Equal(t, 100, *gift.TotalCount)

	unique := gifts.Gifts[1]
	require.Equal(t, "unique", unique.Type)
	require.Equal(t, "12", unique.OwnedGiftID)
	require.False(t, unique.IsSaved)
	require.True(t, unique.CanBeTransferred)
	require.Equal(t, int64(100), unique.TransferStarCount)
	uniqueGift := unique.Gift.(*converter.UniqueGift)
	require.Equal(t, "Pepe-7", uniqueGift.Name)
	require.Equal(t, "rare", uniqueGift.Model.Rarity)
	require.Equal(t, 15, uniqueGift.Backdrop.RarityPerMille)
	require.Equal(t, 3, uniqueGift.Backdrop.Colors.SymbolColor)

	data, err := json.Marshal(gifts)
	require.NoError(t, err)
	require.Contains(t, string(data), `"remaining_count":0`)
	require.Contains(t, string(data), `"rarity_per_mille":0`)
}

func TestSendChecklistBuildsTodoMedia(t *testing.T) {
	b := mediaTestBot(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		request := businessQuery(t, input).(*tg.MessagesSendMediaRequest)
		todo := request.Media.(*tg.InputMediaTodo).Todo
		require.Equal(t, "Plan", todo.Title.Text)
		require.True(t, todo.OthersCanComplete)
		require.Len(t, todo.List, 2)
		require.Equal(t, 2, todo.List[1].ID)
		require.Equal(t, "Ship", todo.List[1].Title.Text)
		require.Len(t, todo.List[1].Title.Entities, 1)
		output.(*tg.UpdatesBox).Updates = &tg.UpdateShortSentMessage{ID: 90}
		return nil
	})

	message, err := b.SendChecklist(context.Background(), &converter.SendChecklistRequest{
		BusinessConnectionID: "business-test", ChatID: 123,
		Checklist: json.RawMessage(`"{\"title\":\"Plan\",\"others_can_mark_tasks_as_done\":true,` +
			`\"tasks\":[{\"id\":1,\"text\":\"Build\"},{\"id\":2,\"text\":\"<b>Ship</b>\",\"parse_mode\":\"HTML\"}]}"`),
	})
	require.NoError(t, err)
	require.Equal(t, int64(90), message.MessageID)
}

func TestConvertChecklistMedia(t *testing.T) {
	entities := converter.NewEntityContext([]tg.UserClass{&tg.User{ID: 456, FirstName: "Done"}}, nil)
	checklist := converter.ConvertChecklist(&tg.MessageMediaToDo{
		Todo: tg.TodoList{Title: tg.TextWithEntities{Text: "Plan"}, OthersCanAppend: true, List: []tg.TodoItem{
			{ID: 1, Title: tg.TextWithEntities{Text: "A"}}, {ID: 2, Title: tg.TextWithEntities{Text: "B"}},
		}},
		Completions: []tg.TodoCompletion{{ID: 2, CompletedBy: &tg.PeerUser{UserID: 456}, Date: 1700000000}},
	}, entities)
	require.Equal(t, "Plan", checklist.Title)
	require.True(t, checklist.OthersCanAddTasks)
	require.Nil(t, checklist.Tasks[0].CompletedByUser)
	require.Equal(t, "Done", checklist.Tasks[1].CompletedByUser.FirstName)
	require.Equal(t, 1700000000, checklist.Tasks[1].CompletionDate)
}

func TestSendRichMessageBuildsBlocks(t *testing.T) {
	b := mediaTestBot(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		request := input.(*tg.MessagesSendMessageRequest)
		rich, ok := request.GetRichMessage()
		require.True(t, ok)
		message := rich.(*tg.InputRichMessage)
		require.True(t, message.Noautolink)
		require.Len(t, message.Blocks, 3)

		paragraph := message.Blocks[0].(*tg.PageBlockParagraph)
		concat := paragraph.Text.(*tg.TextConcat)
		require.Equal(t, &tg.TextPlain{Text: "Hi "}, concat.Texts[0])
		require.Equal(t, int64(456), concat.Texts[1].(*tg.TextMentionName).UserID)
		date := concat.Texts[2].(*tg.TextDate)
		require.True(t, date.LongTime)
		require.True(t, date.ShortDate)
		require.Len(t, message.Users, 1)

		list := message.Blocks[1].(*tg.PageBlockOrderedList)
		require.Equal(t, "iv.", list.Items[0].(*tg.PageListOrderedItemBlocks).Num)

		table := message.Blocks[2].(*tg.PageBlockTable)
		require.True(t, table.Rows[0].Cells[0].AlignCenter)
		require.True(t, table.Rows[0].Cells[0].ValignMiddle)
		output.(*tg.UpdatesBox).Updates = &tg.UpdateShortSentMessage{ID: 5}
		return nil
	})

	message, err := b.SendRichMessage(context.Background(), &converter.SendRichMessageRequest{
		ChatID: 123,
		RichMessage: json.RawMessage(`{"skip_entity_detection":true,"blocks":[
			{"type":"paragraph","text":["Hi ",{"type":"text_mention","text":"you","user":{"id":456}},
				{"type":"date_time","text":"now","unix_time":1700000000,"date_time_format":"tTd"}]},
			{"type":"list","items":[{"type":"i","value":4,"blocks":[{"type":"divider"}]}]},
			{"type":"table","cells":[[{"text":"H","is_header":true}]]}
		]}`),
	})
	require.NoError(t, err)
	require.Equal(t, int64(5), message.MessageID)
}

func TestSendRichMessageMarkdownAndUnsupportedBlocks(t *testing.T) {
	b := mediaTestBot(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		rich, _ := input.(*tg.MessagesSendMessageRequest).GetRichMessage()
		require.Equal(t, "# Title", rich.(*tg.InputRichMessageMarkdown).Markdown)
		output.(*tg.UpdatesBox).Updates = &tg.UpdateShortSentMessage{ID: 6}
		return nil
	})

	_, err := b.SendRichMessage(context.Background(), &converter.SendRichMessageRequest{
		ChatID: 123, RichMessage: json.RawMessage(`{"markdown":"# Title"}`),
	})
	require.NoError(t, err)

	for _, rich := range []string{
		`{"blocks":[{"type":"heading","text":"x","size":1}]}`,
		`{"blocks":[{"type":"paragraph","text":{"type":"button","button":{"text":"x"}}}]}`,
	} {
		_, err := b.SendRichMessage(context.Background(), &converter.SendRichMessageRequest{
			ChatID: 123, RichMessage: json.RawMessage(rich),
		})
		require.ErrorContains(t, err, "requires MTProto layer 229")
	}
}

func TestOrderedListLabelAndDateFormat(t *testing.T) {
	require.Equal(t, "3.", orderedListLabel(3, "1"))
	require.Equal(t, "AB.", orderedListLabel(28, "A"))
	require.Equal(t, "xiv.", orderedListLabel(14, "i"))
	require.Equal(t, "MCMXCIV.", orderedListLabel(1994, "I"))

	date := &tg.TextDate{}
	require.NoError(t, applyDateTimeFormat(date, "r"))
	require.True(t, date.Relative)
	date = &tg.TextDate{}
	require.NoError(t, applyDateTimeFormat(date, "tTDw"))
	require.True(t, date.LongTime)
	require.False(t, date.ShortTime)
	require.True(t, date.LongDate)
	require.True(t, date.DayOfWeek)
	require.Error(t, applyDateTimeFormat(&tg.TextDate{}, "x"))
	require.Equal(t, "a%20b%23", anchorEncode("a b#"))
}

func TestDeleteEphemeralMessageUsesReceiver(t *testing.T) {
	b := mediaTestBot(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		request := input.(*tg.EphemeralDeleteMessageRequest)
		require.Equal(t, 9, request.ID)
		require.Equal(t, int64(456), request.ReceiverID.(*tg.InputUser).UserID)
		output.(*tg.BoolBox).Bool = &tg.BoolTrue{}
		return nil
	})

	ok, err := b.DeleteEphemeralMessage(context.Background(), &converter.DeleteEphemeralMessageRequest{
		ChatID: 123, ReceiverUserID: 456, EphemeralMessageID: 9,
	})
	require.NoError(t, err)
	require.True(t, ok)

	_, err = b.EditEphemeralMessageText(context.Background(), &converter.EditEphemeralMessageTextRequest{})
	require.ErrorContains(t, err, "requires MTProto layer 229")
}

func TestManagedBotAccessSettings(t *testing.T) {
	b := mediaTestBot(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		switch request := input.(type) {
		case *tg.BotsGetAccessSettingsRequest:
			res := output.(*tg.BotsAccessSettings)
			res.Restricted = true
			res.AddUsers = []tg.UserClass{&tg.User{ID: 456, FirstName: "Friend"}}
		case *tg.BotsEditAccessSettingsRequest:
			require.True(t, request.Restricted)
			require.Len(t, request.AddUsers, 1)
			output.(*tg.BoolBox).Bool = &tg.BoolTrue{}
		default:
			t.Fatalf("unexpected request %T", input)
		}
		return nil
	})

	settings, err := b.GetManagedBotAccessSettings(context.Background(), 123)
	require.NoError(t, err)
	require.True(t, settings.IsAccessRestricted)
	require.Equal(t, "Friend", settings.AddedUsers[0].FirstName)

	ok, err := b.SetManagedBotAccessSettings(context.Background(), &converter.SetManagedBotAccessSettingsRequest{
		UserID: 123, IsAccessRestricted: true, AddedUserIDs: []int64{456},
	})
	require.NoError(t, err)
	require.True(t, ok)
}

func TestPhotoFileNameAddsExtensionFromContent(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	jpeg := []byte("\xff\xd8\xff\xe0\x00\x10JFIF")
	gif := []byte("GIF89a\x01\x00\x01\x00")

	require.Equal(t, "4498e8c8-63dc.png", photoFileName("4498e8c8-63dc", png))
	require.Equal(t, "4498e8c8-63dc.jpg", photoFileName("4498e8c8-63dc", jpeg))
	require.Equal(t, "a.gif", photoFileName("a", gif))
	require.Equal(t, "photo.jpg", photoFileName("", jpeg))
	// A wrong extension is replaced, a valid one is kept as is.
	require.Equal(t, "post.png", photoFileName("post.dat", png))
	require.Equal(t, "Cat.JPG", photoFileName("Cat.JPG", jpeg))
	// Unknown content falls back to .jpg.
	require.Equal(t, "x.jpg", photoFileName("x", []byte("not an image")))
}

func TestHandleDoesNotBlockOnRedis(t *testing.T) {
	b := mediaTestBot(nil)
	b.botID = 42
	b.logger = zap.NewNop()
	// Nothing listens on port 1: every Redis call fails and the worker keeps retrying.
	b.redisStore = storage.NewRedisStore("127.0.0.1:1", "", 0)
	t.Cleanup(b.stopUpdateWorker)

	start := time.Now()
	for i := 0; i < 10; i++ {
		require.NoError(t, b.Handle(context.Background(), &tg.UpdateShort{
			Update: &tg.UpdateNewMessage{Message: &tg.Message{ID: i, PeerID: &tg.PeerUser{UserID: 123}}},
		}))
	}
	// Handle runs on the MTProto read loop and must only enqueue.
	require.Less(t, time.Since(start), 500*time.Millisecond)
}
