package botmanager

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"telego-bot-api/internal/converter"

	"github.com/gotd/td/tg"
)

// errNeedsLayer229 marks Bot API features whose MTProto constructors appeared in layer 229,
// while the gateway still speaks layer 228 (gotd v0.161).
func errNeedsLayer229(feature string) error {
	return fmt.Errorf("%s is not supported by the gateway yet: it requires MTProto layer 229", feature)
}

// richBuilder converts a Bot API rich message to MTProto, collecting the media and users
// referenced by its blocks.
type richBuilder struct {
	b            *BotInstance
	ctx          context.Context
	peer         tg.InputPeerClass
	connectionID string
	files        map[string][]byte
	fileNames    map[string]string

	photos    []tg.InputPhotoClass
	documents []tg.InputDocumentClass
	users     []tg.InputUserClass
	userSeen  map[int64]bool
}

// decodeJSONArg decodes a parameter that may be passed as a JSON value or as a JSON-encoded string.
func decodeJSONArg(raw json.RawMessage) (interface{}, error) {
	var encoded string
	if err := json.Unmarshal(raw, &encoded); err == nil {
		raw = json.RawMessage(encoded)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value interface{}
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

func jsonString(object map[string]interface{}, key string) string {
	value, _ := object[key].(string)
	return value
}

func jsonBool(object map[string]interface{}, key string) bool {
	switch value := object[key].(type) {
	case bool:
		return value
	case string:
		return value == "true" || value == "1"
	}
	return false
}

func jsonInt64(object map[string]interface{}, key string) (int64, bool) {
	switch value := object[key].(type) {
	case json.Number:
		if i, err := value.Int64(); err == nil {
			return i, true
		}
		if f, err := value.Float64(); err == nil {
			return int64(f), true
		}
	case string:
		if i, err := strconv.ParseInt(value, 10, 64); err == nil {
			return i, true
		}
	}
	return 0, false
}

func jsonInt(object map[string]interface{}, key string) int {
	value, _ := jsonInt64(object, key)
	return int(value)
}

func jsonFloat(object map[string]interface{}, key string) (float64, bool) {
	switch value := object[key].(type) {
	case json.Number:
		f, err := value.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(value, 64)
		return f, err == nil
	}
	return 0, false
}

// buildRichMessage converts the rich_message parameter to InputRichMessage.
func (rb *richBuilder) buildRichMessage(raw json.RawMessage) (tg.InputRichMessageClass, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("Rich message must be non-empty")
	}
	value, err := decodeJSONArg(raw)
	if err != nil {
		return nil, fmt.Errorf("Can't parse rich message JSON object")
	}
	object, ok := value.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("Object expected as rich message")
	}
	isRTL := jsonBool(object, "is_rtl")
	noAutolink := jsonBool(object, "skip_entity_detection")

	if blocksValue, ok := object["blocks"]; ok {
		blocks, err := rb.blocks(blocksValue)
		if err != nil {
			return nil, err
		}
		message := &tg.InputRichMessage{Rtl: isRTL, Noautolink: noAutolink, Blocks: blocks}
		if len(rb.photos) != 0 {
			message.SetPhotos(rb.photos)
		}
		if len(rb.documents) != 0 {
			message.SetDocuments(rb.documents)
		}
		if len(rb.users) != 0 {
			message.SetUsers(rb.users)
		}
		return message, nil
	}
	for _, source := range []string{"markdown", "html"} {
		if _, ok := object[source]; !ok {
			continue
		}
		text, ok := object[source].(string)
		if !ok {
			return nil, fmt.Errorf("Field \"%s\" must be of type String", source)
		}
		files, err := rb.richFiles(object["media"])
		if err != nil {
			return nil, err
		}
		if source == "markdown" {
			message := &tg.InputRichMessageMarkdown{Rtl: isRTL, Noautolink: noAutolink, Markdown: text}
			if len(files) != 0 {
				message.SetFiles(files)
			}
			return message, nil
		}
		message := &tg.InputRichMessageHTML{Rtl: isRTL, Noautolink: noAutolink, HTML: text}
		if len(files) != 0 {
			message.SetFiles(files)
		}
		return message, nil
	}
	return nil, fmt.Errorf("Rich message must be non-empty")
}

