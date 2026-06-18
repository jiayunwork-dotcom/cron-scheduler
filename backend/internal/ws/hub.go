package ws

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	MessageTypeStatus  = "status"
	MessageTypeLog     = "log"
	MessageTypeHeartbeat = "heartbeat"
)

type ClientMessage struct {
	Action   string `json:"action"`
	TaskName string `json:"task_name"`
}

type ServerMessage struct {
	Type      string      `json:"type"`
	TaskName  string      `json:"task_name"`
	Timestamp int64       `json:"timestamp"`
	Data      interface{} `json:"data"`
}

type StatusData struct {
	Status     string `json:"status"`
	OldStatus  string `json:"old_status,omitempty"`
	ExecutionID string `json:"execution_id"`
}

type LogData struct {
	Stream string `json:"stream"`
	Line   string `json:"line"`
}

type Client struct {
	conn     *websocket.Conn
	send     chan []byte
	hub      *Hub
	tasks    map[string]bool
	tasksMu  sync.Mutex
}

type Hub struct {
	clients    map[*Client]bool
	taskSubs   map[string]map[*Client]bool
	broadcast  chan ServerMessage
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		taskSubs:   make(map[string]map[*Client]bool),
		broadcast:  make(chan ServerMessage, 100),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

func (h *Hub) Run() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.tasksMu.Lock()
				for taskName := range client.tasks {
					if subs, ok := h.taskSubs[taskName]; ok {
						delete(subs, client)
						if len(subs) == 0 {
							delete(h.taskSubs, taskName)
						}
					}
				}
				client.tasksMu.Unlock()
				close(client.send)
			}
			h.mu.Unlock()

		case msg := <-h.broadcast:
			h.mu.RLock()
			if subs, ok := h.taskSubs[msg.TaskName]; ok {
				data, _ := json.Marshal(msg)
				for client := range subs {
					select {
					case client.send <- data:
					default:
						close(client.send)
						delete(subs, client)
						if len(subs) == 0 {
							delete(h.taskSubs, taskName)
						}
					}
				}
			}
			h.mu.RUnlock()

		case <-ticker.C:
			h.mu.RLock()
			heartbeat := ServerMessage{
				Type:      MessageTypeHeartbeat,
				Timestamp: time.Now().Unix(),
			}
			data, _ := json.Marshal(heartbeat)
			for client := range h.clients {
				select {
				case client.send <- data:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *Hub) Subscribe(client *Client, taskName string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	client.tasksMu.Lock()
	client.tasks[taskName] = true
	client.tasksMu.Unlock()

	if _, ok := h.taskSubs[taskName]; !ok {
		h.taskSubs[taskName] = make(map[*Client]bool)
	}
	h.taskSubs[taskName][client] = true
}

func (h *Hub) Unsubscribe(client *Client, taskName string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	client.tasksMu.Lock()
	delete(client.tasks, taskName)
	client.tasksMu.Unlock()

	if subs, ok := h.taskSubs[taskName]; ok {
		delete(subs, client)
		if len(subs) == 0 {
			delete(h.taskSubs, taskName)
		}
	}
}

func (h *Hub) UnsubscribeAll(taskName string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if subs, ok := h.taskSubs[taskName]; ok {
		for client := range subs {
			client.tasksMu.Lock()
			delete(client.tasks, taskName)
			client.tasksMu.Unlock()
		}
		delete(h.taskSubs, taskName)
	}
}

func (h *Hub) BroadcastStatus(taskName, executionID, oldStatus, newStatus string) {
	msg := ServerMessage{
		Type:      MessageTypeStatus,
		TaskName:  taskName,
		Timestamp: time.Now().Unix(),
		Data: StatusData{
			Status:     newStatus,
			OldStatus:  oldStatus,
			ExecutionID: executionID,
		},
	}
	h.broadcast <- msg
}

func (h *Hub) BroadcastLog(taskName, stream, line string) {
	msg := ServerMessage{
		Type:      MessageTypeLog,
		TaskName:  taskName,
		Timestamp: time.Now().Unix(),
		Data: LogData{
			Stream: stream,
			Line:   line,
		},
	}
	h.broadcast <- msg
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("websocket error: %v", err)
			}
			break
		}

		var clientMsg ClientMessage
		if err := json.Unmarshal(message, &clientMsg); err != nil {
			log.Printf("parse message error: %v", err)
			continue
		}

		switch clientMsg.Action {
		case "subscribe":
			if clientMsg.TaskName != "" {
				c.hub.Subscribe(c, clientMsg.TaskName)
			}
		case "unsubscribe":
			if clientMsg.TaskName != "" {
				c.hub.Unsubscribe(c, clientMsg.TaskName)
			}
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
