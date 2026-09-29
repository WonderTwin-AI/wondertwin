package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// appFromJWT checks the JSON web token a GitHub App authenticates with and
// returns the app it names. It checks what GitHub checks about the claims
// (an issuer that is the app's ID or client ID, an expiry in the future and
// at most ten minutes away) but not the RS256 signature: the emulator never
// holds the app's public key.
func (h *Handler) appFromJWT(w http.ResponseWriter, r *http.Request) (store.App, bool) {
	p := principalFrom(r)
	if p.Kind != principalApp {
		ghError(w, 401, "A JSON web token could not be decoded")
		return store.App{}, false
	}
	parts := strings.Split(p.Token, ".")
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	var claims struct {
		Iss json.RawMessage `json:"iss"`
		Exp json.Number     `json:"exp"`
		Iat json.Number     `json:"iat"`
	}
	if err != nil || json.Unmarshal(raw, &claims) != nil {
		ghError(w, 401, "A JSON web token could not be decoded")
		return store.App{}, false
	}
	iss := strings.Trim(string(claims.Iss), `"`)
	var app store.App
	found := false
	for _, a := range h.store.Apps.List() {
		if iss == strconv.FormatInt(a.ID, 10) || iss == a.ClientID {
			app, found = a, true
		}
	}
	if !found {
		ghError(w, 401, "'Issuer' claim ('iss') must be an Integer")
		return store.App{}, false
	}
	exp, err := claims.Exp.Int64()
	now := h.store.Clock.Now().Unix()
	if err != nil || exp <= now {
		ghError(w, 401, "'Expiration time' claim ('exp') must be a numeric value representing the future time at which the assertion expires")
		return store.App{}, false
	}
	if exp-now > 600+60 {
		ghError(w, 401, "'Expiration time' claim ('exp') is too far in the future")
		return store.App{}, false
	}
	return app, true
}

func (x renderer) app(a store.App) map[string]any {
	installs := 0
	for _, i := range x.h.store.Installations.List() {
		if i.AppID == a.ID {
			installs++
		}
	}
	return map[string]any{
		"id":                  a.ID,
		"slug":                a.Slug,
		"node_id":             store.NodeID("A", a.ID),
		"client_id":           a.ClientID,
		"owner":               x.account(a.OwnerLogin),
		"name":                a.Name,
		"description":         nullable(a.Description),
		"external_url":        x.web("/apps/%s", a.Slug),
		"html_url":            x.web("/apps/%s", a.Slug),
		"created_at":          a.CreatedAt,
		"updated_at":          a.UpdatedAt,
		"permissions":         a.Permissions,
		"events":              nonNil(a.Events),
		"installations_count": installs,
	}
}

func (x renderer) installation(i store.Installation) map[string]any {
	app, _ := x.h.store.GetApp(i.AppID)
	account := x.account(i.AccountLogin)
	return map[string]any{
		"id":                        i.ID,
		"account":                   account,
		"repository_selection":      i.RepositorySelection,
		"access_tokens_url":         x.api("/app/installations/%d/access_tokens", i.ID),
		"repositories_url":          x.api("/installation/repositories"),
		"html_url":                  x.web("/settings/installations/%d", i.ID),
		"app_id":                    i.AppID,
		"client_id":                 app.ClientID,
		"app_slug":                  app.Slug,
		"target_id":                 account["id"],
		"target_type":               i.TargetType,
		"permissions":               i.Permissions,
		"events":                    nonNil(i.Events),
		"created_at":                i.CreatedAt,
		"updated_at":                i.UpdatedAt,
		"single_file_name":          nil,
		"has_multiple_single_files": false,
		"single_file_paths":         []string{},
		"suspended_by":              nil,
		"suspended_at":              nil,
	}
}

// GetApp handles GET /app
func (h *Handler) GetApp(w http.ResponseWriter, r *http.Request) {
	app, ok := h.appFromJWT(w, r)
	if !ok {
		return
	}
	ghJSON(w, 200, h.rd(r).app(app))
}

