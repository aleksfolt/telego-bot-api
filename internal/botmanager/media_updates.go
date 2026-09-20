package botmanager

import (
	"telego-bot-api/internal/converter"

	"github.com/gotd/td/tg"
)

// unpackUpdates normalizes the containers returned by send and edit RPCs.
func unpackUpdates(updates tg.UpdatesClass) ([]tg.UpdateClass, []tg.UserClass, []tg.ChatClass) {
	switch u := updates.(type) {
	case *tg.Updates:
		return u.Updates, u.Users, u.Chats
	case *tg.UpdatesCombined:
		return u.Updates, u.Users, u.Chats
	case *tg.UpdateShort:
		return []tg.UpdateClass{u.Update}, nil, nil
	default:
		return nil, nil, nil
	}
}

func messageFromUpdate(update tg.UpdateClass) *tg.Message {
	var message tg.MessageClass
	switch u := update.(type) {
	case *tg.UpdateNewMessage:
		message = u.Message
	case *tg.UpdateNewChannelMessage:
		message = u.Message
	case *tg.UpdateEditMessage:
		message = u.Message
	case *tg.UpdateEditChannelMessage:
		message = u.Message
	case *tg.UpdateBotNewBusinessMessage:
		message = u.Message
	case *tg.UpdateBotEditBusinessMessage:
		message = u.Message
	}
	msg, _ := message.(*tg.Message)
	return msg
}

func extractPhotoFromUpdates(updates tg.UpdatesClass) *tg.Photo {
	if u, ok := updates.(*tg.UpdateShortSentMessage); ok {
		return photoFromMedia(u.Media)
	}
	list, _, _ := unpackUpdates(updates)
	for _, update := range list {
		if msg := messageFromUpdate(update); msg != nil {
			if photo := photoFromMedia(msg.Media); photo != nil {
				return photo
			}
		}
	}
	return nil
}

func photoFromMedia(media tg.MessageMediaClass) *tg.Photo {
	if m, ok := media.(*tg.MessageMediaPhoto); ok {
		photo, _ := m.Photo.(*tg.Photo)
		return photo
	}
	return nil
}

func extractDocFromUpdates(updates tg.UpdatesClass) *tg.Document {
	if u, ok := updates.(*tg.UpdateShortSentMessage); ok {
		return documentFromMedia(u.Media)
	}
	list, _, _ := unpackUpdates(updates)
	for _, update := range list {
		if msg := messageFromUpdate(update); msg != nil {
			if doc := documentFromMedia(msg.Media); doc != nil {
				return doc
			}
		}
	}
	return nil
}

func documentFromMedia(media tg.MessageMediaClass) *tg.Document {
	if m, ok := media.(*tg.MessageMediaDocument); ok {
		doc, _ := m.Document.(*tg.Document)
		return doc
	}
	return nil
}

func extractPollFromUpdates(updates tg.UpdatesClass) *converter.Poll {
	if u, ok := updates.(*tg.UpdateShortSentMessage); ok {
		if media, ok := u.Media.(*tg.MessageMediaPoll); ok {
			return converter.ConvertPoll(media.Poll, media.Results)
		}
	}
	list, _, _ := unpackUpdates(updates)
	for _, update := range list {
		if msg := messageFromUpdate(update); msg != nil {
			if media, ok := msg.Media.(*tg.MessageMediaPoll); ok {
				return converter.ConvertPoll(media.Poll, media.Results)
			}
		}
	}
	return nil
}

func extractStoryID(updates tg.UpdatesClass) int {
	list, _, _ := unpackUpdates(updates)
	for _, update := range list {
		switch value := update.(type) {
		case *tg.UpdateStoryID:
			return value.ID
		case *tg.UpdateStory:
			return value.Story.GetID()
		}
	}
	return 0
}
