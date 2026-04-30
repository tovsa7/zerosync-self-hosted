package signaling

import (
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/gorilla/websocket"
	"github.com/google/uuid"

	"github.com/tovsa7/zerosync-self-hosted/auth"
	"github.com/tovsa7/zerosync-self-hosted/relay"
	"github.com/tovsa7/zerosync-self-hosted/room"
)

const (
	// RELAY payloads are base64-encoded, so the wire size is ~4/3 of the binary
	// limit. Account for base64 expansion plus JSON framing overhead.
	wsReadLimit   = (relay.MaxBlobSize*4)/3 + 4096 // base64(64KB) + JSON overhead
	wsReadTimeout = 60 * time.Second
)

// Handler handles one WebSocket connection from connection open to close.
//
// Spec:
//   - On connect: read HELLO, validate, add peer to room, send PEER_LIST,
//     broadcast PEER_JOINED.
//   - RELAY: validate payload ≤ 64 KB, broadcast RELAY_DELIVER to room peers.
//   - ICE_OFFER/ICE_ANSWER/ICE_CANDIDATE: forward to targetPeerId.
//   - PING: respond with PONG.
//   - On disconnect: remove peer from room, broadcast PEER_LEFT.
//   - peerID and roomID are hashed with SHA-256 before any log statement.
//   - Server does NOT verify HMAC — roomKey is unknown to server.
//   - License limits are enforced at HELLO time via validator.
type Handler struct {
	rooms     *room.Registry
	nonces    *NonceStore
	validator auth.Validator
}

// NewHandler creates a Handler. validator may be auth.NoopValidator{} for
// self-hosted deployments without license enforcement.
func NewHandler(rooms *room.Registry, nonces *NonceStore, validator auth.Validator) *Handler {
	return &Handler{rooms: rooms, nonces: nonces, validator: validator}
}

// Serve drives a single WebSocket connection to completion.
// It blocks until the connection is closed.
func (h *Handler) Serve(conn *websocket.Conn) {
	conn.SetReadLimit(wsReadLimit)
	conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
		return nil
	})

	peer, err := h.doHello(conn)
	if err != nil {
		slog.Debug("HELLO failed", "err", err)
		conn.Close()
		return
	}

	rm := h.rooms.GetOrCreate(peer.roomID)
	defer h.cleanup(peer, rm)

	go peer.writePump()

	// Send PEER_LIST to the new peer.
	existing := rm.PeerIDs()
	filtered := make([]string, 0, len(existing))
	for _, id := range existing {
		if id != peer.peerID {
			filtered = append(filtered, id)
		}
	}
	peer.SendJSON(peerListMsg{Type: "PEER_LIST", Peers: filtered})

	// Send RELAY_NODES to the new peer (immediately after PEER_LIST).
	peer.SendJSON(h.buildRelayNodes(rm))

	// Broadcast PEER_JOINED to existing peers.
	h.broadcastExcept(rm, peer.peerID, peerJoinedMsg{
		Type:   "PEER_JOINED",
		PeerID: peer.peerID,
	})

	// If the new peer is a relay, broadcast updated RELAY_NODES to all existing peers.
	if peer.peerType == PeerTypeRelay {
		h.broadcastExcept(rm, peer.peerID, h.buildRelayNodes(rm))
	}

	// Read loop.
	for {
		conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var base struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &base); err != nil {
			peer.SendJSON(errorMsg{Type: "ERROR", Code: ErrCodeBadRequest, Message: "invalid JSON"})
			continue
		}

		switch base.Type {
		case "RELAY":
			if !h.handleRelay(peer, rm, raw) {
				return
			}
		case "ICE_OFFER", "ICE_ANSWER", "ICE_CANDIDATE":
			h.handleICE(peer, rm, base.Type, raw)
		case "PING":
			peer.SendJSON(pongMsg{Type: "PONG"})
		default:
			peer.SendJSON(errorMsg{Type: "ERROR", Code: ErrCodeBadRequest, Message: "unknown message type"})
		}
	}
}

