package signaling

import (
	"sync"
	"time"
)

// NonceTTL is the replay-protection window for HELLO nonces.
const NonceTTL = 30 * time.Second

const nonceGCInterval = 10 * time.Second

// NonceStore tracks seen nonces and rejects replays within NonceTTL.
//
// Spec:
//   - Seen(nonce) returns true and records the nonce if it has not been seen
//     within the last NonceTTL. Returns false (replay) if the nonce is known.
//   - Expired entries are purged every 10 s by a background goroutine.
//   - All methods are safe for concurrent use.
type NonceStore struct {
	mu     sync.Mutex
	seen   map[string]time.Time // nonce → expiry
	stopGC chan struct{}
	now    func() time.Time
}

// NewNonceStore creates a NonceStore and starts the background GC goroutine.
func NewNonceStore() *NonceStore {
	ns := &NonceStore{
		seen:   make(map[string]time.Time),
		stopGC: make(chan struct{}),
		now:    time.Now,
	}
	go ns.gc()
	return ns
}

// Seen records nonce and returns true, unless the nonce was already recorded
// within the last NonceTTL, in which case it returns false (replay detected).
func (ns *NonceStore) Seen(nonce string) bool {
	ns.mu.Lock()
	defer ns.mu.Unlock()
	now := ns.now()
	if exp, ok := ns.seen[nonce]; ok && now.Before(exp) {
		return false // replay
	}
	ns.seen[nonce] = now.Add(NonceTTL)
	return true
}

// Stop halts the GC goroutine.
func (ns *NonceStore) Stop() {
	close(ns.stopGC)
}

func (ns *NonceStore) gc() {
	ticker := time.NewTicker(nonceGCInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := ns.now()
			ns.mu.Lock()
			for k, exp := range ns.seen {
				if now.After(exp) {
					delete(ns.seen, k)
				}
			}
			ns.mu.Unlock()
		case <-ns.stopGC:
			return
		}
	}
}
