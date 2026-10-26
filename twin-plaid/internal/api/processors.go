package api

import (
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/seed"
	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/store"
)

// processorTarget reads access_token, account_id and, when wantProcessor, the
// processor from the request and resolves the Item and its account. It answers
// the error and returns ok false when it cannot. A processor token is for an
// Auth account, the checking or savings accounts that carry bank numbers; any
// other account answers NO_AUTH_ACCOUNTS, the nearest documented error
// (INFERRED: Plaid's own answer is not published).
func (h *Handler) processorTarget(w http.ResponseWriter, r *http.Request, wantProcessor bool) (it store.Item, acct store.Account, processor string, ok bool) {
	call, _ := CallFrom(r)
	token, ok1 := asString(call.Body["access_token"])
	accountID, ok2 := asString(call.Body["account_id"])
	processor, ok3 := asString(call.Body["processor"])
	if !ok1 || !ok2 || !ok3 {
		h.invalidBody(w)
		return
	}
	var missing []string
	for _, f := range []struct {
		field string
		blank bool
	}{{"access_token", token == ""}, {"account_id", accountID == ""}, {"processor", wantProcessor && processor == ""}} {
		if f.blank {
			missing = append(missing, f.field)
		}
	}
	if len(missing) > 0 {
		h.writeMissing(w, missing)
		return
	}
	if wantProcessor && !seed.IsProcessor(processor) {
		h.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "INVALID_FIELD", "processor "+processor+" is not a processor Plaid supports")
		return
	}
	if it, ok = h.liveItemFor(w, r); !ok {
		return
	}
	for _, a := range h.store.AccountsForItem(it.ID) {
		if a.ID == accountID {
			if a.Numbers == nil {
				e, _ := seed.LookupItemError("NO_AUTH_ACCOUNTS")
				h.writeItemError(w, e)
				return it, a, processor, false
			}
			return it, a, processor, true
		}
	}
	h.writeError(w, http.StatusBadRequest, "INVALID_INPUT", "INVALID_ACCOUNT_ID", "failed to find requested account ID for requested item")
	return it, acct, processor, false
}

// processorTokenCreate is POST /processor/token/create: a token a partner such
// as Dwolla or Adyen uses to read one account. It lasts until the Item is removed.
func (h *Handler) processorTokenCreate(w http.ResponseWriter, r *http.Request) {
	if it, acct, processor, ok := h.processorTarget(w, r, true); ok {
		t := h.store.CreateProcessorToken(it.ID, acct.ID, processor)
		h.writeJSON(w, http.StatusOK, map[string]any{"processor_token": t.Token})
	}
}

// processorStripeBankAccountTokenCreate is POST
// /processor/stripe/bank_account_token/create: a btok_ token for Stripe.
func (h *Handler) processorStripeBankAccountTokenCreate(w http.ResponseWriter, r *http.Request) {
	if it, acct, _, ok := h.processorTarget(w, r, false); ok {
		t := h.store.CreateStripeBankAccountToken(it.ID, acct.ID)
		h.writeJSON(w, http.StatusOK, map[string]any{"stripe_bank_account_token": t.Token})
	}
}
