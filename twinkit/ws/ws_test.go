package ws

import (
	"sync"
	"testing"
)

// mockConn implements the Conn interface for testing.
type mockConn struct {
	id       string
	messages [][]byte
	closed   bool
	mu       sync.Mutex
}

func (c *mockConn) ID() string { return c.id }
func (c *mockConn) Send(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return &ConnectionError{Message: "closed"}
	}
	c.messages = append(c.messages, data)
	return nil
}
func (c *mockConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func newMockConn(id string) *mockConn {
	return &mockConn{id: id}
}

func TestRegisterAndBroadcast(t *testing.T) {
	h := NewHandler(Config{})
	c1 := newMockConn("c1")
	c2 := newMockConn("c2")

	h.Register(c1)
	h.Register(c2)

	if h.ConnectionCount() != 2 {
		t.Errorf("count = %d, want 2", h.ConnectionCount())
	}

	h.Broadcast([]byte("hello"))

	if len(c1.messages) != 1 || string(c1.messages[0]) != "hello" {
		t.Error("c1 should have received 'hello'")
	}
	if len(c2.messages) != 1 {
		t.Error("c2 should have received 'hello'")
	}
}

func TestUnregister(t *testing.T) {
	h := NewHandler(Config{})
	c := newMockConn("c1")
	h.Register(c)
	h.Unregister("c1")

	if h.ConnectionCount() != 0 {
		t.Error("should be 0 after unregister")
	}

	h.Broadcast([]byte("nobody"))
	if len(c.messages) != 0 {
		t.Error("unregistered conn should not receive")
	}
}

func TestGroups(t *testing.T) {
	h := NewHandler(Config{})
	c1 := newMockConn("c1")
	c2 := newMockConn("c2")
	c3 := newMockConn("c3")

	h.Register(c1)
	h.Register(c2)
	h.Register(c3)

	h.JoinGroup("c1", "channel-1")
	h.JoinGroup("c2", "channel-1")
	h.JoinGroup("c3", "channel-2")

	h.BroadcastToGroup("channel-1", []byte("ch1 msg"))

	if len(c1.messages) != 1 {
		t.Error("c1 should receive channel-1 message")
	}
	if len(c2.messages) != 1 {
		t.Error("c2 should receive channel-1 message")
	}
	if len(c3.messages) != 0 {
		t.Error("c3 should NOT receive channel-1 message")
	}
}

func TestLeaveGroup(t *testing.T) {
	h := NewHandler(Config{})
	c := newMockConn("c1")
	h.Register(c)
	h.JoinGroup("c1", "room")

	h.LeaveGroup("c1", "room")

	h.BroadcastToGroup("room", []byte("missed"))
	if len(c.messages) != 0 {
		t.Error("should not receive after leaving group")
	}
}

func TestSendTo(t *testing.T) {
	h := NewHandler(Config{})
	c1 := newMockConn("c1")
	c2 := newMockConn("c2")
	h.Register(c1)
	h.Register(c2)

	h.SendTo("c1", []byte("direct"))

	if len(c1.messages) != 1 {
		t.Error("c1 should receive direct message")
	}
	if len(c2.messages) != 0 {
		t.Error("c2 should NOT receive direct message")
	}
}

func TestSendTo_NotFound(t *testing.T) {
	h := NewHandler(Config{})
	err := h.SendTo("nonexistent", []byte("hello"))
	if err == nil {
		t.Error("expected error for nonexistent connection")
	}
}

func TestCallbacks(t *testing.T) {
	var connected, disconnected int
	var lastMsg []byte

	h := NewHandler(Config{
		OnConnect:    func(c Conn) { connected++ },
		OnMessage:    func(c Conn, data []byte) { lastMsg = data },
		OnDisconnect: func(c Conn) { disconnected++ },
	})

	c := newMockConn("c1")
	h.Register(c)
	if connected != 1 {
		t.Errorf("connected = %d, want 1", connected)
	}

	h.HandleMessage(c, []byte("ping"))
	if string(lastMsg) != "ping" {
		t.Errorf("lastMsg = %q, want ping", lastMsg)
	}

	h.Unregister("c1")
	if disconnected != 1 {
		t.Errorf("disconnected = %d, want 1", disconnected)
	}
}

func TestUnregister_RemovesFromGroups(t *testing.T) {
	h := NewHandler(Config{})
	c := newMockConn("c1")
	h.Register(c)
	h.JoinGroup("c1", "room")

	h.Unregister("c1")

	members := h.GroupMembers("room")
	if len(members) != 0 {
		t.Error("unregistered conn should be removed from groups")
	}
}

func TestBroadcastJSON(t *testing.T) {
	h := NewHandler(Config{})
	c := newMockConn("c1")
	h.Register(c)

	h.BroadcastJSON(map[string]string{"type": "hello"})

	if len(c.messages) != 1 {
		t.Fatal("expected 1 message")
	}
	if string(c.messages[0]) != `{"type":"hello"}` {
		t.Errorf("msg = %q", string(c.messages[0]))
	}
}

func TestListGroups(t *testing.T) {
	h := NewHandler(Config{})
	c := newMockConn("c1")
	h.Register(c)
	h.JoinGroup("c1", "room-a")
	h.JoinGroup("c1", "room-b")

	groups := h.ListGroups()
	if len(groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(groups))
	}
}
