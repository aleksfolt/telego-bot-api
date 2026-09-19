package storage_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAckSingleUpdatePreservesNeighbours(t *testing.T) {
	store := newTestStore(t)
	t.Cleanup(func() { store.Close() })
	const botID int64 = 90101
	cleanupBot(t, botID)
	t.Cleanup(func() { cleanupBot(t, botID) })
	first := appendUpdate(t, store, botID, "failed")
	appendUpdate(t, store, botID, "delivered")
	third := appendUpdate(t, store, botID, "pending")
	ctx := context.Background()
	entries, err := store.ReadUpdates(ctx, botID, 0, 100, 0)
	require.NoError(t, err)
	require.NoError(t, store.AckUpdate(ctx, botID, entries[1].StreamID))
	entries, err = store.ReadUpdates(ctx, botID, 0, 100, 0)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	require.Equal(t, first, entries[0].UpdateID)
	require.Equal(t, third, entries[1].UpdateID)
}

func TestReadUpdatesFindsOffsetBeyondFirstPage(t *testing.T) {
	store := newTestStore(t)
	t.Cleanup(func() { store.Close() })
	const botID int64 = 90102
	cleanupBot(t, botID)
	t.Cleanup(func() { cleanupBot(t, botID) })
	last := 0
	for i := 0; i < 205; i++ {
		last = appendUpdate(t, store, botID, "event")
	}
	entries, err := store.ReadUpdates(context.Background(), botID, last, 1, 0)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, last, entries[0].UpdateID)
}
