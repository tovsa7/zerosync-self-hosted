package room

import (
	"fmt"
	"testing"

	"pgregory.net/rapid"
)

// mockPeer is a minimal Peer implementation for tests.
type mockPeer struct{ id string }

func (m *mockPeer) ID() string { return m.id }

func peer(id string) Peer { return &mockPeer{id: id} }

// ─── Unit tests ───────────────────────────────────────────────────────────────

func TestAddPeer_ReturnsFalseOnDuplicate(t *testing.T) {
	r := newRoom("r1")
	if !r.AddPeer(peer("a")) {
		t.Fatal("first AddPeer should return true")
	}
	if r.AddPeer(peer("a")) {
		t.Fatal("duplicate AddPeer should return false")
	}
	if r.Len() != 1 {
		t.Fatalf("expected 1 peer, got %d", r.Len())
	}
}

func TestRemovePeer_Idempotent(t *testing.T) {
	r := newRoom("r1")
	r.AddPeer(peer("a"))
	r.RemovePeer("a")
	r.RemovePeer("a") // must not panic
	if r.Len() != 0 {
		t.Fatal("expected empty room after remove")
	}
}

func TestHasPeer(t *testing.T) {
	r := newRoom("r1")
	r.AddPeer(peer("x"))
	if !r.HasPeer("x") {
		t.Fatal("HasPeer should return true after AddPeer")
	}
	r.RemovePeer("x")
	if r.HasPeer("x") {
		t.Fatal("HasPeer should return false after RemovePeer")
	}
}

func TestPeerIDs_Snapshot(t *testing.T) {
	r := newRoom("r1")
	r.AddPeer(peer("a"))
	r.AddPeer(peer("b"))
	ids := r.PeerIDs()
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %d", len(ids))
	}
}

// ─── Property-based tests ─────────────────────────────────────────────────────

// Property: peer set invariant.
//
//	∀ sequence of join/leave events:
//	  peerId appears in room.peers iff last event for that peerId was join.
func TestProperty_PeerSetInvariant(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		r := newRoom("prop-room")

		// Mirror map: ground-truth state.
		inRoom := make(map[string]bool)

		// Generate a sequence of up to 50 join/leave operations.
		n := rapid.IntRange(1, 50).Draw(t, "n")
		for i := 0; i < n; i++ {
			// Pick one of a small set of peer IDs to exercise duplicates.
			id := fmt.Sprintf("peer-%d", rapid.IntRange(0, 4).Draw(t, "peerIdx"))
			join := rapid.Bool().Draw(t, "join")

			if join {
				r.AddPeer(peer(id))
				inRoom[id] = true
			} else {
				r.RemovePeer(id)
				inRoom[id] = false
			}
		}

		// Verify invariant: HasPeer matches ground truth.
		for id, expected := range inRoom {
			if r.HasPeer(id) != expected {
				t.Fatalf("peer %q: HasPeer=%v, expected=%v", id, r.HasPeer(id), expected)
			}
		}

		// Verify PeerIDs count matches ground-truth count.
		expectedCount := 0
		for _, v := range inRoom {
			if v {
				expectedCount++
			}
		}
		if r.Len() != expectedCount {
			t.Fatalf("Len=%d, expectedCount=%d", r.Len(), expectedCount)
		}
	})
}
