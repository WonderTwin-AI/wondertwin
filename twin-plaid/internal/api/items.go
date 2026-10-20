package api

import (
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/store"
)

// itemFor resolves the access_token in the request body to its Item. When it
// cannot, the error is written and ok is false: an absent token is
// MISSING_FIELDS, an unknown or removed one INVALID_ACCESS_TOKEN.
func (h *Handler) itemFor(w http.ResponseWriter, r *http.Request) (it store.Item, ok bool) {
	call, _ := CallFrom(r)
	token, isStr := asString(call.Body["access_token"])
	switch {
	case !isStr:
		h.invalidBody(w)
	case token == "":
		h.writeMissing(w, []string{"access_token"})
	default:
		if it, ok = h.store.ItemForAccessToken(token); !ok {
			h.writeError(w, http.StatusBadRequest, "INVALID_INPUT", "INVALID_ACCESS_TOKEN",
				"provided access token is invalid. Access tokens are in the format: access-<environment>-<identifier>")
		}
	}
	return it, ok
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
		ItemID: it.ID, InstitutionID: it.InstitutionID, Error: nil,
		AvailableProducts: []string{}, BilledProducts: orEmpty(it.BilledProducts),
		Products: it.Products, ConsentedProducts: it.ConsentedProducts, UpdateType: it.UpdateType,
	}
	if it.Webhook != "" {
		out.Webhook = &it.Webhook
	}
	return out
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