// richFiles converts the media of a Markdown or HTML rich message.
func (rb *richBuilder) richFiles(value interface{}) ([]tg.InputRichFileClass, error) {
	if value == nil {
		return nil, nil
	}
	items, ok := value.([]interface{})
	if !ok {
		return nil, fmt.Errorf("Field \"media\" must be of type Array")
	}
	files := make([]tg.InputRichFileClass, 0, len(items))
	for _, itemValue := range items {
		item, ok := itemValue.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("Can't parse InputRichMessageMedia: expected an Object")
		}
		id := jsonString(item, "id")
		if id == "" {
			return nil, fmt.Errorf("Can't parse InputRichMessageMedia: field \"id\" must be non-empty")
		}
		media, ok := item["media"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("Can't parse InputRichMessageMedia: field \"media\" must be an Object")
		}
		photo, document, err := rb.media(media, jsonString(media, "type"))
		if err != nil {
			return nil, err
		}
		if photo != nil {
			files = append(files, &tg.InputRichFilePhoto{ID: id, Photo: photo})
		} else {
			files = append(files, &tg.InputRichFileDocument{ID: id, Document: document})
		}
	}
	return files, nil
}

// media resolves an InputMedia object to an uploaded photo or document, uploading it when needed.
func (rb *richBuilder) media(object map[string]interface{}, expectedType string) (*tg.InputPhoto, *tg.InputDocument, error) {
	raw, err := json.Marshal(object)
	if err != nil {
		return nil, nil, err
	}
	var item converter.InputMediaItem
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, nil, fmt.Errorf("Can't parse InputMedia: %w", err)
	}
	if item.Type != expectedType {
		return nil, nil, fmt.Errorf("Unexpected media type \"%s\" for block \"%s\"", item.Type, expectedType)
	}
	switch item.Type {
	case "photo", "video", "animation", "audio", "document", "voice", "voice_note":
	default:
		return nil, nil, fmt.Errorf("Unsupported media type \"%s\"", item.Type)
	}
	if item.Type == "voice_note" {
		item.Type = "audio"
	}
	media, err := rb.b.resolveInputSingleMedia(rb.ctx, rb.peer, rb.connectionID, item, rb.files, rb.fileNames)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve media: %w", err)
	}
	switch value := media.(type) {
	case *tg.InputMediaPhoto:
		if photo, ok := value.ID.(*tg.InputPhoto); ok {
			return photo, nil, nil
		}
	case *tg.InputMediaDocument:
		if document, ok := value.ID.(*tg.InputDocument); ok {
			return nil, document, nil
		}
	}
	// Freshly uploaded or external media must be turned into a stored photo or document first.
	uploaded, err := rb.b.raw().MessagesUploadMedia(rb.ctx, &tg.MessagesUploadMediaRequest{
		BusinessConnectionID: rb.connectionID, Peer: rb.peer, Media: media,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("upload media: %w", err)
	}
	switch value := uploaded.(type) {
	case *tg.MessageMediaPhoto:
		if photo, ok := value.Photo.(*tg.Photo); ok {
			return &tg.InputPhoto{ID: photo.ID, AccessHash: photo.AccessHash, FileReference: photo.FileReference}, nil, nil
		}
	case *tg.MessageMediaDocument:
		if document, ok := value.Document.(*tg.Document); ok {
			return nil, &tg.InputDocument{ID: document.ID, AccessHash: document.AccessHash, FileReference: document.FileReference}, nil
		}
	}
	return nil, nil, fmt.Errorf("upload media: unexpected result %T", uploaded)
}

