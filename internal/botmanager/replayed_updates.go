package botmanager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gotd/td/tg"
	"go.uber.org/zap"
	"telego-bot-api/internal/converter"
	"telego-bot-api/internal/metrics"
)

// replayedUpdateWindow is how long a delivered business update is remembered, and
// therefore how far back an update replayed by updates.getDifference can still be told
// apart from one the bot already received. Older replays are dropped, as before.
var replayedUpdateWindow = messageUpdateMaxAge

// Outcomes of an update replayed by updates.getDifference, see metrics.IncReplayedUpdates.
const (
	replayRecovered   = "recovered"
	replayDuplicate   = "duplicate"
	replayStale       = "stale"
	replayUnsupported = "unsupported"
)

// replayedQtsUpdate reports a qts-sequenced update without a qts position. Telegram puts
// bot business updates into updates.difference with qts 0 and the gap manager dispatches
// them on every getDifference (reconnects, gaps and its 15-minute idle check). Some of
// them the bot already received, others were missed on the live connection and exist
// only there, so they are deduplicated by identity instead of being dropped.
func replayedQtsUpdate(u tg.UpdateClass) bool {
	q, ok := u.(interface{ GetQts() int })
	return ok && q.GetQts() == 0
}

// businessUpdateIdentity returns a stable identity of a business message update and the
// time of the event it reports (0 when the update carries none). The identity is built
// from MTProto fields that are the same in the live update and in its replay; converted
// Bot API payloads are not, e.g. file references in file_id change.
func businessUpdateIdentity(u tg.UpdateClass) (identity string, date int, ok bool) {
	switch u := u.(type) {
	case *tg.UpdateBotNewBusinessMessage:
		msg, notEmpty := u.Message.AsNotEmpty()
		if !notEmpty {
			return "", 0, false
		}
		return hashIdentity("new", u.ConnectionID, peerIdentity(msg.GetPeerID()), strconv.Itoa(msg.GetID())),
			messageDate(u.Message), true
	case *tg.UpdateBotEditBusinessMessage:
		msg, notEmpty := u.Message.AsNotEmpty()
		if !notEmpty {
			return "", 0, false
		}
		// Several edits of one message differ by edit date; the text tells apart edits
		// made within the same second.
		date, text := messageDate(u.Message), ""
		if m, ok := u.Message.(*tg.Message); ok {
			if editDate, ok := m.GetEditDate(); ok {
				date = editDate
			}
			text = m.Message
		}
		return hashIdentity("edit", u.ConnectionID, peerIdentity(msg.GetPeerID()), strconv.Itoa(msg.GetID()),
			strconv.Itoa(date), text), date, true
	case *tg.UpdateBotDeleteBusinessMessage:
		ids := append([]int(nil), u.Messages...)
		sort.Ints(ids)
		parts := make([]string, len(ids))
		for i, id := range ids {
			parts[i] = strconv.Itoa(id)
		}
		return hashIdentity("delete", u.ConnectionID, peerIdentity(u.Peer), strings.Join(parts, ",")), 0, true
	}
	return "", 0, false
}

func messageDate(msg tg.MessageClass) int {
	if dated, ok := msg.(interface{ GetDate() int }); ok {
		return dated.GetDate()
	}
	return 0
}

func peerIdentity(peer tg.PeerClass) string {
	switch p := peer.(type) {
	case *tg.PeerUser:
		return "user" + strconv.FormatInt(p.UserID, 10)
	case *tg.PeerChat:
		return "chat" + strconv.FormatInt(p.ChatID, 10)
	case *tg.PeerChannel:
		return "channel" + strconv.FormatInt(p.ChannelID, 10)
	}
	return ""
}

// hashIdentity folds the parts into a short fixed-size Redis key suffix.
func hashIdentity(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:12])
}

// rememberDeliveredUpdate records a business update received on the live connection, so
// its replay by a later getDifference is recognized. A failure only risks a duplicate.
func (b *BotInstance) rememberDeliveredUpdate(botID int64, u tg.UpdateClass) {
	identity, _, ok := businessUpdateIdentity(u)
	if !ok || b.redisStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// The memory must have a start no later than its first entry.
	_, err := b.redisStore.UpdatesSeenSince(ctx)
	if err == nil {
		_, err = b.redisStore.MarkUpdateSeen(ctx, botID, identity, replayedUpdateWindow)
	}
	if err != nil {
		b.logger.Warn("Failed to remember a delivered business update; its replay may be delivered twice",
			zap.String("type", replayedUpdateKind(u)), zap.Error(err))
	}
}

