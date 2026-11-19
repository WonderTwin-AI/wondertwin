package api_test

import (
	"context"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// openSocket connects the way an SDK does: apps.connections.open with an app
// token, then the URL it returns, then the hello.
func openSocket(t *testing.T, ctx context.Context, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	m := connectionsOpen(t, srv.URL, map[string][]string{"Authorization": {"Bearer xapp-1-A_SIM-1-secret"}}, "")
	raw, _ := m["url"].(string)
	conn, _, err := dialSocket(t, ctx, raw)
	if err != nil {
		t.Fatalf("dial %s: %v", raw, err)
	}
	readHello(t, ctx, conn)
	return conn
}

func readEnvelope(t *testing.T, ctx context.Context, conn *websocket.Conn) map[string]any {
	t.Helper()
	var env map[string]any
	if err := wsjson.Read(ctx, conn, &env); err != nil {
		t.Fatalf("reading an envelope: %v", err)
	}
	return env
}

func postMessage(t *testing.T, srv *httptest.Server, channel, text string) {
	t.Helper()
	if _, m := call(t, srv, "POST", "/api/chat.postMessage", formType, url.Values{"channel": {channel}, "text": {text}}.Encode(), true); m["ok"] != true {
		t.Fatalf("chat.postMessage: %v", m)
	}
}

// socketModeOn seeds a channel, drains its channel_created event, and turns
// Socket Mode on, with a Request URL set to show HTTP delivery stops.
func socketModeOn(t *testing.T) (*httptest.Server, *receiver, string) {
	t.Helper()
	srv, tc := setupSlack(t)
	rc := newReceiver(t)
	ch := seedChannel(tc, "socket")
	call(t, srv, "POST", "/admin/webhooks/flush", "", "", false)
	cfg := configureEvents(t, srv, map[string]any{"request_url": rc.srv.URL, "socket_mode": true})
	if cfg["socket_mode"] != true || cfg["request_url"] != rc.srv.URL {
		t.Fatalf("events config = %v, want socket_mode on and the Request URL kept", cfg)
	}
	return srv, rc, ch
}

func TestSocketEvents(t *testing.T) {
	t.Run("an event arrives as an events_api envelope and its ack is logged", func(t *testing.T) {
		srv, rc, ch := socketModeOn(t)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn := openSocket(t, ctx, srv)

		postMessage(t, srv, ch, "over the socket")
		env := readEnvelope(t, ctx, conn)
		if env["type"] != "events_api" || env["accepts_response_payload"] != false ||
			env["retry_attempt"] != float64(0) || env["retry_reason"] != "" {
			t.Errorf("envelope = %v", env)
		}
		id, _ := env["envelope_id"].(string)
		if len(id) != 36 {
			t.Errorf("envelope_id = %q, want a UUID", id)
		}
		payload, _ := env["payload"].(map[string]any)
		event, _ := payload["event"].(map[string]any)
		if payload["type"] != "event_callback" || payload["event_id"] == nil || event["type"] != "message" || event["text"] != "over the socket" {
			t.Errorf("payload = %v, want the event_callback HTTP would send", payload)
		}

		if err := wsjson.Write(ctx, conn, map[string]any{"envelope_id": id, "payload": map[string]any{}}); err != nil {
			t.Fatalf("ack: %v", err)
		}
		var d map[string]any
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			if log := deliveryLog(t, srv, 1); len(log) == 1 {
				if d, _ = log[0].(map[string]any); d["acked"] == true {
					break
				}
			}
		}
		if d["transport"] != "socket" || d["envelope_id"] != id || d["acked"] != true {
			t.Errorf("delivery = %v, want the acked socket delivery", d)
		}
		if n := rc.count(); n != 0 {
			t.Errorf("the Request URL got %d deliveries with Socket Mode on", n)
		}
	})

	t.Run("an unacked envelope is never resent", func(t *testing.T) {
		srv, _, ch := socketModeOn(t)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn := openSocket(t, ctx, srv)

		postMessage(t, srv, ch, "first")
		postMessage(t, srv, ch, "second")
		for _, want := range []string{"first", "second"} {
			event := readEnvelope(t, ctx, conn)["payload"].(map[string]any)["event"].(map[string]any)
			if event["text"] != want {
				t.Fatalf("envelope carries %q, want %q and no resend", event["text"], want)
			}
		}
		quiet, stop := context.WithTimeout(ctx, 300*time.Millisecond)
		defer stop()
		var extra map[string]any
		if err := wsjson.Read(quiet, conn, &extra); err == nil {
			t.Errorf("unexpected envelope %v", extra)
		}
	})

	t.Run("events with no connection open are dropped", func(t *testing.T) {
		srv, rc, ch := socketModeOn(t)
		postMessage(t, srv, ch, "nobody listening")
		log := deliveryLog(t, srv, 1)
		if len(log) != 1 || log[0].(map[string]any)["error"] == nil {
			t.Fatalf("delivery log = %v, want one dropped socket delivery", log)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn := openSocket(t, ctx, srv)
		quiet, stop := context.WithTimeout(ctx, 300*time.Millisecond)
		defer stop()
		var late map[string]any
		if err := wsjson.Read(quiet, conn, &late); err == nil {
			t.Errorf("a dropped event arrived later: %v", late)
		}
		if n := rc.count(); n != 0 {
			t.Errorf("the Request URL got %d deliveries with Socket Mode on", n)
		}
	})

	t.Run("turning Socket Mode off resumes HTTP delivery", func(t *testing.T) {
		srv, rc, ch := socketModeOn(t)
		configureEvents(t, srv, map[string]any{"socket_mode": false})
		postMessage(t, srv, ch, "back to http")
		if event := envelope(t, rc.wait(t, 1)[0])["event"].(map[string]any); event["text"] != "back to http" {
			t.Errorf("HTTP delivery carried %v", event)
		}
	})
}

func (rc *receiver) count() int {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return len(rc.got)
}
