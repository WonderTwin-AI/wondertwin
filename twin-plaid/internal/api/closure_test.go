package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// closureOperations is the scope of build plan plaid-bp1: every operation the
// app emulator serves. The manifest must list exactly these as implemented.
var closureOperations = []string{
	"accounts/balance/get", "accounts/get", "auth/get",
	"identity/get", "identity/match",
	"institutions/get", "institutions/get_by_id", "institutions/search",
	"item/access_token/invalidate", "item/get", "item/public_token/exchange",
	"item/remove", "item/webhook/update",
	"link/token/create", "link/token/get",
	"processor/stripe/bank_account_token/create", "processor/token/create",
	"sandbox/item/fire_webhook", "sandbox/item/reset_login",
	"sandbox/public_token/create", "sandbox/transactions/create",
	"transactions/get", "transactions/recurring/get", "transactions/refresh",
	"transactions/sync", "webhook_verification_key/get",
}

type manifestFile struct {
	TwinID   string `json:"twin_id"`
	Coverage struct {
		Implemented    []string `json:"resources_implemented"`
		NotImplemented []string `json:"resources_not_implemented"`
	} `json:"coverage"`
	ServiceSurface struct {
		ResourceCount int `json:"resource_count"`
	} `json:"service_surface"`
}

func readJSON(t *testing.T, path string, into any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func TestManifestListsEveryOperationAsImplemented(t *testing.T) {
	var m manifestFile
	readJSON(t, "../../twin-manifest.json", &m)

	got := append([]string(nil), m.Coverage.Implemented...)
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(closureOperations, ",") {
		t.Fatalf("implemented = %v\nwant %v", got, closureOperations)
	}
	if len(m.Coverage.NotImplemented) != 0 {
		t.Fatalf("resources_not_implemented = %v, want none", m.Coverage.NotImplemented)
	}
	if m.ServiceSurface.ResourceCount != len(closureOperations) {
		t.Fatalf("resource_count = %d, want %d", m.ServiceSurface.ResourceCount, len(closureOperations))
	}
}

func TestEveryManifestOperationIsMounted(t *testing.T) {
	twin := twincore.New(&twincore.Config{Name: "twin-plaid-test"})
	memStore := store.New()
	memStore.Rand = twin.Rand
	api.NewHandler(memStore, twin.Middleware()).Routes(twin.Router)

	var m manifestFile
	readJSON(t, "../../twin-manifest.json", &m)
	for _, op := range m.Coverage.Implemented {
		req := httptest.NewRequest(http.MethodPost, "/"+op, strings.NewReader(`{"client_id":"c","secret":"s"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		twin.Router.ServeHTTP(rec, req)

		var body struct {
			ErrorCode string `json:"error_code"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if body.ErrorCode == "NOT_FOUND" {
			t.Errorf("POST /%s is listed as implemented but is not mounted", op)
		}
	}
}

func TestDivergencesAreDeclaredInTheSharedShape(t *testing.T) {
	var d struct {
		TwinID           string `json:"twin_id"`
		Tier             string `json:"tier"`
		VendorAPIVersion string `json:"vendor_api_version"`
		ReviewedAt       string `json:"reviewed_at"`
		Divergences      []map[string]json.RawMessage
	}
	readJSON(t, "../../divergences.json", &d)

	if d.TwinID != "twin-plaid" || d.Tier == "" || d.VendorAPIVersion == "" || d.ReviewedAt == "" {
		t.Fatalf("header incomplete: %+v", d)
	}
	if len(d.Divergences) == 0 {
		t.Fatal("no divergences declared")
	}
	want := []string{"operator_note", "quirk_ids", "reason", "references", "route"}
	seen := map[string]bool{}
	for i, e := range d.Divergences {
		var keys []string
		for k := range e {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if strings.Join(keys, ",") != strings.Join(want, ",") {
			t.Fatalf("entry %d has keys %v, want %v", i, keys, want)
		}
		var route string
		_ = json.Unmarshal(e["route"], &route)
		if route == "" || seen[route] {
			t.Fatalf("entry %d: route %q is empty or repeated", i, route)
		}
		seen[route] = true
		var refs map[string]string
		if err := json.Unmarshal(e["references"], &refs); err != nil || refs["docs"] == "" || refs["manifest"] == "" {
			t.Fatalf("entry %d (%s): references need docs and manifest: %s", i, route, e["references"])
		}
	}
}
