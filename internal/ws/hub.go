package ws

import (
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"dashboard/internal/types"
)

// Message is the envelope pushed to browsers over the socket.
type Message struct {
	Type string `json:"type"` // snapshot | reload | ping
	Data any    `json:"data"`
}

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 30 * time.Second
	maxMsgSize = 1 << 20
)

type client struct {
	conn *websocket.Conn
	send chan Message
}

// Hub keeps the set of connected clients, remembers the latest snapshot so new
// clients immediately get current state, and broadcasts updates.
type Hub struct {
	mu      sync.RWMutex
	clients map[*client]struct{}
	last    *types.Snapshot
}

func New() *Hub {
	return &Hub{clients: map[*client]struct{}{}}
}

// Broadcast stores and pushes a full snapshot to all clients.
func (h *Hub) Broadcast(snap *types.Snapshot) {
	h.mu.Lock()
	h.last = snap
	clients := make([]*client, 0, len(h.clients))
	for cl := range h.clients {
		clients = append(clients, cl)
	}
	h.mu.Unlock()
	msg := Message{Type: "snapshot", Data: snap}
	for _, cl := range clients {
		h.enqueue(cl, msg)
	}
}

// Publish sends an arbitrary message (e.g. config reload events) to all clients.
func (h *Hub) Publish(msg Message) {
	h.mu.RLock()
	clients := make([]*client, 0, len(h.clients))
	for cl := range h.clients {
		clients = append(clients, cl)
	}
	h.mu.RUnlock()
	for _, cl := range clients {
		h.enqueue(cl, msg)
	}
}

func (h *Hub) LastSnapshot() *types.Snapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.last
}

func (h *Hub) enqueue(cl *client, msg Message) {
	select {
	case cl.send <- msg:
	default:
		// slow consumer: drop
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin:     func(*http.Request) bool { return true },
}

// Handle upgrades an echo request to a WebSocket connection.
func (h *Hub) Handle(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	cl := &client{conn: conn, send: make(chan Message, 64)}

	h.mu.Lock()
	h.clients[cl] = struct{}{}
	last := h.last
	h.mu.Unlock()

	if last != nil {
		cl.send <- Message{Type: "snapshot", Data: last}
	}

	go h.writePump(cl)
	go h.readPump(cl)
}

func (h *Hub) readPump(cl *client) {
	defer func() {
		h.mu.Lock()
		delete(h.clients, cl)
		h.mu.Unlock()
		cl.conn.Close()
		close(cl.send)
	}()
	cl.conn.SetReadLimit(maxMsgSize)
	_ = cl.conn.SetReadDeadline(time.Now().Add(pongWait))
	cl.conn.SetPongHandler(func(string) error {
		return cl.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		if _, _, err := cl.conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (h *Hub) writePump(cl *client) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		cl.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-cl.send:
			_ = cl.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = cl.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := cl.conn.WriteJSON(msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = cl.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := cl.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