// doHello reads and validates the initial HELLO message.
// Returns the constructed Peer (not yet added to a room).
func (h *Handler) doHello(conn *websocket.Conn) (*Peer, error) {
	conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}

	var msg helloMsg
	if err := json.Unmarshal(raw, &msg); err != nil || msg.Type != "HELLO" {
		sendError(conn, ErrCodeBadRequest, "expected HELLO")
		return nil, err
	}

	if msg.RoomID == "" || msg.PeerID == "" || msg.Nonce == "" {
		sendError(conn, ErrCodeBadRequest, "missing required HELLO fields")
		return nil, errBadRequest("missing fields")
	}

	// Validate peerType: "user" (default), "relay", or reject.
	peerType := msg.PeerType
	if peerType == "" {
		peerType = PeerTypeUser
	}
	if peerType != PeerTypeUser && peerType != PeerTypeRelay {
		sendError(conn, ErrCodeBadRequest, "invalid peerType")
		return nil, errBadRequest("invalid peerType")
	}

	// Validate peerId is a UUIDv4.
	if _, err := uuid.Parse(msg.PeerID); err != nil {
		sendError(conn, ErrCodeBadRequest, "peerId must be a UUIDv4")
		return nil, errBadRequest("invalid peerId")
	}

	// Nonce replay check.
	if !h.nonces.Seen(msg.Nonce) {
		sendError(conn, ErrCodeNonceReplay, "nonce already seen")
		return nil, errBadRequest("nonce replay")
	}

	// License: room limit check (only if this HELLO would create a new room).
	if h.rooms.Get(msg.RoomID) == nil {
		if err := h.validator.CheckRoomLimit(h.rooms.Len()); err != nil {
			errCode, errMsg := licenseErrCode(err)
			sendError(conn, errCode, errMsg)
			return nil, errBadRequest("room limit exceeded")
		}
	}

	// Room capacity check.
	rm := h.rooms.GetOrCreate(msg.RoomID)
	if rm.Len() >= room.MaxPeersPerRoom {
		sendError(conn, ErrCodeRoomFull, "room is full")
		return nil, errBadRequest("room full")
	}

	// License: peer limit check.
	if err := h.validator.CheckPeerLimit(rm.Len()); err != nil {
		errCode, errMsg := licenseErrCode(err)
		sendError(conn, errCode, errMsg)
		return nil, errBadRequest("peer limit exceeded")
	}

	// PeerID uniqueness check.
	if rm.HasPeer(msg.PeerID) {
		sendError(conn, ErrCodeDuplicatePeerID, "peerId already in room")
		return nil, errBadRequest("duplicate peerId")
	}

	peer := &Peer{
		peerID:   msg.PeerID,
		roomID:   msg.RoomID,
		peerType: peerType,
		region:   msg.Region,
		conn:     conn,
		send:     make(chan []byte, sendQueueSize),
		done:     make(chan struct{}),
	}

	if !rm.AddPeer(peer) {
		// Race: another goroutine added the same peerId between HasPeer and AddPeer.
		sendError(conn, ErrCodeDuplicatePeerID, "peerId already in room")
		return nil, errBadRequest("duplicate peerId race")
	}

	slog.Info("peer joined",
		"room", hashID(peer.roomID),
		"peer", hashID(peer.peerID),
	)
	return peer, nil
}

// handleRelay processes a RELAY message. Returns false if the connection must be closed.
//
// Per PROTOCOL.md: server MUST close the connection if payload exceeds 64 KB.
func (h *Handler) handleRelay(from *Peer, rm *room.Room, raw []byte) bool {
	var msg relayMsg
	if err := json.Unmarshal(raw, &msg); err != nil || msg.Payload == "" {
		from.SendJSON(errorMsg{Type: "ERROR", Code: ErrCodeBadRequest, Message: "invalid RELAY"})
		return true
	}
	if msg.RoomID != from.roomID {
		from.SendJSON(errorMsg{Type: "ERROR", Code: ErrCodeBadRequest, Message: "roomId mismatch"})
		return true
	}

	// Decode payload to enforce the 64 KB binary limit precisely.
	decoded, err := decodeBase64(msg.Payload)
	if err != nil {
		from.SendJSON(errorMsg{Type: "ERROR", Code: ErrCodeBadRequest, Message: "payload is not valid base64"})
		return true
	}
	if len(decoded) > relay.MaxBlobSize {
		// PROTOCOL.md: reject and close connection when max blob size exceeded.
		// Send the error first; cleanup (deferred in Serve) will drain the send
		// queue via writePump before closing the websocket.
		from.SendJSON(errorMsg{Type: "ERROR", Code: ErrCodeBadRequest, Message: "payload too large"})
		return false
	}

	deliver := relayDeliverMsg{
		Type:       "RELAY_DELIVER",
		FromPeerID: from.peerID,
		Payload:    msg.Payload,
	}
	h.broadcastExcept(rm, from.peerID, deliver)
	return true
}

