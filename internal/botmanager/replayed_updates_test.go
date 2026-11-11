package botmanager

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gotd/td/tg"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestBusinessUpdateIdentityIsSameForLiveAndReplayedUpdate(t *testing.T) {
	message := func(id, editDate int, text string) *tg.Message {
		m := &tg.Message{ID: id, PeerID: &tg.PeerUser{UserID: 123}, Message: text, Date: 1000}
		if editDate != 0 {
			m.SetEditDate(editDate)
		}
		return m
	}
	identity := func(u tg.UpdateClass) string {
		id, _, ok := businessUpdateIdentity(u)
		require.True(t, ok)
		return id
	}

	live := identity(&tg.UpdateBotNewBusinessMessage{ConnectionID: "c", Message: message(1, 0, "hi"), Qts: 7})
	require.Equal(t, live, identity(&tg.UpdateBotNewBusinessMessage{ConnectionID: "c", Message: message(1, 0, "hi")}))
	require.NotEqual(t, live, identity(&tg.UpdateBotNewBusinessMessage{ConnectionID: "c", Message: message(2, 0, "hi")}))
	require.NotEqual(t, live, identity(&tg.UpdateBotNewBusinessMessage{ConnectionID: "other", Message: message(1, 0, "hi")}))

	edit := identity(&tg.UpdateBotEditBusinessMessage{ConnectionID: "c", Message: message(1, 2000, "a"), Qts: 8})
	require.NotEqual(t, live, edit)
	require.Equal(t, edit, identity(&tg.UpdateBotEditBusinessMessage{ConnectionID: "c", Message: message(1, 2000, "a")}))
	require.NotEqual(t, edit, identity(&tg.UpdateBotEditBusinessMessage{ConnectionID: "c", Message: message(1, 2001, "a")}))
	require.NotEqual(t, edit, identity(&tg.UpdateBotEditBusinessMessage{ConnectionID: "c", Message: message(1, 2000, "b")}))
	_, date, _ := businessUpdateIdentity(&tg.UpdateBotEditBusinessMessage{ConnectionID: "c", Message: message(1, 2000, "a")})
	require.Equal(t, 2000, date)

	deleted := identity(&tg.UpdateBotDeleteBusinessMessage{ConnectionID: "c", Peer: &tg.PeerUser{UserID: 123}, Messages: []int{3, 1}, Qts: 9})
	require.Equal(t, deleted, identity(&tg.UpdateBotDeleteBusinessMessage{ConnectionID: "c", Peer: &tg.PeerUser{UserID: 123}, Messages: []int{1, 3}}))
	require.NotEqual(t, deleted, identity(&tg.UpdateBotDeleteBusinessMessage{ConnectionID: "c", Peer: &tg.PeerUser{UserID: 123}, Messages: []int{1}}))

	_, _, ok := businessUpdateIdentity(&tg.UpdateBotStopped{UserID: 1})
	require.False(t, ok)
}

// replayTestBot is deliveryTestBot whose Redis has remembered delivered updates since the
// given moment.
func replayTestBot(t *testing.T, id int64, seenSince time.Time) *BotInstance {
	t.Helper()
	b := deliveryTestBot(t, id)
	rdb := redis.NewClient(&redis.Options{Addr: os.Getenv("REDIS_ADDR"), DB: 15})
	t.Cleanup(func() { rdb.Close() })
	require.NoError(t, rdb.Set(context.Background(), "telego:seen_since", seenSince.Unix(), 0).Err())
	return b
}

func TestReplayedUpdateKnowable(t *testing.T) {
	now := time.Now()
	unix := func(ago time.Duration) int { return int(now.Add(-ago).Unix()) }
	longAgo := now.Add(-72 * time.Hour)

	require.True(t, replayedUpdateKnowable(unix(time.Hour), longAgo, now))
	require.False(t, replayedUpdateKnowable(unix(48*time.Hour), longAgo, now), "older than the window")
	require.False(t, replayedUpdateKnowable(unix(2*time.Hour), now.Add(-time.Hour), now), "before the memory started")
	require.True(t, replayedUpdateKnowable(unix(30*time.Minute), now.Add(-time.Hour), now))
	// A deletion has no date.
	require.True(t, replayedUpdateKnowable(0, longAgo, now))
	require.False(t, replayedUpdateKnowable(0, now.Add(-time.Hour), now), "the memory does not span the window yet")
}

