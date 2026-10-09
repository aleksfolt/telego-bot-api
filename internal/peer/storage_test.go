package peer

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestResolvePeerDoesNotHideMissingUserHash(t *testing.T) {
	storage := NewStorage()
	if _, err := storage.ResolvePeer(123); err == nil {
		t.Fatal("missing user must fall through to persistent peer lookup")
	}

	storage.SaveUser(123, 0)
	peer, err := storage.ResolvePeer(123)
	if err != nil {
		t.Fatalf("known zero-hash user must remain valid: %v", err)
	}
	user, ok := peer.(*tg.InputPeerUser)
	if !ok || user.UserID != 123 {
		t.Fatalf("unexpected peer: %#v", peer)
	}
}

func TestIngestPeersSkipsMinAccessHashes(t *testing.T) {
	s := NewStorage()
	s.IngestPeers([]tg.UserClass{&tg.User{ID: 7, AccessHash: 111}}, nil)
	// A min constructor must not overwrite the valid access_hash.
	s.IngestPeers([]tg.UserClass{&tg.User{ID: 7, AccessHash: 222, Min: true}},
		[]tg.ChatClass{&tg.Channel{ID: 9, AccessHash: 333, Min: true}})

	peer, err := s.ResolvePeer(7)
	if err != nil {
		t.Fatalf("resolve user: %v", err)
	}
	if hash := peer.(*tg.InputPeerUser).AccessHash; hash != 111 {
		t.Fatalf("access_hash = %d, want 111: min user overwrote it", hash)
	}
	if _, err := s.ResolvePeer(-1000000000009); err == nil {
		t.Fatal("min channel access_hash must not be stored")
	}
}
