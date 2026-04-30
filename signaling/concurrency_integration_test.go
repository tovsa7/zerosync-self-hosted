//go:build integration

package signaling

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// TestConcurrency_SimultaneousJoins launches 10 goroutines that join the same
// room concurrently. Verifies all peers eventually see each other and no
// data races occur (run with -race).
func TestConcurrency_SimultaneousJoins(t *testing.T) {
	dial, registry, stop := testServerWithRegistry(t)
	defer stop()

	roomID := "concurrent-joins"
	const n = 10

	type result struct {
		peerID string
		conn   *websocket.Conn
		err    error
	}

	results := make(chan result, n)
	var start sync.WaitGroup
	start.Add(1)

	for i := 0; i < n; i++ {
		go func() {
			start.Wait() // All goroutines start at the same time.
			peerID := uuid.NewString()
			conn := dial()
			sendHello(t, conn, roomID, peerID)

			// Read PEER_LIST.
			msg := readJSON(t, conn)
			if msg["type"] == "ERROR" {
				results <- result{peerID: peerID, conn: conn, err: fmt.Errorf("error: %v", msg["code"])}
				return
			}
			results <- result{peerID: peerID, conn: conn}
		}()
	}

	start.Done() // Release all goroutines.

	conns := make([]*websocket.Conn, 0, n)
	for i := 0; i < n; i++ {
		r := <-results
		if r.err != nil {
			t.Fatalf("peer %s failed: %v", r.peerID, r.err)
		}
		conns = append(conns, r.conn)
	}
	defer closeAll(conns)

	// Give the server a moment to process all joins.
	time.Sleep(200 * time.Millisecond)

	// Verify the room has exactly n peers.
	rm := registry.Get(roomID)
	if rm == nil {
		t.Fatal("room should exist")
	}
	if rm.Len() != n {
		t.Fatalf("expected %d peers in room, got %d", n, rm.Len())
	}
}

// TestConcurrency_RapidConnectDisconnect launches 20 goroutines that each
// connect, send one RELAY, and disconnect. An observer peer stays connected
// the whole time. Verifies no panics, no races, no ERROR messages to observer.
func TestConcurrency_RapidConnectDisconnect(t *testing.T) {
	dial, _, stop := testServerWithRegistry(t)
	defer stop()

	roomID := "concurrent-rapid"

	// Observer connects first.
	observer, _ := connectPeer(t, dial, roomID, uuid.NewString())
	defer observer.Close()

	const n = 20
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn := dial()
			peerID := uuid.NewString()
			sendHello(t, conn, roomID, peerID)

			// Read PEER_LIST (or ERROR if room is racing).
			readJSON(t, conn)

			// Send one RELAY.
			payload := base64.StdEncoding.EncodeToString([]byte("rapid"))
			msg := relayMsg{Type: "RELAY", RoomID: roomID, Payload: payload}
			b, _ := json.Marshal(msg)
			conn.WriteMessage(websocket.TextMessage, b) //nolint:errcheck

			// Small delay to let the message propagate, then disconnect.
			time.Sleep(10 * time.Millisecond)
			conn.Close()
		}()
	}

	wg.Wait()

	// Give server time to process all disconnects.
	time.Sleep(300 * time.Millisecond)

	// Drain all messages from the observer. We expect a mix of PEER_JOINED,
	// RELAY_DELIVER, and PEER_LEFT — but never ERROR.
	for {
		msg, ok := tryReadJSON(t, observer, 200*time.Millisecond)
		if !ok {
			break
		}
		if msg["type"] == "ERROR" {
			t.Fatalf("observer received unexpected ERROR: %v", msg)
		}
	}
}

// TestConcurrency_SimultaneousRelays has 5 peers each send a unique RELAY
// simultaneously. Each peer should receive exactly 4 RELAY_DELIVER messages.
func TestConcurrency_SimultaneousRelays(t *testing.T) {
	dial, _, stop := testServerWithRegistry(t)
	defer stop()

	roomID := "concurrent-relays"
	const n = 5
	conns, peerIDs := connectN(t, dial, roomID, n)
	defer closeAll(conns)

	payloads := make([]string, n)
	for i := 0; i < n; i++ {
		payloads[i] = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("peer-%d", i)))
	}

	// All peers send RELAY at the same time.
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sendRelay(t, conns[idx], roomID, payloads[idx])
		}(i)
	}
	wg.Wait()

	// Each peer should receive exactly n-1 RELAY_DELIVER messages.
	for i := 0; i < n; i++ {
		received := make(map[string]string) // fromPeerId → payload
		for j := 0; j < n-1; j++ {
			msg := expectMessage(t, conns[i], "RELAY_DELIVER")
			from := msg["fromPeerId"].(string)
			received[from] = msg["payload"].(string)
		}

		// Verify: should NOT have received from self.
		if _, ok := received[peerIDs[i]]; ok {
			t.Fatalf("peer %d received its own RELAY", i)
		}

		// Verify: got messages from all other peers.
		for j := 0; j < n; j++ {
			if j == i {
				continue
			}
			p, ok := received[peerIDs[j]]
			if !ok {
				t.Fatalf("peer %d did not receive RELAY from peer %d", i, j)
			}
			if p != payloads[j] {
				t.Fatalf("peer %d: payload from peer %d mismatch", i, j)
			}
		}
	}
}
