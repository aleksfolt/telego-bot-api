package converter

import (
	"reflect"
	"strings"

	"github.com/gotd/td/telegram/message/entity"
	"github.com/gotd/td/telegram/message/html"
	"github.com/gotd/td/telegram/message/markdown"
	"github.com/gotd/td/tg"
)

// ParseTextFormatting parses formatted text according to parseMode ("HTML", "Markdown", "MarkdownV2").
// It returns the clean text (stripped of markup tags) and the corresponding MTProto entities.
func ParseTextFormatting(text, parseMode string) (string, []tg.MessageEntityClass, error) {
	if text == "" {
		return "", nil, nil
	}

	mode := strings.ToLower(strings.TrimSpace(parseMode))
	switch mode {
	case "html":
		var b entity.Builder
		if err := html.HTML(strings.NewReader(text), &b, html.Options{}); err != nil {
			// If HTML parsing encountered a syntax error, return original text without crashing
			return text, nil, err
		}
		cleanText, entities := b.Complete()
		return cleanText, clampEntityBounds(cleanText, entities), nil

	case "markdown", "markdownv2":
		var b entity.Builder
		if err := markdown.Markdown(strings.NewReader(text), &b, markdown.Options{}); err != nil {
			return text, nil, err
		}
		cleanText, entities := b.Complete()
		return cleanText, clampEntityBounds(cleanText, entities), nil

	default:
		// Plain text / unparsed
		return text, nil, nil
	}
}

// clampEntityBounds works around entity.Builder.Complete leaving outer nested
// entities one or more UTF-16 code units too long when it trims trailing space.
// Telegram rejects the whole request when any entity extends past the text.
func clampEntityBounds(text string, entities []tg.MessageEntityClass) []tg.MessageEntityClass {
	textLength := entity.ComputeLength(text)
	valid := entities[:0]

	for _, messageEntity := range entities {
		offset := messageEntity.GetOffset()
		length := messageEntity.GetLength()
		if offset < 0 || length <= 0 || offset >= textLength {
			continue
		}

		if maxLength := textLength - offset; length > maxLength {
			value := reflect.ValueOf(messageEntity)
			if value.Kind() != reflect.Pointer || value.IsNil() {
				continue
			}

			lengthField := value.Elem().FieldByName("Length")
			if !lengthField.IsValid() || !lengthField.CanSet() || lengthField.Kind() != reflect.Int {
				continue
			}
			lengthField.SetInt(int64(maxLength))
		}

		valid = append(valid, messageEntity)
	}

	return valid
}
