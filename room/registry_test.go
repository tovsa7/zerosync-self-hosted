package room

import (
	"testing"
)

func TestRegistry_GetOrCreate(t *testing.T) {
	reg := NewRegistry()
	defer reg.Stop()

	r1 := reg.GetOrCreate("room-a")
	r2 := reg.GetOrCreate("room-a")
	if r1 != r2 {
		t.Fatal("GetOrCreate should return same Room pointer for same ID")
	}
}

func TestRegistry_Get_MissingReturnsNil(t *testing.T) {
	reg := NewRegistry()
	defer reg.Stop()

	if reg.Get("nonexistent") != nil {
		t.Fatal("Get of unknown room should return nil")
	}
}

func TestRegistry_Remove(t *testing.T) {
	reg := NewRegistry()
	defer reg.Stop()

	reg.GetOrCreate("room-x")
	if reg.Len() != 1 {
		t.Fatal("expected 1 room after GetOrCreate")
	}
	reg.Remove("room-x")
	if reg.Len() != 0 {
		t.Fatal("expected 0 rooms after Remove")
	}
	reg.Remove("room-x") // idempotent, must not panic
}

func TestRegistry_GC_RemovesEmptyRooms(t *testing.T) {
	reg := &Registry{
		rooms:  make(map[string]*Room),
		stopGC: make(chan struct{}),
	}
	// Don't start the background goroutine — call gc logic directly.
	reg.GetOrCreate("empty-room")
	if reg.Len() != 1 {
		t.Fatal("expected 1 room")
	}

	// Trigger GC manually.
	reg.mu.Lock()
	for id, rm := range reg.rooms {
		if rm.Empty() {
			delete(reg.rooms, id)
		}
	}
	reg.mu.Unlock()

	if reg.Len() != 0 {
		t.Fatal("GC should have removed empty room")
	}
}

func TestRegistry_GC_KeepsRoomsWithPeers(t *testing.T) {
	reg := &Registry{
		rooms:  make(map[string]*Room),
		stopGC: make(chan struct{}),
	}
	rm := reg.GetOrCreate("active-room")
	rm.AddPeer(peer("p1"))

	// Trigger GC manually.
	reg.mu.Lock()
	for id, r := range reg.rooms {
		if r.Empty() {
			delete(reg.rooms, id)
		}
	}
	reg.mu.Unlock()

	if reg.Len() != 1 {
		t.Fatal("GC should keep rooms that have peers")
	}
}
