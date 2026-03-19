package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/ledger/accounting"
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
	engineAcct := accounting.Account{
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

func xeroTypeToEngine(t string) accounting.AccountType {
	switch t {
	case "ASSET", "BANK", "CURRENT", "FIXED":
		return accounting.AccountTypeAsset
	case "EQUITY":
		return accounting.AccountTypeEquity
	case "EXPENSE", "DIRECTCOSTS", "OVERHEADS":
		return accounting.AccountTypeExpense
	case "LIABILITY", "CURRLIAB", "TERMLIAB":
		return accounting.AccountTypeLiability
	case "REVENUE", "SALES", "OTHERINCOME":
		return accounting.AccountTypeRevenue
	default:
		return accounting.AccountTypeAsset
	}
}

func xeroClassToEngine(class, acctType string) accounting.AccountClass {
	switch class {
	case "ASSET":
		return accounting.ClassCurrentAsset
	case "EQUITY":
		return accounting.ClassEquity
	case "EXPENSE":
		return accounting.ClassExpense
	case "LIABILITY":
		return accounting.ClassCurrentLiability
	case "REVENUE":
		return accounting.ClassRevenue
	default:
		// Infer from type.
		switch acctType {
		case "BANK":
			return accounting.ClassCurrentAsset
		case "REVENUE", "SALES":
			return accounting.ClassRevenue
		case "DIRECTCOSTS":
			return accounting.ClassDirectCost
		case "EXPENSE", "OVERHEADS":
			return accounting.ClassExpense
		case "CURRLIAB":
			return accounting.ClassCurrentLiability
		case "TERMLIAB":
			return accounting.ClassLongTermLiab
		default:
			return accounting.ClassCurrentAsset
		}
	}
}
