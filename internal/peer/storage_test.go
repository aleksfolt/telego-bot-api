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
