package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

func setupSocketSlack(t *testing.T) (*httptest.Server, *api.Handler) {
	t.Helper()
	memStore := store.New()
	twin := twincore.New(&twincore.Config{Name: "twin-slack-test"})
	handler := api.NewHandler(memStore, twin.Middleware())
	handler.Routes(twin.Router)
	admin.NewHandler(handler.AdminState(), twin.Middleware(), memStore.Clock).Routes(twin.Router)
	srv := httptest.NewServer(twin.Router)
	t.Cleanup(srv.Close)
	return srv, handler
}

func socketURL(srv *httptest.Server, ticket string) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http") + api.SocketPath + "?ticket=" + ticket + "&app_id=" + store.DefaultAppID
}

func dialSocket(t *testing.T, ctx context.Context, url string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	conn, resp, err := websocket.Dial(ctx, url, nil)
	if conn != nil {
		t.Cleanup(func() { conn.CloseNow() })
	}
	return conn, resp, err
}

func readHello(t *testing.T, ctx context.Context, conn *websocket.Conn) map[string]any {
	t.Helper()
	var msg map[string]any
	if err := wsjson.Read(ctx, conn, &msg); err != nil {
		t.Fatalf("reading the first message: %v", err)
	}
	if msg["type"] != "hello" {
		t.Fatalf("first message is %v, want a hello", msg)
	}
	return msg
}

func TestSocketConnect(t *testing.T) {
	t.Run("hello is the first message", func(t *testing.T) {
		srv, h := setupSocketSlack(t)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		conn, resp, err := dialSocket(t, ctx, socketURL(srv, h.IssueSocketTicket()))
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		if got := resp.Header.Get("Sec-WebSocket-Extensions"); got != "" {
			t.Errorf("compression offered as %q; it should be declined", got)
		}
		hello := readHello(t, ctx, conn)
		if hello["num_connections"] != float64(1) {
			t.Errorf("num_connections = %v, want 1", hello["num_connections"])
		}
		info, _ := hello["connection_info"].(map[string]any)
		if info["app_id"] != store.DefaultAppID {
			t.Errorf("connection_info.app_id = %v, want %s", info["app_id"], store.DefaultAppID)
		}
		debug, _ := hello["debug_info"].(map[string]any)
		if debug["approximate_connection_time"] != float64(3600) {
			t.Errorf("debug_info.approximate_connection_time = %v, want 3600", debug["approximate_connection_time"])
		}

		second, _, err := dialSocket(t, ctx, socketURL(srv, h.IssueSocketTicket()))
		if err != nil {
			t.Fatalf("dial a second connection: %v", err)
		}
		if got := readHello(t, ctx, second)["num_connections"]; got != float64(2) {
			t.Errorf("second hello num_connections = %v, want 2", got)
		}
	})

	t.Run("pings are answered", func(t *testing.T) {
		srv, h := setupSocketSlack(t)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn, _, err := dialSocket(t, ctx, socketURL(srv, h.IssueSocketTicket()))
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		readHello(t, ctx, conn)
		// The client reads in the background, which is how it receives pongs.
		go func() {
			for {
				if _, _, err := conn.Read(ctx); err != nil {
					return
				}
			}
		}()
		for range 3 {
			if err := conn.Ping(ctx); err != nil {
				t.Fatalf("ping: %v", err)
			}
		}
	})

	t.Run("a ticket opens one connection", func(t *testing.T) {
		srv, h := setupSocketSlack(t)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ticket := h.IssueSocketTicket()
		if _, _, err := dialSocket(t, ctx, socketURL(srv, ticket)); err != nil {
			t.Fatalf("first dial: %v", err)
		}
		for _, url := range []string{socketURL(srv, ticket), socketURL(srv, "never-issued"), socketURL(srv, "")} {
			_, resp, err := dialSocket(t, ctx, url)
			if err == nil {
				t.Fatalf("dial %s succeeded, want a refusal", url)
			}
			if resp == nil || resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("dial %s answered %v, want 401", url, resp)
			}
		}
	})

	t.Run("admin reset closes every socket and clears tickets", func(t *testing.T) {
		srv, h := setupSocketSlack(t)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn, _, err := dialSocket(t, ctx, socketURL(srv, h.IssueSocketTicket()))
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		readHello(t, ctx, conn)
		unused := h.IssueSocketTicket()

		resp, err := http.Post(srv.URL+"/admin/reset", "application/json", nil)
		if err != nil {
			t.Fatalf("reset: %v", err)
		}
		resp.Body.Close()

		_, _, err = conn.Read(ctx)
		if status := websocket.CloseStatus(err); status != websocket.StatusNormalClosure {
			t.Errorf("read after reset: %v, want a normal close", err)
		}
		if ctx.Err() != nil {
			t.Fatal("the reset did not close the socket")
		}
		if _, resp, err := dialSocket(t, ctx, socketURL(srv, unused)); err == nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("a ticket issued before the reset still opens a connection (err %v)", err)
		}
	})
}
