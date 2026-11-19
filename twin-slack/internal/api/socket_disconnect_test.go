package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func socketDisconnect(t *testing.T, srv *httptest.Server, body string) (int, map[string]any) {
	t.Helper()
	return call(t, srv, "POST", "/admin/socket/disconnect", jsonType, body, false)
}

func readDisconnect(t *testing.T, ctx context.Context, conn *websocket.Conn, reason string) {
	t.Helper()
	var msg map[string]any
	if err := wsjson.Read(ctx, conn, &msg); err != nil {
		t.Fatalf("reading the disconnect message: %v", err)
	}
	debug, _ := msg["debug_info"].(map[string]any)
	if msg["type"] != "disconnect" || msg["reason"] != reason || debug["host"] == nil {
		t.Fatalf("message = %v, want a %s disconnect with debug_info.host", msg, reason)
	}
}

// closedNormally reads until the connection closes, and fails unless the
// server closed it.
func closedNormally(t *testing.T, ctx context.Context, conn *websocket.Conn) {
	t.Helper()
	_, _, err := conn.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
		t.Fatalf("read: %v, want a normal close from the server", err)
	}
}

func TestSocketDisconnect(t *testing.T) {
	t.Run("warning leaves the connection open", func(t *testing.T) {
		srv, _ := setupSlack(t)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn := openSocket(t, ctx, srv)
		if status, m := socketDisconnect(t, srv, `{"reason":"warning"}`); status != 200 || m["connections"] != float64(1) {
			t.Fatalf("disconnect: %d %v", status, m)
		}
		readDisconnect(t, ctx, conn, "warning")
		quiet, stop := context.WithTimeout(ctx, 300*time.Millisecond)
		defer stop()
		if _, _, err := conn.Read(quiet); websocket.CloseStatus(err) != -1 {
			t.Errorf("the connection closed after a warning: %v", err)
		}
	})

	t.Run("refresh_requested keeps the connection until it is replaced", func(t *testing.T) {
		srv, _ := setupSlack(t)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		old := openSocket(t, ctx, srv)
		if status, _ := socketDisconnect(t, srv, ""); status != 200 {
			t.Fatalf("disconnect answered %d", status)
		}
		readDisconnect(t, ctx, old, "refresh_requested")

		openSocket(t, ctx, srv)
		closedNormally(t, ctx, old)
	})

	t.Run("refresh_requested lets a replacement open at the limit", func(t *testing.T) {
		srv, _ := setupSlack(t)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for range 10 {
			openSocket(t, ctx, srv)
		}
		if status, m := socketDisconnect(t, srv, `{"reason":"refresh_requested"}`); status != 200 || m["connections"] != float64(10) {
			t.Fatalf("disconnect: %d %v", status, m)
		}
		m := connectionsOpen(t, srv.URL, http.Header{"Authorization": {"Bearer xapp-1-A_SIM-1-secret"}}, "")
		if m["ok"] != true {
			t.Fatalf("apps.connections.open with ten retiring: %v, want ok", m)
		}
		raw, _ := m["url"].(string)
		conn, _, err := dialSocket(t, ctx, raw)
		if err != nil {
			t.Fatalf("dial %s: %v", raw, err)
		}
		readHello(t, ctx, conn)
	})

	t.Run("link_disabled closes the connection", func(t *testing.T) {
		srv, _ := setupSlack(t)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn := openSocket(t, ctx, srv)
		socketDisconnect(t, srv, `{"reason":"link_disabled"}`)
		readDisconnect(t, ctx, conn, "link_disabled")
		closedNormally(t, ctx, conn)
	})

	t.Run("turning Socket Mode off sends link_disabled", func(t *testing.T) {
		srv, _ := setupSlack(t)
		configureEvents(t, srv, map[string]any{"socket_mode": true})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn := openSocket(t, ctx, srv)
		configureEvents(t, srv, map[string]any{"socket_mode": false})
		readDisconnect(t, ctx, conn, "link_disabled")
		closedNormally(t, ctx, conn)
	})

	t.Run("an unknown reason is refused", func(t *testing.T) {
		srv, _ := setupSlack(t)
		if status, _ := socketDisconnect(t, srv, `{"reason":"maintenance"}`); status != http.StatusBadRequest {
			t.Errorf("disconnect answered %d, want 400", status)
		}
	})

	t.Run("an eleventh connection is refused", func(t *testing.T) {
		srv, _ := setupSlack(t)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for range 10 {
			openSocket(t, ctx, srv)
		}
		m := connectionsOpen(t, srv.URL, http.Header{"Authorization": {"Bearer xapp-1-A_SIM-1-secret"}}, "")
		if m["ok"] != false || m["error"] != "too_many_websockets" {
			t.Errorf("apps.connections.open with ten open: %v, want too_many_websockets", m)
		}
	})
}
