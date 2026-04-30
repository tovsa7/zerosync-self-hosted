// Package auth defines the contract between the signaling server and any
// pluggable validator that gates room creation and peer joins.
//
// The self-hosted (Apache 2.0) build uses NoopValidator, which allows
// everything. Enterprise builds inject a JWT-based validator that enforces
// per-tier room and peer limits. Sentinel errors declared here are the
// stable wire contract: the signaling layer maps each one to a specific
// wire-protocol error code, so any Validator implementation MUST return
// these exact errors (or wrap them with errors.Is-compatible semantics).
package auth

import "errors"

// Validator gates new room creation and peer joins.
//
// Implementations must be safe for concurrent use — the signaling layer
// calls these methods from per-connection goroutines.
type Validator interface {
	// CheckRoomLimit returns an error if creating one additional room
	// (current+1) would exceed the tier limit. current is the number of
	// existing rooms before the new one.
	//
	// Returns nil to allow. Returns ErrRoomLimitExceeded or
	// ErrLicenseExpired to deny.
	CheckRoomLimit(current int) error

	// CheckPeerLimit returns an error if adding one additional peer to a
	// room (current+1) would exceed the tier limit. current is the number
	// of peers already in the room.
	//
	// Returns nil to allow. Returns ErrPeerLimitExceeded or
	// ErrLicenseExpired to deny.
	CheckPeerLimit(current int) error
}

// NoopValidator allows all rooms and peers without restriction. It is the
// default validator for the open-source self-hosted server: no license
// enforcement, no quota gates.
type NoopValidator struct{}

// CheckRoomLimit always returns nil.
func (NoopValidator) CheckRoomLimit(int) error { return nil }

// CheckPeerLimit always returns nil.
func (NoopValidator) CheckPeerLimit(int) error { return nil }

// Sentinel errors. Validator implementations must return these exact
// values (not wrapped strings) so the signaling layer can map them to
// the correct wire error code via errors.Is comparison.
var (
	// ErrLicenseExpired indicates the active license token has expired.
	ErrLicenseExpired = errors.New("auth: license expired")

	// ErrRoomLimitExceeded indicates the tier room limit would be exceeded
	// by creating one more room.
	ErrRoomLimitExceeded = errors.New("auth: room limit exceeded")

	// ErrPeerLimitExceeded indicates the tier peer limit would be exceeded
	// by adding one more peer to a room.
	ErrPeerLimitExceeded = errors.New("auth: peer limit exceeded")
)
