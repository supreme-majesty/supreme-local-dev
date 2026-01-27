package api

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/supreme-majesty/supreme-local-dev/pkg/daemon"
	"github.com/supreme-majesty/supreme-local-dev/pkg/events"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // In dev, allow all. In prod, we can restrict to sld.test
	},
}

type Hub struct {
	clients    map[*websocket.Conn]bool
	broadcast  chan interface{}
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	mutex      sync.Mutex
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan interface{}),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mutex.Lock()
			h.clients[client] = true
			h.mutex.Unlock()
			fmt.Println("WS: Client connected")

		case client := <-h.unregister:
			h.mutex.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.Close()
			}
			h.mutex.Unlock()
			fmt.Println("WS: Client disconnected")

		case message := <-h.broadcast:
			h.mutex.Lock()
			for client := range h.clients {
				err := client.WriteJSON(message)
				if err != nil {
					fmt.Printf("WS: Write error: %v\n", err)
					client.Close()
					delete(h.clients, client)
				}
			}
			h.mutex.Unlock()
		}
	}
}

func (s *Server) handleWebSocket(hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			fmt.Printf("WS: Upgrade error: %v\n", err)
			return
		}

		hub.register <- conn

		// Listen for close
		go func() {
			defer func() {
				hub.unregister <- conn
			}()
			for {
				_, _, err := conn.ReadMessage()
				if err != nil {
					break
				}
			}
		}()
	}
}

// SetupEventBridge connects the EventBus to the WebSocket Hub
func SetupEventBridge(hub *Hub) {
	d, err := daemon.GetClient()
	if err != nil {
		fmt.Printf("EventBridge: Failed to get daemon client: %v\n", err)
		return
	}

	// Subscribe to X-Ray logs
	d.Events.Subscribe(events.XRayLog, func(e events.Event) {
		hub.broadcast <- map[string]interface{}{
			"type": "xray:log",
			"data": e.Payload,
		}
	})

	// Subscribe to Sites updates
	d.Events.Subscribe(events.SitesUpdated, func(e events.Event) {
		hub.broadcast <- map[string]interface{}{
			"type": "sites:updated",
			"data": e.Payload,
		}
	})

	// Subscribe to Log entries
	d.Events.Subscribe(events.LogEntry, func(e events.Event) {
		hub.broadcast <- map[string]interface{}{
			"type": "log:entry",
			"data": e.Payload,
		}
	})

	// Subscribe to Artisan output
	d.Events.Subscribe(events.ArtisanOutput, func(e events.Event) {
		hub.broadcast <- map[string]interface{}{
			"type": "artisan:output",
			"data": e.Payload,
		}
	})

	// Subscribe to Artisan command completion
	d.Events.Subscribe(events.ArtisanDone, func(e events.Event) {
		hub.broadcast <- map[string]interface{}{
			"type": "artisan:done",
			"data": e.Payload,
		}
	})
}
