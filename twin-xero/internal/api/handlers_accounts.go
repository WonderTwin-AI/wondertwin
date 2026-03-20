package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/ledger"
	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func (h *Handler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var req store.Account
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		xeroError(w, http.StatusBadRequest, "ValidationException", "Invalid JSON: "+err.Error())
		return
	}

	if req.Name == "" {
		xeroError(w, http.StatusBadRequest, "ValidationException", "Name is required")
		return
	}

	if req.AccountID == "" {
		req.AccountID = newUUID()
	}
	if req.Status == "" {
		req.Status = "ACTIVE"
	}

	// Register with the accounting engine for ledger tracking.
	engineAcct := ledger.Account{
		ID:       req.AccountID,
		Code:     req.Code,
		Name:     req.Name,
		Type:     xeroTypeToEngine(req.Type),
		Class:    xeroClassToEngine(req.Class, req.Type),
		Currency: req.Currency,
	}
	h.engine.CreateAccount(engineAcct)

	h.store.Accounts.Set(req.AccountID, req)

	xeroJSON(w, http.StatusOK, map[string]any{"Accounts": []store.Account{req}})
}

func (h *Handler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	all := h.store.Accounts.List()
	xeroJSON(w, http.StatusOK, map[string]any{"Accounts": paginate(all, page, pageSize)})
}

func (h *Handler) GetAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "AccountID")
	a, ok := h.store.Accounts.Get(id)
	if !ok {
		xeroError(w, http.StatusNotFound, "ValidationException", "Account not found")
		return
	}
	xeroJSON(w, http.StatusOK, map[string]any{"Accounts": []store.Account{a}})
}

func (h *Handler) UpdateAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "AccountID")
	existing, ok := h.store.Accounts.Get(id)
	if !ok {
		xeroError(w, http.StatusNotFound, "ValidationException", "Account not found")
		return
	}

	var req store.Account
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		xeroError(w, http.StatusBadRequest, "ValidationException", "Invalid JSON: "+err.Error())
		return
	}

	req.AccountID = id
	if req.Name == "" {
		req.Name = existing.Name
	}
	if req.Code == "" {
		req.Code = existing.Code
	}
	if req.Type == "" {
		req.Type = existing.Type
	}
	if req.Status == "" {
		req.Status = existing.Status
	}
	h.store.Accounts.Set(id, req)
	xeroJSON(w, http.StatusOK, map[string]any{"Accounts": []store.Account{req}})
}

func (h *Handler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "AccountID")
	acct, ok := h.store.Accounts.Get(id)
	if !ok {
		xeroError(w, http.StatusNotFound, "ValidationException", "Account not found")
		return
	}
	acct.Status = "ARCHIVED"
	h.store.Accounts.Set(id, acct)
	xeroJSON(w, http.StatusOK, map[string]any{"Accounts": []store.Account{acct}})
}

func xeroTypeToEngine(t string) ledger.AccountType {
	switch t {
	case "ASSET", "BANK", "CURRENT", "FIXED":
		return ledger.AccountTypeAsset
	case "EQUITY":
		return ledger.AccountTypeEquity
	case "EXPENSE", "DIRECTCOSTS", "OVERHEADS":
		return ledger.AccountTypeExpense
	case "LIABILITY", "CURRLIAB", "TERMLIAB":
		return ledger.AccountTypeLiability
	case "REVENUE", "SALES", "OTHERINCOME":
		return ledger.AccountTypeRevenue
	default:
		return ledger.AccountTypeAsset
	}
}

func xeroClassToEngine(class, acctType string) ledger.AccountClass {
	switch class {
	case "ASSET":
		return ledger.ClassCurrentAsset
	case "EQUITY":
		return ledger.ClassEquity
	case "EXPENSE":
		return ledger.ClassExpense
	case "LIABILITY":
		return ledger.ClassCurrentLiability
	case "REVENUE":
		return ledger.ClassRevenue
	default:
		// Infer from type.
		switch acctType {
		case "BANK":
			return ledger.ClassCurrentAsset
		case "REVENUE", "SALES":
			return ledger.ClassRevenue
		case "DIRECTCOSTS":
			return ledger.ClassDirectCost
		case "EXPENSE", "OVERHEADS":
			return ledger.ClassExpense
		case "CURRLIAB":
			return ledger.ClassCurrentLiability
		case "TERMLIAB":
			return ledger.ClassLongTermLiab
		default:
			return ledger.ClassCurrentAsset
		}
	}
}
