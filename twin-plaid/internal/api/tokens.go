package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/seed"
	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/store"
)

// asString reads a JSON value that may be absent (nil) or a string. Any other
// type is a parse failure for Plaid (probe p19), so ok is false.
func asString(v any) (s string, ok bool) {
	if v == nil {
		return "", true
	}
	s, ok = v.(string)
	return
}

// asStrings reads a JSON value that may be absent or an array of strings.
func asStrings(v any) (out []string, ok bool) {
	if v == nil {
		return nil, true
	}
	list, isList := v.([]any)
	for _, e := range list {
		s, isStr := e.(string)
		if !isStr {
			return nil, false
		}
		out = append(out, s)
	}
	return out, isList
}

// sandboxPublicTokenCreate is POST /sandbox/public_token/create: Link
// bypassed for tests. It issues a public token; the Item is made when that
// token is exchanged (quirk LIFECYCLE-001).
func (h *Handler) sandboxPublicTokenCreate(w http.ResponseWriter, r *http.Request) {
	call, _ := CallFrom(r)
	inst, ok1 := asString(call.Body["institution_id"])
	products, ok2 := asStrings(call.Body["initial_products"])
	opts, isObj := call.Body["options"].(map[string]any)
	webhook, ok3 := asString(opts["webhook"])
	user, ok4 := asString(opts["override_username"])
	password, ok5 := asString(opts["override_password"])
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || (call.Body["options"] != nil && !isObj) {
		h.invalidBody(w)
		return
	}
	var missing []string
	if inst == "" {
		missing = append(missing, "institution_id")
	}
	if len(products) == 0 {
		missing = append(missing, "initial_products")
	}
	if len(missing) > 0 {
		h.writeMissing(w, missing)
		return
	}
	// The error_{CODE} password, with user_good, fails the create with that
	// documented error (quirk LIFECYCLE-005).
	if code, isErr := strings.CutPrefix(password, "error_"); isErr && (user == "" || user == "user_good") {
		if e, found := seed.LookupPasswordError(code); found {
			h.writeItemError(w, e)
			return
		}
	}
	pt := h.store.CreatePublicToken(inst, products, webhook)
	h.writeJSON(w, http.StatusOK, map[string]any{"public_token": pt.Token})
}

// itemPublicTokenExchange is POST /item/public_token/exchange: it spends a
// public token for an access token and the Item it was made for.
func (h *Handler) itemPublicTokenExchange(w http.ResponseWriter, r *http.Request) {
	call, _ := CallFrom(r)
	token, ok := asString(call.Body["public_token"])
	if !ok {
		h.invalidBody(w)
		return
	}
	if token == "" {
		h.writeMissing(w, []string{"public_token"})
		return
	}
	it, at, err := h.store.ExchangePublicToken(token)
	switch {
	case errors.Is(err, store.ErrUnknownToken):
		h.writeError(w, http.StatusBadRequest, "INVALID_INPUT", "INVALID_PUBLIC_TOKEN",
			"could not find matching public token")
	case errors.Is(err, store.ErrExpiredToken):
		h.writeError(w, http.StatusBadRequest, "INVALID_INPUT", "INVALID_PUBLIC_TOKEN",
			"provided public token has expired; public tokens expire 30 minutes after creation")
	default:
		h.seedAccounts(it.ID)
		h.writeJSON(w, http.StatusOK, map[string]any{"access_token": at.Token, "item_id": it.ID})
	}
}
