package signaling

import (
	"crypto/sha256"
	"fmt"
)

// hashID returns the first 16 hex chars of SHA-256(id).
// Used in log statements so that roomID and peerID are never logged in plaintext.
//
// Invariant: server logs no plaintext identifiers (SECURITY.md §4).
func hashID(id string) string {
	sum := sha256.Sum256([]byte(id))
	return fmt.Sprintf("%x", sum[:8])
}
