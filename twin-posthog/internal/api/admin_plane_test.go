package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-posthog/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-posthog/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// setupAdminPlane wires the emulator the way cmd/twin-posthog/main.go does,
// including the config provider, so the admin plane under test is the one a
// launched emulator serves.
func setupAdminPlane(t *testing.T) *httptest.Server {
	t.Helper()
	memStore := store.New()
	twin := twincore.New(&twincore.Config{Name: "twin-posthog-test", Port: 4114})
	api.NewHandler(memStore, twin.Middleware()).Routes(twin.Router)
	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.SetConfigProvider(twin)
	adminHandler.Routes(twin.Router)
	srv := httptest.NewServer(twin.Router)
	t.Cleanup(srv.Close)
	return srv
}

func getBody(t *testing.T, req *http.Request) (int, []byte) {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func TestAdminPlane(t *testing.T) {
	srv := setupAdminPlane(t)

	t.Run("health answers exactly status ok", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/admin/health", nil)
		code, body := getBody(t, req)
		if code != 200 {
			t.Fatalf("expected 200, got %d", code)
		}
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("health body is not JSON: %v", err)
		}
		if len(got) != 1 || got["status"] != "ok" {
			t.Errorf("expected exactly {\"status\":\"ok\"}, got %s", body)
		}
	})

	t.Run("config answers the effective configuration as JSON", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/admin/config", nil)
		code, body := getBody(t, req)
		if code != 200 {
			t.Fatalf("expected 200, got %d", code)
		}
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("config body is not JSON: %v", err)
		}
		if len(got) == 0 {
			t.Errorf("expected a non-empty configuration, got %s", body)
		}
	})

	t.Run("quirks answers a JSON list", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/admin/quirks", nil)
		code, body := getBody(t, req)
		if code != 200 {
			t.Fatalf("expected 200, got %d", code)
		}
		var got []any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("quirks body is not a JSON list: %v (%s)", err, body)
		}
	})

	t.Run("responses do not build URLs from the Host header", func(t *testing.T) {
		// This emulator returns no absolute URL on the surface it serves today,
		// so runtime/forwarded-host holds by there being none. The test fails
		// if a route starts echoing the gateway host as a client-visible URL
		// without honouring X-Forwarded-Host, and the slice that adds such a
		// route must replace this case with one that asserts the forwarded URL.
		for _, path := range []string{"/admin/health", "/admin/config", "/admin/state"} {
			req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
			req.Header.Set("X-Forwarded-Host", "posthog.localhost.localstack.cloud:4566")
			req.Header.Set("X-Forwarded-Proto", "https")
			code, body := getBody(t, req)
			if code != 200 {
				t.Fatalf("%s: expected 200, got %d", path, code)
			}
			if strings.Contains(string(body), "http://127.0.0.1") || strings.Contains(string(body), "http://"+strings.TrimPrefix(srv.URL, "http://")) {
				t.Errorf("%s leaks the listener address: %s", path, body)
			}
		}
	})
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// TestAdminPlaneLaunch holds runtime/launch: the binary starts as
// `<binary> --port=N` and serves health on that port.
func TestAdminPlaneLaunch(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and launches the emulator binary")
	}
	bin := filepath.Join(t.TempDir(), "twin-posthog")
	build := exec.Command("go", "build", "-o", bin, "../../cmd/twin-posthog")
	var out bytes.Buffer
	build.Stdout, build.Stderr = &out, &out
	if err := build.Run(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out.String())
	}

	port := freePort(t)
	cmd := exec.Command(bin, fmt.Sprintf("--port=%d", port))
	cmd.Env = append(os.Environ(), "PORT=")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	url := fmt.Sprintf("http://127.0.0.1:%d/admin/health", port)
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, err := http.Get(url)
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != 200 || !strings.Contains(string(b), `"ok"`) {
				t.Fatalf("health on --port: %d %s", resp.StatusCode, b)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("emulator did not serve health on --port=%d: %v", port, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