// Right after the first start of a version that remembers delivered updates, replays of
// what was delivered before must not be taken for missed updates.
func TestReplayedUpdatesFromBeforeTheMemoryStartedAreDropped(t *testing.T) {
	b := replayTestBot(t, 815, time.Now().Add(-time.Minute))
	ctx := context.Background()
	conn := fmt.Sprintf("business-test-%d", time.Now().UnixNano())
	peer := &tg.PeerUser{UserID: 123}
	now := int(time.Now().Unix())
	message := func(id, date int) *tg.Message {
		return &tg.Message{ID: id, PeerID: peer, FromID: &tg.PeerUser{UserID: 456}, Message: "hi", Date: date}
	}
	push := func(u tg.UpdateClass) {
		require.NoError(t, b.Handle(ctx, &tg.Updates{Updates: []tg.UpdateClass{u}}))
	}

	push(&tg.UpdateBotNewBusinessMessage{ConnectionID: conn, Message: message(1, now-3600)})
	push(&tg.UpdateBotDeleteBusinessMessage{ConnectionID: conn, Peer: peer, Messages: []int{1}})
	push(&tg.UpdateBotNewBusinessMessage{ConnectionID: conn, Message: message(2, now)})
	push(&tg.UpdateBotNewBusinessMessage{ConnectionID: conn, Message: message(99, now), Qts: 8})

	require.Eventually(t, func() bool {
		count, err := b.redisStore.PendingUpdatesCount(ctx, b.botID)
		return err == nil && count >= 2
	}, 5*time.Second, 10*time.Millisecond)
	updates, err := b.GetUpdates(ctx, 0, 100, 0, nil)
	require.NoError(t, err)
	b.stopUpdateWorker()

	require.Len(t, updates, 2)
	require.Equal(t, int64(2), updates[0].BusinessMessage.MessageID)
	require.Equal(t, int64(99), updates[1].BusinessMessage.MessageID)
}