func (h *Handler) handleICE(from *Peer, rm *room.Room, msgType string, raw []byte) {
	var msg iceMsg
	if err := json.Unmarshal(raw, &msg); err != nil || msg.TargetPeerID == "" {
		from.SendJSON(errorMsg{Type: "ERROR", Code: ErrCodeBadRequest, Message: "invalid ICE message"})
		return
	}
	if msg.RoomID != from.roomID {
		from.SendJSON(errorMsg{Type: "ERROR", Code: ErrCodeBadRequest, Message: "roomId mismatch"})
		return
	}

	target := h.peerByID(rm, msg.TargetPeerID)
	if target == nil {
		// Target not in room — silently drop (peer may have left).
		return
	}

	target.SendJSON(iceForwardMsg{
		Type:       msgType,
		FromPeerID: from.peerID,
		RoomID:     from.roomID,
		Payload:    msg.Payload,
	})
}

func (h *Handler) cleanup(p *Peer, rm *room.Room) {
	wasRelay := p.peerType == PeerTypeRelay
	rm.RemovePeer(p.peerID)
	close(p.done)

	slog.Info("peer left",
		"room", hashID(p.roomID),
		"peer", hashID(p.peerID),
	)

	h.broadcastExcept(rm, p.peerID, peerLeftMsg{
		Type:   "PEER_LEFT",
		PeerID: p.peerID,
	})

	// If a relay node left, broadcast updated RELAY_NODES to remaining peers.
	if wasRelay {
		h.broadcastExcept(rm, p.peerID, h.buildRelayNodes(rm))
	}
}

// buildRelayNodes constructs a RELAY_NODES message containing only relay-type
// peers currently in the room. User peers are never included.
func (h *Handler) buildRelayNodes(rm *room.Room) relayNodesMsg {
	peers := rm.Peers()
	relays := make([]relayPeerInfo, 0)
	for _, p := range peers {
		peer := p.(*Peer)
		if peer.peerType == PeerTypeRelay {
			relays = append(relays, relayPeerInfo{
				PeerID: peer.peerID,
				Region: peer.region,
			})
		}
	}
	return relayNodesMsg{Type: "RELAY_NODES", Peers: relays}
}

// broadcastExcept sends v (as JSON) to all peers in rm except excludePeerID.
func (h *Handler) broadcastExcept(rm *room.Room, excludePeerID string, v any) {
	for _, p := range rm.Peers() {
		if p.ID() != excludePeerID {
			p.(*Peer).SendJSON(v)
		}
	}
}

// peerByID looks up a Peer by ID within a room.
func (h *Handler) peerByID(rm *room.Room, peerID string) *Peer {
	for _, p := range rm.Peers() {
		if p.ID() == peerID {
			return p.(*Peer)
		}
	}
	return nil
}

// licenseErrCode maps an auth sentinel error to a wire error code and message.
func licenseErrCode(err error) (code, message string) {
	switch err {
	case auth.ErrLicenseExpired:
		return ErrCodeLicenseExpired, "license expired"
	case auth.ErrRoomLimitExceeded:
		return ErrCodeRoomLimitExceeded, "room limit exceeded for your license tier"
	case auth.ErrPeerLimitExceeded:
		return ErrCodePeerLimitExceeded, "peer limit exceeded for your license tier"
	default:
		return ErrCodeBadRequest, "license error"
	}
}

// sendError writes an ERROR message directly on conn (before Peer is created).
func sendError(conn *websocket.Conn, code, message string) {
	b, _ := json.Marshal(errorMsg{Type: "ERROR", Code: code, Message: message})
	conn.WriteMessage(websocket.TextMessage, b) //nolint:errcheck
}

// decodeBase64 decodes standard or URL-safe base64 (with or without padding).
func decodeBase64(s string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		b, err = base64.URLEncoding.DecodeString(s)
	}
	if err != nil {
		b, err = base64.RawStdEncoding.DecodeString(s)
	}
	return b, err
}

type handlerError string

func errBadRequest(s string) handlerError { return handlerError(s) }
func (e handlerError) Error() string      { return string(e) }
