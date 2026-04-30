package signaling

import (
	"fmt"
	"testing"
	"time"

	"pgregory.net/rapid"
)

func newTestNonceStore(now func() time.Time) *NonceStore {
	return &NonceStore{
		seen:   make(map[string]time.Time),
		stopGC: make(chan struct{}),
		now:    now,
	}
}

func TestNonceStore_FirstSeen_ReturnsTrue(t *testing.T) {
	ns := newTestNonceStore(time.Now)
	if !ns.Seen("abc") {
		t.Fatal("first Seen should return true")
	}
}

func TestNonceStore_Replay_ReturnsFalse(t *testing.T) {
	ns := newTestNonceStore(time.Now)
	ns.Seen("abc")
	if ns.Seen("abc") {
		t.Fatal("replay Seen should return false")
	}
}

func TestNonceStore_ExpiredNonce_AcceptedAgain(t *testing.T) {
	var now time.Time
	ns := newTestNonceStore(func() time.Time { return now })

	now = time.Time{}
	ns.Seen("abc")

	// Advance past TTL.
	now = time.Time{}.Add(NonceTTL + time.Second)
	if !ns.Seen("abc") {
		t.Fatal("nonce should be accepted again after TTL")
	}
}

// Property: ∀ nonce, Seen(nonce) returns true exactly once within a TTL window.
func TestProperty_NonceReplayWithinTTL(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ns := newTestNonceStore(time.Now)

		nonce := fmt.Sprintf("n-%d", rapid.IntRange(0, 999).Draw(t, "n"))
		calls := rapid.IntRange(1, 10).Draw(t, "calls")

		trueCount := 0
		for i := 0; i < calls; i++ {
			if ns.Seen(nonce) {
				trueCount++
			}
		}
		if trueCount != 1 {
			t.Fatalf("expected exactly 1 true, got %d for %d calls", trueCount, calls)
		}
	})
}
