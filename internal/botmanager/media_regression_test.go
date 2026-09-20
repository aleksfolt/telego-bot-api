package botmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/fileid"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"telego-bot-api/internal/converter"
	"telego-bot-api/internal/peer"
)

// Use synthetic Telegram responses without connecting to Telegram or Redis.
func mediaTestBot(fn telegram.InvokeFunc) *BotInstance {
	client := telegram.NewClient(1, "test", telegram.Options{Middlewares: []telegram.Middleware{
		telegram.MiddlewareFunc(func(tg.Invoker) telegram.InvokeFunc { return fn }),
	}})
	bot := &BotInstance{
		client: client,
		raw:    tg.NewClient(fn), peers: peer.NewStorage(),
		converter: converter.NewMTProtoConverter(), logger: zap.NewNop(),
		self:            &converter.User{ID: 1, IsBot: true},
		busConns:        map[string]*converter.BusinessConnection{"business-test": {ID: "business-test", IsEnabled: true}},
		busConnDCs:      map[string]int{"business-test": 4},
		businessDCPools: make(map[int]telegram.CloseInvoker),
		mediaCache:      make(map[string]*tg.InputPhoto), docCache: make(map[string]*tg.InputDocument),
	}
	bot.businessDCFactory = func(context.Context, int) (telegram.CloseInvoker, error) {
		return testCloseInvoker{InvokeFunc: fn}, nil
	}
	bot.peers.SaveUser(123, 0)
	bot.peers.SaveUser(456, 0)
	return bot
}

type testCloseInvoker struct {
	telegram.InvokeFunc
}

func (testCloseInvoker) Close() error { return nil }

func TestRegressionExtractMediaUpdateVariants(t *testing.T) {
	doc := &tg.Document{ID: 123, AccessHash: 456, DCID: 2, MimeType: "audio/ogg"}
	photo := &tg.Photo{ID: 789, AccessHash: 456, DCID: 2}
	wrappers := map[string]func(tg.MessageClass) tg.UpdateClass{
		"new":           func(m tg.MessageClass) tg.UpdateClass { return &tg.UpdateNewMessage{Message: m} },
		"channel":       func(m tg.MessageClass) tg.UpdateClass { return &tg.UpdateNewChannelMessage{Message: m} },
		"edit":          func(m tg.MessageClass) tg.UpdateClass { return &tg.UpdateEditMessage{Message: m} },
		"channel_edit":  func(m tg.MessageClass) tg.UpdateClass { return &tg.UpdateEditChannelMessage{Message: m} },
		"business":      func(m tg.MessageClass) tg.UpdateClass { return &tg.UpdateBotNewBusinessMessage{Message: m} },
		"business_edit": func(m tg.MessageClass) tg.UpdateClass { return &tg.UpdateBotEditBusinessMessage{Message: m} },
	}
	containers := map[string]func(tg.UpdateClass) tg.UpdatesClass{
		"updates":  func(u tg.UpdateClass) tg.UpdatesClass { return &tg.Updates{Updates: []tg.UpdateClass{u}} },
		"combined": func(u tg.UpdateClass) tg.UpdatesClass { return &tg.UpdatesCombined{Updates: []tg.UpdateClass{u}} },
		"short":    func(u tg.UpdateClass) tg.UpdatesClass { return &tg.UpdateShort{Update: u} },
	}
	for name, wrap := range wrappers {
		for containerName, container := range containers {
			t.Run(name+"/"+containerName, func(t *testing.T) {
				docUpdates := container(wrap(&tg.Message{ID: 42, Media: &tg.MessageMediaDocument{Document: doc}}))
				photoUpdates := container(wrap(&tg.Message{ID: 43, Media: &tg.MessageMediaPhoto{Photo: photo}}))
				require.Same(t, doc, extractDocFromUpdates(docUpdates))
				require.Same(t, photo, extractPhotoFromUpdates(photoUpdates))
				require.Equal(t, int64(42), extractSentMessageID(docUpdates))
				require.Equal(t, int64(43), extractSentMessageID(photoUpdates))
			})
		}
	}
	t.Run("short_sent", func(t *testing.T) {
		require.Same(t, doc, extractDocFromUpdates(&tg.UpdateShortSentMessage{ID: 42, Media: &tg.MessageMediaDocument{Document: doc}}))
		require.Same(t, photo, extractPhotoFromUpdates(&tg.UpdateShortSentMessage{ID: 43, Media: &tg.MessageMediaPhoto{Photo: photo}}))
	})
	t.Run("empty", func(t *testing.T) {
		require.Nil(t, extractDocFromUpdates(nil))
		require.Nil(t, extractPhotoFromUpdates(&tg.Updates{}))
		require.Equal(t, int64(42), extractSentMessageID(&tg.Updates{Updates: []tg.UpdateClass{&tg.UpdateMessageID{ID: 42}}}))
	})
}

