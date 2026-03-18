package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
)

// BatchRequest is the QBO batch request format.
type BatchRequest struct {
	BatchItemRequest []BatchItemRequest `json:"BatchItemRequest"`
}

// BatchItemRequest is a single operation in a batch.
type BatchItemRequest struct {
	BId       string         `json:"bId"`
	Operation string         `json:"operation,omitempty"` // create, update, delete, query
	Query     string         `json:"Query,omitempty"`     // for query operations
	Entity    string         `json:"entity,omitempty"`    // entity type name (e.g., "Invoice")
	Body      map[string]any `json:"body,omitempty"`      // entity body for create/update
}

// BatchResponse is the QBO batch response format.
type BatchResponse struct {
	BatchItemResponse []BatchItemResponse `json:"BatchItemResponse"`
	Time              string              `json:"time"`
}

// BatchItemResponse is a single result in a batch response.
type BatchItemResponse struct {
	BId           string         `json:"bId"`
	Fault         *BatchFault    `json:"Fault,omitempty"`
	QueryResponse map[string]any `json:"QueryResponse,omitempty"`
	Entity        map[string]any `json:",omitempty"` // flattened entity fields
}

// BatchFault is an error in a batch item.
type BatchFault struct {
	Type  string          `json:"type"`
	Error []map[string]any `json:"Error"`
}

// Batch handles the QBO batch endpoint — up to 30 operations per request.
func (h *Handler) Batch(w http.ResponseWriter, r *http.Request) {
	var req BatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}

	if len(req.BatchItemRequest) > 30 {
		validationFault(w, "500", "Batch limit exceeded", "Maximum 30 operations per batch request.")
		return
	}

	var responses []map[string]any

	for _, item := range req.BatchItemRequest {
		result := h.executeBatchItem(r, item)
		result["bId"] = item.BId
		responses = append(responses, result)
	}

	qboJSON(w, http.StatusOK, map[string]any{
		"BatchItemResponse": responses,
		"time":              h.store.Now(),
	})
}

func (h *Handler) executeBatchItem(parentReq *http.Request, item BatchItemRequest) map[string]any {
	op := strings.ToLower(item.Operation)

	// Query operation.
	if item.Query != "" || op == "query" {
		query := item.Query
		if query == "" {
			return batchFault("ValidationFault", "500", "Query is required for query operations")
		}
		// Execute query internally by calling our query handler.
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/v3/company/0/query?query="+query, nil)
		req.Header = parentReq.Header.Clone()

		// Use chi context from parent.
		h.Query(rec, req)

		var resp map[string]any
		json.Unmarshal(rec.Body.Bytes(), &resp)
		if qr, ok := resp["QueryResponse"]; ok {
			return map[string]any{"QueryResponse": qr}
		}
		return resp
	}

	// Create/update/delete — route to the entity handler.
	entityName := strings.ToLower(item.Entity)
	if entityName == "" {
		return batchFault("ValidationFault", "500", "Entity type is required")
	}

	var method string
	path := "/v3/company/0/" + entityName
	switch op {
	case "create", "update", "":
		method = "POST"
	case "delete":
		method = "POST"
		path += "?operation=delete"
	default:
		return batchFault("ValidationFault", "500", "Unknown operation: "+item.Operation)
	}

	body, _ := json.Marshal(item.Body)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header = parentReq.Header.Clone()
	req.Header.Set("Content-Type", "application/json")

	h.routeEntityRequest(rec, req, entityName)

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	return resp
}

// routeEntityRequest dispatches to the correct handler based on entity name.
func (h *Handler) routeEntityRequest(w http.ResponseWriter, r *http.Request, entity string) {
	switch entity {
	case "customer":
		h.CreateOrUpdateCustomer(w, r)
	case "vendor":
		h.CreateOrUpdateVendor(w, r)
	case "account":
		h.CreateOrUpdateAccount(w, r)
	case "item":
		h.CreateOrUpdateItem(w, r)
	case "invoice":
		h.CreateOrUpdateInvoice(w, r)
	case "bill":
		h.CreateOrUpdateBill(w, r)
	case "payment":
		h.CreateOrUpdatePayment(w, r)
	case "billpayment":
		h.CreateOrUpdateBillPayment(w, r)
	case "creditmemo":
		h.CreateOrUpdateCreditMemo(w, r)
	case "vendorcredit":
		h.CreateOrUpdateVendorCredit(w, r)
	case "salesreceipt":
		h.CreateOrUpdateSalesReceipt(w, r)
	case "deposit":
		h.CreateOrUpdateDeposit(w, r)
	case "transfer":
		h.CreateOrUpdateTransfer(w, r)
	case "journalentry":
		h.CreateOrUpdateJournalEntry(w, r)
	case "estimate":
		h.CreateOrUpdateEstimate(w, r)
	case "purchase":
		h.CreateOrUpdatePurchase(w, r)
	case "employee":
		h.CreateOrUpdateEmployee(w, r)
	default:
		validationFault(w, "500", "Unknown entity", "Entity '"+entity+"' is not supported in batch.")
	}
}

func batchFault(faultType, code, message string) map[string]any {
	return map[string]any{
		"Fault": map[string]any{
			"type": faultType,
			"Error": []map[string]any{{
				"Message": message,
				"code":    code,
			}},
		},
	}
}
