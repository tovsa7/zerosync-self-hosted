//go:build integration

package signaling

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestKeepalive_PingPong verifies the basic PING/PONG handshake.
func TestKeepalive_PingPong(t *testing.T) {
	dial, _, stop := testServerWithRegistry(t)
	defer stop()

	conn, _ := connectPeer(t, dial, "keepalive-ping", uuid.NewString())
	defer conn.Close()

	sendRaw(t, conn, map[string]string{"type": "PING"})
	expectMessage(t, conn, "PONG")
}

// TestKeepalive_MultiplePingPongs sends 10 PINGs in sequence and verifies
// that each one gets a PONG back.
func TestKeepalive_MultiplePingPongs(t *testing.T) {
	dial, _, stop := testServerWithRegistry(t)
	defer stop()

	conn, _ := connectPeer(t, dial, "keepalive-multi", uuid.NewString())
	defer conn.Close()

	for i := 0; i < 10; i++ {
		sendRaw(t, conn, map[string]string{"type": "PING"})
		expectMessage(t, conn, "PONG")
	}
}

// TestICE_ForwardToTargetPeer verifies that ICE_OFFER and ICE_ANSWER are
// forwarded correctly between two peers in the same room.
func TestICE_ForwardToTargetPeer(t *testing.T) {
	dial, _, stop := testServerWithRegistry(t)
	defer stop()

	roomID := "keepalive-ice"
	conns, peerIDs := connectN(t, dial, roomID, 2)
	defer closeAll(conns)

	// A sends ICE_OFFER to B.
	sendICE(t, conns[0], roomID, peerIDs[1], "ICE_OFFER", "c2RwLW9mZmVy")

	msg := expectMessage(t, conns[1], "ICE_OFFER")
	if msg["fromPeerId"] != peerIDs[0] {
		t.Fatalf("expected fromPeerId=%q, got %q", peerIDs[0], msg["fromPeerId"])
	}
	if msg["payload"] != "c2RwLW9mZmVy" {
		t.Fatalf("payload mismatch: %v", msg["payload"])
	}

	// B sends ICE_ANSWER back to A.
	sendICE(t, conns[1], roomID, peerIDs[0], "ICE_ANSWER", "c2RwLWFuc3dlcg==")

	msg = expectMessage(t, conns[0], "ICE_ANSWER")
	if msg["fromPeerId"] != peerIDs[1] {
		t.Fatalf("expected fromPeerId=%q, got %q", peerIDs[1], msg["fromPeerId"])
	}
	if msg["payload"] != "c2RwLWFuc3dlcg==" {
		t.Fatalf("payload mismatch: %v", msg["payload"])
	}
}

// TestICE_TargetNotInRoom_SilentDrop verifies that an ICE_OFFER targeting
// a nonexistent peer is silently dropped (no ERROR returned).
func TestICE_TargetNotInRoom_SilentDrop(t *testing.T) {
	dial, _, stop := testServerWithRegistry(t)
	defer stop()

	roomID := "keepalive-ice-drop"
	conn, _ := connectPeer(t, dial, roomID, uuid.NewString())
	defer conn.Close()

	// Send ICE_OFFER to a nonexistent peer.
	fakePeer := uuid.NewString()
	sendICE(t, conn, roomID, fakePeer, "ICE_OFFER", "c2RwLWRhdGE=")

	// No response should come.
	expectNoMessage(t, conn, 200*time.Millisecond)
}

// TestUnknownMessageType_ReturnsError verifies that an unknown message type
// results in an ERROR with BAD_REQUEST.
func TestUnknownMessageType_ReturnsError(t *testing.T) {
	dial, _, stop := testServerWithRegistry(t)
	defer stop()

	conn, _ := connectPeer(t, dial, "keepalive-unknown", uuid.NewString())
	defer conn.Close()

	sendRaw(t, conn, map[string]string{"type": "FOOBAR"})

	errMsg := expectMessage(t, conn, "ERROR")
	if errMsg["code"] != ErrCodeBadRequest {
		t.Fatalf("expected BAD_REQUEST, got %v", errMsg["code"])
	}
}
