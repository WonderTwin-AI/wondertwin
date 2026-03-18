package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/ledger/accounting"
	"github.com/wondertwin-ai/wondertwin/twinkit/store/journal"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-qbo/internal/store"
)

func setupTestHandler() (*Handler, *chi.Mux) {
	memStore := store.New()
	j := journal.New(memStore.Clock)
	engine := accounting.NewEngine(
		accounting.WithJournal(j),
		accounting.WithClock(memStore.Clock),
	)
	cfg := &twincore.Config{Name: "test-qbo", Port: 0}
	twin := twincore.New(cfg)
	h := NewHandler(memStore, engine, nil, twin.Middleware())
	r := chi.NewRouter()
	h.Routes(r)
	return h, r
}

func qboRequest(method, path string, body any) *http.Request {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Authorization", "Bearer test_token")
	req.Header.Set("Content-Type", "application/json")
	return req
}

func doReq(r *chi.Mux, method, path string, body any) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, qboRequest(method, path, body))
	return w
}

func parseResp(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
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
	req := httptest.NewRequest("GET", "/v3/company/123/customer/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

// --- Customer CRUD ---

func TestCustomer_CreateAndGet(t *testing.T) {
	_, r := setupTestHandler()
	w := doReq(r, "POST", "/v3/company/123/customer", map[string]any{
		"DisplayName": "Test Customer",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create status = %d: %s", w.Code, w.Body.String())
	}
	resp := parseResp(t, w)
	cust := resp["Customer"].(map[string]any)
	if cust["DisplayName"] != "Test Customer" {
		t.Error("DisplayName mismatch")
	}
	if cust["SyncToken"] != "0" {
		t.Errorf("SyncToken = %v, want '0'", cust["SyncToken"])
	}
	if cust["domain"] != "QBO" {
		t.Error("domain should be QBO")
	}
	id := cust["Id"].(string)

	// Get
	w = doReq(r, "GET", "/v3/company/123/customer/"+id, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get status = %d", w.Code)
	}
}

func TestCustomer_UpdateWithSyncToken(t *testing.T) {
	_, r := setupTestHandler()
	// Create
	w := doReq(r, "POST", "/v3/company/123/customer", map[string]any{
		"DisplayName": "Original",
	})
	resp := parseResp(t, w)
	cust := resp["Customer"].(map[string]any)
	id := cust["Id"].(string)

	// Update with correct SyncToken
	w = doReq(r, "POST", "/v3/company/123/customer", map[string]any{
		"Id": id, "SyncToken": "0", "DisplayName": "Updated",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d: %s", w.Code, w.Body.String())
	}
	resp = parseResp(t, w)
	cust = resp["Customer"].(map[string]any)
	if cust["SyncToken"] != "1" {
		t.Errorf("SyncToken = %v, want '1'", cust["SyncToken"])
	}
	if cust["DisplayName"] != "Updated" {
		t.Error("DisplayName not updated")
	}
}

func TestCustomer_StaleSyncToken(t *testing.T) {
	_, r := setupTestHandler()
	w := doReq(r, "POST", "/v3/company/123/customer", map[string]any{
		"DisplayName": "Test",
	})
	resp := parseResp(t, w)
	id := resp["Customer"].(map[string]any)["Id"].(string)

	// Update with stale SyncToken
	w = doReq(r, "POST", "/v3/company/123/customer", map[string]any{
		"Id": id, "SyncToken": "99", "DisplayName": "Bad Update",
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	resp = parseResp(t, w)
	fault := resp["Fault"].(map[string]any)
	errors := fault["Error"].([]any)
	errObj := errors[0].(map[string]any)
	if errObj["code"] != "5010" {
		t.Errorf("error code = %v, want 5010", errObj["code"])
	}
}

// --- Invoice Lifecycle ---

func TestInvoice_CreateAndPayment(t *testing.T) {
	_, r := setupTestHandler()

	// Create customer
	w := doReq(r, "POST", "/v3/company/123/customer", map[string]any{
		"DisplayName": "Acme",
	})
	custID := parseResp(t, w)["Customer"].(map[string]any)["Id"].(string)

	// Create invoice
	w = doReq(r, "POST", "/v3/company/123/invoice", map[string]any{
		"CustomerRef": map[string]any{"value": custID},
		"Line": []map[string]any{{
			"Amount":     500.00,
			"DetailType": "SalesItemLineDetail",
			"SalesItemLineDetail": map[string]any{
				"Qty": 1, "UnitPrice": 500.00,
			},
		}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create invoice: %d %s", w.Code, w.Body.String())
	}
	resp := parseResp(t, w)
	inv := resp["Invoice"].(map[string]any)
	invID := inv["Id"].(string)
	if inv["TotalAmt"].(float64) != 500 {
		t.Errorf("TotalAmt = %v, want 500", inv["TotalAmt"])
	}
	if inv["Balance"].(float64) != 500 {
		t.Errorf("Balance = %v, want 500", inv["Balance"])
	}

	// Apply full payment
	w = doReq(r, "POST", "/v3/company/123/payment", map[string]any{
		"CustomerRef": map[string]any{"value": custID},
		"TotalAmt":    500.00,
		"Line": []map[string]any{{
			"Amount":    500.00,
			"LinkedTxn": []map[string]any{{"TxnId": invID, "TxnType": "Invoice"}},
		}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create payment: %d %s", w.Code, w.Body.String())
	}

	// Verify invoice balance is now 0
	w = doReq(r, "GET", "/v3/company/123/invoice/"+invID, nil)
	resp = parseResp(t, w)
	inv = resp["Invoice"].(map[string]any)
	if inv["Balance"].(float64) != 0 {
		t.Errorf("Balance after payment = %v, want 0", inv["Balance"])
	}
}

func TestInvoice_Void(t *testing.T) {
	_, r := setupTestHandler()
	w := doReq(r, "POST", "/v3/company/123/invoice", map[string]any{
		"CustomerRef": map[string]any{"value": "1"},
		"Line": []map[string]any{{
			"Amount": 100.00, "DetailType": "SalesItemLineDetail",
			"SalesItemLineDetail": map[string]any{"Qty": 1, "UnitPrice": 100},
		}},
	})
	resp := parseResp(t, w)
	inv := resp["Invoice"].(map[string]any)
	invID := inv["Id"].(string)

	// Void
	w = doReq(r, "POST", "/v3/company/123/invoice?operation=void", map[string]any{
		"Id": invID, "SyncToken": "0",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("void: %d %s", w.Code, w.Body.String())
	}
	resp = parseResp(t, w)
	inv = resp["Invoice"].(map[string]any)
	if inv["Balance"].(float64) != 0 {
		t.Errorf("voided Balance = %v, want 0", inv["Balance"])
	}
}

// --- Query Tests ---

func queryURL(q string) string {
	return "/v3/company/123/query?query=" + url.QueryEscape(q)
}

func TestQuery_Basic(t *testing.T) {
	_, r := setupTestHandler()
	// Create 3 customers
	for i := 0; i < 3; i++ {
		doReq(r, "POST", "/v3/company/123/customer", map[string]any{
			"DisplayName": "Customer",
		})
	}

	w := doReq(r, "GET", queryURL("SELECT * FROM Customer"), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("query: %d %s", w.Code, w.Body.String())
	}
	resp := parseResp(t, w)
	qr := resp["QueryResponse"].(map[string]any)
	custs := qr["Customer"].([]any)
	if len(custs) != 3 {
		t.Errorf("expected 3 customers, got %d", len(custs))
	}
	if qr["totalCount"].(float64) != 3 {
		t.Errorf("totalCount = %v, want 3", qr["totalCount"])
	}
}

func TestQuery_Pagination(t *testing.T) {
	_, r := setupTestHandler()
	for i := 0; i < 5; i++ {
		doReq(r, "POST", "/v3/company/123/customer", map[string]any{
			"DisplayName": "C",
		})
	}

	w := doReq(r, "GET", queryURL("SELECT * FROM Customer STARTPOSITION 1 MAXRESULTS 2"), nil)
	resp := parseResp(t, w)
	qr := resp["QueryResponse"].(map[string]any)
	custs := qr["Customer"].([]any)
	if len(custs) != 2 {
		t.Errorf("page 1: expected 2, got %d", len(custs))
	}

	w = doReq(r, "GET", queryURL("SELECT * FROM Customer STARTPOSITION 3 MAXRESULTS 2"), nil)
	resp = parseResp(t, w)
	qr = resp["QueryResponse"].(map[string]any)
	custs = qr["Customer"].([]any)
	if len(custs) != 2 {
		t.Errorf("page 2: expected 2, got %d", len(custs))
	}
}

// --- Bill & BillPayment ---

func TestBill_CreateAndPay(t *testing.T) {
	_, r := setupTestHandler()
	w := doReq(r, "POST", "/v3/company/123/bill", map[string]any{
		"VendorRef": map[string]any{"value": "1"},
		"Line": []map[string]any{{
			"Amount": 300.00, "DetailType": "AccountBasedExpenseLineDetail",
			"AccountBasedExpenseLineDetail": map[string]any{
				"AccountRef": map[string]any{"value": "1"},
			},
		}},
	})
	resp := parseResp(t, w)
	bill := resp["Bill"].(map[string]any)
	billID := bill["Id"].(string)
	if bill["Balance"].(float64) != 300 {
		t.Errorf("Balance = %v, want 300", bill["Balance"])
	}

	// Pay the bill
	w = doReq(r, "POST", "/v3/company/123/billpayment", map[string]any{
		"VendorRef": map[string]any{"value": "1"},
		"TotalAmt":  300.00,
		"PayType":   "Check",
		"Line": []map[string]any{{
			"Amount":    300.00,
			"LinkedTxn": []map[string]any{{"TxnId": billID, "TxnType": "Bill"}},
		}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("billpayment: %d %s", w.Code, w.Body.String())
	}

	// Verify bill balance = 0
	w = doReq(r, "GET", "/v3/company/123/bill/"+billID, nil)
	resp = parseResp(t, w)
	bill = resp["Bill"].(map[string]any)
	if bill["Balance"].(float64) != 0 {
		t.Errorf("Bill Balance after payment = %v, want 0", bill["Balance"])
	}
}

// --- Account ---

func TestAccount_Create(t *testing.T) {
	_, r := setupTestHandler()
	w := doReq(r, "POST", "/v3/company/123/account", map[string]any{
		"Name":        "Sales Revenue",
		"AccountType": "Income",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create account: %d %s", w.Code, w.Body.String())
	}
	resp := parseResp(t, w)
	acct := resp["Account"].(map[string]any)
	if acct["Classification"] != "Revenue" {
		t.Errorf("Classification = %v, want Revenue", acct["Classification"])
	}
}

// --- Error Format ---

func TestFaultFormat(t *testing.T) {
	_, r := setupTestHandler()
	w := doReq(r, "GET", "/v3/company/123/customer/nonexistent", nil)
	resp := parseResp(t, w)
	fault, ok := resp["Fault"].(map[string]any)
	if !ok {
		t.Fatal("expected Fault envelope")
	}
	if fault["type"] != "ValidationFault" {
		t.Errorf("fault type = %v", fault["type"])
	}
	errors := fault["Error"].([]any)
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errors))
	}
	errObj := errors[0].(map[string]any)
	if errObj["code"] != "610" {
		t.Errorf("code = %v, want 610", errObj["code"])
	}
}

// --- NotFound ---

func TestNotFound(t *testing.T) {
	_, r := setupTestHandler()
	w := doReq(r, "GET", "/v3/company/123/invoice/999", nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (QBO returns 400 for not found)", w.Code)
	}
}

// --- Reports ---

func TestReports_TrialBalance(t *testing.T) {
	_, r := setupTestHandler()
	w := doReq(r, "GET", "/v3/company/123/reports/TrialBalance", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	resp := parseResp(t, w)
	if resp["Header"] == nil {
		t.Error("expected Header in report response")
	}
}

// --- Journal Entry Generation (GAP-001) ---

func TestJournalEntries_InvoiceCreatesEntries(t *testing.T) {
	h, r := setupTestHandler()

	// Create AR account.
	doReq(r, "POST", "/v3/company/123/account", map[string]any{
		"Name": "Accounts Receivable", "AccountType": "Accounts Receivable",
	})

	// Create income account.
	doReq(r, "POST", "/v3/company/123/account", map[string]any{
		"Name": "Sales", "AccountType": "Income",
	})

	// Create invoice — should generate journal entries.
	doReq(r, "POST", "/v3/company/123/invoice", map[string]any{
		"CustomerRef": map[string]any{"value": "1"},
		"Line": []map[string]any{{
			"Amount": 1000.00, "DetailType": "SalesItemLineDetail",
			"SalesItemLineDetail": map[string]any{"Qty": 1, "UnitPrice": 1000},
		}},
	})

	// Journal should have entries.
	txs := h.engine.Journal().Transactions()
	if len(txs) == 0 {
		t.Fatal("expected journal transactions after invoice create, got 0")
	}

	// Trial balance should now have data.
	w := doReq(r, "GET", "/v3/company/123/reports/TrialBalance", nil)
	resp := parseResp(t, w)
	rows := resp["Rows"]
	if rows == nil {
		t.Fatal("expected Rows in trial balance")
	}
	rowList, ok := rows.([]any)
	if !ok || len(rowList) == 0 {
		t.Error("expected non-empty trial balance rows after invoice creation")
	}
}

func TestJournalEntries_PaymentCreatesEntries(t *testing.T) {
	h, r := setupTestHandler()

	// Create accounts.
	doReq(r, "POST", "/v3/company/123/account", map[string]any{
		"Name": "Accounts Receivable", "AccountType": "Accounts Receivable",
	})
	doReq(r, "POST", "/v3/company/123/account", map[string]any{
		"Name": "Bank", "AccountType": "Bank",
	})

	// Create invoice.
	w := doReq(r, "POST", "/v3/company/123/invoice", map[string]any{
		"CustomerRef": map[string]any{"value": "1"},
		"Line": []map[string]any{{
			"Amount": 500.00, "DetailType": "SalesItemLineDetail",
			"SalesItemLineDetail": map[string]any{"Qty": 1, "UnitPrice": 500},
		}},
	})
	invID := parseResp(t, w)["Invoice"].(map[string]any)["Id"].(string)

	txsBefore := len(h.engine.Journal().Transactions())

	// Create payment with bank deposit.
	w = doReq(r, "POST", "/v3/company/123/payment", map[string]any{
		"CustomerRef":         map[string]any{"value": "1"},
		"TotalAmt":            500.00,
		"DepositToAccountRef": map[string]any{"value": "2"},
		"Line": []map[string]any{{
			"Amount":    500.00,
			"LinkedTxn": []map[string]any{{"TxnId": invID, "TxnType": "Invoice"}},
		}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("payment create: %d %s", w.Code, w.Body.String())
	}

	txsAfter := len(h.engine.Journal().Transactions())
	if txsAfter <= txsBefore {
		t.Errorf("expected new journal transaction from payment, before=%d after=%d", txsBefore, txsAfter)
	}
}

func TestJournalEntries_BillCreatesEntries(t *testing.T) {
	h, r := setupTestHandler()

	doReq(r, "POST", "/v3/company/123/account", map[string]any{
		"Name": "Accounts Payable", "AccountType": "Accounts Payable",
	})
	doReq(r, "POST", "/v3/company/123/account", map[string]any{
		"Name": "Office Supplies", "AccountType": "Expense",
	})

	txsBefore := len(h.engine.Journal().Transactions())

	doReq(r, "POST", "/v3/company/123/bill", map[string]any{
		"VendorRef": map[string]any{"value": "1"},
		"Line": []map[string]any{{
			"Amount": 200.00, "DetailType": "AccountBasedExpenseLineDetail",
			"AccountBasedExpenseLineDetail": map[string]any{
				"AccountRef": map[string]any{"value": "2"},
			},
		}},
	})

	txsAfter := len(h.engine.Journal().Transactions())
	if txsAfter <= txsBefore {
		t.Errorf("expected new journal transaction from bill, before=%d after=%d", txsBefore, txsAfter)
	}
}

func TestJournalEntries_ManualJournalEntry(t *testing.T) {
	h, r := setupTestHandler()

	doReq(r, "POST", "/v3/company/123/account", map[string]any{
		"Name": "Bank", "AccountType": "Bank",
	})
	doReq(r, "POST", "/v3/company/123/account", map[string]any{
		"Name": "Revenue", "AccountType": "Income",
	})

	txsBefore := len(h.engine.Journal().Transactions())

	doReq(r, "POST", "/v3/company/123/journalentry", map[string]any{
		"Line": []map[string]any{
			{"Amount": 500.00, "DetailType": "JournalEntryLineDetail",
				"JournalEntryLineDetail": map[string]any{
					"PostingType": "Debit", "AccountRef": map[string]any{"value": "1"},
				}},
			{"Amount": 500.00, "DetailType": "JournalEntryLineDetail",
				"JournalEntryLineDetail": map[string]any{
					"PostingType": "Credit", "AccountRef": map[string]any{"value": "2"},
				}},
		},
	})

	txsAfter := len(h.engine.Journal().Transactions())
	if txsAfter <= txsBefore {
		t.Errorf("expected new journal transaction from JE, before=%d after=%d", txsBefore, txsAfter)
	}
}