func (rb *richBuilder) blocks(value interface{}) ([]tg.PageBlockClass, error) {
	if value == nil {
		return []tg.PageBlockClass{}, nil
	}
	items, ok := value.([]interface{})
	if !ok {
		return nil, fmt.Errorf("Can't parse InputRichBlock: expected an Array")
	}
	blocks := make([]tg.PageBlockClass, 0, len(items))
	for _, item := range items {
		block, err := rb.block(item)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func (rb *richBuilder) caption(value interface{}) (tg.PageCaption, error) {
	caption := tg.PageCaption{Text: &tg.TextEmpty{}, Credit: &tg.TextEmpty{}}
	if value == nil {
		return caption, nil
	}
	object, ok := value.(map[string]interface{})
	if !ok {
		return caption, fmt.Errorf("RichBlockCaption must be an object")
	}
	var err error
	if caption.Text, err = rb.text(object["text"]); err != nil {
		return caption, err
	}
	if caption.Credit, err = rb.text(object["credit"]); err != nil {
		return caption, err
	}
	return caption, nil
}

func (rb *richBuilder) block(value interface{}) (tg.PageBlockClass, error) {
	object, ok := value.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("Object expected as InputRichMessageBlock")
	}
	blockType := jsonString(object, "type")
	if blockType == "" {
		return nil, fmt.Errorf("Field \"type\" must be non-empty")
	}
	text := func(key string) (tg.RichTextClass, error) { return rb.text(object[key]) }

	switch blockType {
	case "heading", "buttons", "document", "expandable_blockquote":
		return nil, errNeedsLayer229(fmt.Sprintf("Rich block \"%s\"", blockType))
	case "paragraph", "footer", "thinking":
		t, err := text("text")
		if err != nil {
			return nil, err
		}
		switch blockType {
		case "paragraph":
			return &tg.PageBlockParagraph{Text: t}, nil
		case "footer":
			return &tg.PageBlockFooter{Text: t}, nil
		default:
			return &tg.PageBlockThinking{Text: t}, nil
		}
	case "pre":
		t, err := text("text")
		if err != nil {
			return nil, err
		}
		return &tg.PageBlockPreformatted{Text: t, Language: jsonString(object, "language")}, nil
	case "divider":
		return &tg.PageBlockDivider{}, nil
	case "mathematical_expression":
		return &tg.PageBlockMath{Source: jsonString(object, "expression")}, nil
	case "anchor":
		return &tg.PageBlockAnchor{Name: jsonString(object, "name")}, nil
	case "list":
		return rb.list(object["items"])
	case "blockquote":
		blocks, err := rb.blocks(object["blocks"])
		if err != nil {
			return nil, err
		}
		credit, err := text("credit")
		if err != nil {
			return nil, err
		}
		return &tg.PageBlockBlockquoteBlocks{Blocks: blocks, Caption: credit}, nil
	case "pullquote":
		t, err := text("text")
		if err != nil {
			return nil, err
		}
		credit, err := text("credit")
		if err != nil {
			return nil, err
		}
		return &tg.PageBlockPullquote{Text: t, Caption: credit}, nil
	case "collage", "slideshow":
		blocks, err := rb.blocks(object["blocks"])
		if err != nil {
			return nil, err
		}
		caption, err := rb.caption(object["caption"])
		if err != nil {
			return nil, err
		}
		if blockType == "collage" {
			return &tg.PageBlockCollage{Items: blocks, Caption: caption}, nil
		}
		return &tg.PageBlockSlideshow{Items: blocks, Caption: caption}, nil
	case "table":
		return rb.table(object)
	case "details":
		summary, err := text("summary")
		if err != nil {
			return nil, err
		}
		blocks, err := rb.blocks(object["blocks"])
		if err != nil {
			return nil, err
		}
		return &tg.PageBlockDetails{Open: jsonBool(object, "is_open"), Blocks: blocks, Title: summary}, nil
	case "map":
		location, ok := object["location"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("Field \"location\" must be an Object")
		}
		latitude, okLat := jsonFloat(location, "latitude")
		longitude, okLong := jsonFloat(location, "longitude")
		if !okLat || !okLong {
			return nil, fmt.Errorf("Location latitude and longitude are required")
		}
		geo := &tg.InputGeoPoint{Lat: latitude, Long: longitude}
		if accuracy, ok := jsonFloat(location, "horizontal_accuracy"); ok && accuracy > 0 {
			geo.SetAccuracyRadius(int(accuracy))
		}
		caption, err := rb.caption(object["caption"])
		if err != nil {
			return nil, err
		}
		return &tg.InputPageBlockMap{Geo: geo, Zoom: jsonInt(object, "zoom"), W: jsonInt(object, "width"),
			H: jsonInt(object, "height"), Caption: caption}, nil
	case "photo", "video", "animation", "audio", "voice_note":
		mediaObject, ok := object[blockType].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("Field \"%s\" must be an Object", blockType)
		}
		photo, document, err := rb.media(mediaObject, blockType)
		if err != nil {
			return nil, err
		}
		caption, err := rb.caption(object["caption"])
		if err != nil {
			return nil, err
		}
		spoiler := jsonBool(mediaObject, "has_spoiler")
		if blockType == "photo" {
			if photo == nil {
				return nil, fmt.Errorf("Photo expected for block \"photo\"")
			}
			rb.photos = append(rb.photos, photo)
			return &tg.PageBlockPhoto{Spoiler: spoiler, PhotoID: photo.ID, Caption: caption}, nil
		}
		if document == nil {
			return nil, fmt.Errorf("Document expected for block \"%s\"", blockType)
		}
		rb.documents = append(rb.documents, document)
		switch blockType {
		case "video":
			return &tg.PageBlockVideo{Spoiler: spoiler, VideoID: document.ID, Caption: caption}, nil
		case "animation":
			return &tg.PageBlockVideo{Autoplay: true, Loop: true, Spoiler: spoiler, VideoID: document.ID, Caption: caption}, nil
		default:
			return &tg.PageBlockAudio{AudioID: document.ID, Caption: caption}, nil
		}
	}
	return nil, fmt.Errorf("type \"%s\" is unsupported", blockType)
}

