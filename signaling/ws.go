package signaling

import (
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	// Accept all origins — clients connect from browsers and native apps.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Upgrade upgrades r to a WebSocket connection.
func Upgrade(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	return upgrader.Upgrade(w, r, nil)
}
