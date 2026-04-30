package relay

import (
	"fmt"
	"sync"
	"time"
)

const (
	// MaxBlobSize is the maximum allowed payload size in bytes (64 KB).
	MaxBlobSize = 64 * 1024

	// BlobTTL is how long a relay blob is retained before eviction.
	BlobTTL = 30 * time.Second

	gcInterval = 10 * time.Second
)

// ErrBlobTooLarge is returned by Put when the payload exceeds MaxBlobSize.
var ErrBlobTooLarge = fmt.Errorf("relay blob exceeds maximum size of %d bytes", MaxBlobSize)

type blob struct {
	data      []byte
	expiresAt time.Time
}

// BlobStore is an in-memory store for relay blobs with TTL-based eviction.
//
// Spec:
//   - Put stores data under key. Returns ErrBlobTooLarge if len(data) > MaxBlobSize.
//   - Stored blobs expire BlobTTL (30 s) after insertion.
//   - Get returns (data, true) if key exists and has not expired; (nil, false) otherwise.
//   - A GC goroutine evicts expired blobs every 10 s.
//   - All methods are safe for concurrent use.
type BlobStore struct {
	mu     sync.Mutex
	blobs  map[string]blob
	stopGC chan struct{}
	now    func() time.Time // injectable for tests
}

// NewBlobStore creates a BlobStore and starts the background GC goroutine.
func NewBlobStore() *BlobStore {
	s := &BlobStore{
		blobs:  make(map[string]blob),
		stopGC: make(chan struct{}),
		now:    time.Now,
	}
	go s.gc()
	return s
}

// Put stores data under key with a TTL of BlobTTL.
// Returns ErrBlobTooLarge if len(data) > MaxBlobSize.
func (s *BlobStore) Put(key string, data []byte) error {
	if len(data) > MaxBlobSize {
		return ErrBlobTooLarge
	}
	cp := make([]byte, len(data))
	copy(cp, data)

	s.mu.Lock()
	s.blobs[key] = blob{data: cp, expiresAt: s.now().Add(BlobTTL)}
	s.mu.Unlock()
	return nil
}

// Get returns the blob for key if it exists and has not expired.
func (s *BlobStore) Get(key string) ([]byte, bool) {
	s.mu.Lock()
	b, ok := s.blobs[key]
	s.mu.Unlock()
	if !ok || s.now().After(b.expiresAt) {
		return nil, false
	}
	return b.data, true
}

// Delete removes a blob by key. No-op if it does not exist.
func (s *BlobStore) Delete(key string) {
	s.mu.Lock()
	delete(s.blobs, key)
	s.mu.Unlock()
}

// Len returns the number of blobs currently stored (including expired ones
// not yet evicted by GC).
func (s *BlobStore) Len() int {
	s.mu.Lock()
	n := len(s.blobs)
	s.mu.Unlock()
	return n
}

// Stop halts the GC goroutine. Call on server shutdown.
func (s *BlobStore) Stop() {
	close(s.stopGC)
}

func (s *BlobStore) gc() {
	ticker := time.NewTicker(gcInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := s.now()
			s.mu.Lock()
			for k, b := range s.blobs {
				if now.After(b.expiresAt) {
					delete(s.blobs, k)
				}
			}
			s.mu.Unlock()
		case <-s.stopGC:
			return
		}
	}
}