func TestRegressionSendPhotoReturnsUploadedMetadata(t *testing.T) {
	photo := &tg.Photo{ID: 123, AccessHash: 456, DCID: 2, Sizes: []tg.PhotoSizeClass{
		&tg.PhotoSize{Type: "x", W: 320, H: 240, Size: 1024},
	}}
	b := mediaTestBot(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		switch input.(type) {
		case *tg.UploadSaveFilePartRequest:
			output.(*tg.BoolBox).Bool = &tg.BoolTrue{}
		case *tg.MessagesSendMediaRequest:
			output.(*tg.UpdatesBox).Updates = &tg.Updates{Updates: []tg.UpdateClass{
				&tg.UpdateNewMessage{Message: &tg.Message{ID: 42, Media: &tg.MessageMediaPhoto{Photo: photo}}},
			}}
		default:
			t.Fatalf("unexpected request %T", input)
		}
		return nil
	})
	msg, err := b.SendPhoto(context.Background(), &converter.SendPhotoRequest{ChatID: 123, PhotoData: []byte("test photo")})
	require.NoError(t, err)
	require.NotEmpty(t, msg.Photo)
	_, err = fileid.DecodeFileID(msg.Photo[0].FileID)
	require.NoError(t, err, "uploaded photo must return a reusable Telegram file_id")
	require.Equal(t, 320, msg.Photo[0].Width)
	require.Equal(t, 240, msg.Photo[0].Height)
	require.Equal(t, 1024, msg.Photo[0].FileSize)
	require.NotEqual(t, "photo", msg.Photo[0].FileUniqueID)
	require.Equal(t, int64(42), msg.MessageID)
}

func TestRegressionClearCaptionSetsPresenceFlag(t *testing.T) {
	b := mediaTestBot(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		var wire bin.Buffer
		require.NoError(t, input.Encode(&wire))
		var decoded tg.MessagesEditMessageRequest
		require.NoError(t, decoded.Decode(&wire))
		_, present := decoded.GetMessage()
		require.True(t, present, "empty caption must be sent explicitly, not omitted")
		output.(*tg.UpdatesBox).Updates = &tg.Updates{}
		return nil
	})
	_, err := b.EditMessageCaption(context.Background(), &converter.EditMessageCaptionRequest{ChatID: 123, MessageID: 42, Caption: ""})
	require.NoError(t, err)
}

func TestRegressionSendMediaGroupChannelResponse(t *testing.T) {
	fid, err := fileid.EncodeFileID(fileid.FromPhoto(&tg.Photo{ID: 123, AccessHash: 456, DCID: 2}, 'x'))
	require.NoError(t, err)
	media, err := json.Marshal([]converter.InputMediaItem{{Type: "photo", Media: fid}, {Type: "photo", Media: fid}})
	require.NoError(t, err)
	b := mediaTestBot(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		output.(*tg.UpdatesBox).Updates = &tg.Updates{Updates: []tg.UpdateClass{
			&tg.UpdateNewChannelMessage{Message: &tg.Message{ID: 42}},
			&tg.UpdateNewChannelMessage{Message: &tg.Message{ID: 43}},
		}}
		return nil
	})
	b.peers.SaveChannel(123, 456)
	messages, err := b.SendMediaGroup(context.Background(), &converter.SendMediaGroupRequest{ChatID: -1000000000123, Media: media})
	require.NoError(t, err, "successful channel album must not turn into an API error")
	require.Len(t, messages, 2)
}

func TestRegressionSendMediaGroupPreservesThreadAndReply(t *testing.T) {
	fid, err := fileid.EncodeFileID(fileid.FromPhoto(&tg.Photo{ID: 123, AccessHash: 456, DCID: 2}, 'x'))
	require.NoError(t, err)
	media, err := json.Marshal([]converter.InputMediaItem{{Type: "photo", Media: fid}, {Type: "photo", Media: fid}})
	require.NoError(t, err)
	b := mediaTestBot(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		req := input.(*tg.MessagesSendMultiMediaRequest)
		require.Equal(t, &tg.InputReplyToMessage{ReplyToMsgID: 100, TopMsgID: 99}, req.ReplyTo,
			"album must carry message_thread_id and reply_parameters")
		output.(*tg.UpdatesBox).Updates = &tg.Updates{Updates: []tg.UpdateClass{&tg.UpdateNewMessage{Message: &tg.Message{ID: 42}}}}
		return nil
	})
	_, err = b.SendMediaGroup(context.Background(), &converter.SendMediaGroupRequest{ChatID: 123, Media: media, MessageThreadID: 99, ReplyParameters: &converter.ReplyParameters{MessageID: 100}})
	require.NoError(t, err)
}