func (rb *richBuilder) list(value interface{}) (tg.PageBlockClass, error) {
	items, ok := value.([]interface{})
	if !ok || len(items) == 0 {
		return nil, fmt.Errorf("List must be non-empty")
	}
	var unordered []tg.PageListItemClass
	var ordered []tg.PageListOrderedItemClass
	for _, itemValue := range items {
		item, ok := itemValue.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("Object expected as InputRichBlockListItem")
		}
		blocks, err := rb.blocks(item["blocks"])
		if err != nil {
			return nil, err
		}
		hasCheckbox := jsonBool(item, "has_checkbox")
		checked := hasCheckbox && jsonBool(item, "is_checked")
		itemType := jsonString(item, "type")
		if itemType == "" {
			unordered = append(unordered, &tg.PageListItemBlocks{Checkbox: hasCheckbox, Checked: checked, Blocks: blocks})
			continue
		}
		if len(itemType) != 1 || !strings.Contains("aAiI1", itemType) {
			return nil, fmt.Errorf("Invalid list item type specified")
		}
		number := jsonInt(item, "value")
		ordered = append(ordered, &tg.PageListOrderedItemBlocks{Checkbox: hasCheckbox, Checked: checked,
			Num: orderedListLabel(number, itemType), Blocks: blocks, Value: number, Type: itemType})
	}
	if len(unordered) != 0 && len(ordered) != 0 {
		return nil, fmt.Errorf("List must be either ordered or unordered")
	}
	if len(ordered) != 0 {
		return &tg.PageBlockOrderedList{Items: ordered}, nil
	}
	return &tg.PageBlockList{Items: unordered}, nil
}

// orderedListLabel mirrors TDLib's get_ordered_list_label.
func orderedListLabel(number int, listType string) string {
	if (listType == "a" || listType == "A") && number > 0 {
		result := ""
		for n := number; n > 0; n /= 26 {
			n--
			result = string(rune('A'+n%26)) + result
		}
		if listType == "a" {
			result = strings.ToLower(result)
		}
		return result + "."
	}
	if (listType == "i" || listType == "I") && number > 0 && number < 4000 {
		values := []int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
		symbols := []string{"M", "CM", "D", "CD", "C", "XC", "L", "XL", "X", "IX", "V", "IV", "I"}
		result := ""
		for i, v := range values {
			for number >= v {
				result += symbols[i]
				number -= v
			}
		}
		if listType == "i" {
			result = strings.ToLower(result)
		}
		return result + "."
	}
	return strconv.Itoa(number) + "."
}

