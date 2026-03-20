package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/ledger"
	"github.com/wondertwin-ai/wondertwin/twinkit/state/journal"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-qbo/internal/store"
)

func setupTestHandler() (*Handler, *chi.Mux) {
	memStore := store.New()
	j := journal.New(memStore.Clock)
	engine := ledger.NewEngine(
		ledger.WithJournal(j),
		ledger.WithClock(memStore.Clock),
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

func TestCustomer_SparseUpdate(t *testing.T) {
	_, r := setupTestHandler()
	// Create with full fields.
	w := doReq(r, "POST", "/v3/company/123/customer", map[string]any{
		"DisplayName": "Alice Smith",
		"GivenName":   "Alice",
		"FamilyName":  "Smith",
		"CompanyName": "Acme Corp",
	})
	resp := parseResp(t, w)
	id := resp["Customer"].(map[string]any)["Id"].(string)

	// Sparse update — only change DisplayName, keep everything else.
	w = doReq(r, "POST", "/v3/company/123/customer", map[string]any{
		"Id":          id,
		"SyncToken":   "0",
		"sparse":      true,
		"DisplayName": "Alice Johnson",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("sparse update: %d %s", w.Code, w.Body.String())
	}
	resp = parseResp(t, w)
	cust := resp["Customer"].(map[string]any)
	if cust["DisplayName"] != "Alice Johnson" {
		t.Errorf("DisplayName = %v, want 'Alice Johnson'", cust["DisplayName"])
	}
	// GivenName should be preserved from the original.
	if cust["GivenName"] != "Alice" {
		t.Errorf("GivenName = %v, want 'Alice' (should be preserved in sparse update)", cust["GivenName"])
	}
	if cust["CompanyName"] != "Acme Corp" {
		t.Errorf("CompanyName = %v, want 'Acme Corp' (should be preserved)", cust["CompanyName"])
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

func TestQuery_WhereFilter(t *testing.T) {
	_, r := setupTestHandler()
	doReq(r, "POST", "/v3/company/123/customer", map[string]any{"DisplayName": "Alice"})
	doReq(r, "POST", "/v3/company/123/customer", map[string]any{"DisplayName": "Bob"})
	doReq(r, "POST", "/v3/company/123/customer", map[string]any{"DisplayName": "Charlie"})

	// Filter by DisplayName equality.
	w := doReq(r, "GET", queryURL("SELECT * FROM Customer WHERE DisplayName = 'Bob'"), nil)
	resp := parseResp(t, w)
	qr := resp["QueryResponse"].(map[string]any)
	custs := qr["Customer"].([]any)
	if len(custs) != 1 {
		t.Fatalf("WHERE DisplayName='Bob': expected 1, got %d", len(custs))
	}
	if custs[0].(map[string]any)["DisplayName"] != "Bob" {
		t.Error("wrong customer returned")
	}
}

func TestQuery_WhereLike(t *testing.T) {
	_, r := setupTestHandler()
	doReq(r, "POST", "/v3/company/123/customer", map[string]any{"DisplayName": "Alice Smith"})
	doReq(r, "POST", "/v3/company/123/customer", map[string]any{"DisplayName": "Alice Jones"})
	doReq(r, "POST", "/v3/company/123/customer", map[string]any{"DisplayName": "Bob Smith"})

	w := doReq(r, "GET", queryURL("SELECT * FROM Customer WHERE DisplayName LIKE 'Alice%'"), nil)
	resp := parseResp(t, w)
	qr := resp["QueryResponse"].(map[string]any)
	custs := qr["Customer"].([]any)
	if len(custs) != 2 {
		t.Fatalf("WHERE LIKE 'Alice%%': expected 2, got %d", len(custs))
	}
}

func TestQuery_WhereBalance(t *testing.T) {
	_, r := setupTestHandler()
	// Create two invoices with different amounts.
	doReq(r, "POST", "/v3/company/123/invoice", map[string]any{
		"CustomerRef": map[string]any{"value": "1"},
		"Line": []map[string]any{{"Amount": 100.00, "DetailType": "SalesItemLineDetail",
			"SalesItemLineDetail": map[string]any{"Qty": 1, "UnitPrice": 100}}},
	})
	doReq(r, "POST", "/v3/company/123/invoice", map[string]any{
		"CustomerRef": map[string]any{"value": "1"},
		"Line": []map[string]any{{"Amount": 500.00, "DetailType": "SalesItemLineDetail",
			"SalesItemLineDetail": map[string]any{"Qty": 1, "UnitPrice": 500}}},
	})

	// Filter Balance > 200.
	w := doReq(r, "GET", queryURL("SELECT * FROM Invoice WHERE Balance > '200'"), nil)
	resp := parseResp(t, w)
	qr := resp["QueryResponse"].(map[string]any)
	invs := qr["Invoice"].([]any)
	if len(invs) != 1 {
		t.Fatalf("WHERE Balance > 200: expected 1, got %d", len(invs))
	}
	if invs[0].(map[string]any)["TotalAmt"].(float64) != 500 {
		t.Error("wrong invoice returned")
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

// --- Void/Delete Operations (GAP-003) ---

func TestBill_Void(t *testing.T) {
	_, r := setupTestHandler()
	w := doReq(r, "POST", "/v3/company/123/bill", map[string]any{
		"VendorRef": map[string]any{"value": "1"},
		"Line": []map[string]any{{"Amount": 200.00, "DetailType": "AccountBasedExpenseLineDetail",
			"AccountBasedExpenseLineDetail": map[string]any{"AccountRef": map[string]any{"value": "1"}}}},
	})
	resp := parseResp(t, w)
	billID := resp["Bill"].(map[string]any)["Id"].(string)

	w = doReq(r, "POST", "/v3/company/123/bill?operation=void", map[string]any{
		"Id": billID, "SyncToken": "0",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("void bill: %d %s", w.Code, w.Body.String())
	}
	resp = parseResp(t, w)
	bill := resp["Bill"].(map[string]any)
	if bill["Balance"].(float64) != 0 {
		t.Errorf("voided Balance = %v, want 0", bill["Balance"])
	}
	if bill["TotalAmt"].(float64) != 0 {
		t.Errorf("voided TotalAmt = %v, want 0", bill["TotalAmt"])
	}
}

func TestCreditMemo_Delete(t *testing.T) {
	_, r := setupTestHandler()
	w := doReq(r, "POST", "/v3/company/123/creditmemo", map[string]any{
		"CustomerRef": map[string]any{"value": "1"},
		"Line": []map[string]any{{"Amount": 50.00, "DetailType": "SalesItemLineDetail",
			"SalesItemLineDetail": map[string]any{"Qty": 1, "UnitPrice": 50}}},
	})
	cmID := parseResp(t, w)["CreditMemo"].(map[string]any)["Id"].(string)

	w = doReq(r, "POST", "/v3/company/123/creditmemo?operation=delete", map[string]any{
		"Id": cmID, "SyncToken": "0",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("delete credit memo: %d %s", w.Code, w.Body.String())
	}

	// Should be gone.
	w = doReq(r, "GET", "/v3/company/123/creditmemo/"+cmID, nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 after delete, got %d", w.Code)
	}
}

func TestSalesReceipt_Void(t *testing.T) {
	_, r := setupTestHandler()
	w := doReq(r, "POST", "/v3/company/123/salesreceipt", map[string]any{
		"CustomerRef": map[string]any{"value": "1"},
		"Line": []map[string]any{{"Amount": 75.00, "DetailType": "SalesItemLineDetail",
			"SalesItemLineDetail": map[string]any{"Qty": 1, "UnitPrice": 75}}},
	})
	srID := parseResp(t, w)["SalesReceipt"].(map[string]any)["Id"].(string)

	w = doReq(r, "POST", "/v3/company/123/salesreceipt?operation=void", map[string]any{
		"Id": srID, "SyncToken": "0",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("void sales receipt: %d %s", w.Code, w.Body.String())
	}
	resp := parseResp(t, w)
	sr := resp["SalesReceipt"].(map[string]any)
	if sr["TotalAmt"].(float64) != 0 {
		t.Errorf("voided TotalAmt = %v, want 0", sr["TotalAmt"])
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

// --- Journal Reversals on Void ---

func TestInvoice_VoidReversesJournal(t *testing.T) {
	h, r := setupTestHandler()
	// Create AR account.
	doReq(r, "POST", "/v3/company/123/account", map[string]any{
		"Name": "Accounts Receivable", "AccountType": "Accounts Receivable",
	})

	// Create invoice.
	w := doReq(r, "POST", "/v3/company/123/invoice", map[string]any{
		"CustomerRef": map[string]any{"value": "1"},
		"Line": []map[string]any{{
			"Amount": 200.00, "DetailType": "SalesItemLineDetail",
			"SalesItemLineDetail": map[string]any{"Qty": 1, "UnitPrice": 200},
		}},
	})
	invID := parseResp(t, w)["Invoice"].(map[string]any)["Id"].(string)
	txsBefore := len(h.engine.Journal().Transactions())

	// Void — should add reversing journal entry.
	doReq(r, "POST", "/v3/company/123/invoice?operation=void", map[string]any{
		"Id": invID, "SyncToken": "0",
	})
	txsAfter := len(h.engine.Journal().Transactions())
	if txsAfter <= txsBefore {
		t.Errorf("expected reversing journal entry on void, before=%d after=%d", txsBefore, txsAfter)
	}
}

func TestBill_VoidReversesJournal(t *testing.T) {
	h, r := setupTestHandler()
	doReq(r, "POST", "/v3/company/123/account", map[string]any{
		"Name": "Accounts Payable", "AccountType": "Accounts Payable",
	})
	w := doReq(r, "POST", "/v3/company/123/bill", map[string]any{
		"VendorRef": map[string]any{"value": "1"},
		"Line": []map[string]any{{"Amount": 150.00, "DetailType": "AccountBasedExpenseLineDetail",
			"AccountBasedExpenseLineDetail": map[string]any{"AccountRef": map[string]any{"value": "1"}}}},
	})
	billID := parseResp(t, w)["Bill"].(map[string]any)["Id"].(string)
	txsBefore := len(h.engine.Journal().Transactions())

	doReq(r, "POST", "/v3/company/123/bill?operation=void", map[string]any{
		"Id": billID, "SyncToken": "0",
	})
	txsAfter := len(h.engine.Journal().Transactions())
	if txsAfter <= txsBefore {
		t.Errorf("expected reversing journal on bill void, before=%d after=%d", txsBefore, txsAfter)
	}
}

func TestPayment_VoidReversesJournal(t *testing.T) {
	h, r := setupTestHandler()
	// Create invoice first.
	w := doReq(r, "POST", "/v3/company/123/invoice", map[string]any{
		"CustomerRef": map[string]any{"value": "1"},
		"Line": []map[string]any{{
			"Amount": 300.00, "DetailType": "SalesItemLineDetail",
			"SalesItemLineDetail": map[string]any{"Qty": 1, "UnitPrice": 300},
		}},
	})
	invID := parseResp(t, w)["Invoice"].(map[string]any)["Id"].(string)

	// Create payment.
	w = doReq(r, "POST", "/v3/company/123/payment", map[string]any{
		"CustomerRef": map[string]any{"value": "1"},
		"TotalAmt":    300.00,
		"Line": []map[string]any{{
			"Amount":    300.00,
			"LinkedTxn": []map[string]any{{"TxnId": invID, "TxnType": "Invoice"}},
		}},
	})
	pmtID := parseResp(t, w)["Payment"].(map[string]any)["Id"].(string)
	txsBefore := len(h.engine.Journal().Transactions())

	// Void payment.
	doReq(r, "POST", "/v3/company/123/payment?operation=void", map[string]any{
		"Id": pmtID, "SyncToken": "0",
	})
	txsAfter := len(h.engine.Journal().Transactions())
	if txsAfter <= txsBefore {
		t.Errorf("expected reversing journal on payment void, before=%d after=%d", txsBefore, txsAfter)
	}
}

// --- Sparse Updates ---

func TestInvoice_SparseUpdate(t *testing.T) {
	_, r := setupTestHandler()
	// Create invoice with DocNumber.
	w := doReq(r, "POST", "/v3/company/123/invoice", map[string]any{
		"CustomerRef": map[string]any{"value": "1"},
		"DocNumber":   "INV-001",
		"Line": []map[string]any{{
			"Amount": 100.00, "DetailType": "SalesItemLineDetail",
			"SalesItemLineDetail": map[string]any{"Qty": 1, "UnitPrice": 100},
		}},
	})
	resp := parseResp(t, w)
	invID := resp["Invoice"].(map[string]any)["Id"].(string)

	// Sparse update — only change DocNumber.
	w = doReq(r, "POST", "/v3/company/123/invoice", map[string]any{
		"Id": invID, "SyncToken": "0", "sparse": true,
		"DocNumber": "INV-002",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("sparse update: %d %s", w.Code, w.Body.String())
	}
	resp = parseResp(t, w)
	inv := resp["Invoice"].(map[string]any)
	if inv["DocNumber"] != "INV-002" {
		t.Errorf("DocNumber = %v, want INV-002", inv["DocNumber"])
	}
	// Customer ref should be preserved.
	custRef := inv["CustomerRef"].(map[string]any)
	if custRef["value"] != "1" {
		t.Errorf("CustomerRef.value = %v, want '1' (should be preserved in sparse update)", custRef["value"])
	}
}

func TestVendor_SparseUpdate(t *testing.T) {
	_, r := setupTestHandler()
	w := doReq(r, "POST", "/v3/company/123/vendor", map[string]any{
		"DisplayName": "Acme Vendor",
		"CompanyName": "Acme Inc",
	})
	resp := parseResp(t, w)
	id := resp["Vendor"].(map[string]any)["Id"].(string)

	w = doReq(r, "POST", "/v3/company/123/vendor", map[string]any{
		"Id": id, "SyncToken": "0", "sparse": true,
		"DisplayName": "Acme Vendor Updated",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("sparse update: %d %s", w.Code, w.Body.String())
	}
	resp = parseResp(t, w)
	v := resp["Vendor"].(map[string]any)
	if v["DisplayName"] != "Acme Vendor Updated" {
		t.Error("DisplayName not updated")
	}
	if v["CompanyName"] != "Acme Inc" {
		t.Errorf("CompanyName = %v, want 'Acme Inc' (should be preserved)", v["CompanyName"])
	}
}

// --- Estimate Conversion ---

func TestEstimate_ConvertToInvoice(t *testing.T) {
	_, r := setupTestHandler()
	// Create estimate.
	w := doReq(r, "POST", "/v3/company/123/estimate", map[string]any{
		"CustomerRef": map[string]any{"value": "1"},
		"Line": []map[string]any{{
			"Amount": 750.00, "DetailType": "SalesItemLineDetail",
			"SalesItemLineDetail": map[string]any{"Qty": 3, "UnitPrice": 250},
		}},
	})
	estID := parseResp(t, w)["Estimate"].(map[string]any)["Id"].(string)

	// Convert to invoice.
	w = doReq(r, "POST", "/v3/company/123/estimate/"+estID+"/convert", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("convert: %d %s", w.Code, w.Body.String())
	}
	resp := parseResp(t, w)
	inv := resp["Invoice"].(map[string]any)
	if inv["TotalAmt"].(float64) != 750 {
		t.Errorf("Invoice TotalAmt = %v, want 750", inv["TotalAmt"])
	}

	// Verify estimate is now Closed.
	w = doReq(r, "GET", "/v3/company/123/estimate/"+estID, nil)
	resp = parseResp(t, w)
	est := resp["Estimate"].(map[string]any)
	if est["TxnStatus"] != "Closed" {
		t.Errorf("TxnStatus = %v, want Closed", est["TxnStatus"])
	}
	if est["LinkedTxnId"] == nil || est["LinkedTxnId"] == "" {
		t.Error("LinkedTxnId should be set to the new invoice ID")
	}
}

func TestEstimate_InvalidStatusTransition(t *testing.T) {
	_, r := setupTestHandler()
	w := doReq(r, "POST", "/v3/company/123/estimate", map[string]any{
		"CustomerRef": map[string]any{"value": "1"},
		"Line": []map[string]any{{"Amount": 100.00, "DetailType": "SalesItemLineDetail",
			"SalesItemLineDetail": map[string]any{"Qty": 1, "UnitPrice": 100}}},
	})
	estID := parseResp(t, w)["Estimate"].(map[string]any)["Id"].(string)

	// Close it.
	doReq(r, "POST", "/v3/company/123/estimate", map[string]any{
		"Id": estID, "SyncToken": "0", "TxnStatus": "Closed",
		"CustomerRef": map[string]any{"value": "1"},
		"Line": []map[string]any{{"Amount": 100.00, "DetailType": "SalesItemLineDetail",
			"SalesItemLineDetail": map[string]any{"Qty": 1, "UnitPrice": 100}}},
	})

	// Try to reopen — should fail.
	w = doReq(r, "POST", "/v3/company/123/estimate", map[string]any{
		"Id": estID, "SyncToken": "1", "TxnStatus": "Pending",
		"CustomerRef": map[string]any{"value": "1"},
		"Line": []map[string]any{{"Amount": 100.00, "DetailType": "SalesItemLineDetail",
			"SalesItemLineDetail": map[string]any{"Qty": 1, "UnitPrice": 100}}},
	})
	if w.Code == http.StatusOK {
		resp := parseResp(t, w)
		if _, hasFault := resp["Fault"]; !hasFault {
			t.Error("expected rejection of Closed → Pending transition")
		}
	}
}

// --- Customer/Vendor Balance Tracking ---

func TestCustomer_BalanceUpdatedByInvoice(t *testing.T) {
	_, r := setupTestHandler()
	// Create customer.
	w := doReq(r, "POST", "/v3/company/123/customer", map[string]any{
		"DisplayName": "Balance Test",
	})
	custID := parseResp(t, w)["Customer"].(map[string]any)["Id"].(string)

	// Create invoice for 400.
	doReq(r, "POST", "/v3/company/123/invoice", map[string]any{
		"CustomerRef": map[string]any{"value": custID},
		"Line": []map[string]any{{
			"Amount": 400.00, "DetailType": "SalesItemLineDetail",
			"SalesItemLineDetail": map[string]any{"Qty": 1, "UnitPrice": 400},
		}},
	})

	// Customer balance should be 400.
	w = doReq(r, "GET", "/v3/company/123/customer/"+custID, nil)
	resp := parseResp(t, w)
	cust := resp["Customer"].(map[string]any)
	if cust["Balance"].(float64) != 400 {
		t.Errorf("Customer Balance = %v, want 400", cust["Balance"])
	}
}

func TestCustomer_BalanceReducedByPayment(t *testing.T) {
	_, r := setupTestHandler()
	w := doReq(r, "POST", "/v3/company/123/customer", map[string]any{
		"DisplayName": "Pay Test",
	})
	custID := parseResp(t, w)["Customer"].(map[string]any)["Id"].(string)

	// Create invoice.
	w = doReq(r, "POST", "/v3/company/123/invoice", map[string]any{
		"CustomerRef": map[string]any{"value": custID},
		"Line": []map[string]any{{
			"Amount": 600.00, "DetailType": "SalesItemLineDetail",
			"SalesItemLineDetail": map[string]any{"Qty": 1, "UnitPrice": 600},
		}},
	})
	invID := parseResp(t, w)["Invoice"].(map[string]any)["Id"].(string)

	// Pay 600.
	doReq(r, "POST", "/v3/company/123/payment", map[string]any{
		"CustomerRef": map[string]any{"value": custID},
		"TotalAmt":    600.00,
		"Line": []map[string]any{{
			"Amount":    600.00,
			"LinkedTxn": []map[string]any{{"TxnId": invID, "TxnType": "Invoice"}},
		}},
	})

	// Customer balance should be 0.
	w = doReq(r, "GET", "/v3/company/123/customer/"+custID, nil)
	resp := parseResp(t, w)
	if resp["Customer"].(map[string]any)["Balance"].(float64) != 0 {
		t.Errorf("Customer Balance after payment = %v, want 0", resp["Customer"].(map[string]any)["Balance"])
	}
}

func TestVendor_BalanceUpdatedByBill(t *testing.T) {
	_, r := setupTestHandler()
	w := doReq(r, "POST", "/v3/company/123/vendor", map[string]any{
		"DisplayName": "Bill Vendor",
	})
	vendorID := parseResp(t, w)["Vendor"].(map[string]any)["Id"].(string)

	doReq(r, "POST", "/v3/company/123/bill", map[string]any{
		"VendorRef": map[string]any{"value": vendorID},
		"Line": []map[string]any{{"Amount": 250.00, "DetailType": "AccountBasedExpenseLineDetail",
			"AccountBasedExpenseLineDetail": map[string]any{"AccountRef": map[string]any{"value": "1"}}}},
	})

	w = doReq(r, "GET", "/v3/company/123/vendor/"+vendorID, nil)
	resp := parseResp(t, w)
	if resp["Vendor"].(map[string]any)["Balance"].(float64) != 250 {
		t.Errorf("Vendor Balance = %v, want 250", resp["Vendor"].(map[string]any)["Balance"])
	}
}

// --- Credit Memo Application ---

func TestCreditMemo_ApplyToInvoice(t *testing.T) {
	_, r := setupTestHandler()
	// Create invoice.
	w := doReq(r, "POST", "/v3/company/123/invoice", map[string]any{
		"CustomerRef": map[string]any{"value": "1"},
		"Line": []map[string]any{{
			"Amount": 500.00, "DetailType": "SalesItemLineDetail",
			"SalesItemLineDetail": map[string]any{"Qty": 1, "UnitPrice": 500},
		}},
	})
	invID := parseResp(t, w)["Invoice"].(map[string]any)["Id"].(string)

	// Create credit memo for 100.
	w = doReq(r, "POST", "/v3/company/123/creditmemo", map[string]any{
		"CustomerRef": map[string]any{"value": "1"},
		"Line": []map[string]any{{
			"Amount": 100.00, "DetailType": "SalesItemLineDetail",
			"SalesItemLineDetail": map[string]any{"Qty": 1, "UnitPrice": 100},
		}},
	})
	cmID := parseResp(t, w)["CreditMemo"].(map[string]any)["Id"].(string)

	// Apply credit memo to invoice.
	w = doReq(r, "POST", "/v3/company/123/creditmemo", map[string]any{
		"Id": cmID, "SyncToken": "0",
		"CustomerRef": map[string]any{"value": "1"},
		"Line": []map[string]any{{"Amount": 100.00, "DetailType": "SalesItemLineDetail",
			"SalesItemLineDetail": map[string]any{"Qty": 1, "UnitPrice": 100}}},
		"LinkedTxn": []map[string]any{{"TxnId": invID, "TxnType": "Invoice"}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("apply credit memo: %d %s", w.Code, w.Body.String())
	}

	// Invoice balance should be reduced.
	w = doReq(r, "GET", "/v3/company/123/invoice/"+invID, nil)
	resp := parseResp(t, w)
	inv := resp["Invoice"].(map[string]any)
	if inv["Balance"].(float64) != 400 {
		t.Errorf("Invoice Balance after credit = %v, want 400", inv["Balance"])
	}
}

// --- Recurring Transaction CRUD ---

func TestRecurringTransaction_CreateAndGet(t *testing.T) {
	_, r := setupTestHandler()
	w := doReq(r, "POST", "/v3/company/123/recurringtransaction", map[string]any{
		"TemplateName": "Monthly Rent",
		"TemplateType": "Bill",
		"Schedule":     "Monthly",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create recurring: %d %s", w.Code, w.Body.String())
	}
	resp := parseResp(t, w)
	rt := resp["RecurringTransaction"].(map[string]any)
	id := rt["Id"].(string)
	if rt["TemplateName"] != "Monthly Rent" {
		t.Error("TemplateName mismatch")
	}

	// Get
	w = doReq(r, "GET", "/v3/company/123/recurringtransaction/"+id, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get recurring: %d", w.Code)
	}
}

// --- Batch Endpoint Hardening ---

func TestBatch_PerOperationError(t *testing.T) {
	_, r := setupTestHandler()
	// Batch with one valid create and one invalid (missing body for create).
	w := doReq(r, "POST", "/v3/company/123/batch", map[string]any{
		"BatchItemRequest": []map[string]any{
			{
				"bId": "1", "operation": "create", "entity": "customer",
				"body": map[string]any{"DisplayName": "Batch Customer"},
			},
			{
				"bId": "2", "operation": "create", "entity": "customer",
				// No body — should fail.
			},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("batch: %d %s", w.Code, w.Body.String())
	}
	resp := parseResp(t, w)
	items := resp["BatchItemResponse"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 batch responses, got %d", len(items))
	}

	// First item should succeed.
	item1 := items[0].(map[string]any)
	if _, hasFault := item1["Fault"]; hasFault {
		t.Error("first batch item should have succeeded")
	}

	// Second item should have a fault.
	item2 := items[1].(map[string]any)
	if _, hasFault := item2["Fault"]; !hasFault {
		t.Error("second batch item should have a fault (nil body)")
	}
}
