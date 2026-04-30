//go:build integration

package signaling

import (
	"encoding/base64"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"
)

// TestRelay_FanOutToNPeers verifies that a RELAY from one peer is delivered
// to all other peers in the room, but not back to the sender.
func TestRelay_FanOutToNPeers(t *testing.T) {
	dial, _, stop := testServerWithRegistry(t)
	defer stop()

	roomID := "relay-fanout"
	const n = 10
	conns, peerIDs := connectN(t, dial, roomID, n)
	defer closeAll(conns)

	payload := base64.StdEncoding.EncodeToString([]byte("fanout-test"))
	sendRelay(t, conns[0], roomID, payload)

	// Peers 1..9 should all receive RELAY_DELIVER.
	for i := 1; i < n; i++ {
		msg := expectMessage(t, conns[i], "RELAY_DELIVER")
		if msg["fromPeerId"] != peerIDs[0] {
			t.Fatalf("peer %d: expected fromPeerId=%q, got %q", i, peerIDs[0], msg["fromPeerId"])
		}
		if msg["payload"] != payload {
			t.Fatalf("peer %d: payload mismatch", i)
		}
	}

	// Peer 0 (sender) should NOT receive its own message.
	expectNoMessage(t, conns[0], 200*time.Millisecond)
}

// TestRelay_OrderingPreservedPerSender verifies that messages from a single
// sender arrive in order at each receiver.
func TestRelay_OrderingPreservedPerSender(t *testing.T) {
	dial, _, stop := testServerWithRegistry(t)
	defer stop()

	roomID := "relay-ordering"
	conns, _ := connectN(t, dial, roomID, 3)
	defer closeAll(conns)

	const numMessages = 20

	// Peer 0 sends 20 numbered messages.
	for i := 0; i < numMessages; i++ {
		payload := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("msg-%03d", i)))
		sendRelay(t, conns[0], roomID, payload)
	}

	// Peers 1 and 2 should receive all 20 in order.
	for _, receiver := range []int{1, 2} {
		for i := 0; i < numMessages; i++ {
			msg := expectMessage(t, conns[receiver], "RELAY_DELIVER")
			decoded, _ := base64.StdEncoding.DecodeString(msg["payload"].(string))
			expected := fmt.Sprintf("msg-%03d", i)
			if string(decoded) != expected {
				t.Fatalf("peer %d: message %d expected %q, got %q", receiver, i, expected, string(decoded))
			}
		}
	}
}

// TestRelay_InterleavedSenders verifies that when two senders send
// concurrently, each sender's messages arrive in order relative to themselves.
func TestRelay_InterleavedSenders(t *testing.T) {
	dial, _, stop := testServerWithRegistry(t)
	defer stop()

	roomID := "relay-interleaved"
	conns, peerIDs := connectN(t, dial, roomID, 3)
	defer closeAll(conns)

	const perSender = 10

	// Peers 0 and 1 send concurrently.
	var wg sync.WaitGroup
	for _, sender := range []int{0, 1} {
		wg.Add(1)
		go func(s int) {
			defer wg.Done()
			for i := 0; i < perSender; i++ {
				payload := base64.StdEncoding.EncodeToString(
					[]byte(fmt.Sprintf("s%d-msg-%03d", s, i)))
				sendRelay(t, conns[s], roomID, payload)
			}
		}(sender)
	}
	wg.Wait()

	// Peer 2 should receive 20 RELAY_DELIVER messages total.
	// Per-sender order must be preserved.
	fromA := make([]string, 0, perSender)
	fromB := make([]string, 0, perSender)

	for i := 0; i < 2*perSender; i++ {
		msg := expectMessage(t, conns[2], "RELAY_DELIVER")
		decoded, _ := base64.StdEncoding.DecodeString(msg["payload"].(string))
		switch msg["fromPeerId"] {
		case peerIDs[0]:
			fromA = append(fromA, string(decoded))
		case peerIDs[1]:
			fromB = append(fromB, string(decoded))
		default:
			t.Fatalf("unexpected fromPeerId: %v", msg["fromPeerId"])
		}
	}

	if len(fromA) != perSender || len(fromB) != perSender {
		t.Fatalf("expected %d messages from each sender, got A=%d, B=%d",
			perSender, len(fromA), len(fromB))
	}

	// Verify per-sender ordering.
	if !sort.StringsAreSorted(fromA) {
		t.Fatalf("messages from sender A are out of order: %v", fromA)
	}
	if !sort.StringsAreSorted(fromB) {
		t.Fatalf("messages from sender B are out of order: %v", fromB)
	}
}

// TestRelay_EmptyPayload_ReturnsError verifies that an empty RELAY payload
// results in an ERROR.
func TestRelay_EmptyPayload_ReturnsError(t *testing.T) {
	dial, _, stop := testServerWithRegistry(t)
	defer stop()

	roomID := "relay-empty"
	conns, _ := connectN(t, dial, roomID, 1)
	defer closeAll(conns)

	// Send RELAY with empty payload.
	sendRaw(t, conns[0], map[string]string{"type": "RELAY", "roomId": roomID, "payload": ""})

	errMsg := expectMessage(t, conns[0], "ERROR")
	if errMsg["code"] != ErrCodeBadRequest {
		t.Fatalf("expected BAD_REQUEST, got %v", errMsg["code"])
	}
}

// TestRelay_SenderDoesNotReceiveOwnMessage verifies the negative case
// explicitly: a RELAY sender never receives its own RELAY_DELIVER.
func TestRelay_SenderDoesNotReceiveOwnMessage(t *testing.T) {
	dial, _, stop := testServerWithRegistry(t)
	defer stop()

	roomID := "relay-no-echo"
	conns, _ := connectN(t, dial, roomID, 2)
	defer closeAll(conns)

	sendRelay(t, conns[0], roomID, "ZWNoby10ZXN0") // base64("echo-test")

	// Peer 1 receives the message.
	expectMessage(t, conns[1], "RELAY_DELIVER")

	// Peer 0 should NOT receive anything.
	expectNoMessage(t, conns[0], 200*time.Millisecond)
}
