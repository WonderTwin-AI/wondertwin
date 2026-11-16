package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

func setupXero(t *testing.T) (*testutil.TwinClient, *testutil.AdminClient, *store.MemoryStore) {
	t.Helper()
	cfg := &twincore.Config{Name: "twin-xero-test"}
	twin := twincore.New(cfg)
	memStore := store.New()
	memStore.Rand = twin.Rand

	api.NewHandler(memStore, twin.Middleware()).Routes(twin.Router)

	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.SetConfigProvider(twin)
	adminHandler.Routes(twin.Router)

	srv := httptest.NewServer(twin.Router)
	t.Cleanup(srv.Close)
	tc := testutil.NewTwinClient(t, srv)
	return tc, testutil.NewAdminClient(tc), memStore
}

func TestHealthIsExactlyOK(t *testing.T) {
	_, ac, _ := setupXero(t)

	resp := ac.Health().AssertStatus(200)
	if got := strings.TrimSpace(string(resp.Body)); got != `{"status":"ok"}` {
		t.Fatalf("health body = %s, want {\"status\":\"ok\"}", got)
	}
}

func TestAdminStateRoundTrip(t *testing.T) {
	_, ac, _ := setupXero(t)

	ac.GetState().AssertStatus(200)
	ac.LoadState(map[string]any{}).AssertStatus(200)
	ac.LoadState(map[string]any{"unknown": true}).AssertStatus(200)
}

// The state export carries the bank transactions and manual journals, so a
// reset followed by an import brings them back as they were, whole.
func TestAdminStateRoundTripKeepsBankTransactionsAndJournals(t *testing.T) {
	tc, ac, _ := setupXero(t)
	hdr := map[string]string{"Authorization": "Bearer any", "xero-tenant-id": store.DefaultTenantID}
	call := func(method, path string, body any) *testutil.Response {
		t.Helper()
		return tc.DoWithHeaders(method, "/api.xro/2.0"+path, body, hdr)
	}
	first := func(r *testutil.Response, plural, id string) (map[string]any, string) {
		t.Helper()
		items, _ := r.JSONMap()[plural].([]any)
		if len(items) != 1 {
			t.Fatalf("want one %s, got %s", plural, r.Body)
		}
		item := items[0].(map[string]any)
		return item, item[id].(string)
	}

	_, cid := first(call(http.MethodPut, "/Contacts", map[string]any{"Contacts": []any{map[string]any{"Name": "Lender Capital"}}}).AssertStatus(200), "Contacts", "ContactID")
	call(http.MethodPut, "/Accounts", map[string]any{"Code": "2100", "Name": "Merchant Advance Loan", "Type": "TERMLIAB"}).AssertStatus(200)
	_, btID := first(call(http.MethodPut, "/BankTransactions", map[string]any{
		"Type": "RECEIVE", "Contact": map[string]any{"ContactID": cid}, "BankAccount": map[string]any{"Code": "090"}, "Reference": "ADV-1",
		"LineItems": []any{map[string]any{"Description": "Advance", "UnitAmount": 5000, "AccountCode": "200"}},
	}).AssertStatus(200), "BankTransactions", "BankTransactionID")
	_, mjID := first(call(http.MethodPut, "/ManualJournals", map[string]any{
		"Narration": "Advance booked as a loan", "Status": "POSTED", "Date": "2026-11-03",
		"JournalLines": []any{
			map[string]any{"LineAmount": 5000, "AccountCode": "090", "Description": "Advance received"},
			map[string]any{"LineAmount": -5000, "AccountCode": "2100", "Description": "Loan from lender"},
		},
	}).AssertStatus(200), "ManualJournals", "ManualJournalID")

	bt, _ := first(call(http.MethodGet, "/BankTransactions/"+btID, nil).AssertStatus(200), "BankTransactions", "BankTransactionID")
	mj, _ := first(call(http.MethodGet, "/ManualJournals/"+mjID, nil).AssertStatus(200), "ManualJournals", "ManualJournalID")
	state := ac.GetState().AssertStatus(200)

	ac.Reset().AssertStatus(200)
	call(http.MethodGet, "/BankTransactions/"+btID, nil).AssertStatus(404)
	call(http.MethodGet, "/ManualJournals/"+mjID, nil).AssertStatus(404)

	ac.LoadState(json.RawMessage(state.Body)).AssertStatus(200)
	btAfter, _ := first(call(http.MethodGet, "/BankTransactions/"+btID, nil).AssertStatus(200), "BankTransactions", "BankTransactionID")
	mjAfter, _ := first(call(http.MethodGet, "/ManualJournals/"+mjID, nil).AssertStatus(200), "ManualJournals", "ManualJournalID")
	if !reflect.DeepEqual(bt, btAfter) {
		t.Errorf("bank transaction changed across the round trip:\nbefore %v\nafter  %v", bt, btAfter)
	}
	if !reflect.DeepEqual(mj, mjAfter) {
		t.Errorf("manual journal changed across the round trip:\nbefore %v\nafter  %v", mj, mjAfter)
	}
	if bt["Reference"] != "ADV-1" || len(mj["JournalLines"].([]any)) != 2 {
		t.Errorf("the imported records lost their content: %v %v", bt, mj)
	}
}

func TestAdminStateRejectsMalformedJSON(t *testing.T) {
	tc, _, _ := setupXero(t)

	tc.Post("/admin/state", "{not json").AssertStatus(400)
}

func TestAdminResetClearsClock(t *testing.T) {
	_, ac, mem := setupXero(t)

	ac.AdvanceTime("24h").AssertStatus(200)
	if mem.Clock.Offset() != 24*time.Hour {
		t.Fatalf("offset = %s, want 24h", mem.Clock.Offset())
	}
	ac.Reset().AssertStatus(200)
	if mem.Clock.Offset() != 0 {
		t.Fatalf("offset after reset = %s, want 0", mem.Clock.Offset())
	}
}

func TestAdminConfigNamesTheTwin(t *testing.T) {
	tc, _, _ := setupXero(t)

	resp := tc.Get("/admin/config").AssertStatus(200)
	if got := resp.JSONMap()["name"]; got != "twin-xero-test" {
		t.Fatalf("config name = %v, want twin-xero-test", got)
	}
}

func TestAdminQuirksIsAnEmptyListNotNull(t *testing.T) {
	tc, _, _ := setupXero(t)

	resp := tc.Get("/admin/quirks").AssertStatus(200)
	if got := strings.TrimSpace(string(resp.Body)); got != "[]" {
		t.Fatalf("quirks body = %s, want []", got)
	}
}

func TestOriginFollowsForwardedHeaders(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{"direct", nil, "http://127.0.0.1:4115"},
		{"forwarded host and proto", map[string]string{
			"X-Forwarded-Host": "xero.localhost.localstack.cloud:4566", "X-Forwarded-Proto": "https",
		}, "https://xero.localhost.localstack.cloud:4566"},
		{"first of a list", map[string]string{
			"X-Forwarded-Host": "xero.localhost.localstack.cloud:4566, 10.0.0.1", "X-Forwarded-Proto": "http, https",
		}, "http://xero.localhost.localstack.cloud:4566"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4115/x", nil)
			for k, v := range tc.headers {
				r.Header.Set(k, v)
			}
			if got := api.Origin(r); got != tc.want {
				t.Fatalf("Origin = %s, want %s", got, tc.want)
			}
		})
	}
}
