// Package ws provides WebSocket connection management for WonderTwin twins.
// It handles connection registration, broadcast, and heartbeat without
// depending on a specific WebSocket library. Twins provide the actual
// WebSocket upgrade and read/write implementation via the Conn interface.
package ws

import (
	"encoding/json"
	"sync"
	"time"
)

// Conn represents a WebSocket connection. Twins implement this interface
// using their chosen WebSocket library.
type Conn interface {
	// ID returns a unique identifier for this connection.
	ID() string
	// Send sends a message to the client. Returns error if connection is closed.
	Send(data []byte) error
	// Close closes the connection.
	Close() error
}

// Handler manages WebSocket connections and broadcasts.
type Handler struct {
	mu           sync.RWMutex
	connections  map[string]Conn
	groups       map[string]map[string]bool // group → set of conn IDs
	onConnect    func(Conn)
	onMessage    func(Conn, []byte)
	onDisconnect func(Conn)
}

// Config configures the WebSocket handler.
type Config struct {
	OnConnect    func(Conn)
	OnMessage    func(Conn, []byte)
	OnDisconnect func(Conn)
}

// NewHandler creates a new WebSocket handler.
func NewHandler(cfg Config) *Handler {
	return &Handler{
		connections:  make(map[string]Conn),
		groups:       make(map[string]map[string]bool),
		onConnect:    cfg.OnConnect,
		onMessage:    cfg.OnMessage,
		onDisconnect: cfg.OnDisconnect,
	}
}

// Register adds a connection to the handler. Call this after the WebSocket
// upgrade is complete.
func (h *Handler) Register(conn Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.connections[conn.ID()] = conn
	if h.onConnect != nil {
		h.onConnect(conn)
	}
}

// Unregister removes a connection from the handler.
func (h *Handler) Unregister(connID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	conn, ok := h.connections[connID]
	if !ok {
		return
	}
	delete(h.connections, connID)

	// Remove from all groups.
	for group, members := range h.groups {
		delete(members, connID)
		if len(members) == 0 {
			delete(h.groups, group)
		}
	}

	if h.onDisconnect != nil {
		h.onDisconnect(conn)
	}
}

// HandleMessage processes an incoming message from a connection.
// Call this from the twin's WebSocket read loop.
func (h *Handler) HandleMessage(conn Conn, data []byte) {
	if h.onMessage != nil {
		h.onMessage(conn, data)
	}
}

// JoinGroup adds a connection to a named group (e.g., a channel).
func (h *Handler) JoinGroup(connID, group string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.groups[group] == nil {
		h.groups[group] = make(map[string]bool)
	}
	h.groups[group][connID] = true
}

// LeaveGroup removes a connection from a named group.
func (h *Handler) LeaveGroup(connID, group string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if members, ok := h.groups[group]; ok {
		delete(members, connID)
		if len(members) == 0 {
			delete(h.groups, group)
		}
	}
}

// Broadcast sends a message to all connected clients.
func (h *Handler) Broadcast(data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, conn := range h.connections {
		conn.Send(data)
	}
}

// BroadcastJSON marshals v to JSON and broadcasts to all connections.
func (h *Handler) BroadcastJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	h.Broadcast(data)
	return nil
}

// BroadcastToGroup sends a message to all connections in a named group.
func (h *Handler) BroadcastToGroup(group string, data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	members, ok := h.groups[group]
	if !ok {
		return
	}
	for connID := range members {
		if conn, ok := h.connections[connID]; ok {
			conn.Send(data)
		}
	}
}

// BroadcastToGroupJSON marshals v and sends to a group.
func (h *Handler) BroadcastToGroupJSON(group string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	h.BroadcastToGroup(group, data)
	return nil
}

// SendTo sends a message to a specific connection by ID.
func (h *Handler) SendTo(connID string, data []byte) error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conn, ok := h.connections[connID]
	if !ok {
		return ErrConnectionNotFound
	}
	return conn.Send(data)
}

// SendToJSON marshals v and sends to a specific connection.
func (h *Handler) SendToJSON(connID string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return h.SendTo(connID, data)
}

// ConnectionCount returns the number of active connections.
func (h *Handler) ConnectionCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.connections)
}

// GroupMembers returns the connection IDs in a group.
func (h *Handler) GroupMembers(group string) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	members, ok := h.groups[group]
	if !ok {
		return nil
	}
	result := make([]string, 0, len(members))
	for id := range members {
		result = append(result, id)
	}
	return result
}

// ListGroups returns all group names.
func (h *Handler) ListGroups() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := make([]string, 0, len(h.groups))
	for name := range h.groups {
		result = append(result, name)
	}
	return result
}

// Heartbeat sends a ping message to all connections at the given interval.
// Connections that fail to receive the ping are unregistered.
// Returns a stop function.
func (h *Handler) Heartbeat(interval time.Duration, pingData []byte) func() {
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				h.mu.RLock()
				var failed []string
				for id, conn := range h.connections {
					if err := conn.Send(pingData); err != nil {
						failed = append(failed, id)
					}
				}
				h.mu.RUnlock()
				for _, id := range failed {
					h.Unregister(id)
				}
			case <-stop:
				return
			}
		}
	}()
	return func() { close(stop) }
}

// ErrConnectionNotFound is returned when a connection ID is not found.
var ErrConnectionNotFound = &ConnectionError{Message: "connection not found"}

// ConnectionError represents a WebSocket connection error.
type ConnectionError struct {
	Message string
}

func (e *ConnectionError) Error() string {
	return e.Message
}
