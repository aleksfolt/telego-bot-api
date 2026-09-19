package peer

import (
	"fmt"
	"sync"

	"github.com/gotd/td/tg"
)

// Storage stores chat_id -> access_hash mappings for fast in-memory peer resolution.
type Storage struct {
	mu         sync.RWMutex
	userHashes map[int64]int64 // user_id -> access_hash
	chanHashes map[int64]int64 // channel_id -> access_hash
	chatExists map[int64]bool  // basic_chat_id -> exists
}

// NewStorage creates a new in-memory peer storage.
func NewStorage() *Storage {
	return &Storage{
		userHashes: make(map[int64]int64),
		chanHashes: make(map[int64]int64),
		chatExists: make(map[int64]bool),
	}
}

// Reset clears all in-memory peer mappings to release RAM during bot hibernation.
// If peers are needed later, they will be resolved from Redis.
func (s *Storage) Reset() {
	s.mu.Lock()
	s.userHashes = make(map[int64]int64)
	s.chanHashes = make(map[int64]int64)
	s.chatExists = make(map[int64]bool)
	s.mu.Unlock()
}

// SaveUser saves user access_hash.
func (s *Storage) SaveUser(userID, accessHash int64) {
	s.mu.Lock()
	s.userHashes[userID] = accessHash
	s.mu.Unlock()
}

// SaveChannel saves channel/supergroup access_hash.
func (s *Storage) SaveChannel(channelID, accessHash int64) {
	s.mu.Lock()
	s.chanHashes[channelID] = accessHash
	s.mu.Unlock()
}

// SaveChat registers a basic group chat.
func (s *Storage) SaveChat(chatID int64) {
	s.mu.Lock()
	s.chatExists[chatID] = true
	s.mu.Unlock()
}

// IngestPeers extracts and saves peers from incoming MTProto entities.
func (s *Storage) IngestPeers(users []tg.UserClass, chats []tg.ChatClass) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, u := range users {
		if user, ok := u.(*tg.User); ok && user.AccessHash != 0 {
			s.userHashes[user.ID] = user.AccessHash
		}
	}

	for _, c := range chats {
		switch chat := c.(type) {
		case *tg.Channel:
			if chat.AccessHash != 0 {
				s.chanHashes[chat.ID] = chat.AccessHash
			}
		case *tg.Chat:
			s.chatExists[chat.ID] = true
		}
	}
}

// ResolvePeer converts a Bot API chat_id (e.g. -1001234567890 or 12345678) to an MTProto InputPeerClass.
func (s *Storage) ResolvePeer(chatID int64) (tg.InputPeerClass, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 1. Channel / Supergroup: ID is typically -1000000000000 - channel_id
	if chatID < -1000000000000 {
		channelID := -chatID - 1000000000000
		hash, ok := s.chanHashes[channelID]
		if !ok {
			return nil, fmt.Errorf("channel %d (bot_api chat_id %d) not found in peer cache; access_hash missing", channelID, chatID)
		}
		return &tg.InputPeerChannel{
			ChannelID:  channelID,
			AccessHash: hash,
		}, nil
	}

	// 2. Basic Group Chat: ID is negative, e.g. -123456
	if chatID < 0 {
		basicChatID := -chatID
		return &tg.InputPeerChat{
			ChatID: basicChatID,
		}, nil
	}

	// 3. Private User Chat: ID is positive
	hash, ok := s.userHashes[chatID]
	if !ok {
		return nil, fmt.Errorf("user %d not found in peer cache; access_hash missing", chatID)
	}

	return &tg.InputPeerUser{
		UserID:     chatID,
		AccessHash: hash,
	}, nil
}