// ListAppInstallations handles GET /app/installations
func (h *Handler) ListAppInstallations(w http.ResponseWriter, r *http.Request) {
	app, ok := h.appFromJWT(w, r)
	if !ok {
		return
	}
	insts := h.store.Installations.Filter(func(_ string, i store.Installation) bool { return i.AppID == app.ID })
	x := h.rd(r)
	out := []map[string]any{}
	for _, i := range paginate(w, r, insts) {
		out = append(out, x.installation(i))
	}
	ghJSON(w, 200, out)
}

func (h *Handler) appInstallation(w http.ResponseWriter, r *http.Request) (store.App, store.Installation, bool) {
	app, ok := h.appFromJWT(w, r)
	if !ok {
		return app, store.Installation{}, false
	}
	id, _ := strconv.ParseInt(chi.URLParam(r, "installation_id"), 10, 64)
	inst, found := h.store.GetInstallation(id)
	if !found || inst.AppID != app.ID {
		ghError(w, 404, "Not Found")
		return app, inst, false
	}
	return app, inst, true
}

// GetAppInstallation handles GET /app/installations/{installation_id}
func (h *Handler) GetAppInstallation(w http.ResponseWriter, r *http.Request) {
	if _, inst, ok := h.appInstallation(w, r); ok {
		ghJSON(w, 200, h.rd(r).installation(inst))
	}
}

const tokenAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// newInstallationToken returns a token in GitHub's installation-token
// format: ghs_ and 36 alphanumerics.
func newInstallationToken() string {
	b := make([]byte, 36)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(tokenAlphabet))))
		if err != nil {
			panic(err)
		}
		b[i] = tokenAlphabet[n.Int64()]
	}
	return "ghs_" + string(b)
}

// CreateInstallationAccessToken handles POST
// /app/installations/{installation_id}/access_tokens. The token acts as the
// app's bot on the installation's repositories and expires after an hour.
func (h *Handler) CreateInstallationAccessToken(w http.ResponseWriter, r *http.Request) {
	app, inst, ok := h.appInstallation(w, r)
	if !ok {
		return
	}
	var req struct {
		Repositories []string          `json:"repositories"`
		Permissions  map[string]string `json:"permissions"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	perms := inst.Permissions
	if len(req.Permissions) > 0 {
		for k, v := range req.Permissions {
			if have, granted := inst.Permissions[k]; !granted || (have == "read" && v == "write") {
				ghValidationErrors(w, "The permissions requested are not granted to this installation.")
				return
			}
		}
		perms = req.Permissions
	}
	expires := h.store.Clock.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	tok := newInstallationToken()
	h.store.Tokens.Set(tok, store.Token{
		Token: tok, Login: app.BotLogin(), Kind: principalInstallation, InstallationID: inst.ID, ExpiresAt: expires,
	})
	body := map[string]any{
		"token":                tok,
		"expires_at":           expires,
		"permissions":          perms,
		"repository_selection": inst.RepositorySelection,
	}
	if len(req.Repositories) > 0 || inst.RepositorySelection == "selected" {
		x := h.rd(r)
		repos := []map[string]any{}
		for _, rp := range h.store.InstallationRepos(inst) {
			if len(req.Repositories) == 0 || contains(req.Repositories, rp.Name) {
				repos = append(repos, x.repo(rp, app.BotLogin()))
			}
		}
		body["repositories"] = repos
	}
	ghJSON(w, 201, body)
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// ListInstallationRepos handles GET /installation/repositories, which only
// an installation token may call.
func (h *Handler) ListInstallationRepos(w http.ResponseWriter, r *http.Request) {
	p := principalFrom(r)
	if p.Kind != principalInstallation {
		ghError(w, 403, "You must authenticate with an installation access token in order to list repositories for an installation.")
		return
	}
	inst, ok := h.store.GetInstallation(p.InstallationID)
	if !ok {
		ghError(w, 401, "Bad credentials")
		return
	}
	repos := h.store.InstallationRepos(inst)
	x := h.rd(r)
	out := []map[string]any{}
	for _, rp := range paginate(w, r, repos) {
		out = append(out, x.repo(rp, p.Login))
	}
	ghJSON(w, 200, map[string]any{
		"total_count":          len(repos),
		"repositories":         out,
		"repository_selection": inst.RepositorySelection,
	})
}
