package signaling

import (
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/gorilla/websocket"
)

const sendQueueSize = 256

// Peer represents one connected WebSocket client.
//
// Spec:
//   - Each Peer has a unique peerID (UUIDv4, client-generated) and a roomID.
//   - peerType is "user" (default) or "relay". Relay peers forward encrypted
//     blobs without possessing roomKey. Clients skip mutual auth handshake
//     for relay peers.
//   - region is set only for relay peers (e.g. "eu-de"); empty for user peers.
//   - Outgoing messages are serialized through a buffered channel (send queue).
//   - If the send queue is full, the message is dropped and a warning is logged.
//   - Send is safe for concurrent use; sending after Close is a no-op (not a panic).
//   - Close closes the underlying WebSocket connection and drains the send queue.
type Peer struct {
	peerID   string
	roomID   string
	peerType string // "user" | "relay"
	region   string // relay region tag, e.g. "eu-de"; empty for user peers
	conn     *websocket.Conn
	send     chan []byte
	done     chan struct{} // closed when peer is shutting down
	once     sync.Once
}

// ID returns the peer's identifier.
func (p *Peer) ID() string { return p.peerID }

// RoomID returns the room this peer belongs to.
func (p *Peer) RoomID() string { return p.roomID }

// PeerType returns the peer type: "user" or "relay".
func (p *Peer) PeerType() string { return p.peerType }

// Region returns the relay region tag. Empty for user peers.
func (p *Peer) Region() string { return p.region }

// Send enqueues msg for delivery. Drops msg (with a warning) if queue is full.
// Safe to call after the peer has been closed (no-op).
func (p *Peer) Send(msg []byte) {
	select {
	case <-p.done:
		// Peer is shutting down; drop silently.
		return
	default:
	}
	select {
	case p.send <- msg:
	case <-p.done:
	default:
		slog.Warn("peer send queue full, dropping message", "peer", hashID(p.peerID))
	}
}

// SendJSON marshals v and enqueues the result.
func (p *Peer) SendJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		slog.Warn("SendJSON marshal error", "err", err)
		return
	}
	p.Send(b)
}

// Close closes the WebSocket connection.
func (p *Peer) Close() {
	p.once.Do(func() {
		p.conn.Close()
	})
}

// writePump drains the send queue and writes to the WebSocket connection.
// Runs in its own goroutine. Returns when p.done is closed or a write fails.
func (p *Peer) writePump() {
	defer p.Close()
	for {
		select {
		case msg := <-p.send:
			if err := p.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				slog.Debug("peer write error", "peer", hashID(p.peerID), "err", err)
				return
			}
		case <-p.done:
			return
		}
	}
}
