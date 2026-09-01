package ws

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

type Client struct {
	conn *websocket.Conn
	send chan []byte
}

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

func NewClient(conn *websocket.Conn) *Client {
	return &Client{
		conn: conn,
		send: make(chan []byte, 16),
	}
}

func (c *Client) writePump() {
	defer c.conn.Close()
	for data := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
			return
		}
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}

		case data := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.send <- data:
				default:
				}
			}
		}
	}
}

func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] WS upgrade failed for %s: %v", r.RemoteAddr, err)
		return
	}

	client := NewClient(conn)
	h.register <- client

	go client.writePump()

	defer func() {
		h.unregister <- client
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

func (h *Hub) BroadcastUpdate() {
	select {
	case h.broadcast <- []byte(`{"event":"refresh"}`):
	default:
	}
}

func (h *Hub) BroadcastEvent(event string, payload map[string]interface{}) {
	msg := map[string]interface{}{"event": event}
	for k, v := range payload {
		msg[k] = v
	}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[WS] BroadcastEvent marshal error for event %s: %v", event, err)
		return
	}
	select {
	case h.broadcast <- data:
	default:
	}
}
