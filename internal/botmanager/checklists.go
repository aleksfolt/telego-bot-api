package botmanager

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"time"

	"telego-bot-api/internal/converter"

	"github.com/gotd/td/tg"
)

// parseInputChecklist decodes a Bot API InputChecklist (a JSON object or a JSON-encoded string).
func parseInputChecklist(raw json.RawMessage) (tg.TodoList, error) {
	var encoded string
	if err := json.Unmarshal(raw, &encoded); err == nil {
		raw = json.RawMessage(encoded)
	}
	var input converter.InputChecklist
	if len(raw) == 0 || json.Unmarshal(raw, &input) != nil {
		return tg.TodoList{}, fmt.Errorf("Can't parse InputChecklist JSON object")
	}
	title, titleEntities, err := inlineMessageEntities(input.Title, input.ParseMode, input.TitleEntities)
	if err != nil {
		return tg.TodoList{}, fmt.Errorf("Can't parse InputChecklist: %w", err)
	}
	todo := tg.TodoList{
		OthersCanAppend:   input.OthersCanAddTasks,
		OthersCanComplete: input.OthersCanMarkTasksAsDone,
		Title:             tg.TextWithEntities{Text: title, Entities: titleEntities},
		List:              make([]tg.TodoItem, 0, len(input.Tasks)),
	}
	for _, task := range input.Tasks {
		text, entities, err := inlineMessageEntities(task.Text, task.ParseMode, task.TextEntities)
		if err != nil {
			return tg.TodoList{}, fmt.Errorf("Can't parse InputChecklist: %w", err)
		}
		todo.List = append(todo.List, tg.TodoItem{ID: task.ID, Title: tg.TextWithEntities{Text: text, Entities: entities}})
	}
	return todo, nil
}

// sentMessage converts the message contained in updates returned by a send or edit request.
// fallback is returned when Telegram answered without the full message.
func (b *BotInstance) sentMessage(updates tg.UpdatesClass, fallback *converter.Message) *converter.Message {
	list, users, chats := unpackUpdates(updates)
	b.peers.IngestPeers(users, chats)
	entities := converter.NewEntityContext(users, chats)
	for _, update := range list {
		if message := messageFromUpdate(update); message != nil {
			if converted, err := b.converter.ConvertMessage(message, entities); err == nil && converted != nil {
				converted.BusinessConnectionID = fallback.BusinessConnectionID
				return converted
			}
		}
	}
	if id := extractSentMessageID(updates); id != 0 {
		fallback.MessageID = id
	}
	return fallback
}

// SendChecklist sends a checklist on behalf of a connected business account or the bot.
func (b *BotInstance) SendChecklist(ctx context.Context, req *converter.SendChecklistRequest) (*converter.Message, error) {
	todo, err := parseInputChecklist(req.Checklist)
	if err != nil {
		return nil, err
	}
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}
	randomID, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	request := &tg.MessagesSendMediaRequest{Peer: peer, Media: &tg.InputMediaTodo{Todo: todo},
		RandomID: randomID.Int64(), Silent: req.DisableNotification, Noforwards: req.ProtectContent}
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

// EditMessageChecklist replaces the checklist of a message sent on behalf of a business account or the bot.
func (b *BotInstance) EditMessageChecklist(ctx context.Context, req *converter.EditMessageChecklistRequest) (interface{}, error) {
	todo, err := parseInputChecklist(req.Checklist)
	if err != nil {
		return nil, err
	}
	peer, err := b.resolvePeer(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer: %w", err)
	}
	request := &tg.MessagesEditMessageRequest{Peer: peer, ID: int(req.MessageID)}
	request.SetMedia(&tg.InputMediaTodo{Todo: todo})
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
	return b.sentMessage(box.Updates, &converter.Message{MessageID: req.MessageID, From: b.GetMe(),
		Chat: converter.Chat{ID: req.ChatID}, Date: int(time.Now().Unix()), BusinessConnectionID: req.BusinessConnectionID}), nil
}
