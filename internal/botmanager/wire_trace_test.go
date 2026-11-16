package botmanager

import (
	"encoding/binary"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/mt"
	"github.com/gotd/td/mtproto"
	"github.com/gotd/td/proto"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func encodeBody(t *testing.T, e bin.Encoder) []byte {
	t.Helper()
	var b bin.Buffer
	if err := e.Encode(&b); err != nil {
		t.Fatal(err)
	}
	return b.Buf
}

func container(msgs map[int64][]byte, order []int64) []byte {
	out := make([]byte, 8)
	binary.LittleEndian.PutUint32(out, proto.MessageContainerTypeID)
	binary.LittleEndian.PutUint32(out[4:], uint32(len(order)))
	for _, id := range order {
		hdr := make([]byte, 16)
		binary.LittleEndian.PutUint64(hdr, uint64(id))
		binary.LittleEndian.PutUint32(hdr[12:], uint32(len(msgs[id])))
		out = append(out, hdr...)
		out = append(out, msgs[id]...)
	}
	return out
}

func TestWireTracerLogsPingPongAndBadSalt(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	tracer := newWireTracer(zap.New(core), 42)
	msg := mtproto.WireMessage{Conn: mtproto.WireConn{ID: 7, DC: 2}, Direction: mtproto.WireOut}

	tracer.OnMessage(msg, encodeBody(t, &mt.PingDelayDisconnectRequest{PingID: 11, DisconnectDelay: 75}))
	in := mtproto.WireMessage{Conn: msg.Conn, Direction: mtproto.WireIn}
	bad := encodeBody(t, &mt.BadServerSalt{BadMsgID: 5, BadMsgSeqno: 3, ErrorCode: 48, NewServerSalt: 99})
	pong := encodeBody(t, &mt.Pong{MsgID: 5, PingID: 11})
	tracer.OnMessage(in, container(map[int64][]byte{100: bad, 104: pong}, []int64{100, 104}))
	tracer.OnMessage(in, encodeBody(t, &mt.MsgsAck{MsgIDs: []int64{1}})) // ignored

	got := logs.All()
	if len(got) != 3 {
		t.Fatalf("got %d log entries, want 3: %v", len(got), got)
	}
	want := []string{"wire ping", "wire bad_server_salt", "wire pong"}
	for i, w := range want {
		if got[i].Message != w {
			t.Errorf("entry %d = %q, want %q", i, got[i].Message, w)
		}
	}
	fields := got[1].ContextMap()
	if fields["error_code"] != int64(48) || fields["new_server_salt"] != int64(99) || fields["bad_msg_id"] != int64(5) || fields["bot_id"] != int64(42) {
		t.Errorf("unexpected bad_server_salt fields: %v", fields)
	}
}

func TestWireTraceBotsSelection(t *testing.T) {
	SetWireTraceBots([]int64{7467090745})
	defer SetWireTraceBots(nil)
	if !wireTraceEnabled(7467090745) || wireTraceEnabled(1) {
		t.Fatal("only the listed bot must be traced")
	}
}
