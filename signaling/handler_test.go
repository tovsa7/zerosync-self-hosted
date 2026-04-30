package signaling

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/google/uuid"

	"github.com/tovsa7/zerosync-self-hosted/auth"
	"github.com/tovsa7/zerosync-self-hosted/room"
)

// mockValidator is a test-only auth.Validator with simple room/peer caps.
// A cap of 0 means unlimited. Used to verify that the handler maps the auth
// sentinel errors to the correct wire-protocol error codes.
type mockValidator struct {
	maxRooms int
	maxPeers int
}

func (m mockValidator) CheckRoomLimit(current int) error {
	if m.maxRooms == 0 {
		return nil
	}
	if current >= m.maxRooms {
		return auth.ErrRoomLimitExceeded
	}
	return nil
}

func (m mockValidator) CheckPeerLimit(current int) error {
	if m.maxPeers == 0 {
		return nil
	}
	if current >= m.maxPeers {
		return auth.ErrPeerLimitExceeded
	}
	return nil
}

// testServer spins up an httptest.Server with a NoopValidator (allows everything).
func testServer(t *testing.T) (dialer func() *websocket.Conn, stop func()) {
	t.Helper()
	return testServerWithValidator(t, auth.NoopValidator{})
}

// testServerWithValidator spins up an httptest.Server with the given Validator.
func testServerWithValidator(t *testing.T, v auth.Validator) (dialer func() *websocket.Conn, stop func()) {
	t.Helper()
	rooms := room.NewRegistry()
	nonces := NewNonceStore()
	h := NewHandler(rooms, nonces, v)

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

	return dial, func() {
		srv.Close()
		rooms.Stop()
		nonces.Stop()
	}
}

func sendHello(t *testing.T, conn *websocket.Conn, roomID, peerID string) {
	t.Helper()
	sendHelloWithType(t, conn, roomID, peerID, "")
}

func sendHelloWithType(t *testing.T, conn *websocket.Conn, roomID, peerID, peerType string) {
	t.Helper()
	sendHelloFull(t, conn, roomID, peerID, peerType, "")
}

func sendHelloFull(t *testing.T, conn *websocket.Conn, roomID, peerID, peerType, region string) {
	t.Helper()
	msg := helloMsg{
		Type:     "HELLO",
		RoomID:   roomID,
		PeerID:   peerID,
		PeerType: peerType,
		Region:   region,
		Nonce:    uuid.NewString(),
		HMAC:     "stub",
	}
	b, _ := json.Marshal(msg)
	if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
		t.Fatalf("sendHello: %v", err)
	}
}

// readMsgOfType reads messages until it finds one with the given type.
// Skips messages that don't match. Fails on timeout.
func readMsgOfType(t *testing.T, conn *websocket.Conn, msgType string) map[string]any {
	t.Helper()
	for i := 0; i < 10; i++ {
		msg := readJSON(t, conn)
		if msg["type"] == msgType {
			return msg
		}
	}
	t.Fatalf("readMsgOfType: did not find %q within 10 messages", msgType)
	return nil
}

