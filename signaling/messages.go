package signaling

// ─── Client → Server ─────────────────────────────────────────────────────────

type helloMsg struct {
	Type     string `json:"type"`
	RoomID   string `json:"roomId"`
	PeerID   string `json:"peerId"`
	PeerType string `json:"peerType,omitempty"`
	Region   string `json:"region,omitempty"` // relay peers only: region tag (e.g. "eu-de")
	Nonce    string `json:"nonce"`
	HMAC     string `json:"hmac"`
}

type relayMsg struct {
	Type    string `json:"type"`
	RoomID  string `json:"roomId"`
	Payload string `json:"payload"` // base64 AES-GCM ciphertext
}

type iceMsg struct {
	Type         string `json:"type"`
	RoomID       string `json:"roomId"`
	TargetPeerID string `json:"targetPeerId"`
	Payload      string `json:"payload"` // base64 SDP or candidate
}

// ─── Server → Client ─────────────────────────────────────────────────────────

type peerListMsg struct {
	Type  string   `json:"type"`
	Peers []string `json:"peers"`
}

type peerJoinedMsg struct {
	Type   string `json:"type"`
	PeerID string `json:"peerId"`
}

type peerLeftMsg struct {
	Type   string `json:"type"`
	PeerID string `json:"peerId"`
}

type relayDeliverMsg struct {
	Type       string `json:"type"`
	FromPeerID string `json:"fromPeerId"`
	Payload    string `json:"payload"`
}

type iceForwardMsg struct {
	Type       string `json:"type"`
	FromPeerID string `json:"fromPeerId"`
	RoomID     string `json:"roomId"`
	Payload    string `json:"payload"`
}

type errorMsg struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type pongMsg struct {
	Type string `json:"type"`
}

// ─── Relay node messages ────────────────────────────────────────────────────

type relayPeerInfo struct {
	PeerID string `json:"peerId"`
	Region string `json:"region"`
}

type relayNodesMsg struct {
	Type  string          `json:"type"`
	Peers []relayPeerInfo `json:"peers"`
}

// Error codes as defined in PROTOCOL.md.
const (
	ErrCodeRoomFull            = "ROOM_FULL"
	ErrCodeDuplicatePeerID     = "DUPLICATE_PEER_ID"
	ErrCodeNonceReplay         = "NONCE_REPLAY"
	ErrCodeBadRequest          = "BAD_REQUEST"
	ErrCodeLicenseExpired      = "LICENSE_EXPIRED"
	ErrCodeRoomLimitExceeded   = "ROOM_LIMIT_EXCEEDED"
	ErrCodePeerLimitExceeded   = "PEER_LIMIT_EXCEEDED"
)

// Peer types as defined in PROTOCOL.md.
const (
	PeerTypeUser  = "user"
	PeerTypeRelay = "relay"
)
