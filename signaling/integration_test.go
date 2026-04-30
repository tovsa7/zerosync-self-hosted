//go:build integration

package signaling

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/tovsa7/zerosync-self-hosted/auth"
	"github.com/tovsa7/zerosync-self-hosted/room"
)

// ─── Helpers for integration tests ───────────────────────────────────────────

const readTimeout = 3 * time.Second

// testServerWithRegistry is like testServer but also exposes the room.Registry
// so tests can inspect server-side state (room count, peer count, etc.).
func testServerWithRegistry(t *testing.T) (dialer func() *websocket.Conn, registry *room.Registry, stop func()) {
	t.Helper()
	rooms := room.NewRegistry()
	nonces := NewNonceStore()
	h := NewHandler(rooms, nonces, auth.NoopValidator{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r)
		if err != nil {
			t.Logf("upgrade: %v", err)
			return
		}
		h.Serve(conn)
	}))

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	dial := func() *websocket.Conn {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		return conn
	}

	return dial, rooms, func() {
		srv.Close()
		rooms.Stop()
		nonces.Stop()
	}
}

// connectPeer dials the server, sends HELLO, reads the PEER_LIST response,
// drains the RELAY_NODES message that follows it, and returns the connection
// and the list of existing peer IDs.
func connectPeer(t *testing.T, dial func() *websocket.Conn, roomID, peerID string) (*websocket.Conn, []string) {
	t.Helper()
	conn := dial()
	sendHello(t, conn, roomID, peerID)
	msg := readJSON(t, conn)
	if msg["type"] != "PEER_LIST" {
		t.Fatalf("connectPeer: expected PEER_LIST, got %v", msg["type"])
	}
	raw, _ := msg["peers"].([]any)
	peers := make([]string, len(raw))
	for i, v := range raw {
		peers[i] = v.(string)
	}
	// Drain RELAY_NODES which the handler sends immediately after PEER_LIST.
	rn := readJSON(t, conn)
	if rn["type"] != "RELAY_NODES" {
		t.Fatalf("connectPeer: expected RELAY_NODES after PEER_LIST, got %v", rn["type"])
	}
	return conn, peers
}

// connectN connects n peers to the same room sequentially, draining all
// intermediate PEER_JOINED messages so that message state is clean after return.
// Returns the connections and peer IDs in order.
func connectN(t *testing.T, dial func() *websocket.Conn, roomID string, n int) ([]*websocket.Conn, []string) {
	t.Helper()
	conns := make([]*websocket.Conn, n)
	peerIDs := make([]string, n)

	for i := 0; i < n; i++ {
		peerIDs[i] = uuid.NewString()
		conn, peers := connectPeer(t, dial, roomID, peerIDs[i])
		conns[i] = conn

		// Verify peer list contains all previously-joined peers.
		if len(peers) != i {
			t.Fatalf("connectN: peer %d expected %d existing peers, got %d", i, i, len(peers))
		}

		// Drain PEER_JOINED on all previously-connected peers.
		for j := 0; j < i; j++ {
			msg := expectMessage(t, conns[j], "PEER_JOINED")
			if msg["peerId"] != peerIDs[i] {
				t.Fatalf("connectN: peer %d expected PEER_JOINED for %q, got %q",
					j, peerIDs[i], msg["peerId"])
			}
		}
	}
	return conns, peerIDs
}

// expectMessage reads one message and asserts its "type" field matches.
func expectMessage(t *testing.T, conn *websocket.Conn, msgType string) map[string]any {
	t.Helper()
	msg := readJSON(t, conn)
	if msg["type"] != msgType {
		t.Fatalf("expectMessage: expected %q, got %q (full: %v)", msgType, msg["type"], msg)
	}
	return msg
}

// tryReadJSON reads one JSON message with the given timeout.
// Returns (message, true) on success, or (nil, false) on timeout/error.
func tryReadJSON(t *testing.T, conn *websocket.Conn, timeout time.Duration) (map[string]any, bool) {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(timeout))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		return nil, false
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("tryReadJSON: unmarshal: %v", err)
	}
	return m, true
}

// expectNoMessage asserts that no message arrives within the given duration.
func expectNoMessage(t *testing.T, conn *websocket.Conn, within time.Duration) {
	t.Helper()
	msg, ok := tryReadJSON(t, conn, within)
	if ok {
		t.Fatalf("expectNoMessage: got message %v", msg)
	}
}

// sendRelay sends a RELAY message on conn.
func sendRelay(t *testing.T, conn *websocket.Conn, roomID, payload string) {
	t.Helper()
	msg := relayMsg{Type: "RELAY", RoomID: roomID, Payload: payload}
	b, _ := json.Marshal(msg)
	if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
		t.Fatalf("sendRelay: %v", err)
	}
}

// sendICE sends an ICE message of the given type (ICE_OFFER, ICE_ANSWER, ICE_CANDIDATE).
func sendICE(t *testing.T, conn *websocket.Conn, roomID, targetPeerID, msgType, payload string) {
	t.Helper()
	msg := iceMsg{Type: msgType, RoomID: roomID, TargetPeerID: targetPeerID, Payload: payload}
	b, _ := json.Marshal(msg)
	if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
		t.Fatalf("sendICE: %v", err)
	}
}

// sendRaw sends an arbitrary JSON object as a text message.
func sendRaw(t *testing.T, conn *websocket.Conn, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
		t.Fatalf("sendRaw: %v", err)
	}
}

// closeAll closes all connections in the slice.
func closeAll(conns []*websocket.Conn) {
	for _, c := range conns {
		if c != nil {
			c.Close()
		}
	}
}
