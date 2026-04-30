//go:build integration

package signaling

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tovsa7/zerosync-self-hosted/relay"
	"github.com/tovsa7/zerosync-self-hosted/room"
)

// TestLimits_RoomCapacity50 connects 50 peers (the maximum), then verifies
// that the 51st peer receives ROOM_FULL.
func TestLimits_RoomCapacity50(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 50-peer test in short mode")
	}

	dial, _, stop := testServerWithRegistry(t)
	defer stop()

	roomID := "limits-capacity"
	conns, _ := connectN(t, dial, roomID, room.MaxPeersPerRoom)
	defer closeAll(conns)

	// 51st peer should be rejected.
	c := dial()
	defer c.Close()
	sendHello(t, c, roomID, uuid.NewString())

	msg := readJSON(t, c)
	if msg["type"] != "ERROR" {
		t.Fatalf("expected ERROR for 51st peer, got %v", msg["type"])
	}
	if msg["code"] != ErrCodeRoomFull {
		t.Fatalf("expected ROOM_FULL, got %v", msg["code"])
	}
}

// TestLimits_OversizedRelay_ClosesConnection verifies that a relay payload
// exceeding 64 KB results in an ERROR and connection close.
func TestLimits_OversizedRelay_ClosesConnection(t *testing.T) {
	dial, _, stop := testServerWithRegistry(t)
	defer stop()

	roomID := "limits-oversize"
	peerA := uuid.NewString()
	peerB := uuid.NewString()

	cA, _ := connectPeer(t, dial, roomID, peerA)
	defer cA.Close()

	cB, _ := connectPeer(t, dial, roomID, peerB)
	defer cB.Close()

	// Drain PEER_JOINED on A.
	expectMessage(t, cA, "PEER_JOINED")

	// A sends a relay with payload > 64 KB.
	oversized := make([]byte, relay.MaxBlobSize+1)
	for i := range oversized {
		oversized[i] = 'A'
	}
	payload := base64.StdEncoding.EncodeToString(oversized)
	sendRelay(t, cA, roomID, payload)

	// A should receive ERROR.
	errMsg := expectMessage(t, cA, "ERROR")
	if errMsg["code"] != ErrCodeBadRequest {
		t.Fatalf("expected BAD_REQUEST, got %v", errMsg["code"])
	}

	// A's connection should be closed — next read should fail.
	_, ok := tryReadJSON(t, cA, time.Second)
	if ok {
		t.Fatal("expected A's connection to be closed after oversized relay")
	}

	// B should receive PEER_LEFT for A.
	left := expectMessage(t, cB, "PEER_LEFT")
	if left["peerId"] != peerA {
		t.Fatalf("expected PEER_LEFT for %q, got %q", peerA, left["peerId"])
	}
}

// TestLimits_ExactlyMaxRelay_Succeeds verifies that a relay payload of exactly
// 64 KB is accepted and delivered.
func TestLimits_ExactlyMaxRelay_Succeeds(t *testing.T) {
	dial, _, stop := testServerWithRegistry(t)
	defer stop()

	roomID := "limits-exact-max"
	conns, peerIDs := connectN(t, dial, roomID, 2)
	defer closeAll(conns)

	// Peer 0 sends exactly 64 KB.
	exact := make([]byte, relay.MaxBlobSize)
	for i := range exact {
		exact[i] = 'B'
	}
	payload := base64.StdEncoding.EncodeToString(exact)
	sendRelay(t, conns[0], roomID, payload)

	// Peer 1 should receive RELAY_DELIVER.
	msg := expectMessage(t, conns[1], "RELAY_DELIVER")
	if msg["fromPeerId"] != peerIDs[0] {
		t.Fatalf("expected fromPeerId=%q, got %q", peerIDs[0], msg["fromPeerId"])
	}
	if msg["payload"] != payload {
		t.Fatal("payload mismatch for exact-max relay")
	}
}

// TestLimits_NonceReplay_AcrossRooms verifies that nonce replay detection is
// global — reusing a nonce in a different room still fails.
func TestLimits_NonceReplay_AcrossRooms(t *testing.T) {
	dial, _, stop := testServerWithRegistry(t)
	defer stop()

	nonce := uuid.NewString()

	// First connection uses the nonce in room-A.
	c1 := dial()
	defer c1.Close()
	msg := helloMsg{Type: "HELLO", RoomID: "room-nonce-a", PeerID: uuid.NewString(), Nonce: nonce, HMAC: "stub"}
	sendRaw(t, c1, msg)
	readJSON(t, c1) // PEER_LIST

	// Second connection reuses the same nonce in room-B.
	c2 := dial()
	defer c2.Close()
	msg2 := helloMsg{Type: "HELLO", RoomID: "room-nonce-b", PeerID: uuid.NewString(), Nonce: nonce, HMAC: "stub"}
	sendRaw(t, c2, msg2)

	errMsg := readJSON(t, c2)
	if errMsg["type"] != "ERROR" {
		t.Fatalf("expected ERROR, got %v", errMsg["type"])
	}
	if errMsg["code"] != ErrCodeNonceReplay {
		t.Fatalf("expected NONCE_REPLAY, got %v", errMsg["code"])
	}
}

// TestLimits_InvalidBase64Relay_ReturnsError verifies that an invalid base64
// payload returns ERROR but does NOT close the connection.
func TestLimits_InvalidBase64Relay_ReturnsError(t *testing.T) {
	dial, _, stop := testServerWithRegistry(t)
	defer stop()

	roomID := "limits-bad-b64"
	conns, _ := connectN(t, dial, roomID, 2)
	defer closeAll(conns)

	// Send RELAY with invalid base64.
	sendRelay(t, conns[0], roomID, "!!!not-base64!!!")

	errMsg := expectMessage(t, conns[0], "ERROR")
	if errMsg["code"] != ErrCodeBadRequest {
		t.Fatalf("expected BAD_REQUEST, got %v", errMsg["code"])
	}
	if !strings.Contains(errMsg["message"].(string), "base64") {
		t.Fatalf("error message should mention base64, got %q", errMsg["message"])
	}

	// Connection should still be alive — send a PING and expect PONG.
	sendRaw(t, conns[0], map[string]string{"type": "PING"})
	expectMessage(t, conns[0], "PONG")
}
