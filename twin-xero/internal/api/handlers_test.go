package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/ledger/accounting"
	"github.com/wondertwin-ai/wondertwin/twinkit/store/journal"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func setupTestHandler() (*Handler, *chi.Mux) {
	memStore := store.New()
	clock := memStore.Clock
	j := journal.New(clock)
	engine := accounting.NewEngine(
		accounting.WithJournal(j),
		accounting.WithClock(clock),
	)

	cfg := &twincore.Config{Name: "test-xero", Port: 0}
	twin := twincore.New(cfg)

	h := NewHandler(memStore, engine, nil, twin.Middleware())
	r := chi.NewRouter()
	h.Routes(r)
	return h, r
}

func xeroRequest(method, path string, body any) *http.Request {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Authorization", "Bearer test_token")
	req.Header.Set("Xero-Tenant-Id", "test-tenant-id")
	req.Header.Set("Content-Type", "application/json")
	return req
}

func doRequest(r *chi.Mux, method, path string, body any) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, xeroRequest(method, path, body))
	return w
}

func parseResponse(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("parse response: %v\nbody: %s", err, w.Body.String())
	}
	return result
}

// --- Auth Tests ---

func TestAuth_MissingBearer(t *testing.T) {
	_, r := setupTestHandler()
	req := httptest.NewRequest("GET", "/api.xro/2.0/Contacts", nil)
	req.Header.Set("Xero-Tenant-Id", "tenant")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestAuth_MissingTenant(t *testing.T) {
	_, r := setupTestHandler()
	req := httptest.NewRequest("GET", "/api.xro/2.0/Contacts", nil)
	req.Header.Set("Authorization", "Bearer test")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// --- Contact Tests ---

func TestContacts_CreateAndList(t *testing.T) {
	_, r := setupTestHandler()
	w := doRequest(r, "POST", "/api.xro/2.0/Contacts", map[string]any{
		"Name": "Acme Corp",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create status = %d: %s", w.Code, w.Body.String())
	}

	resp := parseResponse(t, w)
	contacts := resp["Contacts"].([]any)
	if len(contacts) != 1 {
		t.Fatalf("expected 1 contact, got %d", len(contacts))
	}
	c := contacts[0].(map[string]any)
	if c["Name"] != "Acme Corp" {
		t.Errorf("name = %s, want Acme Corp", c["Name"])
	}
	contactID := c["ContactID"].(string)

	// List.
	w = doRequest(r, "GET", "/api.xro/2.0/Contacts", nil)
	resp = parseResponse(t, w)
	contacts = resp["Contacts"].([]any)
	if len(contacts) != 1 {
		t.Errorf("list: expected 1 contact, got %d", len(contacts))
	}

	// Get.
	w = doRequest(r, "GET", "/api.xro/2.0/Contacts/"+contactID, nil)
	if w.Code != http.StatusOK {
		t.Errorf("get status = %d", w.Code)
	}
}

func TestContacts_NotFound(t *testing.T) {
	_, r := setupTestHandler()
	w := doRequest(r, "GET", "/api.xro/2.0/Contacts/nonexistent", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// --- Account Tests ---

func TestAccounts_Create(t *testing.T) {
	_, r := setupTestHandler()
	w := doRequest(r, "POST", "/api.xro/2.0/Accounts", map[string]any{
		"Code": "200",
		"Name": "Sales",
		"Type": "REVENUE",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create status = %d: %s", w.Code, w.Body.String())
	}
	resp := parseResponse(t, w)
	accts := resp["Accounts"].([]any)
	if len(accts) != 1 {
		t.Fatalf("expected 1 account")
	}
}

// --- Invoice Lifecycle Test ---

func TestInvoice_FullLifecycle(t *testing.T) {
	h, r := setupTestHandler()

	// Create accounts needed for the lifecycle.
	createAccount(t, r, "200", "Accounts Receivable", "ASSET")
	createAccount(t, r, "400", "Sales Revenue", "REVENUE")
	createAccount(t, r, "090", "Business Bank", "BANK")

	h.engine.SetReceivableAccount(findAccountID(t, h, "200"))
	bankAcctID := findAccountID(t, h, "090")

	// Create invoice at AUTHORISED.
	w := doRequest(r, "POST", "/api.xro/2.0/Invoices", map[string]any{
		"Invoices": []map[string]any{{
			"Type":         "ACCREC",
			"Status":       "AUTHORISED",
			"CurrencyCode": "USD",
			"Contact":      map[string]any{"ContactID": "c1"},
			"Date":         "2026-03-15",
			"LineItems": []map[string]any{{
				"Description": "Consulting",
				"Quantity":    1,
				"UnitAmount":  500.00,
				"AccountCode": "400",
			}},
		}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create invoice status = %d: %s", w.Code, w.Body.String())
	}
	resp := parseResponse(t, w)
	invoices := resp["Invoices"].([]any)
	inv := invoices[0].(map[string]any)
	if inv["Status"] != "AUTHORISED" {
		t.Errorf("status = %s, want AUTHORISED", inv["Status"])
	}
	invID := inv["InvoiceID"].(string)

	// Apply payment.
	w = doRequest(r, "PUT", "/api.xro/2.0/Payments", map[string]any{
		"Payments": []map[string]any{{
			"Invoice": map[string]any{"InvoiceID": invID},
			"Account": map[string]any{"AccountID": bankAcctID},
			"Amount":  500.00,
			"Date":    "2026-03-16",
		}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("payment status = %d: %s", w.Code, w.Body.String())
	}

	// Verify invoice is now PAID.
	w = doRequest(r, "GET", "/api.xro/2.0/Invoices/"+invID, nil)
	resp = parseResponse(t, w)
	invoices = resp["Invoices"].([]any)
	inv = invoices[0].(map[string]any)
	if inv["Status"] != "PAID" {
		t.Errorf("status = %s, want PAID", inv["Status"])
	}
	if inv["AmountDue"].(float64) != 0 {
		t.Errorf("AmountDue = %v, want 0", inv["AmountDue"])
	}
}

// --- Items Tests ---

func TestItems_CreateAndList(t *testing.T) {
	_, r := setupTestHandler()
	w := doRequest(r, "POST", "/api.xro/2.0/Items", map[string]any{
		"Items": []map[string]any{{
			"Code": "WIDGET",
			"Name": "Widget",
		}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create status = %d: %s", w.Code, w.Body.String())
	}

	w = doRequest(r, "GET", "/api.xro/2.0/Items", nil)
	resp := parseResponse(t, w)
	items := resp["Items"].([]any)
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
}

// --- Bank Transaction Tests ---

func TestBankTransaction_Create(t *testing.T) {
	_, r := setupTestHandler()
	w := doRequest(r, "PUT", "/api.xro/2.0/BankTransactions", map[string]any{
		"BankTransactions": []map[string]any{{
			"Type":        "RECEIVE",
			"BankAccount": map[string]any{"AccountID": "bank_001"},
			"Contact":     map[string]any{"ContactID": "c1"},
			"LineItems": []map[string]any{{
				"Description": "Payment",
				"Quantity":    1,
				"UnitAmount":  100.00,
			}},
		}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
}

// --- Report Tests ---

func TestReports_TrialBalance(t *testing.T) {
	h, r := setupTestHandler()
	createAccount(t, r, "200", "AR", "ASSET")
	createAccount(t, r, "400", "Revenue", "REVENUE")
	h.engine.SetReceivableAccount(findAccountID(t, h, "200"))

	// Create authorised invoice to generate journal entries.
	doRequest(r, "POST", "/api.xro/2.0/Invoices", map[string]any{
		"Invoices": []map[string]any{{
			"Type":         "ACCREC",
			"Status":       "AUTHORISED",
			"CurrencyCode": "USD",
			"Contact":      map[string]any{"ContactID": "c1"},
			"LineItems": []map[string]any{{
				"Description": "Service",
				"Quantity":    1,
				"UnitAmount":  1000.00,
				"AccountCode": "400",
			}},
		}},
	})

	w := doRequest(r, "GET", "/api.xro/2.0/Reports/TrialBalance", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	resp := parseResponse(t, w)
	reports := resp["Reports"].([]any)
	if len(reports) != 1 {
		t.Fatal("expected 1 report")
	}
	report := reports[0].(map[string]any)
	if report["TotalDebit"] != report["TotalCredit"] {
		t.Errorf("trial balance unequal: debit=%v credit=%v", report["TotalDebit"], report["TotalCredit"])
	}
}

// --- Pagination Tests ---

func TestPagination(t *testing.T) {
	_, r := setupTestHandler()
	// Create 3 contacts.
	for i := 0; i < 3; i++ {
		doRequest(r, "POST", "/api.xro/2.0/Contacts", map[string]any{
			"Name": "Contact",
		})
	}

	// Page 1, size 2.
	w := doRequest(r, "GET", "/api.xro/2.0/Contacts?page=1&pageSize=2", nil)
	resp := parseResponse(t, w)
	contacts := resp["Contacts"].([]any)
	if len(contacts) != 2 {
		t.Errorf("page 1: expected 2, got %d", len(contacts))
	}

	// Page 2, size 2.
	w = doRequest(r, "GET", "/api.xro/2.0/Contacts?page=2&pageSize=2", nil)
	resp = parseResponse(t, w)
	contacts = resp["Contacts"].([]any)
	if len(contacts) != 1 {
		t.Errorf("page 2: expected 1, got %d", len(contacts))
	}
}

// --- Manual Journal Tests ---

func TestManualJournal_Create(t *testing.T) {
	_, r := setupTestHandler()
	createAccount(t, r, "090", "Bank", "BANK")
	createAccount(t, r, "400", "Revenue", "REVENUE")

	w := doRequest(r, "PUT", "/api.xro/2.0/ManualJournals", map[string]any{
		"ManualJournals": []map[string]any{{
			"Narration": "Adjustment",
			"JournalLines": []map[string]any{
				{"AccountCode": "090", "LineAmount": 100.00},
				{"AccountCode": "400", "LineAmount": -100.00},
			},
		}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	resp := parseResponse(t, w)
	mjs := resp["ManualJournals"].([]any)
	mj := mjs[0].(map[string]any)
	if mj["Status"] != "POSTED" {
		t.Errorf("status = %s, want POSTED", mj["Status"])
	}
}

// --- Helpers ---

func createAccount(t *testing.T, r *chi.Mux, code, name, acctType string) string {
	t.Helper()
	w := doRequest(r, "POST", "/api.xro/2.0/Accounts", map[string]any{
		"Code": code,
		"Name": name,
		"Type": acctType,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create account %s failed: %d %s", code, w.Code, w.Body.String())
	}
	resp := parseResponse(t, w)
	accts := resp["Accounts"].([]any)
	return accts[0].(map[string]any)["AccountID"].(string)
}

func findAccountID(t *testing.T, h *Handler, code string) string {
	t.Helper()
	acct, ok := h.store.FindAccountByCode(code)
	if !ok {
		t.Fatalf("account with code %s not found", code)
	}
	return acct.AccountID
}

