package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
)

// Socket Mode, as apis/events-api/using-socket-mode.md describes it: the app
// gets a WebSocket URL from apps.connections.open, connects to it, and is sent
// a hello before anything else.
const (
	// socketPath is where Socket Mode URLs point, as wss-primary.slack.com/link/
	// does on Slack.
	socketPath = "/link/"

	// approximateConnectionTime is the hello's estimate of how long, in
	// seconds, the connection lasts before Slack refreshes it. The guide's
	// example gives 3600.
	approximateConnectionTime = 3600

	// socketWriteTimeout bounds one frame written to an app that stopped reading.
	socketWriteTimeout = 10 * time.Second
)

// socketConn is one open Socket Mode connection.
type socketConn struct {
	id   string
	conn *websocket.Conn
	// done is cancelled when the connection closes, from either side.
	ctx  context.Context
	done context.CancelFunc
}

// sockets holds the tickets apps.connections.open has issued and the
// connections opened with them.
type sockets struct {
	mu      sync.Mutex
	tickets map[string]bool
	conns   map[string]*socketConn
}

func newSockets() *sockets {
	return &sockets{tickets: map[string]bool{}, conns: map[string]*socketConn{}}
}

// issueTicket returns a new ticket for a Socket Mode URL.
func (s *sockets) issueTicket() string {
	ticket := randomHex(16)
	s.mu.Lock()
	s.tickets[ticket] = true
	s.mu.Unlock()
	return ticket
}

// redeem uses up a ticket. Slack does not say whether a ticket can be used
// twice (socket-005); this app emulator accepts each one once, and every SDK
// asks for a new URL before it reconnects.
func (s *sockets) redeem(ticket string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.tickets[ticket] {
		return false
	}
	delete(s.tickets, ticket)
	return true
}

func (s *sockets) add(c *socketConn) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conns[c.id] = c
	return len(s.conns)
}

func (s *sockets) remove(c *socketConn) {
	s.mu.Lock()
	delete(s.conns, c.id)
	s.mu.Unlock()
}

// reset forgets every ticket and closes every connection, as POST /admin/reset
// does for the rest of the app emulator. Each app reconnects through
// apps.connections.open, as it does after any close from Slack.
func (s *sockets) reset() {
	s.mu.Lock()
	conns := s.conns
	s.tickets = map[string]bool{}
	s.conns = map[string]*socketConn{}
	s.mu.Unlock()
	// A close waits for the app's reply, so the reset does not: a test
	// harness often resets before it reads from the socket again.
	for _, c := range conns {
		go func() {
			c.conn.Close(websocket.StatusNormalClosure, "")
			c.done()
		}()
	}
}

// SocketConnect serves the WebSocket a Socket Mode URL points at. The ticket
// in the URL is the credential; one not issued, or already used, is refused
// before the upgrade.
func (h *Handler) SocketConnect(w http.ResponseWriter, r *http.Request) {
	if !h.sockets.redeem(r.URL.Query().Get("ticket")) {
		http.Error(w, "invalid or used ticket", http.StatusUnauthorized)
		return
	}
	// Compression is declined: no SDK client needs it, and some only accept
	// a response that names permessage-deflate exactly.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode:    websocket.CompressionDisabled,
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	// Envelopes carry whole events; the SDKs keep each one under 1 MiB, and
	// acks are far smaller, so the app's frames are not limited further.
	conn.SetReadLimit(-1)

	ctx, done := context.WithCancel(context.Background())
	c := &socketConn{id: randomHex(8), conn: conn, ctx: ctx, done: done}
	count := h.sockets.add(c)
	defer h.sockets.remove(c)
	defer done()

	if err := c.send(socketHello(store.DefaultAppID, count)); err != nil {
		conn.Close(websocket.StatusInternalError, "")
		return
	}
	// Reading answers the app's pings with pongs echoing their payload and
	// replies to its close. Acks are read here too, until a later slice acts
	// on them.
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
	}
}

// send writes one message as a single text frame of JSON.
func (c *socketConn) send(v any) error {
	ctx, cancel := context.WithTimeout(c.ctx, socketWriteTimeout)
	defer cancel()
	return wsjson.Write(ctx, c.conn, v)
}

// socketHello is the first message on every connection. Which fields Slack
// always sends, and whether num_connections counts this one, are open
// (socket-004); this follows the guide's example and counts it.
func socketHello(appID string, numConnections int) map[string]any {
	return map[string]any{
		"type":            "hello",
		"num_connections": numConnections,
		"connection_info": map[string]any{"app_id": appID},
		"debug_info": map[string]any{
			"host":                        "applink-emulator",
			"started":                     time.Now().UTC().Format("2006-01-02 15:04:05.000"),
			"build_number":                1,
			"approximate_connection_time": approximateConnectionTime,
		},
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
