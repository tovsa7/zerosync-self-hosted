//go:build integration

package signaling

import (
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// TestIsolation_CrossRoomMessagesDoNotLeak verifies that a RELAY in one room
// is never delivered to a peer in a different room.
func TestIsolation_CrossRoomMessagesDoNotLeak(t *testing.T) {
	dial, _, stop := testServerWithRegistry(t)
	defer stop()

	cA, _ := connectPeer(t, dial, "room-alpha", uuid.NewString())
	defer cA.Close()

	cB, _ := connectPeer(t, dial, "room-beta", uuid.NewString())
	defer cB.Close()

	// A sends RELAY in room-alpha.
	sendRelay(t, cA, "room-alpha", "c2VjcmV0") // base64("secret")

	// B (in room-beta) must NOT receive anything.
	expectNoMessage(t, cB, 200*time.Millisecond)

	// B sends RELAY in room-beta.
	sendRelay(t, cB, "room-beta", "b3RoZXI=") // base64("other")

	// A (in room-alpha) must NOT receive anything.
	expectNoMessage(t, cA, 200*time.Millisecond)
}

// TestIsolation_MultipleRoomsSimultaneous creates 5 rooms with 3 peers each
// and verifies messages stay within their respective rooms.
func TestIsolation_MultipleRoomsSimultaneous(t *testing.T) {
	dial, _, stop := testServerWithRegistry(t)
	defer stop()

	const numRooms = 5
	const peersPerRoom = 3

	type roomState struct {
		roomID  string
		conns   []*websocket.Conn
		peerIDs []string
	}
	rooms := make([]roomState, numRooms)

	for i := 0; i < numRooms; i++ {
		roomID := fmt.Sprintf("multi-room-%d", i)
		conns, peerIDs := connectN(t, dial, roomID, peersPerRoom)
		rooms[i] = roomState{roomID: roomID, conns: conns, peerIDs: peerIDs}
	}
	defer func() {
		for _, r := range rooms {
			closeAll(r.conns)
		}
	}()

	// Peer 0 in each room sends a RELAY with a room-specific payload.
	payloads := make([]string, numRooms)
	for i, r := range rooms {
		payloads[i] = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("room-%d", i)))
		sendRelay(t, r.conns[0], r.roomID, payloads[i])
	}

	// Each room's peers 1 and 2 should get exactly one RELAY_DELIVER from peer 0.
	for i, r := range rooms {
		for j := 1; j < peersPerRoom; j++ {
			msg := expectMessage(t, r.conns[j], "RELAY_DELIVER")
			if msg["fromPeerId"] != r.peerIDs[0] {
				t.Fatalf("room %d, peer %d: expected fromPeerId=%q, got %q",
					i, j, r.peerIDs[0], msg["fromPeerId"])
			}
		}
	}

	// Verify no cross-room leakage: peer 0 in each room should NOT receive
	// messages from other rooms (it should receive nothing at all since it was
	// the sender in its own room).
	for i, r := range rooms {
		expectNoMessage(t, r.conns[0], 200*time.Millisecond)
		_ = i
	}
}

// TestIsolation_ICEForwardingDoesNotCrossRooms verifies that an ICE_OFFER
// targeting a peerID in a different room is silently dropped.
func TestIsolation_ICEForwardingDoesNotCrossRooms(t *testing.T) {
	dial, _, stop := testServerWithRegistry(t)
	defer stop()

	peerA := uuid.NewString()
	peerB := uuid.NewString()

	cA, _ := connectPeer(t, dial, "room-ice-1", peerA)
	defer cA.Close()

	cB, _ := connectPeer(t, dial, "room-ice-2", peerB)
	defer cB.Close()

	// A sends ICE_OFFER targeting B (who is in a different room).
	sendICE(t, cA, "room-ice-1", peerB, "ICE_OFFER", "c2RwLWRhdGE=")

	// B should NOT receive anything.
	expectNoMessage(t, cB, 200*time.Millisecond)
}
