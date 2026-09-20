package botmanager

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/require"
	"telego-bot-api/internal/converter"
)

func businessQuery(t *testing.T, input bin.Encoder) bin.Object {
	t.Helper()
	wrapped, ok := input.(*tg.InvokeWithBusinessConnectionRequest)
	require.True(t, ok, "business request must use invokeWithBusinessConnection")
	require.Equal(t, "business-test", wrapped.ConnectionID)
	return wrapped.Query
}

func TestRegressionBusinessCreateInvoiceLinkUsesConnectionDatacenter(t *testing.T) {
	b := mediaTestBot(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		require.IsType(t, &tg.PaymentsExportInvoiceRequest{}, businessQuery(t, input))
		output.(*tg.PaymentsExportedInvoice).URL = "https://t.me/$invoice"
		return nil
	})

	link, err := b.CreateInvoiceLink(context.Background(), &converter.CreateInvoiceLinkRequest{
		BusinessConnectionID: "business-test",
		Title:                "Test",
		Description:          "Test invoice",
		Payload:              "payload",
		Currency:             "XTR",
		Prices:               json.RawMessage(`[{"label":"Test","amount":1}]`),
	})
	require.NoError(t, err)
	require.Equal(t, "https://t.me/$invoice", link)
}

func TestRegressionBusinessSendGameUsesConnectionDatacenter(t *testing.T) {
	b := mediaTestBot(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		require.IsType(t, &tg.MessagesSendMediaRequest{}, businessQuery(t, input))
		output.(*tg.UpdatesBox).Updates = &tg.UpdateShortSentMessage{ID: 71}
		return nil
	})

	message, err := b.SendGame(context.Background(), &converter.SendGameRequest{
		BusinessConnectionID: "business-test", ChatID: 123, GameShortName: "game",
	})
	require.NoError(t, err)
	require.Equal(t, int64(71), message.MessageID)
}

func TestRegressionBusinessStopPollDoesNotReadBotHistory(t *testing.T) {
	b := mediaTestBot(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		query := businessQuery(t, input)
		edit, ok := query.(*tg.MessagesEditMessageRequest)
		require.True(t, ok)
		media, present := edit.GetMedia()
		require.True(t, present)
		require.True(t, media.(*tg.InputMediaPoll).Poll.Closed)

		poll := tg.Poll{ID: 99, Closed: true, Question: tg.TextWithEntities{Text: "Done"}, Answers: []tg.PollAnswerClass{}}
		output.(*tg.UpdatesBox).Updates = &tg.Updates{Updates: []tg.UpdateClass{
			&tg.UpdateBotEditBusinessMessage{ConnectionID: "business-test", Message: &tg.Message{
				ID: 72, Media: &tg.MessageMediaPoll{Poll: poll},
			}},
		}}
		return nil
	})

	poll, err := b.StopPoll(context.Background(), &converter.StopPollRequest{
		BusinessConnectionID: "business-test", ChatID: 123, MessageID: 72,
	})
	require.NoError(t, err)
	require.Equal(t, "99", poll.ID)
	require.True(t, poll.IsClosed)
}

func TestRegressionBusinessAccountReadsPropagateErrors(t *testing.T) {
	sentinel := errors.New("telegram rejected request")
	b := mediaTestBot(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		businessQuery(t, input)
		return sentinel
	})
	b.busConns["business-test"].UserChatID = 123

	_, err := b.GetBusinessAccountStarBalance(context.Background(), "business-test")
	require.ErrorIs(t, err, sentinel)

	_, err = b.GetBusinessAccountGifts(context.Background(), &converter.GetBusinessAccountGiftsRequest{
		BusinessConnectionID: "business-test",
	})
	require.ErrorIs(t, err, sentinel)
}

func TestRegressionBusinessGiftSettingsUseConnectionDatacenter(t *testing.T) {
	b := mediaTestBot(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		request, ok := businessQuery(t, input).(*tg.AccountSetGlobalPrivacySettingsRequest)
		require.True(t, ok)
		require.True(t, request.Settings.DisplayGiftsButton)
		require.False(t, request.Settings.DisallowedGifts.DisallowUnlimitedStargifts)
		require.True(t, request.Settings.DisallowedGifts.DisallowLimitedStargifts)
		require.False(t, request.Settings.DisallowedGifts.DisallowUniqueStargifts)
		require.True(t, request.Settings.DisallowedGifts.DisallowPremiumGifts)
		require.True(t, request.Settings.DisallowedGifts.DisallowStargiftsFromChannels)
		*output.(*tg.GlobalPrivacySettings) = request.Settings
		return nil
	})

	ok, err := b.SetBusinessAccountGiftSettings(context.Background(), &converter.SetBusinessAccountGiftSettingsRequest{
		BusinessConnectionID: "business-test",
		ShowGiftButton:       true,
		AcceptedGiftTypes: json.RawMessage(`{
			"unlimited_gifts":true,"limited_gifts":false,"unique_gifts":true,
			"premium_subscription":false,"gifts_from_channels":false
		}`),
	})
	require.NoError(t, err)
	require.True(t, ok)
}

func TestRegressionBusinessProfilePhotoUploadsInConnectionDatacenter(t *testing.T) {
	var uploaded, changed bool
	invoke := telegram.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		switch request := input.(type) {
		case *tg.UploadSaveFilePartRequest:
			uploaded = true
			output.(*tg.BoolBox).Bool = &tg.BoolTrue{}
		case *tg.InvokeWithBusinessConnectionRequest:
			require.True(t, uploaded, "file parts must be uploaded before changing the photo")
			require.IsType(t, &tg.PhotosUploadProfilePhotoRequest{}, request.Query)
			changed = true
			output.(*tg.PhotosPhoto).Photo = &tg.PhotoEmpty{}
		default:
			t.Fatalf("unexpected request %T", input)
		}
		return nil
	})
	b := mediaTestBot(invoke)

	ok, err := b.SetBusinessAccountProfilePhoto(context.Background(), &converter.SetBusinessAccountProfilePhotoRequest{
		BusinessConnectionID: "business-test", PhotoType: "static", PhotoData: []byte("jpeg"),
	})
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, changed)
}

func TestRegressionBusinessRepostStoryUsesForwardMetadataAndNewID(t *testing.T) {
	b := mediaTestBot(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		request, ok := input.(*tg.StoriesSendStoryRequest)
		require.True(t, ok, "stories.sendStory must be sent directly in the business DC")
		from, present := request.GetFwdFromID()
		require.True(t, present)
		require.IsType(t, &tg.InputPeerUser{}, from)
		storyID, present := request.GetFwdFromStory()
		require.True(t, present)
		require.Equal(t, 44, storyID)
		output.(*tg.UpdatesBox).Updates = &tg.Updates{Updates: []tg.UpdateClass{
			&tg.UpdateStoryID{ID: 91, RandomID: request.RandomID},
		}}
		return nil
	})
	b.busConns["business-test"].UserChatID = 123

	result, err := b.RepostStory(context.Background(), &converter.RepostStoryRequest{
		BusinessConnectionID: "business-test", FromChatID: 456, StoryID: 44,
	})
	require.NoError(t, err)
	story, ok := result.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, 91, story["id"])
}