func (rb *richBuilder) table(object map[string]interface{}) (tg.PageBlockClass, error) {
	rowsValue, ok := object["cells"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("Field \"cells\" must be of type Array")
	}
	rows := make([]tg.PageTableRow, 0, len(rowsValue))
	for _, rowValue := range rowsValue {
		cellsValue, ok := rowValue.([]interface{})
		if !ok {
			return nil, fmt.Errorf("Can't parse PageBlockTableCell: expected an Array")
		}
		row := tg.PageTableRow{Cells: make([]tg.PageTableCell, 0, len(cellsValue))}
		for _, cellValue := range cellsValue {
			cellObject, ok := cellValue.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("RichBlockTableCell must be an object")
			}
			cellText, err := rb.text(cellObject["text"])
			if err != nil {
				return nil, err
			}
			isHeader := jsonBool(cellObject, "is_header")
			cell := tg.PageTableCell{Header: isHeader, Text: cellText}
			if colspan := jsonInt(cellObject, "colspan"); colspan > 0 {
				cell.SetColspan(colspan)
			}
			if rowspan := jsonInt(cellObject, "rowspan"); rowspan > 0 {
				cell.SetRowspan(rowspan)
			}
			switch align := jsonString(cellObject, "align"); {
			case align == "left" || (align == "" && !isHeader):
			case align == "center" || (align == "" && isHeader):
				cell.AlignCenter = true
			case align == "right":
				cell.AlignRight = true
			default:
				return nil, fmt.Errorf("Invalid horizontal alignment specified")
			}
			switch jsonString(cellObject, "valign") {
			case "top":
			case "middle", "":
				cell.ValignMiddle = true
			case "bottom":
				cell.ValignBottom = true
			default:
				return nil, fmt.Errorf("Invalid vertical alignment specified")
			}
			row.Cells = append(row.Cells, cell)
		}
		rows = append(rows, row)
	}
	title, err := rb.text(object["caption"])
	if err != nil {
		return nil, err
	}
	// is_compact has no layer 228 counterpart and is ignored.
	return &tg.PageBlockTable{Bordered: jsonBool(object, "is_bordered"), Striped: jsonBool(object, "is_striped"),
		Title: title, Rows: rows}, nil
}

// text converts a Bot API RichText value: a string, an array of RichText, or a typed object.
func (rb *richBuilder) text(value interface{}) (tg.RichTextClass, error) {
	switch v := value.(type) {
	case nil:
		return &tg.TextEmpty{}, nil
	case string:
		if v == "" {
			return &tg.TextEmpty{}, nil
		}
		return &tg.TextPlain{Text: v}, nil
	case []interface{}:
		if len(v) == 0 {
			return &tg.TextEmpty{}, nil
		}
		texts := make([]tg.RichTextClass, 0, len(v))
		for _, item := range v {
			t, err := rb.text(item)
			if err != nil {
				return nil, err
			}
			texts = append(texts, t)
		}
		return &tg.TextConcat{Texts: texts}, nil
	case map[string]interface{}:
		return rb.typedText(v)
	}
	return nil, fmt.Errorf("Invalid rich text specified")
}

