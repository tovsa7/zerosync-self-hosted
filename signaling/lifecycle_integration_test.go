//go:build integration

package signaling

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestLifecycle_ThreePeersJoinRelayAndLeave verifies the full room lifecycle:
// three peers join, exchange relays, one leaves, remaining peers continue.
func TestLifecycle_ThreePeersJoinRelayAndLeave(t *testing.T) {
	dial, _, stop := testServerWithRegistry(t)
	defer stop()

	roomID := "lifecycle-3-peers"
	conns, peerIDs := connectN(t, dial, roomID, 3)
	defer closeAll(conns)

	// A sends RELAY — B and C should receive RELAY_DELIVER.
	sendRelay(t, conns[0], roomID, "dGVzdA==") // base64("test")

	for _, i := range []int{1, 2} {
		msg := expectMessage(t, conns[i], "RELAY_DELIVER")
		if msg["fromPeerId"] != peerIDs[0] {
			t.Fatalf("expected fromPeerId=%q, got %q", peerIDs[0], msg["fromPeerId"])
		}
		if msg["payload"] != "dGVzdA==" {
			t.Fatalf("payload mismatch: %v", msg["payload"])
		}
	}

	// B disconnects — A and C receive PEER_LEFT.
	conns[1].Close()

	for _, i := range []int{0, 2} {
		msg := expectMessage(t, conns[i], "PEER_LEFT")
		if msg["peerId"] != peerIDs[1] {
			t.Fatalf("expected peerId=%q in PEER_LEFT, got %q", peerIDs[1], msg["peerId"])
		}
	}

	// C sends RELAY — only A should receive it (B is gone).
	sendRelay(t, conns[2], roomID, "YWZ0ZXI=") // base64("after")

	msg := expectMessage(t, conns[0], "RELAY_DELIVER")
	if msg["fromPeerId"] != peerIDs[2] {
		t.Fatalf("expected fromPeerId=%q, got %q", peerIDs[2], msg["fromPeerId"])
	}
}

// TestLifecycle_LastPeerLeaves_RoomBecomesEmpty verifies that when the last
// peer disconnects, the room has zero peers (ready for GC).
func TestLifecycle_LastPeerLeaves_RoomBecomesEmpty(t *testing.T) {
	dial, registry, stop := testServerWithRegistry(t)
	defer stop()

	roomID := "lifecycle-empty"
	conn, _ := connectPeer(t, dial, roomID, uuid.NewString())

	rm := registry.Get(roomID)
	if rm == nil {
		t.Fatal("room should exist after peer joins")
	}
	if rm.Len() != 1 {
		t.Fatalf("expected 1 peer, got %d", rm.Len())
	}

	conn.Close()

	// Give the server time to process the disconnect.
	time.Sleep(100 * time.Millisecond)

	if rm.Len() != 0 {
		t.Fatalf("expected 0 peers after disconnect, got %d", rm.Len())
	}
}

// TestLifecycle_PeerRejoinsAfterLeaving verifies that a new peer can join
// a room after all previous peers have left.
func TestLifecycle_PeerRejoinsAfterLeaving(t *testing.T) {
	dial, _, stop := testServerWithRegistry(t)
	defer stop()

	roomID := "lifecycle-rejoin"

	// Peer A joins and leaves.
	c1, _ := connectPeer(t, dial, roomID, uuid.NewString())
	c1.Close()
	time.Sleep(100 * time.Millisecond)

	// Peer B joins — should get empty PEER_LIST.
	peerB := uuid.NewString()
	c2, peers := connectPeer(t, dial, roomID, peerB)
	defer c2.Close()

	if len(peers) != 0 {
		t.Fatalf("expected empty peer list after rejoin, got %v", peers)
	}

	// Peer C joins — should see only B.
	peerC := uuid.NewString()
	c3, peers := connectPeer(t, dial, roomID, peerC)
	defer c3.Close()

	if len(peers) != 1 || peers[0] != peerB {
		t.Fatalf("expected [%q], got %v", peerB, peers)
	}

	// B should get PEER_JOINED for C.
	msg := expectMessage(t, c2, "PEER_JOINED")
	if msg["peerId"] != peerC {
		t.Fatalf("expected PEER_JOINED for %q, got %q", peerC, msg["peerId"])
	}
}
