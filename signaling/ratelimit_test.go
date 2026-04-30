package signaling

import (
	"testing"
)

func TestConnLimiter_AllowsUpToMax(t *testing.T) {
	l := NewConnLimiter(3)
	for i := range 3 {
		if !l.Acquire("1.2.3.4") {
			t.Fatalf("expected Acquire to succeed on attempt %d", i+1)
		}
	}
}

func TestConnLimiter_RejectsOverMax(t *testing.T) {
	l := NewConnLimiter(2)
	l.Acquire("1.2.3.4")
	l.Acquire("1.2.3.4")
	if l.Acquire("1.2.3.4") {
		t.Fatal("expected Acquire to fail when limit reached")
	}
}

func TestConnLimiter_ReleaseOpensSlot(t *testing.T) {
	l := NewConnLimiter(1)
	if !l.Acquire("1.2.3.4") {
		t.Fatal("first Acquire should succeed")
	}
	if l.Acquire("1.2.3.4") {
		t.Fatal("second Acquire should fail (limit=1)")
	}
	l.Release("1.2.3.4")
	if !l.Acquire("1.2.3.4") {
		t.Fatal("Acquire after Release should succeed")
	}
}

func TestConnLimiter_IndependentPerIP(t *testing.T) {
	l := NewConnLimiter(1)
	if !l.Acquire("1.1.1.1") {
		t.Fatal("1.1.1.1 first Acquire should succeed")
	}
	if !l.Acquire("2.2.2.2") {
		t.Fatal("2.2.2.2 should have its own independent limit")
	}
	if l.Acquire("1.1.1.1") {
		t.Fatal("1.1.1.1 second Acquire should fail")
	}
}

func TestConnLimiter_ReleaseDeletesZeroEntry(t *testing.T) {
	l := NewConnLimiter(2)
	l.Acquire("1.2.3.4")
	l.Release("1.2.3.4")
	l.mu.Lock()
	_, exists := l.counts["1.2.3.4"]
	l.mu.Unlock()
	if exists {
		t.Fatal("entry should be deleted after count reaches zero")
	}
}

func TestConnLimiter_ZeroMax_Unlimited(t *testing.T) {
	l := NewConnLimiter(0)
	for range 100 {
		if !l.Acquire("1.2.3.4") {
			t.Fatal("max=0 (unlimited) should always allow connections")
		}
	}
}

func TestConnLimiter_NegativeMax_Unlimited(t *testing.T) {
	l := NewConnLimiter(-1)
	if !l.Acquire("1.2.3.4") {
		t.Fatal("negative max (unlimited) should always allow connections")
	}
}
