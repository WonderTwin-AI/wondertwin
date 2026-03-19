package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/ledger/accounting"
	"github.com/wondertwin-ai/wondertwin/twinkit/state/journal"
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

	// Create contact.
	contactID := createContact(t, r, "Test Customer")

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
			"Contact":      map[string]any{"ContactID": contactID},
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
	contactID := createContact(t, r, "BankTxn Contact")
	w := doRequest(r, "PUT", "/api.xro/2.0/BankTransactions", map[string]any{
		"BankTransactions": []map[string]any{{
			"Type":        "RECEIVE",
			"BankAccount": map[string]any{"AccountID": "bank_001"},
			"Contact":     map[string]any{"ContactID": contactID},
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
	contactID := createContact(t, r, "TB Contact")
	createAccount(t, r, "200", "AR", "ASSET")
	createAccount(t, r, "400", "Revenue", "REVENUE")
	h.engine.SetReceivableAccount(findAccountID(t, h, "200"))

	// Create authorised invoice to generate journal entries.
	doRequest(r, "POST", "/api.xro/2.0/Invoices", map[string]any{
		"Invoices": []map[string]any{{
			"Type":         "ACCREC",
			"Status":       "AUTHORISED",
			"CurrencyCode": "USD",
			"Contact":      map[string]any{"ContactID": contactID},
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

func createContact(t *testing.T, r *chi.Mux, name string) string {
	t.Helper()
	w := doRequest(r, "POST", "/api.xro/2.0/Contacts", map[string]any{
		"Name": name,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create contact %s failed: %d %s", name, w.Code, w.Body.String())
	}
	resp := parseResponse(t, w)
	contacts := resp["Contacts"].([]any)
	return contacts[0].(map[string]any)["ContactID"].(string)
}

// --- Update/Delete Tests ---

func TestContact_UpdateAndDelete(t *testing.T) {
	_, r := setupTestHandler()
	id := createContact(t, r, "Original Name")

	// Update.
	w := doRequest(r, "PUT", "/api.xro/2.0/Contacts/"+id, map[string]any{
		"Contacts": []map[string]any{{"Name": "Updated Name"}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}
	resp := parseResponse(t, w)
	c := resp["Contacts"].([]any)[0].(map[string]any)
	if c["Name"] != "Updated Name" {
		t.Errorf("Name = %v, want Updated Name", c["Name"])
	}

	// Delete (archive).
	w = doRequest(r, "DELETE", "/api.xro/2.0/Contacts/"+id, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	resp = parseResponse(t, w)
	c = resp["Contacts"].([]any)[0].(map[string]any)
	if c["ContactStatus"] != "ARCHIVED" {
		t.Errorf("ContactStatus = %v, want ARCHIVED", c["ContactStatus"])
	}
}

func TestInvoice_UpdateAndDelete(t *testing.T) {
	_, r := setupTestHandler()
	contactID := createContact(t, r, "Inv Customer")
	createAccount(t, r, "400", "Revenue", "REVENUE")

	// Create draft invoice.
	w := doRequest(r, "POST", "/api.xro/2.0/Invoices", map[string]any{
		"Invoices": []map[string]any{{
			"Type":    "ACCREC",
			"Contact": map[string]any{"ContactID": contactID},
			"LineItems": []map[string]any{{
				"Description": "Service", "Quantity": 1, "UnitAmount": 100, "AccountCode": "400",
			}},
		}},
	})
	invID := parseResponse(t, w)["Invoices"].([]any)[0].(map[string]any)["InvoiceID"].(string)

	// Delete draft invoice.
	w = doRequest(r, "DELETE", "/api.xro/2.0/Invoices/"+invID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	resp := parseResponse(t, w)
	inv := resp["Invoices"].([]any)[0].(map[string]any)
	if inv["Status"] != "DELETED" {
		t.Errorf("Status = %v, want DELETED", inv["Status"])
	}
}

func TestItem_UpdateAndDelete(t *testing.T) {
	_, r := setupTestHandler()
	w := doRequest(r, "POST", "/api.xro/2.0/Items", map[string]any{
		"Items": []map[string]any{{"Code": "ITEM1", "Name": "Widget"}},
	})
	id := parseResponse(t, w)["Items"].([]any)[0].(map[string]any)["ItemID"].(string)

	// Update.
	w = doRequest(r, "PUT", "/api.xro/2.0/Items/"+id, map[string]any{
		"Items": []map[string]any{{"Name": "Super Widget"}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}
	resp := parseResponse(t, w)
	if resp["Items"].([]any)[0].(map[string]any)["Name"] != "Super Widget" {
		t.Error("Name not updated")
	}

	// Delete.
	w = doRequest(r, "DELETE", "/api.xro/2.0/Items/"+id, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: %d", w.Code)
	}
	w = doRequest(r, "GET", "/api.xro/2.0/Items/"+id, nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", w.Code)
	}
}

// --- Bank Transaction Ledger Test ---

func TestBankTransaction_PostsJournal(t *testing.T) {
	h, r := setupTestHandler()
	contactID := createContact(t, r, "BT Customer")
	createAccount(t, r, "090", "Bank", "BANK")
	createAccount(t, r, "400", "Revenue", "REVENUE")

	txsBefore := len(h.engine.Journal().Transactions())

	doRequest(r, "PUT", "/api.xro/2.0/BankTransactions", map[string]any{
		"BankTransactions": []map[string]any{{
			"Type":        "RECEIVE",
			"BankAccount": map[string]any{"Code": "090"},
			"Contact":     map[string]any{"ContactID": contactID},
			"LineItems": []map[string]any{{
				"Description": "Sale", "Quantity": 1, "UnitAmount": 200, "AccountCode": "400",
			}},
		}},
	})

	txsAfter := len(h.engine.Journal().Transactions())
	if txsAfter <= txsBefore {
		t.Errorf("expected journal entries from bank transaction, before=%d after=%d", txsBefore, txsAfter)
	}
}

// --- Credit Note Allocation Test ---

func TestCreditNote_AllocateToInvoice(t *testing.T) {
	h, r := setupTestHandler()
	contactID := createContact(t, r, "CN Customer")
	createAccount(t, r, "200", "AR", "ASSET")
	createAccount(t, r, "400", "Revenue", "REVENUE")
	h.engine.SetReceivableAccount(findAccountID(t, h, "200"))

	// Create invoice.
	w := doRequest(r, "POST", "/api.xro/2.0/Invoices", map[string]any{
		"Invoices": []map[string]any{{
			"Type": "ACCREC", "Status": "AUTHORISED", "CurrencyCode": "USD",
			"Contact": map[string]any{"ContactID": contactID},
			"LineItems": []map[string]any{{
				"Description": "Service", "Quantity": 1, "UnitAmount": 500, "AccountCode": "400",
			}},
		}},
	})
	invID := parseResponse(t, w)["Invoices"].([]any)[0].(map[string]any)["InvoiceID"].(string)

	// Create credit note.
	w = doRequest(r, "POST", "/api.xro/2.0/CreditNotes", map[string]any{
		"CreditNotes": []map[string]any{{
			"Type": "ACCRECCREDIT", "Status": "AUTHORISED", "CurrencyCode": "USD",
			"Contact": map[string]any{"ContactID": contactID},
			"LineItems": []map[string]any{{
				"Description": "Discount", "Quantity": 1, "UnitAmount": 100, "AccountCode": "400",
			}},
		}},
	})
	cnID := parseResponse(t, w)["CreditNotes"].([]any)[0].(map[string]any)["CreditNoteID"].(string)

	// Allocate credit note to invoice.
	w = doRequest(r, "PUT", "/api.xro/2.0/CreditNotes/"+cnID+"/Allocations", map[string]any{
		"Allocations": []map[string]any{{
			"Invoice": map[string]any{"InvoiceID": invID},
			"Amount":  100,
		}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("allocate: %d %s", w.Code, w.Body.String())
	}

	// Invoice AmountDue should be reduced.
	w = doRequest(r, "GET", "/api.xro/2.0/Invoices/"+invID, nil)
	inv := parseResponse(t, w)["Invoices"].([]any)[0].(map[string]any)
	if inv["AmountDue"].(float64) != 400 {
		t.Errorf("AmountDue = %v, want 400", inv["AmountDue"])
	}
}

// --- Payment Deletion Test ---

func TestPayment_Delete(t *testing.T) {
	h, r := setupTestHandler()
	contactID := createContact(t, r, "Pay Del Customer")
	createAccount(t, r, "200", "AR", "ASSET")
	createAccount(t, r, "400", "Revenue", "REVENUE")
	createAccount(t, r, "090", "Bank", "BANK")
	h.engine.SetReceivableAccount(findAccountID(t, h, "200"))
	bankAcctID := findAccountID(t, h, "090")

	// Create invoice.
	w := doRequest(r, "POST", "/api.xro/2.0/Invoices", map[string]any{
		"Invoices": []map[string]any{{
			"Type": "ACCREC", "Status": "AUTHORISED", "CurrencyCode": "USD",
			"Contact": map[string]any{"ContactID": contactID},
			"LineItems": []map[string]any{{
				"Description": "Service", "Quantity": 1, "UnitAmount": 300, "AccountCode": "400",
			}},
		}},
	})
	invID := parseResponse(t, w)["Invoices"].([]any)[0].(map[string]any)["InvoiceID"].(string)

	// Pay fully.
	w = doRequest(r, "PUT", "/api.xro/2.0/Payments", map[string]any{
		"Payments": []map[string]any{{
			"Invoice": map[string]any{"InvoiceID": invID},
			"Account": map[string]any{"AccountID": bankAcctID},
			"Amount":  300,
		}},
	})
	pmtID := parseResponse(t, w)["Payments"].([]any)[0].(map[string]any)["PaymentID"].(string)

	// Delete payment.
	w = doRequest(r, "DELETE", "/api.xro/2.0/Payments/"+pmtID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("delete payment: %d %s", w.Code, w.Body.String())
	}
	resp := parseResponse(t, w)
	pmt := resp["Payments"].([]any)[0].(map[string]any)
	if pmt["Status"] != "DELETED" {
		t.Errorf("Status = %v, want DELETED", pmt["Status"])
	}

	// Invoice should revert to AUTHORISED.
	w = doRequest(r, "GET", "/api.xro/2.0/Invoices/"+invID, nil)
	inv := parseResponse(t, w)["Invoices"].([]any)[0].(map[string]any)
	if inv["Status"] != "AUTHORISED" {
		t.Errorf("Invoice Status = %v, want AUTHORISED", inv["Status"])
	}
	if inv["AmountDue"].(float64) != 300 {
		t.Errorf("AmountDue = %v, want 300", inv["AmountDue"])
	}
}

// --- Contact Validation Test ---

func TestInvoice_RejectsInvalidContact(t *testing.T) {
	_, r := setupTestHandler()
	createAccount(t, r, "400", "Revenue", "REVENUE")

	w := doRequest(r, "POST", "/api.xro/2.0/Invoices", map[string]any{
		"Invoices": []map[string]any{{
			"Type":    "ACCREC",
			"Contact": map[string]any{"ContactID": "nonexistent"},
			"LineItems": []map[string]any{{
				"Description": "Service", "Quantity": 1, "UnitAmount": 100, "AccountCode": "400",
			}},
		}},
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid contact, got %d", w.Code)
	}
}

// --- Tracking Categories Test ---

func TestTrackingCategories_CRUD(t *testing.T) {
	_, r := setupTestHandler()

	// Create.
	w := doRequest(r, "POST", "/api.xro/2.0/TrackingCategories", map[string]any{
		"Name": "Region",
		"Options": []map[string]any{
			{"Name": "North"},
			{"Name": "South"},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	resp := parseResponse(t, w)
	tc := resp["TrackingCategories"].([]any)[0].(map[string]any)
	id := tc["TrackingCategoryID"].(string)
	if tc["Name"] != "Region" {
		t.Errorf("Name = %v, want Region", tc["Name"])
	}

	// List.
	w = doRequest(r, "GET", "/api.xro/2.0/TrackingCategories", nil)
	resp = parseResponse(t, w)
	if len(resp["TrackingCategories"].([]any)) != 1 {
		t.Error("expected 1 tracking category")
	}

	// Delete.
	w = doRequest(r, "DELETE", "/api.xro/2.0/TrackingCategories/"+id, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: %d", w.Code)
	}
	resp = parseResponse(t, w)
	tc = resp["TrackingCategories"].([]any)[0].(map[string]any)
	if tc["Status"] != "DELETED" {
		t.Errorf("Status = %v, want DELETED", tc["Status"])
	}
}

// --- Prepayment Test ---

func TestPrepayment_Create(t *testing.T) {
	_, r := setupTestHandler()
	w := doRequest(r, "PUT", "/api.xro/2.0/Prepayments", map[string]any{
		"Prepayments": []map[string]any{{
			"Type":        "RECEIVE-PREPAYMENT",
			"Contact":     map[string]any{"ContactID": "c1"},
			"BankAccount": map[string]any{"AccountID": "bank1"},
			"LineItems": []map[string]any{{
				"Description": "Advance", "Quantity": 1, "UnitAmount": 1000,
			}},
		}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	resp := parseResponse(t, w)
	pp := resp["Prepayments"].([]any)[0].(map[string]any)
	if pp["Total"].(float64) != 1000 {
		t.Errorf("Total = %v, want 1000", pp["Total"])
	}
	if pp["RemainingCredit"].(float64) != 1000 {
		t.Errorf("RemainingCredit = %v, want 1000", pp["RemainingCredit"])
	}
}