// acceptReplayedUpdate decides whether an update replayed by updates.getDifference is
// delivered: only one that was not delivered before and is recent enough to know that.
// Business connection updates are decided after conversion, by businessConnectionChanged.
func (b *BotInstance) acceptReplayedUpdate(botID int64, u tg.UpdateClass) bool {
	if _, ok := u.(*tg.UpdateBotBusinessConnect); ok {
		return true
	}
	identity, date, ok := businessUpdateIdentity(u)
	if !ok || b.redisStore == nil {
		// Other qts updates have no identity to compare by: dropped, like TDLib does.
		metrics.DefaultRegistry.IncReplayedUpdates(replayUnsupported)
		return false
	}

	var knowable, first bool
	err := b.retryRedis("mark replayed update", func(ctx context.Context) error {
		since, err := b.redisStore.UpdatesSeenSince(ctx)
		if err != nil {
			return err
		}
		if knowable = replayedUpdateKnowable(date, since, time.Now()); !knowable {
			return nil
		}
		first, err = b.redisStore.MarkUpdateSeen(ctx, botID, identity, replayedUpdateWindow)
		return err
	})
	if err != nil {
		b.logger.Error("Replayed business update dropped: cannot check whether it was delivered",
			zap.String("type", replayedUpdateKind(u)), zap.Error(err))
		return false
	}
	if !knowable {
		metrics.DefaultRegistry.IncReplayedUpdates(replayStale)
		return false
	}
	if !first {
		metrics.DefaultRegistry.IncReplayedUpdates(replayDuplicate)
		return false
	}
	metrics.DefaultRegistry.IncReplayedUpdates(replayRecovered)
	b.logger.Info("Recovered a business update missed on the live connection",
		append([]zap.Field{zap.String("type", replayedUpdateKind(u))}, replayedUpdateFields(u)...)...)
	return true
}

// replayedUpdateKnowable reports whether the memory of delivered updates covers the
// event of a replayed update, i.e. whether "not remembered" means "not delivered".
// The memory starts at since (e.g. the first start of a version that keeps it) and
// holds replayedUpdateWindow. An update without a date (a deletion) may report an event
// of any age, so it is knowable only once the memory spans the whole window.
func replayedUpdateKnowable(date int, since, now time.Time) bool {
	if date == 0 {
		return now.Sub(since) >= replayedUpdateWindow
	}
	event := time.Unix(int64(date), 0)
	return !event.Before(since) && now.Sub(event) <= replayedUpdateWindow
}

// businessConnectionChanged reports whether a replayed business connection update tells
// something new: the connection is unknown, or its state differs from the known one.
// Replays have no date of the change, so they are compared by state.
func (b *BotInstance) businessConnectionChanged(connection *converter.BusinessConnection) bool {
	known := b.knownBusinessConnection(connection.ID)
	changed := known == nil ||
		known.IsEnabled != connection.IsEnabled ||
		known.CanReply != connection.CanReply ||
		!reflect.DeepEqual(known.Rights, connection.Rights)
	if !changed {
		metrics.DefaultRegistry.IncReplayedUpdates(replayDuplicate)
		return false
	}
	metrics.DefaultRegistry.IncReplayedUpdates(replayRecovered)
	b.logger.Info("Recovered a business update missed on the live connection",
		zap.String("type", "business_connection"), zap.String("connection_id", connection.ID),
		zap.Bool("is_enabled", connection.IsEnabled), zap.Bool("known", known != nil))
	return true
}

// knownBusinessConnection returns the connection as last seen by this server, from memory
// or Redis, without asking Telegram.
func (b *BotInstance) knownBusinessConnection(connectionID string) *converter.BusinessConnection {
	if connection, _, ok := b.cachedBusinessConnection(connectionID, false); ok {
		return connection
	}
	if b.redisStore == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	data, err := b.redisStore.GetBusinessConnection(ctx, connectionID)
	if err != nil || len(data) == 0 {
		return nil
	}
	var record storedBusinessConnection
	if err := json.Unmarshal(data, &record); err != nil || record.ID == "" {
		return nil
	}
	return &record.BusinessConnection
}

func replayedUpdateKind(u tg.UpdateClass) string {
	switch u.(type) {
	case *tg.UpdateBotNewBusinessMessage:
		return "business_message"
	case *tg.UpdateBotEditBusinessMessage:
		return "edited_business_message"
	case *tg.UpdateBotDeleteBusinessMessage:
		return "deleted_business_messages"
	}
	return "unknown"
}

func replayedUpdateFields(u tg.UpdateClass) []zap.Field {
	switch u := u.(type) {
	case *tg.UpdateBotNewBusinessMessage:
		return []zap.Field{zap.String("connection_id", u.ConnectionID), zap.Int("message_id", u.Message.GetID())}
	case *tg.UpdateBotEditBusinessMessage:
		return []zap.Field{zap.String("connection_id", u.ConnectionID), zap.Int("message_id", u.Message.GetID())}
	case *tg.UpdateBotDeleteBusinessMessage:
		return []zap.Field{zap.String("connection_id", u.ConnectionID), zap.Ints("message_ids", u.Messages)}
	}
	return nil
}
