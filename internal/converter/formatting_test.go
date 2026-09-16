package converter

import (
	"testing"
	"unicode/utf16"

	"github.com/stretchr/testify/require"
)

func TestParseTextFormattingNestedBlockquoteEntityBounds(t *testing.T) {
	input := "<b>Voice effects</b>\n\n<blockquote>\nChoose an effect for outgoing voice messages.\n\nCurrent mode: <b>Through the wall</b>\nIntensity: <b>Strong</b>\nOutgoing voice messages will be replaced automatically.\n</blockquote>"

	text, entities, err := ParseTextFormatting(input, "HTML")
	require.NoError(t, err)

	textLength := len(utf16.Encode([]rune(text)))
	for _, entity := range entities {
		require.LessOrEqual(t, entity.GetOffset()+entity.GetLength(), textLength,
			"entity %T is outside text %q", entity, text)
	}
}

func TestParseTextFormattingMarkdown(t *testing.T) {
	input := "*bold* and _italic_ and `code` and [link](https://example.com)"
	text, entities, err := ParseTextFormatting(input, "Markdown")
	require.NoError(t, err)
	require.Equal(t, "bold and italic and code and link", text)
	require.Len(t, entities, 4)
}

func TestParseTextFormattingMarkdownV2(t *testing.T) {
	input := "*bold text* and `code block`"
	text, entities, err := ParseTextFormatting(input, "MarkdownV2")
	require.NoError(t, err)
	require.Equal(t, "bold text and code block", text)
	require.Len(t, entities, 2)
}