func TestRegressionMediaNotificationSettings(t *testing.T) {
	photoID, err := fileid.EncodeFileID(fileid.FromPhoto(&tg.Photo{ID: 123, AccessHash: 456, DCID: 2}, 'x'))
	require.NoError(t, err)
	docID, err := fileid.EncodeFileID(fileid.FromDocument(&tg.Document{ID: 124, AccessHash: 456, DCID: 2, MimeType: "audio/ogg"}))
	require.NoError(t, err)
	ctx := context.Background()
	senders := map[string]func(*BotInstance, bool, string) (*converter.Message, error){
		"photo": func(b *BotInstance, silent bool, conn string) (*converter.Message, error) {
			return b.SendPhoto(ctx, &converter.SendPhotoRequest{ChatID: 123, Photo: photoID, DisableNotification: silent, BusinessConnectionID: conn})
		},
		"video": func(b *BotInstance, silent bool, conn string) (*converter.Message, error) {
			return b.SendVideo(ctx, &converter.SendVideoRequest{ChatID: 123, Video: docID, DisableNotification: silent, BusinessConnectionID: conn})
		},
		"document": func(b *BotInstance, silent bool, conn string) (*converter.Message, error) {
			return b.SendDocument(ctx, &converter.SendDocumentRequest{ChatID: 123, Document: docID, DisableNotification: silent, BusinessConnectionID: conn})
		},
		"voice": func(b *BotInstance, silent bool, conn string) (*converter.Message, error) {
			return b.SendVoice(ctx, &converter.SendVoiceRequest{ChatID: 123, Voice: docID, DisableNotification: silent, BusinessConnectionID: conn})
		},
		"video_note": func(b *BotInstance, silent bool, conn string) (*converter.Message, error) {
			return b.SendVideoNote(ctx, &converter.SendVideoNoteRequest{ChatID: 123, VideoNote: docID, DisableNotification: silent, BusinessConnectionID: conn})
		},
		"audio": func(b *BotInstance, silent bool, conn string) (*converter.Message, error) {
			return b.SendAudio(ctx, &converter.SendAudioRequest{ChatID: 123, Audio: docID, DisableNotification: silent, BusinessConnectionID: conn})
		},
		"sticker": func(b *BotInstance, silent bool, conn string) (*converter.Message, error) {
			return b.SendSticker(ctx, &converter.SendStickerRequest{ChatID: 123, Sticker: docID, DisableNotification: silent, BusinessConnectionID: conn})
		},
		"animation": func(b *BotInstance, silent bool, conn string) (*converter.Message, error) {
			return b.SendAnimation(ctx, &converter.SendAnimationRequest{ChatID: 123, Animation: docID, DisableNotification: silent, BusinessConnectionID: conn})
		},
	}
	for name, send := range senders {
		for _, conn := range []string{"", "business-test"} {
			for _, silent := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/business=%t/silent=%t", name, conn != "", silent), func(t *testing.T) {
					calls := 0
					b := mediaTestBot(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
						calls++
						if conn != "" {
							wrapped, ok := input.(*tg.InvokeWithBusinessConnectionRequest)
							require.True(t, ok)
							require.Equal(t, conn, wrapped.ConnectionID)
							input = wrapped.Query
						}
						var wire bin.Buffer
						require.NoError(t, input.Encode(&wire))
						var req tg.MessagesSendMediaRequest
						require.NoError(t, req.Decode(&wire))
						require.Equal(t, silent, req.Silent)
						output.(*tg.UpdatesBox).Updates = &tg.UpdateShortSentMessage{ID: 42}
						return nil
					})
					msg, err := send(b, silent, conn)
					require.NoError(t, err)
					require.Equal(t, int64(42), msg.MessageID)
					require.Equal(t, conn, msg.BusinessConnectionID)
					require.Equal(t, 1, calls)
				})
			}
		}
	}
}

