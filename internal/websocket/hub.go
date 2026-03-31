package websocket

import (
	"encoding/json"
	"log"
	"os"
	"strings"
	"sync"
	"time"

)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

// Hub manages connected clients and broadcasts events.
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	quit       chan struct{}
	mu         sync.RWMutex
}

// NewHub creates a new Hub.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		quit:       make(chan struct{}),
	}
}

// Run starts the hub event loop. Call Stop() to shut it down.
func (h *Hub) Run() {
	for {
		select {
		case <-h.quit:
			return

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("[WS] Client connected (total: %d)", h.ClientCount())

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			log.Printf("[WS] Client disconnected (total: %d)", h.ClientCount())

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Stop shuts down the hub gracefully.
func (h *Hub) Stop() {
	h.mu.Lock()
	select {
	case <-h.quit:
		// Already stopped
	default:
		close(h.quit)
	}
	h.mu.Unlock()

	// Drain all clients
	h.mu.Lock()
	for client := range h.clients {
		close(client.send)
		delete(h.clients, client)
	}
	h.mu.Unlock()
}

// Broadcast sends raw bytes to all connected clients.
func (h *Hub) Broadcast(event []byte) {
	h.broadcast <- event
}

// Register adds a client to the hub.
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Unregister removes a client from the hub.
func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// TaskEvent is broadcast when a task status changes.
type TaskEvent struct {
	TaskID     string    `json:"task_id"`
	FromStatus string    `json:"from_status"`
	ToStatus   string    `json:"to_status"`
	Agent      string    `json:"agent"`
	Timestamp  time.Time `json:"timestamp"`
}

// Bytes serializes the TaskEvent to JSON.
func (t *TaskEvent) Bytes() ([]byte, error) {
	return json.Marshal(t)
}

// BroadcastTaskEvent creates and broadcasts a TaskEvent from raw fields.
// Avoids an extra DB query on every broadcast.
func (h *Hub) BroadcastTaskEvent(taskID, fromStatus, toStatus, agent string) {
	te := TaskEvent{
		TaskID:     taskID,
		FromStatus: fromStatus,
		ToStatus:   toStatus,
		Agent:      agent,
		Timestamp:  time.Now(),
	}
	data, _ := json.Marshal(te)
	h.Broadcast(data)
}

// isOriginAllowed checks if the request origin is allowed by CORS_ALLOWED_ORIGINS.
func isOriginAllowed(origin string) bool {
	if origin == "" {
		return true // no origin header = not a browser
	}
	allowed := os.Getenv("CORS_ALLOWED_ORIGINS")
	if allowed == "" || allowed == "*" {
		return true
	}
	for _, o := range strings.Split(allowed, ",") {
		o = strings.TrimSpace(o)
		if o == origin {
			return true
		}
		// Support wildcard subdomain: https://example.com matches https://app.example.com
		if strings.HasPrefix(o, "https://") {
			root := o[strings.Index(o, "//")+2:]
			if strings.HasSuffix(origin, root) {
				return true
			}
		}
	}
	return false
}
