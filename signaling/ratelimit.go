package signaling

import (
	"net"
	"net/http"
	"sync"
)

// ConnLimiter enforces a per-IP concurrent WebSocket connection limit.
// It is safe for concurrent use.
type ConnLimiter struct {
	mu     sync.Mutex
	counts map[string]int
	max    int
}

// NewConnLimiter returns a ConnLimiter that allows at most maxPerIP concurrent
// connections from a single remote IP. Pass 0 (or any value ≤ 0) to disable
// the limit (unlimited connections allowed).
func NewConnLimiter(maxPerIP int) *ConnLimiter {
	return &ConnLimiter{
		counts: make(map[string]int),
		max:    maxPerIP,
	}
}

// Acquire attempts to reserve a connection slot for the given IP.
// Returns true if the slot was granted, false if the limit is already reached.
// Always returns true when max ≤ 0 (unlimited mode).
func (l *ConnLimiter) Acquire(ip string) bool {
	if l.max <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.counts[ip] >= l.max {
		return false
	}
	l.counts[ip]++
	return true
}

// Available reports whether a fresh slot could currently be acquired for
// the given IP, without actually reserving it. Used by GET /health so a
// prospective client can learn capacity status before opening a WebSocket.
// Always returns true when max ≤ 0 (unlimited mode).
func (l *ConnLimiter) Available(ip string) bool {
	if l.max <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.counts[ip] < l.max
}

// Release frees a previously acquired connection slot for the given IP.
// No-op when max ≤ 0 (unlimited mode).
func (l *ConnLimiter) Release(ip string) {
	if l.max <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.counts[ip]--
	if l.counts[ip] <= 0 {
		delete(l.counts, ip)
	}
}

// RemoteIP extracts the host portion of r.RemoteAddr, stripping the port.
// Falls back to the raw RemoteAddr if parsing fails.
func RemoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