func TestRegressionBusinessAlbumCombined(t *testing.T) {
	photo := &tg.Photo{ID: 123, AccessHash: 456, DCID: 2, Sizes: []tg.PhotoSizeClass{&tg.PhotoSize{Type: "x", W: 320, H: 240, Size: 1024}}}
	fid, err := fileid.EncodeFileID(fileid.FromPhoto(photo, 'x'))
	require.NoError(t, err)
	media, err := json.Marshal([]converter.InputMediaItem{{Type: "photo", Media: fid}, {Type: "photo", Media: fid}})
	require.NoError(t, err)
	b := mediaTestBot(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		wrapped := input.(*tg.InvokeWithBusinessConnectionRequest)
		require.Equal(t, "business-test", wrapped.ConnectionID)
		require.IsType(t, &tg.MessagesSendMultiMediaRequest{}, wrapped.Query)
		list := []tg.UpdateClass{&tg.UpdateMessageID{ID: 42}}
		for _, id := range []int{42, 43} {
			list = append(list, &tg.UpdateBotNewBusinessMessage{ConnectionID: "business-test", Message: &tg.Message{
				ID: id, PeerID: &tg.PeerUser{UserID: 123}, FromID: &tg.PeerUser{UserID: 456},
				Date: 1000, GroupedID: 777, Message: "caption", Media: &tg.MessageMediaPhoto{Photo: photo},
			}})
		}
		output.(*tg.UpdatesBox).Updates = &tg.UpdatesCombined{Updates: list,
			Users: []tg.UserClass{&tg.User{ID: 456, FirstName: "Business owner"}}}
		return nil
	})
	messages, err := b.SendMediaGroup(context.Background(), &converter.SendMediaGroupRequest{ChatID: 123, BusinessConnectionID: "business-test", Media: media})
	require.NoError(t, err)
	require.Len(t, messages, 2)
	for i, msg := range messages {
		require.Equal(t, int64(42+i), msg.MessageID)
		require.Equal(t, "business-test", msg.BusinessConnectionID)
		require.Equal(t, "777", msg.MediaGroupID)
		require.Equal(t, "caption", msg.Caption)
		require.Equal(t, int64(123), msg.Chat.ID)
		require.Equal(t, "Business owner", msg.From.FirstName)
		require.Equal(t, 1000, msg.Date)
		require.Len(t, msg.Photo, 1)
		require.Equal(t, fid, msg.Photo[0].FileID)
	}
}

func TestRegressionEditMediaClearsCaptionAndConvertsResponse(t *testing.T) {
	photo := &tg.Photo{ID: 123, AccessHash: 456, DCID: 2, Sizes: []tg.PhotoSizeClass{&tg.PhotoSize{Type: "x", W: 320, H: 240, Size: 1024}}}
	fid, err := fileid.EncodeFileID(fileid.FromPhoto(photo, 'x'))
	require.NoError(t, err)
	media, err := json.Marshal(converter.InputMediaItem{Type: "photo", Media: fid})
	require.NoError(t, err)
	for _, conn := range []string{"", "business-test"} {
		t.Run(fmt.Sprintf("business=%t", conn != ""), func(t *testing.T) {
			b := mediaTestBot(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
				if conn != "" {
					input = input.(*tg.InvokeWithBusinessConnectionRequest).Query
				}
				var wire bin.Buffer
				require.NoError(t, input.Encode(&wire))
				var request tg.MessagesEditMessageRequest
				require.NoError(t, request.Decode(&wire))
				caption, present := request.GetMessage()
				require.True(t, present)
				require.Empty(t, caption)
				msg := &tg.Message{ID: 42, PeerID: &tg.PeerChannel{ChannelID: 123}, Media: &tg.MessageMediaPhoto{Photo: photo}}
				var update tg.UpdateClass = &tg.UpdateEditChannelMessage{Message: msg}
				if conn != "" {
					msg.PeerID = &tg.PeerUser{UserID: 123}
					update = &tg.UpdateBotEditBusinessMessage{ConnectionID: conn, Message: msg}
				}
				output.(*tg.UpdatesBox).Updates = &tg.UpdatesCombined{Updates: []tg.UpdateClass{update}}
				return nil
			})
			b.peers.SaveChannel(123, 456)
			chatID := int64(-1000000000123)
			if conn != "" {
				chatID = 123
			}
			msg, err := b.EditMessageMedia(context.Background(), &converter.EditMessageMediaRequest{ChatID: chatID, MessageID: 42, BusinessConnectionID: conn, Media: media})
			require.NoError(t, err)
			require.Equal(t, int64(42), msg.MessageID)
			require.Equal(t, conn, msg.BusinessConnectionID)
			require.Len(t, msg.Photo, 1)
			require.Equal(t, fid, msg.Photo[0].FileID)
		})
	}
}
