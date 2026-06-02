package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveAccountID_FlagWinsOverEverything(t *testing.T) {
	t.Setenv("WONDERTWIN_ACCOUNT_ID", "from-env")
	id, src, err := resolveAccountID("from-flag", "", t.TempDir())
	if err != nil {
		t.Fatalf("resolveAccountID: %v", err)
	}
	if id != "from-flag" || !strings.Contains(src, "--org") {
		t.Errorf("got id=%q src=%q", id, src)
	}
}

func TestResolveAccountID_AccountAliasIsHonored(t *testing.T) {
	id, src, err := resolveAccountID("", "from-account", t.TempDir())
	if err != nil {
		t.Fatalf("resolveAccountID: %v", err)
	}
	if id != "from-account" || !strings.Contains(src, "--account") {
		t.Errorf("got id=%q src=%q", id, src)
	}
}

func TestResolveAccountID_EnvBeatsProjectJSON(t *testing.T) {
	dir := t.TempDir()
	writeProjectJSON(t, dir, `{"account_id": "from-project"}`)
	t.Setenv("WONDERTWIN_ACCOUNT_ID", "from-env")
	id, src, err := resolveAccountID("", "", dir)
	if err != nil {
		t.Fatalf("resolveAccountID: %v", err)
	}
	if id != "from-env" || !strings.Contains(src, "WONDERTWIN_ACCOUNT_ID") {
		t.Errorf("got id=%q src=%q", id, src)
	}
}

func TestResolveAccountID_ReadsProjectJSON(t *testing.T) {
	dir := t.TempDir()
	writeProjectJSON(t, dir, `{"account_id": "from-project"}`)
	t.Setenv("WONDERTWIN_ACCOUNT_ID", "")
	id, src, err := resolveAccountID("", "", dir)
	if err != nil {
		t.Fatalf("resolveAccountID: %v", err)
	}
	if id != "from-project" || !strings.Contains(src, "project.json") {
		t.Errorf("got id=%q src=%q", id, src)
	}
}

func TestHumanReason_KnownCodes(t *testing.T) {
	cases := map[string]string{
		"not_entitled":               "subscribe",
		"entitlement_lapsed":         "renew",
		"entitlement_cancelled":      "subscribe",
		"entitlement_unknown_status": "support",
	}
	for code, want := range cases {
		got := humanReason(code)
		if !strings.Contains(got, want) {
			t.Errorf("humanReason(%q) = %q, want it to contain %q", code, got, want)
		}
	}
}

func TestHumanReason_UnknownPassesThroughVerbatim(t *testing.T) {
	// Forward-compat: server may add codes we don't know yet.
	got := humanReason("future_unknown_code")
	if got != "future_unknown_code" {
		t.Errorf("unknown reason should pass through verbatim, got %q", got)
	}
}

func writeProjectJSON(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".wondertwin"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".wondertwin", "project.json"), []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}
