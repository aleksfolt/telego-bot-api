package botmanager

import (
	"encoding/binary"
	"sync/atomic"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/mt"
	"github.com/gotd/td/mtproto"
	"github.com/gotd/td/proto"
	"go.uber.org/zap"
)

// wireTraceBots is the set of bots whose MTProto connection is traced, see SetWireTraceBots.
var wireTraceBots atomic.Pointer[map[int64]struct{}]

// SetWireTraceBots selects the bots whose connection lifecycle and service messages (ping,
// pong, bad_msg_notification, bad_server_salt, new_session_created) are logged at info
// level. It is meant for diagnosing one misbehaving connection without turning on the
// global debug level, which is far too verbose for ~2000 bots. Bots outside the set pay
// nothing: gotd gets no tracer for them.
func SetWireTraceBots(ids []int64) {
	set := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	wireTraceBots.Store(&set)
}

func wireTraceEnabled(botID int64) bool {
	set := wireTraceBots.Load()
	if set == nil {
		return false
	}
	_, ok := (*set)[botID]
	return ok
}

// wireTracer logs what is needed to tell why a connection is declared dead: the ping the
// client sent, the pong that did or did not come back, and the notifications Telegram sent
// about messages it refused (bad_msg_notification, bad_server_salt).
type wireTracer struct {
	log *zap.Logger
}

func newWireTracer(logger *zap.Logger, botID int64) mtproto.WireTracer {
	return wireTracer{log: logger.Named("mtproto.wire").With(zap.Int64("bot_id", botID))}
}

func (t wireTracer) OnEvent(e mtproto.WireEvent) {
	switch e.Type {
	case mtproto.WireRequest:
		return // every RPC of the bot, not needed here
	case mtproto.WireRequestDone:
		if e.Err == nil {
			return
		}
	}
	fields := []zap.Field{
		zap.String("event", e.Type.String()),
		zap.Int64("conn_id", e.Conn.ID),
		zap.Int("dc_id", e.Conn.DC),
	}
	if e.MessageID != 0 {
		fields = append(fields, zap.Int64("msg_id", e.MessageID))
	}
	if e.Old != 0 || e.New != 0 {
		fields = append(fields, zap.Int64("old", e.Old), zap.Int64("new", e.New))
	}
	if e.Reason != "" {
		fields = append(fields, zap.String("reason", e.Reason))
	}
	if e.Latency != 0 {
		fields = append(fields, zap.Duration("latency", e.Latency))
	}
	if e.Err != nil {
		fields = append(fields, zap.Error(e.Err))
	}
	t.log.Info("wire event", fields...)
}

func (t wireTracer) OnMessage(m mtproto.WireMessage, body []byte) {
	t.scan(m, m.MessageID, body, 0)
}

// scan looks at the type of a message and, for a container, at the messages inside it.
func (t wireTracer) scan(m mtproto.WireMessage, msgID int64, body []byte, depth int) {
	if len(body) < 4 {
		return
	}
	typeID := binary.LittleEndian.Uint32(body)
	base := []zap.Field{
		zap.String("dir", m.Direction.String()),
		zap.Int64("conn_id", m.Conn.ID),
		zap.Int("dc_id", m.Conn.DC),
		zap.Int64("msg_id", msgID),
		zap.Int64("salt", m.Salt),
	}
	buf := &bin.Buffer{Buf: body}

	switch typeID {
	case proto.MessageContainerTypeID:
		if depth > 0 || len(body) < 8 {
			return
		}
		count := int(binary.LittleEndian.Uint32(body[4:]))
		rest := body[8:]
		for i := 0; i < count && len(rest) >= 16; i++ {
			innerID := int64(binary.LittleEndian.Uint64(rest))
			size := int(binary.LittleEndian.Uint32(rest[12:]))
			rest = rest[16:]
			if size < 0 || size > len(rest) {
				return
			}
			t.scan(m, innerID, rest[:size], depth+1)
			rest = rest[size:]
		}
	case mt.PingDelayDisconnectRequestTypeID:
		var r mt.PingDelayDisconnectRequest
		if r.Decode(buf) == nil {
			t.log.Info("wire ping", append(base, zap.Int64("ping_id", r.PingID), zap.Int("disconnect_delay", r.DisconnectDelay))...)
		}
	case mt.PingRequestTypeID:
		var r mt.PingRequest
		if r.Decode(buf) == nil {
			t.log.Info("wire ping", append(base, zap.Int64("ping_id", r.PingID))...)
		}
	case mt.PongTypeID:
		var r mt.Pong
		if r.Decode(buf) == nil {
			t.log.Info("wire pong", append(base, zap.Int64("ping_id", r.PingID), zap.Int64("reply_to", r.MsgID))...)
		}
	case mt.BadMsgNotificationTypeID:
		var r mt.BadMsgNotification
		if r.Decode(buf) == nil {
			t.log.Warn("wire bad_msg_notification", append(base,
				zap.Int64("bad_msg_id", r.BadMsgID), zap.Int("bad_msg_seqno", r.BadMsgSeqno),
				zap.Int("error_code", r.ErrorCode))...)
		}
	case mt.BadServerSaltTypeID:
		var r mt.BadServerSalt
		if r.Decode(buf) == nil {
			t.log.Warn("wire bad_server_salt", append(base,
				zap.Int64("bad_msg_id", r.BadMsgID), zap.Int("bad_msg_seqno", r.BadMsgSeqno),
				zap.Int("error_code", r.ErrorCode), zap.Int64("new_server_salt", r.NewServerSalt))...)
		}
	case mt.NewSessionCreatedTypeID:
		var r mt.NewSessionCreated
		if r.Decode(buf) == nil {
			t.log.Info("wire new_session_created", append(base,
				zap.Int64("first_msg_id", r.FirstMsgID), zap.Int64("server_salt", r.ServerSalt))...)
		}
	}
}
