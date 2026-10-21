package api

import (
	"errors"
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/seed"
	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/store"
)

// itemFor resolves the access_token in the request body to its Item. When it
// cannot, the error is written and ok is false: an absent token is
// MISSING_FIELDS, an unknown one INVALID_ACCESS_TOKEN, and one whose Item was
// removed ITEM_NOT_FOUND.
func (h *Handler) itemFor(w http.ResponseWriter, r *http.Request) (it store.Item, ok bool) {
	call, _ := CallFrom(r)
	token, isStr := asString(call.Body["access_token"])
	switch {
	case !isStr:
		h.invalidBody(w)
	case token == "":
		h.writeMissing(w, []string{"access_token"})
	default:
		var err error
		if it, err = h.store.ItemForAccessToken(token); err != nil {
			h.writeAccessTokenError(w, err)
			return it, false
		}
		return it, true
	}
	return it, false
}

// writeAccessTokenError answers a token that does not resolve to a live Item.
func (h *Handler) writeAccessTokenError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrItemRemoved) {
		h.writeError(w, http.StatusBadRequest, "ITEM_ERROR", "ITEM_NOT_FOUND",
			"the Item you requested cannot be found. This Item does not exist, has been previously removed via /item/remove, or has had access removed by the user")
		return
	}
	h.writeError(w, http.StatusBadRequest, "INVALID_INPUT", "INVALID_ACCESS_TOKEN",
		"provided access token is invalid. Access tokens are in the format: access-<environment>-<identifier>")
}

// itemJSON is the Item object. The tags carry the presence rules (quirk
// ENVELOPE-004): optional fields are omitted when empty, never null; required
// nullable fields are always written, as null when empty.
type itemJSON struct {
	ItemID                string   `json:"item_id"`
	InstitutionID         string   `json:"institution_id,omitempty"`
	Webhook               *string  `json:"webhook"`
	Error                 any      `json:"error"`
	AvailableProducts     []string `json:"available_products"`
	BilledProducts        []string `json:"billed_products"`
	Products              []string `json:"products,omitempty"`
	ConsentedProducts     []string `json:"consented_products,omitempty"`
	ConsentExpirationTime *string  `json:"consent_expiration_time"`
	UpdateType            string   `json:"update_type"`
}

func newItemJSON(it store.Item) itemJSON {
	out := itemJSON{
		ItemID: it.ID, InstitutionID: it.InstitutionID, Error: itemErrorObject(it.ErrorCode),
		AvailableProducts: []string{}, BilledProducts: orEmpty(it.BilledProducts),
		Products: it.Products, ConsentedProducts: it.ConsentedProducts, UpdateType: it.UpdateType,
	}
	if it.Webhook != "" {
		out.Webhook = &it.Webhook
	}
	return out
}

// itemErrorObject is the Item's error field: null when healthy, else the
// documented error without a request_id, as Plaid omits it where an error is
// not an answer to a request.
func itemErrorObject(code string) any {
	e, ok := seed.LookupItemError(code)
	if !ok {
		return nil
	}
	return map[string]any{"error_type": e.Type, "error_code": e.Code, "error_message": e.Message,
		"display_message": e.DisplayMessage}
}

// writeItemError answers with a documented error at its documented status.
func (h *Handler) writeItemError(w http.ResponseWriter, e seed.ItemError) {
	h.writeJSON(w, e.HTTP, map[string]any{
		"display_message": e.DisplayMessage, "documentation_url": documentationURL(e.Type, e.Code),
		"error_code": e.Code, "error_message": e.Message, "error_type": e.Type, "suggested_action": nil,
	})
}

// liveItemFor is itemFor for routes that read an Item's data: an Item in an
// error state answers that error instead (quirk LIFECYCLE-005).
func (h *Handler) liveItemFor(w http.ResponseWriter, r *http.Request) (store.Item, bool) {
	it, ok := h.itemFor(w, r)
	if ok && it.ErrorCode != "" {
		if e, found := seed.LookupItemError(it.ErrorCode); found {
			h.writeItemError(w, e)
			return it, false
		}
	}
	return it, ok
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// itemGet is POST /item/get.
func (h *Handler) itemGet(w http.ResponseWriter, r *http.Request) {
	if it, ok := h.itemFor(w, r); ok {
		h.writeJSON(w, http.StatusOK, map[string]any{"item": newItemJSON(it)})
	}
}

// itemRemove is POST /item/remove. The Item and its accounts go; the access
// token stays on record so later calls answer ITEM_NOT_FOUND (quirk LIFECYCLE-002).
func (h *Handler) itemRemove(w http.ResponseWriter, r *http.Request) {
	if it, ok := h.itemFor(w, r); ok {
		h.store.RemoveItem(it.ID)
		h.writeJSON(w, http.StatusOK, map[string]any{})
	}
}

// itemAccessTokenInvalidate is POST /item/access_token/invalidate: it swaps the
// access token for a new one, and the old one stops working.
func (h *Handler) itemAccessTokenInvalidate(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.itemFor(w, r); !ok {
		return
	}
	call, _ := CallFrom(r)
	at, err := h.store.RotateAccessToken(call.Body["access_token"].(string))
	if err != nil {
		h.writeAccessTokenError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"new_access_token": at.Token})
}

// sandboxItemResetLogin is POST /sandbox/item/reset_login: it forces an Item
// into ITEM_LOGIN_REQUIRED, the state update mode repairs.
func (h *Handler) sandboxItemResetLogin(w http.ResponseWriter, r *http.Request) {
	if it, ok := h.itemFor(w, r); ok {
		h.store.SetItemError(it.ID, "ITEM_LOGIN_REQUIRED")
		h.writeJSON(w, http.StatusOK, map[string]any{"reset_login": true})
	}
}