func (rb *richBuilder) typedText(object map[string]interface{}) (tg.RichTextClass, error) {
	textType := jsonString(object, "type")
	if textType == "" {
		return nil, fmt.Errorf("Field \"type\" must be non-empty")
	}
	inner, err := rb.text(object["text"])
	if err != nil {
		return nil, err
	}
	switch textType {
	case "bold":
		return &tg.TextBold{Text: inner}, nil
	case "italic":
		return &tg.TextItalic{Text: inner}, nil
	case "underline":
		return &tg.TextUnderline{Text: inner}, nil
	case "strikethrough":
		return &tg.TextStrike{Text: inner}, nil
	case "spoiler":
		return &tg.TextSpoiler{Text: inner}, nil
	case "code":
		return &tg.TextFixed{Text: inner}, nil
	case "subscript":
		return &tg.TextSubscript{Text: inner}, nil
	case "superscript":
		return &tg.TextSuperscript{Text: inner}, nil
	case "marked":
		return &tg.TextMarked{Text: inner}, nil
	case "mention", "hashtag", "cashtag", "bot_command", "bank_card_number":
		// Like TDLib, these are sent as plain text and detected by the server.
		return inner, nil
	case "date_time":
		unixTime, ok := jsonInt64(object, "unix_time")
		if !ok || unixTime <= 0 {
			return nil, fmt.Errorf("Invalid date specified")
		}
		date := &tg.TextDate{Text: inner, Date: int(unixTime)}
		if err := applyDateTimeFormat(date, jsonString(object, "date_time_format")); err != nil {
			return nil, err
		}
		return date, nil
	case "text_mention":
		user, ok := object["user"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("Field \"user\" must be an Object")
		}
		userID, ok := jsonInt64(user, "id")
		if !ok || userID <= 0 {
			return nil, fmt.Errorf("Field \"id\" must be a valid Number")
		}
		if !rb.userSeen[userID] {
			if rb.userSeen == nil {
				rb.userSeen = make(map[int64]bool)
			}
			rb.userSeen[userID] = true
			rb.users = append(rb.users, rb.b.inputUser(userID))
		}
		return &tg.TextMentionName{Text: inner, UserID: userID}, nil
	case "url":
		url := jsonString(object, "url")
		if url == "" {
			return nil, fmt.Errorf("Field \"url\" must be non-empty")
		}
		return &tg.TextURL{Text: inner, URL: url}, nil
	case "email_address":
		return &tg.TextEmail{Text: inner, Email: jsonString(object, "email_address")}, nil
	case "phone_number":
		return &tg.TextPhone{Text: inner, Phone: jsonString(object, "phone_number")}, nil
	case "custom_emoji":
		emojiID, ok := jsonInt64(object, "custom_emoji_id")
		if !ok || emojiID == 0 {
			return nil, fmt.Errorf("Invalid custom emoji identifier specified")
		}
		return &tg.TextCustomEmoji{DocumentID: emojiID, Alt: jsonString(object, "alternative_text")}, nil
	case "mathematical_expression":
		return &tg.TextMath{Source: jsonString(object, "expression")}, nil
	case "reference":
		return &tg.TextAnchor{Text: inner, Name: jsonString(object, "name")}, nil
	case "anchor":
		return &tg.TextAnchor{Text: &tg.TextEmpty{}, Name: jsonString(object, "name")}, nil
	case "reference_link":
		return &tg.TextURL{Text: inner, URL: "#" + anchorEncode(jsonString(object, "reference_name"))}, nil
	case "anchor_link":
		return &tg.TextURL{Text: inner, URL: "#" + anchorEncode(jsonString(object, "anchor_name"))}, nil
	case "button":
		return nil, errNeedsLayer229("Rich text \"button\"")
	}
	return nil, fmt.Errorf("Unsupported rich text type")
}

// applyDateTimeFormat applies a Bot API date_time_format ("r" or a combination of t, T, d, D, w)
// the way the official Bot API server does: the last time and date precision wins.
func applyDateTimeFormat(date *tg.TextDate, format string) error {
	if format == "" {
		return nil
	}
	if format == "r" || format == "R" {
		date.Relative = true
		return nil
	}
	timePrecision, datePrecision := byte(0), byte(0)
	for i := 0; i < len(format); i++ {
		switch c := format[i]; c {
		case 't', 'T':
			timePrecision = c
		case 'd', 'D':
			datePrecision = c
		case 'w', 'W':
			date.DayOfWeek = true
		default:
			return fmt.Errorf("Invalid date-time format specified")
		}
	}
	date.ShortTime, date.LongTime = timePrecision == 't', timePrecision == 'T'
	date.ShortDate, date.LongDate = datePrecision == 'd', datePrecision == 'D'
	return nil
}

// anchorEncode percent-encodes an anchor name like TDLib's url_encode.
func anchorEncode(value string) string {
	var result strings.Builder
	for i := 0; i < len(value); i++ {
		c := value[i]
		if ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') || strings.IndexByte("-_.~", c) >= 0 {
			result.WriteByte(c)
		} else {
			fmt.Fprintf(&result, "%%%02X", c)
		}
	}
	return result.String()
}

