package relay

import (
	"errors"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// newTestStore returns a BlobStore with a controllable clock.
func newTestStore(now func() time.Time) *BlobStore {
	s := &BlobStore{
		blobs:  make(map[string]blob),
		stopGC: make(chan struct{}),
		now:    now,
	}
	// Do not start GC goroutine in tests — eviction is triggered manually.
	return s
}

// ─── Unit tests ───────────────────────────────────────────────────────────────

func TestPut_Get_RoundTrip(t *testing.T) {
	s := newTestStore(time.Now)
	data := []byte("hello relay")
	if err := s.Put("k1", data); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, ok := s.Get("k1")
	if !ok {
		t.Fatal("Get should return true for existing key")
	}
	if string(got) != string(data) {
		t.Fatalf("Get: got %q, want %q", got, data)
	}
}

func TestPut_TooLarge(t *testing.T) {
	s := newTestStore(time.Now)
	big := make([]byte, MaxBlobSize+1)
	err := s.Put("big", big)
	if !errors.Is(err, ErrBlobTooLarge) {
		t.Fatalf("expected ErrBlobTooLarge, got %v", err)
	}
}

func TestPut_ExactlyMaxSize(t *testing.T) {
	s := newTestStore(time.Now)
	exact := make([]byte, MaxBlobSize)
	if err := s.Put("exact", exact); err != nil {
		t.Fatalf("Put of exactly MaxBlobSize should succeed: %v", err)
	}
}

func TestGet_MissingKey(t *testing.T) {
	s := newTestStore(time.Now)
	_, ok := s.Get("nope")
	if ok {
		t.Fatal("Get of missing key should return false")
	}
}

func TestDelete(t *testing.T) {
	s := newTestStore(time.Now)
	_ = s.Put("k", []byte("v"))
	s.Delete("k")
	_, ok := s.Get("k")
	if ok {
		t.Fatal("Get after Delete should return false")
	}
	s.Delete("k") // idempotent
}

// Property: blob TTL — visible at T+29s, invisible at T+31s.
//
//	∀ blob inserted at T: Get returns true at T+29s, false at T+31s.
func TestProperty_BlobTTL(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		var now time.Time

		s := &BlobStore{
			blobs:  make(map[string]blob),
			stopGC: make(chan struct{}),
			now:    func() time.Time { return now },
		}

		key := rapid.StringMatching(`[a-z]{1,8}`).Draw(t, "key")
		size := rapid.IntRange(0, 100).Draw(t, "size")
		data := make([]byte, size)

		// Insert at T=0.
		now = time.Time{}
		if err := s.Put(key, data); err != nil {
			t.Fatalf("Put failed: %v", err)
		}

		// T+29s: blob must still be visible.
		now = time.Time{}.Add(29 * time.Second)
		if _, ok := s.Get(key); !ok {
			t.Fatal("blob should be visible at T+29s")
		}

		// T+31s: blob must be expired.
		now = time.Time{}.Add(31 * time.Second)
		if _, ok := s.Get(key); ok {
			t.Fatal("blob should be expired at T+31s")
		}
	})
}

// Property: blob size enforcement.
//
//	∀ payload of size N:
//	  Put returns nil          if N ≤ MaxBlobSize
//	  Put returns ErrBlobTooLarge if N > MaxBlobSize
func TestProperty_BlobSizeEnforcement(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := newTestStore(time.Now)
		n := rapid.IntRange(0, MaxBlobSize*2).Draw(t, "n")
		err := s.Put("k", make([]byte, n))
		if n <= MaxBlobSize && err != nil {
			t.Fatalf("expected nil for size %d, got %v", n, err)
		}
		if n > MaxBlobSize && !errors.Is(err, ErrBlobTooLarge) {
			t.Fatalf("expected ErrBlobTooLarge for size %d, got %v", n, err)
		}
	})
}

// Property: Put isolates data — subsequent modification of input slice
// does not affect stored blob.
func TestProperty_PutCopiesData(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := newTestStore(time.Now)
		n := rapid.IntRange(1, 200).Draw(t, "n")
		data := make([]byte, n)
		for i := range data {
			data[i] = byte(rapid.IntRange(0, 255).Draw(t, "b"))
		}
		snapshot := make([]byte, n)
		copy(snapshot, data)

		_ = s.Put("k", data)

		// Mutate original slice.
		for i := range data {
			data[i] ^= 0xFF
		}

		got, ok := s.Get("k")
		if !ok {
			t.Fatal("blob should be retrievable")
		}
		if string(got) != string(snapshot) {
			t.Fatal("Put should copy data — mutation of source should not affect stored blob")
		}
	})
}
