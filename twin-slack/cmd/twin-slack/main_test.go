package main_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func getJSON(t *testing.T, url string) map[string]any {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("GET %s: %q", url, raw)
	}
	return out
}

// The binary reads --port, --seed-file and --webhook-url, and the signing
// secret from SLACK_SIGNING_SECRET, and serves the seeded state.
func TestBinaryFlagsAndSeed(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the binary")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "twin-slack")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	seed := filepath.Join(dir, "seed.json")
	if err := os.WriteFile(seed, []byte(`{"users":{"U_SEED":{"id":"U_SEED","name":"seeded","team_id":"T0001"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// The Request URL answers the url_verification handshake.
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var probe struct{ Challenge string }
		json.NewDecoder(r.Body).Decode(&probe)
		io.WriteString(w, probe.Challenge)
	}))
	defer hook.Close()

	port := freePort(t)
	cmd := exec.Command(bin, "--port", fmt.Sprint(port), "--seed-file", seed, "--webhook-url", hook.URL)
	cmd.Env = append(os.Environ(), "SLACK_SIGNING_SECRET=from-env")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, err := http.Get(base + "/admin/health")
		if err == nil {
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("binary did not start on port %d: %v", port, err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	users := getJSON(t, base+"/admin/users")
	found := false
	for _, u := range users["users"].([]any) {
		if u.(map[string]any)["id"] == "U_SEED" {
			found = true
		}
	}
	if !found || users["total"] == float64(0) {
		t.Errorf("seeded user not served by /admin/users: %v", users)
	}

	cfg := getJSON(t, base+"/admin/events/config")
	if cfg["request_url"] != hook.URL || cfg["signing_secret"] != "from-env" {
		t.Errorf("events config from flags and env: %v", cfg)
	}
}