// SendRichMessage sends a rich message built from blocks, Markdown or HTML.
func (b *BotInstance) SendRichMessage(ctx context.Context, req *converter.SendRichMessageRequest) (*converter.Message, error) {
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}
	builder := &richBuilder{b: b, ctx: ctx, peer: peer, connectionID: req.BusinessConnectionID,
		files: req.Files, fileNames: req.FileNames}
	richMessage, err := builder.buildRichMessage(req.RichMessage)
	if err != nil {
		return nil, err
	}
	randomID, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	request := &tg.MessagesSendMessageRequest{Peer: peer, RandomID: randomID.Int64(),
		Silent: req.DisableNotification, Noforwards: req.ProtectContent}
	request.SetRichMessage(richMessage)
	if req.MessageThreadID != 0 || req.ReplyParameters != nil {
		replyTo := &tg.InputReplyToMessage{}
		if req.MessageThreadID != 0 {
			replyTo.SetTopMsgID(req.MessageThreadID)
		}
		if req.ReplyParameters != nil {
			replyTo.ReplyToMsgID = int(req.ReplyParameters.MessageID)
		}
		request.SetReplyTo(replyTo)
	}
	if req.MessageEffectID != "" {
		effect, err := strconv.ParseInt(req.MessageEffectID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("Invalid message_effect_id specified")
		}
		request.SetEffect(effect)
	}
	if len(req.ReplyMarkup) != 0 {
		markup, err := converter.ParseReplyMarkup(req.ReplyMarkup)
		if err != nil {
			return nil, err
		}
		request.SetReplyMarkup(markup)
	}
	var box tg.UpdatesBox
	if err := b.invokeMaybeBusiness(ctx, req.BusinessConnectionID, request, &box); err != nil {
		return nil, err
	}
	return b.sentMessage(box.Updates, &converter.Message{From: b.GetMe(), Chat: converter.Chat{ID: req.ChatID},
		Date: int(time.Now().Unix()), BusinessConnectionID: req.BusinessConnectionID}), nil
}

// SendRichMessageDraft streams a partial rich message. Layer 228 lacks the can_stop and
// keep_on_stop flags and requires pre-parsed blocks, so it is not supported yet.
func (b *BotInstance) SendRichMessageDraft(ctx context.Context, req *converter.SendRichMessageDraftRequest) (bool, error) {
	return false, errNeedsLayer229("sendRichMessageDraft")
}

// ephemeralReceiver returns the receiver_user_id of an ephemeral message request.
func ephemeralReceiver(receiverUserID, userID int64) (int64, error) {
	if receiverUserID != 0 {
		return receiverUserID, nil
	}
	if userID != 0 {
		return userID, nil
	}
	return 0, fmt.Errorf("Parameter \"receiver_user_id\" is required")
}

// DeleteEphemeralMessage deletes an ephemeral message shown to a user.
func (b *BotInstance) DeleteEphemeralMessage(ctx context.Context, req *converter.DeleteEphemeralMessageRequest) (bool, error) {
	receiverID, err := ephemeralReceiver(req.ReceiverUserID, req.UserID)
	if err != nil {
		return false, err
	}
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return false, fmt.Errorf("resolve peer: %w", err)
	}
	return b.raw().EphemeralDeleteMessage(ctx, &tg.EphemeralDeleteMessageRequest{
		Peer: peer, ReceiverID: b.inputUser(receiverID), ID: int(req.EphemeralMessageID),
	})
}

// EditEphemeralMessageText edits text of an ephemeral message (ephemeral.editMessage is layer 229).
func (b *BotInstance) EditEphemeralMessageText(ctx context.Context, req *converter.EditEphemeralMessageTextRequest) (bool, error) {
	return false, errNeedsLayer229("editEphemeralMessageText")
}

// EditEphemeralMessageMedia edits media of an ephemeral message (ephemeral.editMessage is layer 229).
func (b *BotInstance) EditEphemeralMessageMedia(ctx context.Context, req *converter.EditEphemeralMessageMediaRequest) (bool, error) {
	return false, errNeedsLayer229("editEphemeralMessageMedia")
}

// EditEphemeralMessageCaption edits caption of an ephemeral message (ephemeral.editMessage is layer 229).
func (b *BotInstance) EditEphemeralMessageCaption(ctx context.Context, req *converter.EditEphemeralMessageCaptionRequest) (bool, error) {
	return false, errNeedsLayer229("editEphemeralMessageCaption")
}

// EditEphemeralMessageReplyMarkup edits reply markup of an ephemeral message (ephemeral.editMessage is layer 229).
func (b *BotInstance) EditEphemeralMessageReplyMarkup(ctx context.Context, req *converter.EditEphemeralMessageReplyMarkupRequest) (bool, error) {
	return false, errNeedsLayer229("editEphemeralMessageReplyMarkup")
}
