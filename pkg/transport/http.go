package transport

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"mcp-workspace-manager/pkg/mcp"
	"net/http"
	"sync"

	"github.com/google/uuid"
)

// sseClient represents a connected SSE client.
type sseClient struct {
	id      string
	send    chan []byte // Channel to send data to the client.
	flusher http.Flusher
}

// ConnectionManager manages all active SSE client connections.
type ConnectionManager struct {
	clients map[string]*sseClient
	mu      sync.RWMutex
	handler ToolHandler
}

// NewConnectionManager creates a new connection manager.
func NewConnectionManager(handler ToolHandler) *ConnectionManager {
	return &ConnectionManager{
		clients: make(map[string]*sseClient),
		handler: handler,
	}
}

// registerClient adds a new client to the manager.
func (cm *ConnectionManager) registerClient(w http.ResponseWriter) *sseClient {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil // Should be checked before calling
	}

	client := &sseClient{
		id:      uuid.NewString(),
		send:    make(chan []byte, 256),
		flusher: flusher,
	}
	cm.clients[client.id] = client

	slog.Info("SSE client registered", "clientId", client.id)
	return client
}

// unregisterClient removes a client from the manager.
func (cm *ConnectionManager) unregisterClient(id string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if client, ok := cm.clients[id]; ok {
		close(client.send)
		delete(cm.clients, id)
		slog.Info("SSE client unregistered", "clientId", id)
	}
}

// dispatchCommand sends a command to the appropriate client.
func (cm *ConnectionManager) dispatchCommand(req *mcp.Request, clientId string) {
	cm.mu.RLock()
	client, ok := cm.clients[clientId]
	cm.mu.RUnlock()

	if !ok {
		slog.Warn("Attempted to dispatch command to non-existent client", "clientId", clientId)
		return
	}

	// Run the tool handler to get the response
	response := cm.handler(req)

	// Marshal the response to JSON
	responseBytes, err := json.Marshal(response)
	if err != nil {
		slog.Error("Failed to marshal response for SSE client", "error", err, "clientId", clientId)
		return
	}

	// Send the response to the client's channel
	client.send <- responseBytes
}

// RunHTTP starts the MCP server over HTTP with Server-Sent Events (SSE).
func RunHTTP(port int, handler ToolHandler) {
	slog.Info("Starting HTTP server", "port", port)

	connManager := NewConnectionManager(handler)

	http.HandleFunc("/mcp/stream", connManager.sseStreamHandler)
	http.HandleFunc("/mcp/command", connManager.commandHandler)
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	addr := fmt.Sprintf(":%d", port)
	if err := http.ListenAndServe(addr, nil); err != nil {
		slog.Error("HTTP server failed", "error", err)
	}
}

// sseStreamHandler handles the long-lived SSE connection.
func (cm *ConnectionManager) sseStreamHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	client := cm.registerClient(w)
	if client == nil {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}
	defer cm.unregisterClient(client.id)

	// Send the client its ID
	sendSSEEvent(w, "connection_ready", map[string]string{"clientId": client.id})

	for {
		select {
		case <-r.Context().Done():
			return // Client disconnected
		case message, ok := <-client.send:
			if !ok {
				return // Channel was closed
			}
			fmt.Fprintf(w, "data: %s\n\n", message)
			client.flusher.Flush()
		}
	}
}

// commandHandler receives MCP commands from clients.
func (cm *ConnectionManager) commandHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	clientId := r.Header.Get("X-MCP-Client-ID")
	if clientId == "" {
		http.Error(w, "X-MCP-Client-ID header is required", http.StatusBadRequest)
		return
	}

	var req mcp.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON request body", http.StatusBadRequest)
		return
	}

	go cm.dispatchCommand(&req, clientId)

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte("{\"status\": \"command accepted\"}"))
}

// sendSSEEvent sends a properly formatted SSE event.
func sendSSEEvent(w http.ResponseWriter, event string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "event: %s\n", event)
	fmt.Fprintf(w, "data: %s\n\n", string(jsonData))
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}