func readJSON(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("readJSON: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("readJSON unmarshal: %v", err)
	}
	return m
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestHello_FirstPeer_ReceivesPeerList(t *testing.T) {
	dial, stop := testServer(t)
	defer stop()

	c := dial()
	defer c.Close()

	sendHello(t, c, "room-1", uuid.NewString())

	msg := readJSON(t, c)
	if msg["type"] != "PEER_LIST" {
		t.Fatalf("expected PEER_LIST, got %v", msg["type"])
	}
	peers := msg["peers"].([]any)
	if len(peers) != 0 {
		t.Fatalf("expected empty peer list for first joiner, got %d", len(peers))
	}

	// RELAY_NODES follows PEER_LIST.
	rn := readJSON(t, c)
	if rn["type"] != "RELAY_NODES" {
		t.Fatalf("expected RELAY_NODES after PEER_LIST, got %v", rn["type"])
	}
}

func TestHello_SecondPeer_ReceivesPeerList(t *testing.T) {
	dial, stop := testServer(t)
	defer stop()

	roomID := "room-2"
	peer1ID := uuid.NewString()
	peer2ID := uuid.NewString()

	c1 := dial()
	defer c1.Close()
	sendHello(t, c1, roomID, peer1ID)
	readJSON(t, c1) // consume PEER_LIST
	readJSON(t, c1) // consume RELAY_NODES

	c2 := dial()
	defer c2.Close()
	sendHello(t, c2, roomID, peer2ID)

	// c1 should receive PEER_JOINED.
	joined := readMsgOfType(t, c1, "PEER_JOINED")
	if joined["peerId"] != peer2ID {
		t.Fatalf("expected peerId=%q, got %q", peer2ID, joined["peerId"])
	}

	// c2 should receive PEER_LIST containing peer1.
	pl := readJSON(t, c2)
	if pl["type"] != "PEER_LIST" {
		t.Fatalf("c2 expected PEER_LIST, got %v", pl["type"])
	}
	peers := pl["peers"].([]any)
	if len(peers) != 1 || peers[0].(string) != peer1ID {
		t.Fatalf("c2 expected [%q], got %v", peer1ID, peers)
	}
}

func TestHello_DuplicatePeerID_ReturnsError(t *testing.T) {
	dial, stop := testServer(t)
	defer stop()

	roomID := "room-dup"
	peerID := uuid.NewString()

	c1 := dial()
	defer c1.Close()
	sendHello(t, c1, roomID, peerID)
	readJSON(t, c1) // PEER_LIST
	readJSON(t, c1) // RELAY_NODES

	c2 := dial()
	defer c2.Close()

	// Same peerID — must receive ERROR DUPLICATE_PEER_ID.
	// Use a different nonce to avoid nonce replay.
	msg := helloMsg{Type: "HELLO", RoomID: roomID, PeerID: peerID, Nonce: uuid.NewString(), HMAC: "x"}
	b, _ := json.Marshal(msg)
	c2.WriteMessage(websocket.TextMessage, b)

	errMsg := readJSON(t, c2)
	if errMsg["type"] != "ERROR" {
		t.Fatalf("expected ERROR, got %v", errMsg["type"])
	}
	if errMsg["code"] != ErrCodeDuplicatePeerID {
		t.Fatalf("expected DUPLICATE_PEER_ID, got %v", errMsg["code"])
	}
}

func TestHello_NonceReplay_ReturnsError(t *testing.T) {
	dial, stop := testServer(t)
	defer stop()

	nonce := uuid.NewString()

	c1 := dial()
	defer c1.Close()
	msg := helloMsg{Type: "HELLO", RoomID: "room-nr", PeerID: uuid.NewString(), Nonce: nonce, HMAC: "x"}
	b, _ := json.Marshal(msg)
	c1.WriteMessage(websocket.TextMessage, b)
	readJSON(t, c1) // PEER_LIST
	readJSON(t, c1) // RELAY_NODES

	// Second connection reuses the same nonce.
	c2 := dial()
	defer c2.Close()
	msg2 := helloMsg{Type: "HELLO", RoomID: "room-nr", PeerID: uuid.NewString(), Nonce: nonce, HMAC: "x"}
	b2, _ := json.Marshal(msg2)
	c2.WriteMessage(websocket.TextMessage, b2)

	errMsg := readJSON(t, c2)
	if errMsg["code"] != ErrCodeNonceReplay {
		t.Fatalf("expected NONCE_REPLAY, got %v", errMsg["code"])
	}
}

func TestRelay_BroadcastToRoomPeers(t *testing.T) {
	dial, stop := testServer(t)
	defer stop()

	roomID := "room-relay"

	c1 := dial()
	defer c1.Close()
	sendHello(t, c1, roomID, uuid.NewString())
	readJSON(t, c1) // PEER_LIST
	readJSON(t, c1) // RELAY_NODES

	c2 := dial()
	defer c2.Close()
	sendHello(t, c2, roomID, uuid.NewString())
	readMsgOfType(t, c1, "PEER_JOINED") // on c1
	readJSON(t, c2)                      // PEER_LIST on c2
	readJSON(t, c2)                      // RELAY_NODES on c2

	// c1 sends RELAY.
	relayPayload := "dGVzdC1wYXlsb2Fk" // base64("test-payload")
	relay := relayMsg{Type: "RELAY", RoomID: roomID, Payload: relayPayload}
	b, _ := json.Marshal(relay)
	c1.WriteMessage(websocket.TextMessage, b)

	// c2 should receive RELAY_DELIVER.
	delivered := readJSON(t, c2)
	if delivered["type"] != "RELAY_DELIVER" {
		t.Fatalf("expected RELAY_DELIVER, got %v", delivered["type"])
	}
	if delivered["payload"] != relayPayload {
		t.Fatalf("payload mismatch: got %v", delivered["payload"])
	}
}

func TestPing_ReceivesPong(t *testing.T) {
	dial, stop := testServer(t)
	defer stop()

	c := dial()
	defer c.Close()
	sendHello(t, c, "room-ping", uuid.NewString())
	readJSON(t, c) // PEER_LIST
	readJSON(t, c) // RELAY_NODES

	b, _ := json.Marshal(map[string]string{"type": "PING"})
	c.WriteMessage(websocket.TextMessage, b)

	pong := readJSON(t, c)
	if pong["type"] != "PONG" {
		t.Fatalf("expected PONG, got %v", pong["type"])
	}
}

func TestRelay_RoomIDMismatch_ReturnsError(t *testing.T) {
	dial, stop := testServer(t)
	defer stop()

	c := dial()
	defer c.Close()
	sendHello(t, c, "room-a", uuid.NewString())
	readJSON(t, c) // PEER_LIST
	readJSON(t, c) // RELAY_NODES

	// Send RELAY with a different roomId than the peer's room.
	relay := relayMsg{Type: "RELAY", RoomID: "room-b", Payload: "dGVzdA=="}
	b, _ := json.Marshal(relay)
	c.WriteMessage(websocket.TextMessage, b)

	errMsg := readJSON(t, c)
	if errMsg["type"] != "ERROR" {
		t.Fatalf("expected ERROR, got %v", errMsg["type"])
	}
	if errMsg["code"] != ErrCodeBadRequest {
		t.Fatalf("expected BAD_REQUEST, got %v", errMsg["code"])
	}
}

func TestICE_RoomIDMismatch_ReturnsError(t *testing.T) {
	dial, stop := testServer(t)
	defer stop()

	roomID := "room-ice"
	c1 := dial()
	defer c1.Close()
	sendHello(t, c1, roomID, uuid.NewString())
	readJSON(t, c1) // PEER_LIST
	readJSON(t, c1) // RELAY_NODES

	c2 := dial()
	defer c2.Close()
	target2ID := uuid.NewString()
	sendHello(t, c2, roomID, target2ID)
	readMsgOfType(t, c1, "PEER_JOINED")
	readJSON(t, c2) // PEER_LIST
	readJSON(t, c2) // RELAY_NODES

	// c1 sends ICE with wrong roomId.
	icePayload := iceMsg{
		Type:         "ICE_OFFER",
		RoomID:       "wrong-room",
		TargetPeerID: target2ID,
		Payload:      "sdp-data",
	}
	b, _ := json.Marshal(icePayload)
	c1.WriteMessage(websocket.TextMessage, b)

	errMsg := readJSON(t, c1)
	if errMsg["type"] != "ERROR" {
		t.Fatalf("expected ERROR, got %v", errMsg["type"])
	}
	if errMsg["code"] != ErrCodeBadRequest {
		t.Fatalf("expected BAD_REQUEST, got %v", errMsg["code"])
	}
}

func TestDisconnect_BroadcastsPeerLeft(t *testing.T) {
	dial, stop := testServer(t)
	defer stop()

	roomID := "room-leave"
	peer1ID := uuid.NewString()

	c1 := dial()
	sendHello(t, c1, roomID, peer1ID)
	readJSON(t, c1) // PEER_LIST
	readJSON(t, c1) // RELAY_NODES

	c2 := dial()
	defer c2.Close()
	sendHello(t, c2, roomID, uuid.NewString())
	readMsgOfType(t, c1, "PEER_JOINED")
	readJSON(t, c2) // PEER_LIST
	readJSON(t, c2) // RELAY_NODES

	// c1 disconnects.
	c1.Close()

	// c2 should receive PEER_LEFT.
	left := readMsgOfType(t, c2, "PEER_LEFT")
	if left["peerId"] != peer1ID {
		t.Fatalf("expected peerId=%q, got %q", peer1ID, left["peerId"])
	}
}

// ─── Relay node tests ─────────────────────────────────────────────────────────

func TestPeerType_Absent_TreatedAsUser(t *testing.T) {
	dial, stop := testServer(t)
	defer stop()

	c := dial()
	defer c.Close()

	// peerType omitted (empty string) → server treats as "user".
	sendHello(t, c, "room-pt1", uuid.NewString())
	pl := readJSON(t, c)
	if pl["type"] != "PEER_LIST" {
		t.Fatalf("expected PEER_LIST, got %v", pl["type"])
	}

	rn := readJSON(t, c)
	if rn["type"] != "RELAY_NODES" {
		t.Fatalf("expected RELAY_NODES, got %v", rn["type"])
	}
	// No relay peers → empty list.
	relays := rn["peers"].([]any)
	if len(relays) != 0 {
		t.Fatalf("expected empty RELAY_NODES, got %d", len(relays))
	}
}

func TestPeerType_Relay_PeerMarkedAsRelay(t *testing.T) {
	dial, stop := testServer(t)
	defer stop()

	roomID := "room-pt2"

	// First: a user peer.
	cUser := dial()
	defer cUser.Close()
	sendHello(t, cUser, roomID, uuid.NewString())
	readJSON(t, cUser) // PEER_LIST
	readJSON(t, cUser) // RELAY_NODES

	// Second: a relay peer.
	cRelay := dial()
	defer cRelay.Close()
	relayID := uuid.NewString()
	sendHelloFull(t, cRelay, roomID, relayID, "relay", "eu-de")

	// Relay receives PEER_LIST + RELAY_NODES.
	readJSON(t, cRelay) // PEER_LIST
	rnRelay := readJSON(t, cRelay)
	if rnRelay["type"] != "RELAY_NODES" {
		t.Fatalf("relay expected RELAY_NODES, got %v", rnRelay["type"])
	}

	// User should receive PEER_JOINED + updated RELAY_NODES.
	readMsgOfType(t, cUser, "PEER_JOINED")
	rnUser := readMsgOfType(t, cUser, "RELAY_NODES")
	relays := rnUser["peers"].([]any)
	if len(relays) != 1 {
		t.Fatalf("expected 1 relay, got %d", len(relays))
	}
	relay := relays[0].(map[string]any)
	if relay["peerId"] != relayID {
		t.Fatalf("expected relay peerId=%q, got %q", relayID, relay["peerId"])
	}
	if relay["region"] != "eu-de" {
		t.Fatalf("expected region=eu-de, got %q", relay["region"])
	}
}

func TestPeerType_Unknown_Rejected(t *testing.T) {
	dial, stop := testServer(t)
	defer stop()

	c := dial()
	defer c.Close()

	sendHelloWithType(t, c, "room-pt3", uuid.NewString(), "unknown")

	errMsg := readJSON(t, c)
	if errMsg["type"] != "ERROR" {
		t.Fatalf("expected ERROR, got %v", errMsg["type"])
	}
	if errMsg["code"] != ErrCodeBadRequest {
		t.Fatalf("expected BAD_REQUEST, got %v", errMsg["code"])
	}
}

func TestRelayNodes_SentAfterPeerList(t *testing.T) {
	dial, stop := testServer(t)
	defer stop()

	c := dial()
	defer c.Close()

	sendHello(t, c, "room-rn1", uuid.NewString())

	// First message must be PEER_LIST, second must be RELAY_NODES.
	msg1 := readJSON(t, c)
	if msg1["type"] != "PEER_LIST" {
		t.Fatalf("first message expected PEER_LIST, got %v", msg1["type"])
	}
	msg2 := readJSON(t, c)
	if msg2["type"] != "RELAY_NODES" {
		t.Fatalf("second message expected RELAY_NODES, got %v", msg2["type"])
	}
}

func TestRelayNodes_BroadcastOnRelayJoin(t *testing.T) {
	dial, stop := testServer(t)
	defer stop()

	roomID := "room-rn2"

	c1 := dial()
	defer c1.Close()
	sendHello(t, c1, roomID, uuid.NewString())
	readJSON(t, c1) // PEER_LIST
	rn1 := readJSON(t, c1) // RELAY_NODES (empty)
	relays := rn1["peers"].([]any)
	if len(relays) != 0 {
		t.Fatalf("expected 0 relays initially, got %d", len(relays))
	}

	// Relay node joins.
	cRelay := dial()
	defer cRelay.Close()
	relayID := uuid.NewString()
	sendHelloFull(t, cRelay, roomID, relayID, "relay", "us-east")
	readJSON(t, cRelay) // PEER_LIST
	readJSON(t, cRelay) // RELAY_NODES

	// c1 gets PEER_JOINED + RELAY_NODES broadcast.
	readMsgOfType(t, c1, "PEER_JOINED")
	rnBroadcast := readMsgOfType(t, c1, "RELAY_NODES")
	relays = rnBroadcast["peers"].([]any)
	if len(relays) != 1 {
		t.Fatalf("expected 1 relay after join, got %d", len(relays))
	}
	if relays[0].(map[string]any)["peerId"] != relayID {
		t.Fatalf("relay peerId mismatch")
	}
}

func TestRelayNodes_BroadcastOnRelayLeave(t *testing.T) {
	dial, stop := testServer(t)
	defer stop()

	roomID := "room-rn3"

	c1 := dial()
	defer c1.Close()
	sendHello(t, c1, roomID, uuid.NewString())
	readJSON(t, c1) // PEER_LIST
	readJSON(t, c1) // RELAY_NODES

	// Relay joins.
	cRelay := dial()
	relayID := uuid.NewString()
	sendHelloFull(t, cRelay, roomID, relayID, "relay", "eu-de")
	readJSON(t, cRelay) // PEER_LIST
	readJSON(t, cRelay) // RELAY_NODES

	// Consume relay join messages on c1.
	readMsgOfType(t, c1, "PEER_JOINED")
	readMsgOfType(t, c1, "RELAY_NODES")

	// Relay disconnects.
	cRelay.Close()

	// c1 should receive PEER_LEFT + RELAY_NODES with empty list.
	readMsgOfType(t, c1, "PEER_LEFT")
	rnAfterLeave := readMsgOfType(t, c1, "RELAY_NODES")
	relays := rnAfterLeave["peers"].([]any)
	if len(relays) != 0 {
		t.Fatalf("expected 0 relays after relay left, got %d", len(relays))
	}
}

func TestRelayNodes_ContainsOnlyRelayPeers(t *testing.T) {
	dial, stop := testServer(t)
	defer stop()

	roomID := "room-rn4"

	// Two user peers.
	c1 := dial()
	defer c1.Close()
	sendHello(t, c1, roomID, uuid.NewString())
	readJSON(t, c1) // PEER_LIST
	readJSON(t, c1) // RELAY_NODES

	c2 := dial()
	defer c2.Close()
	sendHello(t, c2, roomID, uuid.NewString())
	readMsgOfType(t, c1, "PEER_JOINED")
	readJSON(t, c2) // PEER_LIST
	readJSON(t, c2) // RELAY_NODES

	// One relay peer.
	cRelay := dial()
	defer cRelay.Close()
	relayID := uuid.NewString()
	sendHelloFull(t, cRelay, roomID, relayID, "relay", "eu-de")
	readJSON(t, cRelay) // PEER_LIST
	readJSON(t, cRelay) // RELAY_NODES

	// c1 receives RELAY_NODES with exactly 1 relay (not 2 users + 1 relay).
	readMsgOfType(t, c1, "PEER_JOINED")
	rn := readMsgOfType(t, c1, "RELAY_NODES")
	relays := rn["peers"].([]any)
	if len(relays) != 1 {
		t.Fatalf("expected exactly 1 relay peer, got %d", len(relays))
	}
	if relays[0].(map[string]any)["peerId"] != relayID {
		t.Fatalf("RELAY_NODES must contain only the relay peer")
	}
}

// ─── License enforcement tests ────────────────────────────────────────────────

func TestHello_RoomLimitExceeded_ReturnsError(t *testing.T) {
	// Limit: 1 room. First room succeeds; second room must return ROOM_LIMIT_EXCEEDED.
	v := mockValidator{maxRooms: 1, maxPeers: 50}
	dial, stop := testServerWithValidator(t, v)
	defer stop()

	c1 := dial()
	defer c1.Close()
	sendHello(t, c1, "room-lim-1", uuid.NewString())
	readJSON(t, c1) // PEER_LIST
	readJSON(t, c1) // RELAY_NODES

	c2 := dial()
	defer c2.Close()
	sendHello(t, c2, "room-lim-2", uuid.NewString()) // different room → new room creation
	errMsg := readJSON(t, c2)
	if errMsg["type"] != "ERROR" {
		t.Fatalf("expected ERROR, got %v", errMsg["type"])
	}
	if errMsg["code"] != ErrCodeRoomLimitExceeded {
		t.Fatalf("expected %q, got %v", ErrCodeRoomLimitExceeded, errMsg["code"])
	}
}

func TestHello_PeerLimitExceeded_ReturnsError(t *testing.T) {
	// Limit: 2 peers per room. Third peer must return PEER_LIMIT_EXCEEDED.
	v := mockValidator{maxRooms: 0, maxPeers: 2}
	dial, stop := testServerWithValidator(t, v)
	defer stop()

	roomID := "room-peer-lim"

	c1 := dial()
	defer c1.Close()
	sendHello(t, c1, roomID, uuid.NewString())
	readJSON(t, c1) // PEER_LIST
	readJSON(t, c1) // RELAY_NODES

	c2 := dial()
	defer c2.Close()
	sendHello(t, c2, roomID, uuid.NewString())
	readMsgOfType(t, c1, "PEER_JOINED")
	readJSON(t, c2) // PEER_LIST
	readJSON(t, c2) // RELAY_NODES

	c3 := dial()
	defer c3.Close()
	sendHello(t, c3, roomID, uuid.NewString())
	errMsg := readJSON(t, c3)
	if errMsg["type"] != "ERROR" {
		t.Fatalf("expected ERROR, got %v", errMsg["type"])
	}
	if errMsg["code"] != ErrCodePeerLimitExceeded {
		t.Fatalf("expected %q, got %v", ErrCodePeerLimitExceeded, errMsg["code"])
	}
}

func TestHello_SameRoom_DoesNotCountAsNewRoom(t *testing.T) {
	// Limit: 1 room. Two peers joining the same room must both succeed.
	v := mockValidator{maxRooms: 1, maxPeers: 50}
	dial, stop := testServerWithValidator(t, v)
	defer stop()

	roomID := "room-same"

	c1 := dial()
	defer c1.Close()
	sendHello(t, c1, roomID, uuid.NewString())
	readJSON(t, c1) // PEER_LIST
	readJSON(t, c1) // RELAY_NODES

	// Second peer joins the same room — must succeed (not a new room).
	c2 := dial()
	defer c2.Close()
	sendHello(t, c2, roomID, uuid.NewString())
	readMsgOfType(t, c1, "PEER_JOINED")
	pl := readJSON(t, c2)
	if pl["type"] != "PEER_LIST" {
		t.Fatalf("expected PEER_LIST (join succeeded), got %v", pl["type"])
	}
}
