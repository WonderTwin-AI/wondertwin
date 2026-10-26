package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"sort"

	"github.com/go-chi/chi/v5"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// The client registry is how strict_auth knows an app. Xero registers an app in
// its developer portal and asks a user to consent, so the emulator has admin
// routes instead: register a client, then issue an authorization code for it.

// RegisterClient serves POST /admin/clients.
func (h *Handler) RegisterClient(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ClientID     string   `json:"client_id"`
		ClientSecret string   `json:"client_secret"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		twincore.Error(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if in.ClientID == "" || in.ClientSecret == "" {
		twincore.Error(w, http.StatusBadRequest, "client_id and client_secret are required")
		return
	}
	if in.RedirectURIs == nil {
		in.RedirectURIs = []string{}
	}
	h.store.Clients.Set(in.ClientID, store.Client{Secret: in.ClientSecret, RedirectURIs: in.RedirectURIs})
	twincore.JSON(w, http.StatusCreated, map[string]any{"client_id": in.ClientID, "redirect_uris": in.RedirectURIs})
}

// ListClients serves GET /admin/clients, without the secrets.
func (h *Handler) ListClients(w http.ResponseWriter, _ *http.Request) {
	ids := h.store.Clients.ListIDs()
	sort.Strings(ids)
	out := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		c, _ := h.store.Clients.Get(id)
		out = append(out, map[string]any{"client_id": id, "redirect_uris": c.RedirectURIs})
	}
	twincore.JSON(w, http.StatusOK, map[string]any{"clients": out})
}

// DeleteClient serves DELETE /admin/clients/{id}.
func (h *Handler) DeleteClient(w http.ResponseWriter, r *http.Request) {
	if !h.store.Clients.Delete(chi.URLParam(r, "id")) {
		twincore.Error(w, http.StatusNotFound, "no such client")
		return
	}
	twincore.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// IssueAuthorizationCode serves POST /admin/authorization-codes: a single-use
// code for a registered client and one of its redirect URIs.
func (h *Handler) IssueAuthorizationCode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ClientID    string `json:"client_id"`
		RedirectURI string `json:"redirect_uri"`
		Scope       string `json:"scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		twincore.Error(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	client, ok := h.store.Clients.Get(in.ClientID)
	if !ok {
		twincore.Error(w, http.StatusNotFound, "no such client")
		return
	}
	if !slices.Contains(client.RedirectURIs, in.RedirectURI) {
		twincore.Error(w, http.StatusBadRequest, "redirect_uri is not registered for this client")
		return
	}
	if in.Scope == "" {
		in.Scope = consentScope
	}
	code := h.entropy() + h.entropy()
	h.store.AuthCodes.Set(code, store.AuthCode{ClientID: in.ClientID, RedirectURI: in.RedirectURI, Scope: in.Scope, AuthEventID: h.store.NewID()})
	twincore.JSON(w, http.StatusCreated, map[string]string{"code": code})
}
