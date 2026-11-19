package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
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
	seq  int
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
	// opened counts connections, so the newest is known.
	opened int
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
	s.opened++
	c.seq = s.opened
	s.conns[c.id] = c
	return len(s.conns)
}

// newest is the connection opened last, or nil with none open. Slack says only
// that a payload may go to any open connection (socket-009); the newest is the
// one an SDK keeps after replacing a connection, so envelopes go there.
func (s *sockets) newest() *socketConn {
	s.mu.Lock()
	defer s.mu.Unlock()
	var newest *socketConn
	for _, c := range s.conns {
		if newest == nil || c.seq > newest.seq {
			newest = c
		}
	}
	return newest
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

// AppsConnectionsOpen handles /api/apps.connections.open, which gives an app a
// new Socket Mode URL on every call; each SDK calls it again before every
// reconnect. Only an app-level token may call it, and only in the
// Authorization header. Which error Slack gives a bot or user token
// (socket-010) and a token sent as a POST parameter (socket-011) is not
// documented: this answers not_allowed_token_type and not_authed.
func (h *Handler) AppsConnectionsOpen(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") == "" {
		slackError(w, "not_authed")
		return
	}
	if principal(r).Type != store.TokenApp {
		slackError(w, "not_allowed_token_type")
		return
	}
	slackOK(w, map[string]any{"url": h.socketURL(r)})
}

// socketURL is a new Socket Mode URL on the host the app called, so it
// reaches this emulator however it is fronted: wss when the app called over
// https, ws otherwise.
func (h *Handler) socketURL(r *http.Request) string {
	base := origin(r)
	if rest, ok := strings.CutPrefix(base, "https://"); ok {
		base = "wss://" + rest
	} else {
		base = "ws://" + strings.TrimPrefix(base, "http://")
	}
	// ticket comes first, as in Slack's example URL.
	return base + socketPath + "?ticket=" + h.sockets.issueTicket() + "&app_id=" + url.QueryEscape(store.DefaultAppID)
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
	// replies to its close. An ack may arrive on any connection, including
	// one other than the envelope's, as the SDKs send it on whichever is open.
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var ack struct {
			EnvelopeID string `json:"envelope_id"`
		}
		if json.Unmarshal(data, &ack) == nil && ack.EnvelopeID != "" {
			h.events.ack(ack.EnvelopeID)
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

// socketEnvelope wraps an event_callback for Socket Mode. The payload is the
// body HTTP delivery would POST (socket-003); an envelope is never resent, so
// retry_attempt is always 0 (socket-002).
func socketEnvelope(envelopeID string, body []byte) map[string]any {
	return map[string]any{
		"envelope_id":              envelopeID,
		"type":                     "events_api",
		"payload":                  json.RawMessage(body),
		"accepts_response_payload": false,
		"retry_attempt":            0,
		"retry_reason":             "",
	}
}

// envelopeID is shaped like Slack's, a UUID.
func envelopeID() string {
	h := randomHex(16)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
