package room

import (
	"sync"
)

// Peer is a minimal interface so that room.go does not import signaling.
type Peer interface {
	ID() string
}

// Room holds the set of peers currently connected to a named room.
//
// Spec:
//   - A peer is "in" a room iff the last operation for that peerID was AddPeer.
//   - AddPeer returns false and is a no-op if peerID is already present.
//   - RemovePeer is a no-op if peerID is not present.
//   - PeerIDs returns a snapshot; callers must not mutate the slice.
//   - All methods are safe for concurrent use.
type Room struct {
	id    string
	peers map[string]Peer
	mu    sync.RWMutex
}

func newRoom(id string) *Room {
	return &Room{
		id:    id,
		peers: make(map[string]Peer),
	}
}

// ID returns the room identifier.
func (r *Room) ID() string { return r.id }

// AddPeer adds p to the room. Returns false if peerID is already present.
func (r *Room) AddPeer(p Peer) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.peers[p.ID()]; exists {
		return false
	}
	r.peers[p.ID()] = p
	return true
}

// RemovePeer removes peerID from the room.
func (r *Room) RemovePeer(peerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.peers, peerID)
}

// HasPeer reports whether peerID is in the room.
func (r *Room) HasPeer(peerID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.peers[peerID]
	return ok
}

// PeerIDs returns a snapshot of all peer IDs in the room.
func (r *Room) PeerIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.peers))
	for id := range r.peers {
		ids = append(ids, id)
	}
	return ids
}

// Peers returns a snapshot of all Peer values in the room.
func (r *Room) Peers() []Peer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ps := make([]Peer, 0, len(r.peers))
	for _, p := range r.peers {
		ps = append(ps, p)
	}
	return ps
}

// Len returns the number of peers currently in the room.
func (r *Room) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.peers)
}

// Empty reports whether the room has no peers.
func (r *Room) Empty() bool {
	return r.Len() == 0
}
