package converter

import (
	"strconv"

	"github.com/gotd/td/tg"
)

// ConvertEntities converts a slice of Bot API MessageEntity into MTProto MessageEntityClass.
func ConvertEntities(entities []MessageEntity) []tg.MessageEntityClass {
	if len(entities) == 0 {
		return nil
	}

	var res []tg.MessageEntityClass
	for _, e := range entities {
		switch e.Type {
		case "bold":
			res = append(res, &tg.MessageEntityBold{Offset: e.Offset, Length: e.Length})
		case "italic":
			res = append(res, &tg.MessageEntityItalic{Offset: e.Offset, Length: e.Length})
		case "underline":
			res = append(res, &tg.MessageEntityUnderline{Offset: e.Offset, Length: e.Length})
		case "strikethrough":
			res = append(res, &tg.MessageEntityStrike{Offset: e.Offset, Length: e.Length})
		case "spoiler":
			res = append(res, &tg.MessageEntitySpoiler{Offset: e.Offset, Length: e.Length})
		case "blockquote", "expandable_blockquote":
			res = append(res, &tg.MessageEntityBlockquote{Offset: e.Offset, Length: e.Length})
		case "code":
			res = append(res, &tg.MessageEntityCode{Offset: e.Offset, Length: e.Length})
		case "pre":
			res = append(res, &tg.MessageEntityPre{Offset: e.Offset, Length: e.Length, Language: e.Language})
		case "text_link":
			res = append(res, &tg.MessageEntityTextURL{Offset: e.Offset, Length: e.Length, URL: e.URL})
		case "mention":
			res = append(res, &tg.MessageEntityMention{Offset: e.Offset, Length: e.Length})
		case "hashtag":
			res = append(res, &tg.MessageEntityHashtag{Offset: e.Offset, Length: e.Length})
		case "cashtag":
			res = append(res, &tg.MessageEntityCashtag{Offset: e.Offset, Length: e.Length})
		case "bot_command":
			res = append(res, &tg.MessageEntityBotCommand{Offset: e.Offset, Length: e.Length})
		case "url":
			res = append(res, &tg.MessageEntityURL{Offset: e.Offset, Length: e.Length})
		case "email":
			res = append(res, &tg.MessageEntityEmail{Offset: e.Offset, Length: e.Length})
		case "phone_number":
			res = append(res, &tg.MessageEntityPhone{Offset: e.Offset, Length: e.Length})
		case "custom_emoji":
			if emojiID, err := strconv.ParseInt(e.CustomEmojiID, 10, 64); err == nil {
				res = append(res, &tg.MessageEntityCustomEmoji{Offset: e.Offset, Length: e.Length, DocumentID: emojiID})
			}
		}
	}
	return res
}

// ConvertMTProtoEntities converts MTProto MessageEntityClass slice into Bot API MessageEntity.
func ConvertMTProtoEntities(entities []tg.MessageEntityClass) []MessageEntity {
	if len(entities) == 0 {
		return nil
	}

	var res []MessageEntity
	for _, e := range entities {
		switch ent := e.(type) {
		case *tg.MessageEntityBold:
			res = append(res, MessageEntity{Type: "bold", Offset: ent.Offset, Length: ent.Length})
		case *tg.MessageEntityItalic:
			res = append(res, MessageEntity{Type: "italic", Offset: ent.Offset, Length: ent.Length})
		case *tg.MessageEntityUnderline:
			res = append(res, MessageEntity{Type: "underline", Offset: ent.Offset, Length: ent.Length})
		case *tg.MessageEntityStrike:
			res = append(res, MessageEntity{Type: "strikethrough", Offset: ent.Offset, Length: ent.Length})
		case *tg.MessageEntitySpoiler:
			res = append(res, MessageEntity{Type: "spoiler", Offset: ent.Offset, Length: ent.Length})
		case *tg.MessageEntityBlockquote:
			res = append(res, MessageEntity{Type: "blockquote", Offset: ent.Offset, Length: ent.Length})
		case *tg.MessageEntityCode:
			res = append(res, MessageEntity{Type: "code", Offset: ent.Offset, Length: ent.Length})
		case *tg.MessageEntityPre:
			res = append(res, MessageEntity{Type: "pre", Offset: ent.Offset, Length: ent.Length, Language: ent.Language})
		case *tg.MessageEntityTextURL:
			res = append(res, MessageEntity{Type: "text_link", Offset: ent.Offset, Length: ent.Length, URL: ent.URL})
		case *tg.MessageEntityMention:
			res = append(res, MessageEntity{Type: "mention", Offset: ent.Offset, Length: ent.Length})
		case *tg.MessageEntityHashtag:
			res = append(res, MessageEntity{Type: "hashtag", Offset: ent.Offset, Length: ent.Length})
		case *tg.MessageEntityCashtag:
			res = append(res, MessageEntity{Type: "cashtag", Offset: ent.Offset, Length: ent.Length})
		case *tg.MessageEntityBotCommand:
			res = append(res, MessageEntity{Type: "bot_command", Offset: ent.Offset, Length: ent.Length})
		case *tg.MessageEntityURL:
			res = append(res, MessageEntity{Type: "url", Offset: ent.Offset, Length: ent.Length})
		case *tg.MessageEntityEmail:
			res = append(res, MessageEntity{Type: "email", Offset: ent.Offset, Length: ent.Length})
		case *tg.MessageEntityPhone:
			res = append(res, MessageEntity{Type: "phone_number", Offset: ent.Offset, Length: ent.Length})
		case *tg.MessageEntityCustomEmoji:
			res = append(res, MessageEntity{
				Type:          "custom_emoji",
				Offset:        ent.Offset,
				Length:        ent.Length,
				CustomEmojiID: strconv.FormatInt(ent.DocumentID, 10),
			})
		}
	}
	return res
}
