package api

import (
	"net/http"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/store"
)

// rfc3339 is the form Plaid writes timestamps in: UTC, whole seconds.
func rfc3339(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// linkTokenCreate is POST /link/token/create. Link is not emulated, so the
// token is only issued and read back. Every documented user field is accepted
// (quirk AUTH-002: be more permissive than the real Sandbox).
func (h *Handler) linkTokenCreate(w http.ResponseWriter, r *http.Request) {
	call, _ := CallFrom(r)
	b := call.Body
	name, ok1 := asString(b["client_name"])
	lang, ok2 := asString(b["language"])
	codes, ok3 := asStrings(b["country_codes"])
	products, ok4 := asStrings(b["products"])
	hook, ok5 := asString(b["webhook"])
	redirect, ok6 := asString(b["redirect_uri"])
	update, ok7 := asString(b["access_token"])
	user, isObj := b["user"].(map[string]any)
	uid, ok8 := asString(user["client_user_id"])
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 || !ok7 || !ok8 || (b["user"] != nil && !isObj) {
		h.invalidBody(w)
		return
	}
	var missing []string
	for _, f := range []struct {
		field  string
		absent bool
	}{{"client_name", name == ""}, {"language", lang == ""}, {"country_codes", len(codes) == 0}, {"user", uid == ""}} {
		if f.absent {
			missing = append(missing, f.field)
		}
	}
	if len(missing) > 0 {
		h.writeMissing(w, missing)
		return
	}
	var itemID string
	if update != "" {
		it, err := h.store.ItemForAccessToken(update)
		if err != nil {
			h.writeAccessTokenError(w, err)
			return
		}
		itemID = it.ID
	}
	lt := h.store.CreateLinkToken(store.LinkToken{ClientName: name, Language: lang, CountryCodes: codes,
		Products: products, Webhook: hook, RedirectURI: redirect, UserID: uid, UpdateItemID: itemID})
	h.writeJSON(w, http.StatusOK, map[string]any{"link_token": lt.Token, "expiration": rfc3339(lt.ExpiresAt)})
}

// linkTokenGet is POST /link/token/get: the configuration a link token was
// created with.
func (h *Handler) linkTokenGet(w http.ResponseWriter, r *http.Request) {
	call, _ := CallFrom(r)
	token, ok := asString(call.Body["link_token"])
	if !ok {
		h.invalidBody(w)
		return
	}
	if token == "" {
		h.writeMissing(w, []string{"link_token"})
		return
	}
	lt, err := h.store.GetLinkToken(token)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_INPUT", "INVALID_LINK_TOKEN",
			"provided link token is invalid or has expired")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"link_token": lt.Token, "created_at": rfc3339(lt.CreatedAt), "expiration": rfc3339(lt.ExpiresAt),
		"metadata": map[string]any{
			"initial_products": orEmpty(lt.Products), "country_codes": lt.CountryCodes,
			"client_name": lt.ClientName, "language": lt.Language,
			"webhook": nullable(lt.Webhook), "redirect_uri": nullable(lt.RedirectURI),
		},
	})
}

// nullable is a required nullable field: its value, or null when empty.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
