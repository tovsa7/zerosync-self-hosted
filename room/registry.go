package room

import (
	"sync"
	"time"
)

const (
	// MaxPeersPerRoom is the maximum number of peers allowed in one room.
	MaxPeersPerRoom = 50

	gcInterval = 60 * time.Second
)

// Registry is a global map of roomID → Room.
//
// Spec:
//   - GetOrCreate returns the existing Room for roomID, or creates a new one.
//   - Remove deletes the room entry; no-op if room does not exist.
//   - A GC goroutine runs every 60 s and removes rooms with no peers.
//   - All methods are safe for concurrent use.
type Registry struct {
	mu    sync.RWMutex
	rooms map[string]*Room

	stopGC chan struct{}
}

// NewRegistry creates a Registry and starts the background GC goroutine.
func NewRegistry() *Registry {
	r := &Registry{
		rooms:  make(map[string]*Room),
		stopGC: make(chan struct{}),
	}
	go r.gc()
	return r
}

// GetOrCreate returns the Room for roomID, creating it if necessary.
func (r *Registry) GetOrCreate(roomID string) *Room {
	// Fast path: room exists.
	r.mu.RLock()
	rm, ok := r.rooms[roomID]
	r.mu.RUnlock()
	if ok {
		return rm
	}

	// Slow path: create.
	r.mu.Lock()
	defer r.mu.Unlock()
	if rm, ok = r.rooms[roomID]; ok {
		return rm
	}
	rm = newRoom(roomID)
	r.rooms[roomID] = rm
	return rm
}

// Get returns the Room for roomID, or nil if it does not exist.
func (r *Registry) Get(roomID string) *Room {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.rooms[roomID]
}

// Remove deletes the room entry. No-op if room does not exist.
func (r *Registry) Remove(roomID string) {
	r.mu.Lock()
	delete(r.rooms, roomID)
	r.mu.Unlock()
}

// Len returns the number of rooms currently tracked.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.rooms)
}

// Stop halts the GC goroutine. Call on server shutdown.
func (r *Registry) Stop() {
	close(r.stopGC)
}

func (r *Registry) gc() {
	ticker := time.NewTicker(gcInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			r.mu.Lock()
			for id, rm := range r.rooms {
				if rm.Empty() {
					delete(r.rooms, id)
				}
			}
			r.mu.Unlock()
		case <-r.stopGC:
			return
		}
	}
}