// Updates replayed inside updates.difference carry qts 0. One the bot already received
// must not be delivered again; one missed on the live connection must be delivered once.
func TestReplayedBusinessUpdatesAreDeliveredOnce(t *testing.T) {
	b := replayTestBot(t, 813, time.Now().Add(-72*time.Hour))
	ctx := context.Background()
	conn := fmt.Sprintf("business-test-%d", time.Now().UnixNano())
	peer := &tg.PeerUser{UserID: 123}
	now := int(time.Now().Unix())
	message := func(id, date int) *tg.Message {
		return &tg.Message{ID: id, PeerID: peer, FromID: &tg.PeerUser{UserID: 456}, Message: "hi", Date: date}
	}
	edited := message(1, now)
	edited.SetEditDate(now)
	push := func(u tg.UpdateClass) {
		require.NoError(t, b.Handle(ctx, &tg.Updates{Updates: []tg.UpdateClass{u}}))
	}

	// Received live, then replayed: delivered once.
	push(&tg.UpdateBotNewBusinessMessage{ConnectionID: conn, Message: message(2, now), Qts: 7})
	push(&tg.UpdateBotNewBusinessMessage{ConnectionID: conn, Message: message(2, now)})
	// Missed live, replayed twice: delivered once.
	push(&tg.UpdateBotNewBusinessMessage{ConnectionID: conn, Message: message(1, now)})
	push(&tg.UpdateBotNewBusinessMessage{ConnectionID: conn, Message: message(1, now)})
	// Too old to know whether it was delivered: dropped.
	push(&tg.UpdateBotNewBusinessMessage{ConnectionID: conn, Message: message(3, now-int((48*time.Hour).Seconds()))})
	// A missed deletion and a missed edit, each replayed twice.
	push(&tg.UpdateBotDeleteBusinessMessage{ConnectionID: conn, Peer: peer, Messages: []int{2}})
	push(&tg.UpdateBotDeleteBusinessMessage{ConnectionID: conn, Peer: peer, Messages: []int{2}})
	push(&tg.UpdateBotEditBusinessMessage{ConnectionID: conn, Message: edited})
	push(&tg.UpdateBotEditBusinessMessage{ConnectionID: conn, Message: edited})
	// A qts update without an identity to compare by is still dropped.
	push(&tg.UpdateBotStopped{UserID: 456, Date: now, Stopped: true})
	// The update worker is sequential: once this one is stored, all the above are processed.
	push(&tg.UpdateBotNewBusinessMessage{ConnectionID: conn, Message: message(99, now), Qts: 8})

	require.Eventually(t, func() bool {
		count, err := b.redisStore.PendingUpdatesCount(ctx, b.botID)
		return err == nil && count >= 5
	}, 5*time.Second, 10*time.Millisecond)
	updates, err := b.GetUpdates(ctx, 0, 100, 0, nil)
	require.NoError(t, err)
	b.stopUpdateWorker()

	require.Len(t, updates, 5)
	require.Equal(t, int64(2), updates[0].BusinessMessage.MessageID)
	require.Equal(t, int64(1), updates[1].BusinessMessage.MessageID)
	require.Equal(t, []int64{2}, updates[2].DeletedBusinessMessages.MessageIDs)
	require.Equal(t, int64(1), updates[3].EditedBusinessMessage.MessageID)
	require.Equal(t, int64(99), updates[4].BusinessMessage.MessageID)
}

// A replayed business connection update is delivered only when it tells something new.
func TestReplayedBusinessConnectionIsDeliveredOnlyWhenChanged(t *testing.T) {
	b := replayTestBot(t, 814, time.Now().Add(-72*time.Hour))
	ctx := context.Background()
	conn := fmt.Sprintf("business-test-%d", time.Now().UnixNano())
	now := int(time.Now().Unix())
	connect := func(disabled bool, qts int) *tg.UpdateBotBusinessConnect {
		return &tg.UpdateBotBusinessConnect{
			Connection: tg.BotBusinessConnection{ConnectionID: conn, UserID: 123, DCID: 2, Date: now, Disabled: disabled},
			Qts:        qts,
		}
	}
	push := func(u tg.UpdateClass) {
		require.NoError(t, b.Handle(ctx, &tg.Updates{Updates: []tg.UpdateClass{u}}))
	}

	push(connect(false, 0)) // unknown connection: delivered
	push(connect(false, 0)) // same state: dropped
	push(connect(true, 0))  // missed disabling: delivered
	push(connect(true, 0))  // same state: dropped
	push(connect(false, 5)) // live: delivered
	push(connect(false, 0)) // its replay: dropped
	push(&tg.UpdateBotNewBusinessMessage{ConnectionID: conn, Qts: 6, Message: &tg.Message{
		ID: 99, PeerID: &tg.PeerUser{UserID: 123}, FromID: &tg.PeerUser{UserID: 456}, Message: "hi", Date: now}})

	require.Eventually(t, func() bool {
		count, err := b.redisStore.PendingUpdatesCount(ctx, b.botID)
		return err == nil && count >= 4
	}, 5*time.Second, 10*time.Millisecond)
	updates, err := b.GetUpdates(ctx, 0, 100, 0, nil)
	require.NoError(t, err)
	b.stopUpdateWorker()

	require.Len(t, updates, 4)
	require.True(t, updates[0].BusinessConnection.IsEnabled)
	require.False(t, updates[1].BusinessConnection.IsEnabled)
	require.True(t, updates[2].BusinessConnection.IsEnabled)
	require.Equal(t, int64(99), updates[3].BusinessMessage.MessageID)
}
