package storage_test

import (
	"context"
	"testing"

	"github.com/gotd/td/telegram/updates"
	"github.com/stretchr/testify/require"
)

func TestUpdatesStateStorage(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()
	const botID = int64(990001)
	cleanupBot(t, botID)
	ctx := context.Background()
	s := store.UpdatesStateStorage()

	_, found, err := s.GetState(ctx, botID)
	require.NoError(t, err)
	require.False(t, found)
	require.Error(t, s.SetPts(ctx, botID, 5), "updating a missing state must fail")

	require.NoError(t, s.SetState(ctx, botID, updates.State{Pts: 1, Qts: 2, Date: 3, Seq: 4}))
	require.NoError(t, s.SetPts(ctx, botID, 10))
	require.NoError(t, s.SetDateSeq(ctx, botID, 30, 40))
	require.NoError(t, s.SetChannelPts(ctx, botID, 777, 55))

	state, found, err := s.GetState(ctx, botID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, updates.State{Pts: 10, Qts: 2, Date: 30, Seq: 40}, state)

	pts, found, err := s.GetChannelPts(ctx, botID, 777)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, 55, pts)

	channels := map[int64]int{}
	require.NoError(t, s.ForEachChannels(ctx, botID, func(_ context.Context, id int64, pts int) error {
		channels[id] = pts
		return nil
	}))
	require.Equal(t, map[int64]int{777: 55}, channels)

	require.NoError(t, s.SetState(ctx, botID, updates.State{Pts: 100}))
	_, found, err = s.GetChannelPts(ctx, botID, 777)
	require.NoError(t, err)
	require.False(t, found, "a new state forgets channel states")
}
