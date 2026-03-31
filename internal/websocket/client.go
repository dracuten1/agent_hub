package websocket

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// CheckOrigin validates the Origin header against CORS_ALLOWED_ORIGINS env var.
var CheckOrigin = func(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	return isOriginAllowed(origin)
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:    CheckOrigin,
}

// Server wraps the hub with a Gin-compatible handler.
type Server struct {
	hub *Hub
}

// NewServer creates a new WebSocket server.
func NewServer(hub *Hub) *Server {
	return &Server{hub: hub}
}

// NewHandler is an alias for NewServer.
func NewHandler(hub *Hub) *Server {
	return NewServer(hub)
}

// HandleWS upgrades HTTP to WebSocket after validating the token.
func (s *Server) HandleWS(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(401, gin.H{"error": "token required"})
		return
	}
	if !validateToken(token) {
		c.JSON(401, gin.H{"error": "invalid token"})
		return
	}

	userID, agent := parseToken(token)

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := NewClient(s.hub, conn, userID, agent)
	s.hub.Register(client)

	go client.WritePump()
	go client.ReadPump()
}

// parseToken extracts userID and agent from token ("userID:agent" format).
func parseToken(token string) (userID, agent string) {
	if token == "" {
		return "", ""
	}
	parts := strings.SplitN(token, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return token, ""
}

// validateToken validates the JWT token.
// TODO: implement actual JWT validation with signature + expiry check.
func validateToken(token string) bool {
	return token != "" && len(token) >= 8
}

// Client represents a single WebSocket client.
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	userID string
	agent  string
}

// NewClient creates a new client.
func NewClient(hub *Hub, conn *websocket.Conn, userID, agent string) *Client {
	return &Client{
		hub:    hub,
		conn:   conn,
		send:   make(chan []byte, 256),
		userID: userID,
		agent:  agent,
	}
}

// WritePump pumps messages from the hub to the WebSocket connection.
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ReadPump pumps messages from the WebSocket connection to the hub.
func (c *Client) ReadPump() {
	defer func() {
		c.hub.Unregister(c)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
	}
}
