package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/ledger/accounting"
	"github.com/wondertwin-ai/wondertwin/twin-qbo/internal/store"
)

func (h *Handler) CreateOrUpdateAccount(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		validationFault(w, "500", "Failed to read body", err.Error())
		return
	}
	var a store.Account
	if err := json.Unmarshal(body, &a); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}

	if a.Id != "" {
		existing, ok := h.store.Accounts.Get(a.Id)
		if !ok {
			notFoundFault(w, "Account", a.Id)
			return
		}
		if err := ValidateSyncToken(existing.SyncToken, a.SyncToken); err != nil {
			staleSyncTokenFault(w)
			return
		}
		if IsSparse(body) {
			merged, err := SparseUpdate(existing, body)
			if err != nil {
				validationFault(w, "500", "Sparse merge failed", err.Error())
				return
			}
			a = merged
		}
		a.SyncToken = IncrementSyncToken(existing.SyncToken)
		a.MetaData.CreateTime = existing.MetaData.CreateTime
		a.MetaData.LastUpdatedTime = h.store.Now()
		a.Domain = "QBO"
		a.Classification = classifyAccountType(a.AccountType)
		h.store.Accounts.Set(a.Id, a)
		h.fireEvent("Account", a.Id, "Update")
	} else {
		if a.Name == "" || a.AccountType == "" {
			validationFault(w, "3200", "Required field missing", "Name and AccountType are required.")
			return
		}
		a.Id = h.store.NextID()
		a.SyncToken = "0"
		a.Active = true
		a.Domain = "QBO"
		a.Classification = classifyAccountType(a.AccountType)
		a.MetaData = h.store.NewMetaData()

		// Register with accounting engine.
		h.engine.CreateAccount(accounting.Account{
			ID:       a.Id,
			Code:     a.AcctNum,
			Name:     a.Name,
			Type:     qboTypeToEngine(a.AccountType),
			Class:    qboClassToEngine(a.AccountType),
			Currency: refValue(a.CurrencyRef),
		})

		h.store.Accounts.Set(a.Id, a)
		h.fireEvent("Account", a.Id, "Create")
	}

	qboJSON(w, http.StatusOK, entityResponse("Account", a))
}

func (h *Handler) GetAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	a, ok := h.store.Accounts.Get(id)
	if !ok {
		notFoundFault(w, "Account", id)
		return
	}
	qboJSON(w, http.StatusOK, entityResponse("Account", a))
}

func classifyAccountType(acctType string) string {
	switch acctType {
	case "Bank", "Accounts Receivable", "Other Current Asset", "Fixed Asset", "Other Asset":
		return "Asset"
	case "Accounts Payable", "Credit Card", "Other Current Liability", "Long Term Liability":
		return "Liability"
	case "Equity":
		return "Equity"
	case "Income", "Other Income":
		return "Revenue"
	case "Cost of Goods Sold", "Expense", "Other Expense":
		return "Expense"
	default:
		return "Asset"
	}
}

func qboTypeToEngine(t string) accounting.AccountType {
	switch t {
	case "Bank", "Accounts Receivable", "Other Current Asset", "Fixed Asset", "Other Asset":
		return accounting.AccountTypeAsset
	case "Accounts Payable", "Credit Card", "Other Current Liability", "Long Term Liability":
		return accounting.AccountTypeLiability
	case "Equity":
		return accounting.AccountTypeEquity
	case "Income", "Other Income":
		return accounting.AccountTypeRevenue
	case "Cost of Goods Sold", "Expense", "Other Expense":
		return accounting.AccountTypeExpense
	default:
		return accounting.AccountTypeAsset
	}
}

func qboClassToEngine(acctType string) accounting.AccountClass {
	switch acctType {
	case "Bank", "Accounts Receivable", "Other Current Asset":
		return accounting.ClassCurrentAsset
	case "Fixed Asset", "Other Asset":
		return accounting.ClassFixedAsset
	case "Accounts Payable", "Credit Card", "Other Current Liability":
		return accounting.ClassCurrentLiability
	case "Long Term Liability":
		return accounting.ClassLongTermLiab
	case "Equity":
		return accounting.ClassEquity
	case "Income":
		return accounting.ClassRevenue
	case "Other Income":
		return accounting.ClassOtherIncome
	case "Cost of Goods Sold":
		return accounting.ClassDirectCost
	case "Expense":
		return accounting.ClassExpense
	case "Other Expense":
		return accounting.ClassOtherExpense
	default:
		return accounting.ClassCurrentAsset
	}
}

func refValue(r *store.Ref) string {
	if r == nil {
		return ""
	}
	return r.Value
}
