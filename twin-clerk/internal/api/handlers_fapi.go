// handlers_fapi.go implements the Clerk Frontend API (FAPI) endpoints.
// These are the endpoints that clerk-js (the browser SDK) calls to manage
// sign-in, sessions, and client state. Unlike the Backend API (which uses
// Bearer token auth), FAPI uses cookies for session management.
//
// Endpoint mapping:
//
//	GET  /v1/environment                          → instance configuration
//	GET  /v1/client                               → current client state
//	POST /v1/client                               → create new client
//	POST /v1/client/sign_ins                      → start sign-in
//	POST /v1/client/sign_ins/{id}/attempt_first_factor → password attempt
//	POST /v1/client/sessions/{id}/tokens          → get session JWT
//	POST /v1/client/sessions/{id}/touch           → refresh session
//	DELETE /v1/client/sessions/{id}               → end session
//	GET  /v1/client/handshake                     → cookie refresh redirect
package api

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-clerk/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

const (
	sessionCookieName   = "__session"
	clientUATCookieName = "__client_uat"
	devBrowserCookie    = "__clerk_db_jwt"
)

// cookieSuffix returns the 8-char suffix Clerk derives from the publishable key.
// When a client sends a suffixed cookie, we try both variants.
func cookieSuffix(publishableKey string) string {
	if publishableKey == "" {
		return ""
	}
	h := sha1.Sum([]byte(publishableKey))
	return base64.RawURLEncoding.EncodeToString(h[:])[:8]
}

// getClientFromCookie resolves the client ID from the __clerk_db_jwt cookie
// or creates a new client if none exists.
func (h *Handler) getOrCreateClient(r *http.Request) store.Client {
	// Try reading client ID from dev browser cookie
	if c, err := r.Cookie(devBrowserCookie); err == nil && c.Value != "" {
		if client, ok := h.store.Clients.Get(c.Value); ok {
			return client
		}
	}

	// Create a new client
	return h.createNewClient()
}

func (h *Handler) createNewClient() store.Client {
	id := h.store.Clients.NextID()
	now := store.Now()
	client := store.Client{
		ID:        id,
		Object:    "client",
		Sessions:  []store.FAPISession{},
		CreatedAt: now,
		UpdatedAt: now,
	}
	h.store.Clients.Set(id, client)
	return client
}

// buildFAPISession converts a backend Session + User into a FAPI session.
func (h *Handler) buildFAPISession(sess store.Session) store.FAPISession {
	user, _ := h.store.Users.Get(sess.UserID)

	identifier := ""
	if len(user.EmailAddresses) > 0 {
		identifier = user.EmailAddresses[0].EmailAddress
	}

	return store.FAPISession{
		ID:              sess.ID,
		Object:          "session",
		Status:          sess.Status,
		ExpireAt:        sess.ExpireAt,
		AbandonAt:       sess.AbandonAt,
		LastActiveAt:    sess.LastActiveAt,
		LastActiveToken: sess.LastActiveToken,
		User:            user,
		PublicUserData: store.PublicUserData{
			FirstName:  user.FirstName,
			LastName:   user.LastName,
			ImageURL:   user.ImageURL,
			Identifier: identifier,
		},
		CreatedAt: sess.CreatedAt,
		UpdatedAt: sess.UpdatedAt,
	}
}

// rebuildClientSessions refreshes the client's session list from the session store.
func (h *Handler) rebuildClientSessions(client *store.Client) {
	var sessions []store.FAPISession
	for _, sess := range h.store.Sessions.List() {
		if sess.ClientID == client.ID && sess.Status == "active" {
			sessions = append(sessions, h.buildFAPISession(sess))
		}
	}
	if sessions == nil {
		sessions = []store.FAPISession{}
	}
	client.Sessions = sessions
}

// setSessionCookies writes the __session, __client_uat, and __clerk_db_jwt cookies.
func (h *Handler) setSessionCookies(w http.ResponseWriter, clientID, jwt string) {
	expires := time.Now().Add(7 * 24 * time.Hour)

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    jwt,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     clientUATCookieName,
		Value:    fmt.Sprintf("%d", time.Now().Unix()),
		Path:     "/",
		Expires:  expires,
		SameSite: http.SameSiteLaxMode,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     devBrowserCookie,
		Value:    clientID,
		Path:     "/",
		Expires:  expires,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearSessionCookies removes all session cookies.
func clearSessionCookies(w http.ResponseWriter) {
	for _, name := range []string{sessionCookieName, clientUATCookieName, devBrowserCookie} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
		})
	}
}

// --- Environment ---

// GetEnvironment handles GET /v1/environment.
// Returns the instance configuration that clerk-js needs to initialize.
func (h *Handler) GetEnvironment(w http.ResponseWriter, r *http.Request) {
	env := map[string]any{
		"object": "environment",
		"id":     "env_twin",
		"auth_config": map[string]any{
			"object":              "auth_config",
			"id":                  "aconf_twin",
			"single_session_mode": false,
			"claimed_at":          store.Now(),
		},
		"display_config": map[string]any{
			"object":                     "display_config",
			"id":                         "dconf_twin",
			"application_name":           "WonderTwin Dev",
			"branded":                    false,
			"instance_environment_type":  "development",
			"logo_image_url":             "",
			"favicon_image_url":          "",
			"home_url":                   "http://localhost:3000",
			"sign_in_url":               "/sign-in",
			"sign_up_url":               "/sign-up",
			"after_sign_in_url":         "/",
			"after_sign_up_url":         "/",
			"after_sign_out_all_url":    "/sign-in",
			"after_sign_out_one_url":    "/sign-in",
			"after_switch_session_url":  "/",
			"support_email":             "",
			"preferred_sign_in_strategy": "password",
			"captcha_public_key":         nil,
			"captcha_widget_type":        nil,
			"captcha_public_key_invisible": nil,
			"captcha_provider":           "turnstile",
			"captcha_oauth_bypass":       nil,
			"show_devmode_warning":       false,
			"terms_url":                 "",
			"privacy_policy_url":        "",
			"waitlist_url":              "",
			"after_join_waitlist_url":   "",
			"theme": map[string]any{
				"general": map[string]any{
					"color":            "#6C47FF",
					"background_color": "#FFFFFF",
					"font_family":      "",
					"border_radius":    "0.5rem",
					"font_color":       "#151515",
					"label_font_weight": "600",
					"padding":           "1em",
					"box_shadow":        "0 2px 8px rgba(0, 0, 0, 0.15)",
				},
				"buttons": map[string]any{
					"font_color":   "#FFFFFF",
					"font_family":  "",
					"font_weight":  "600",
				},
				"accounts": map[string]any{
					"background_color": "#FFFFFF",
				},
			},
		},
		"user_settings": map[string]any{
			"attributes": map[string]any{
				"email_address": map[string]any{
					"enabled":     true,
					"required":    true,
					"used_for_first_factor": true,
					"first_factors": []string{"email_code", "email_link"},
					"used_for_second_factor": false,
					"verifications": []string{"email_code"},
					"verify_at_sign_up": true,
				},
				"password": map[string]any{
					"enabled":     true,
					"required":    true,
				},
			},
			"actions": map[string]any{
				"delete_self": true,
			},
			"social":         map[string]any{},
			"enterprise_sso": map[string]any{},
			"sign_in": map[string]any{
				"second_factor": map[string]any{
					"required": false,
				},
			},
			"sign_up": map[string]any{
				"captcha_enabled": false,
				"progressive":     true,
				"mode":            "public",
			},
			"password_settings": map[string]any{
				"disable_hibp":           true,
				"min_length":             8,
				"max_length":             72,
				"show_zxcvbn":            false,
				"min_zxcvbn_strength":    0,
				"enforce_hibp_on_sign_in": false,
				"allowed_special_characters": "!@#$%^&*",
			},
			"passkey_settings": map[string]any{
				"allow_autofill":    false,
				"show_sign_in_button": false,
			},
			"username_settings": map[string]any{
				"min_length": 4,
				"max_length": 64,
			},
		},
		"organization_settings": map[string]any{
			"enabled":                     true,
			"max_allowed_memberships":     5,
			"force_organization_selection": false,
			"actions": map[string]any{
				"admin_delete": true,
			},
			"domains": map[string]any{
				"enabled":          false,
				"enrollment_modes": []string{},
				"default_role":     nil,
			},
		},
		"commerce_settings": map[string]any{
			"billing": map[string]any{
				"stripe_publishable_key": nil,
				"organization":           map[string]any{"enabled": false, "has_paid_plans": false},
				"user":                   map[string]any{"enabled": false, "has_paid_plans": false},
			},
		},
		"maintenance_mode": false,
	}

	twincore.JSON(w, http.StatusOK, env)
}

// --- Client ---

// GetClient handles GET /v1/client.
// Returns the current client state including all active sessions.
func (h *Handler) GetClient(w http.ResponseWriter, r *http.Request) {
	client := h.getOrCreateClient(r)
	h.rebuildClientSessions(&client)
	client.UpdatedAt = store.Now()
	h.store.Clients.Set(client.ID, client)

	h.setSessionCookies(w, client.ID, h.activeSessionJWT(client))
	twincore.JSON(w, http.StatusOK, client)
}

// CreateClient handles POST /v1/client.
// Creates a new client (browser) and returns it.
func (h *Handler) CreateClient(w http.ResponseWriter, r *http.Request) {
	client := h.createNewClient()

	h.setSessionCookies(w, client.ID, "")
	twincore.JSON(w, http.StatusOK, client)
}

// DestroyClient handles DELETE /v1/client.
// Ends all sessions and destroys the client.
func (h *Handler) DestroyClient(w http.ResponseWriter, r *http.Request) {
	client := h.getOrCreateClient(r)

	// Revoke all sessions belonging to this client
	for _, sess := range h.store.Sessions.List() {
		if sess.ClientID == client.ID {
			sess.Status = "ended"
			sess.UpdatedAt = store.Now()
			h.store.Sessions.Set(sess.ID, sess)
		}
	}

	h.store.Clients.Delete(client.ID)
	clearSessionCookies(w)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "client",
		"id":       client.ID,
		"sessions": []any{},
	})
}

// activeSessionJWT returns the JWT from the last active session, or empty string.
func (h *Handler) activeSessionJWT(client store.Client) string {
	if client.LastActiveSessionID != nil {
		if sess, ok := h.store.Sessions.Get(*client.LastActiveSessionID); ok {
			if sess.LastActiveToken != nil {
				return sess.LastActiveToken.JWT
			}
		}
	}
	if len(client.Sessions) > 0 {
		last := client.Sessions[len(client.Sessions)-1]
		if last.LastActiveToken != nil {
			return last.LastActiveToken.JWT
		}
	}
	return ""
}

// --- Sign-In ---

// CreateSignIn handles POST /v1/client/sign_ins.
// Supports strategy=password with identifier (email) + password.
func (h *Handler) CreateSignIn(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Strategy   string `json:"strategy"`
		Identifier string `json:"identifier"`
		Password   string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		clerkError(w, http.StatusBadRequest, "form_param_invalid",
			"Invalid request body.", err.Error())
		return
	}

	client := h.getOrCreateClient(r)
	now := store.Now()

	// Find user by email
	var matchedUser *store.User
	for _, user := range h.store.Users.List() {
		for _, email := range user.EmailAddresses {
			if strings.EqualFold(email.EmailAddress, req.Identifier) {
				u := user
				matchedUser = &u
				break
			}
		}
		if matchedUser != nil {
			break
		}
	}

	signInID := h.store.SignIns.NextID()

	if matchedUser == nil {
		// User not found — return sign-in in needs_identifier state
		signIn := store.SignIn{
			ID:                   signInID,
			Object:               "sign_in",
			Status:               "needs_identifier",
			SupportedIdentifiers: []string{"email_address"},
			CreatedAt:            now,
			UpdatedAt:            now,
			AbandonAt:            now + 24*60*60*1000,
		}
		h.store.SignIns.Set(signInID, signIn)
		client.SignIn = &signIn
		client.UpdatedAt = now
		h.store.Clients.Set(client.ID, client)

		clerkError(w, http.StatusUnprocessableEntity, "form_identifier_not_found",
			"Couldn't find your account.",
			"No account was found with the provided identifier.")
		return
	}

	// If strategy=password and password provided, attempt immediate verification
	if req.Strategy == "password" && req.Password != "" {
		if matchedUser.PasswordHash != req.Password {
			signIn := store.SignIn{
				ID:                   signInID,
				Object:               "sign_in",
				Status:               "needs_first_factor",
				SupportedIdentifiers: []string{"email_address"},
				Identifier:           req.Identifier,
				FirstFactorVerification: &store.FactorVerification{
					Status:   "failed",
					Strategy: "password",
				},
				UserID:    &matchedUser.ID,
				CreatedAt: now,
				UpdatedAt: now,
				AbandonAt: now + 24*60*60*1000,
			}
			h.store.SignIns.Set(signInID, signIn)
			client.SignIn = &signIn
			client.UpdatedAt = now
			h.store.Clients.Set(client.ID, client)

			clerkError(w, http.StatusUnprocessableEntity, "form_password_incorrect",
				"Password is incorrect. Try again, or use another method.",
				"The provided password is incorrect.")
			return
		}

		// Password correct — create session and complete sign-in
		h.completeSignIn(w, &client, matchedUser, signInID, now)
		return
	}

	// No password — return needs_first_factor
	factors := []store.SignInFactor{
		{Strategy: "password"},
	}
	if len(matchedUser.EmailAddresses) > 0 {
		factors = append(factors, store.SignInFactor{
			Strategy:       "email_code",
			SafeIdentifier: maskEmail(matchedUser.EmailAddresses[0].EmailAddress),
			EmailAddressID: matchedUser.EmailAddresses[0].ID,
		})
	}

	signIn := store.SignIn{
		ID:                    signInID,
		Object:                "sign_in",
		Status:                "needs_first_factor",
		SupportedIdentifiers:  []string{"email_address"},
		Identifier:            req.Identifier,
		SupportedFirstFactors: factors,
		UserID:                &matchedUser.ID,
		CreatedAt:             now,
		UpdatedAt:             now,
		AbandonAt:             now + 24*60*60*1000,
	}
	h.store.SignIns.Set(signInID, signIn)
	client.SignIn = &signIn
	client.UpdatedAt = now
	h.store.Clients.Set(client.ID, client)

	h.setSessionCookies(w, client.ID, "")
	h.respondWithClient(w, client)
}

// AttemptFirstFactor handles POST /v1/client/sign_ins/{id}/attempt_first_factor.
func (h *Handler) AttemptFirstFactor(w http.ResponseWriter, r *http.Request) {
	signInID := chi.URLParam(r, "id")

	signIn, ok := h.store.SignIns.Get(signInID)
	if !ok {
		clerkError(w, http.StatusNotFound, "resource_not_found",
			"Sign-in not found.", fmt.Sprintf("No sign-in with id %s.", signInID))
		return
	}

	var req struct {
		Strategy string `json:"strategy"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		clerkError(w, http.StatusBadRequest, "form_param_invalid",
			"Invalid request body.", err.Error())
		return
	}

	if signIn.UserID == nil {
		clerkError(w, http.StatusUnprocessableEntity, "sign_in_no_user",
			"Sign-in has no associated user.", "")
		return
	}

	user, userOk := h.store.Users.Get(*signIn.UserID)
	if !userOk {
		clerkError(w, http.StatusNotFound, "resource_not_found",
			"User not found.", "")
		return
	}

	client := h.getOrCreateClient(r)
	now := store.Now()

	if req.Strategy == "password" {
		if user.PasswordHash != req.Password {
			signIn.FirstFactorVerification = &store.FactorVerification{
				Status:   "failed",
				Strategy: "password",
			}
			signIn.UpdatedAt = now
			h.store.SignIns.Set(signInID, signIn)
			client.SignIn = &signIn
			h.store.Clients.Set(client.ID, client)

			clerkError(w, http.StatusUnprocessableEntity, "form_password_incorrect",
				"Password is incorrect.", "The provided password is incorrect.")
			return
		}

		// Password correct — complete
		h.completeSignIn(w, &client, &user, signInID, now)
		return
	}

	clerkError(w, http.StatusBadRequest, "strategy_not_supported",
		fmt.Sprintf("Strategy %q is not supported.", req.Strategy), "")
}

// completeSignIn creates a session and marks the sign-in as complete.
func (h *Handler) completeSignIn(w http.ResponseWriter, client *store.Client, user *store.User, signInID string, now int64) {
	// Create a backend session
	sessID := h.store.Sessions.NextID()
	expireAt := now + 7*24*60*60*1000
	abandonAt := now + 30*24*60*60*1000

	// Generate JWT
	token, err := h.jwtMgr.GenerateToken(user.ID, sessID, nil)
	if err != nil {
		clerkError(w, http.StatusInternalServerError, "internal_error",
			"Failed to generate session token.", err.Error())
		return
	}

	session := store.Session{
		ID:           sessID,
		Object:       "session",
		UserID:       user.ID,
		ClientID:     client.ID,
		Status:       "active",
		LastActiveAt: now,
		ExpireAt:     expireAt,
		AbandonAt:    abandonAt,
		CreatedAt:    now,
		UpdatedAt:    now,
		LastActiveToken: &store.TokenResponse{
			Object: "token",
			JWT:    token,
		},
	}
	h.store.Sessions.Set(sessID, session)

	// Update user last sign-in
	user.LastSignInAt = &now
	user.LastActiveAt = &now
	h.store.Users.Set(user.ID, *user)

	// Update sign-in to complete
	signIn := store.SignIn{
		ID:                   signInID,
		Object:               "sign_in",
		Status:               "complete",
		SupportedIdentifiers: []string{"email_address"},
		FirstFactorVerification: &store.FactorVerification{
			Status:   "verified",
			Strategy: "password",
		},
		CreatedSessionID: &sessID,
		UserID:           &user.ID,
		CreatedAt:        now,
		UpdatedAt:        now,
		AbandonAt:        now + 24*60*60*1000,
	}
	h.store.SignIns.Set(signInID, signIn)

	// Update client
	strategy := "password"
	client.SignIn = nil // clear in-progress sign-in
	client.LastActiveSessionID = &sessID
	client.LastAuthenticationStrategy = &strategy
	client.UpdatedAt = now
	h.rebuildClientSessions(client)
	h.store.Clients.Set(client.ID, *client)

	h.setSessionCookies(w, client.ID, token)
	h.respondWithClient(w, *client)
}

// --- Session Token ---

// GetSessionToken handles POST /v1/client/sessions/{id}/tokens.
// Returns a fresh JWT for the session.
func (h *Handler) GetSessionToken(w http.ResponseWriter, r *http.Request) {
	sessID := chi.URLParam(r, "id")

	session, ok := h.store.Sessions.Get(sessID)
	if !ok {
		clerkError(w, http.StatusNotFound, "resource_not_found",
			"Session not found.", fmt.Sprintf("No session with id %s.", sessID))
		return
	}

	if session.Status != "active" {
		clerkError(w, http.StatusUnauthorized, "session_not_active",
			"Session is not active.", "")
		return
	}

	// Check for organization_id in request body
	var req struct {
		OrganizationID string `json:"organization_id"`
	}
	// Body may be empty, ignore decode errors
	json.NewDecoder(r.Body).Decode(&req)

	extraClaims := map[string]any{}
	if req.OrganizationID != "" {
		extraClaims["org_id"] = req.OrganizationID
	}

	var claimsPtr map[string]any
	if len(extraClaims) > 0 {
		claimsPtr = extraClaims
	}

	token, err := h.jwtMgr.GenerateToken(session.UserID, sessID, claimsPtr)
	if err != nil {
		clerkError(w, http.StatusInternalServerError, "internal_error",
			"Failed to generate token.", err.Error())
		return
	}

	// Update session
	now := store.Now()
	session.LastActiveAt = now
	session.UpdatedAt = now
	session.LastActiveToken = &store.TokenResponse{
		Object: "token",
		JWT:    token,
	}
	h.store.Sessions.Set(sessID, session)

	// Update cookies
	client := h.getOrCreateClient(r)
	h.setSessionCookies(w, client.ID, token)

	twincore.JSON(w, http.StatusOK, store.TokenResponse{
		Object: "token",
		JWT:    token,
	})
}

// --- Session Touch ---

// TouchSession handles POST /v1/client/sessions/{id}/touch.
// Refreshes the session's last active timestamp and returns updated client.
func (h *Handler) TouchSession(w http.ResponseWriter, r *http.Request) {
	sessID := chi.URLParam(r, "id")

	session, ok := h.store.Sessions.Get(sessID)
	if !ok {
		clerkError(w, http.StatusNotFound, "resource_not_found",
			"Session not found.", fmt.Sprintf("No session with id %s.", sessID))
		return
	}

	now := store.Now()
	session.LastActiveAt = now
	session.UpdatedAt = now

	// Generate fresh token
	token, err := h.jwtMgr.GenerateToken(session.UserID, sessID, nil)
	if err != nil {
		clerkError(w, http.StatusInternalServerError, "internal_error",
			"Failed to generate token.", err.Error())
		return
	}
	session.LastActiveToken = &store.TokenResponse{
		Object: "token",
		JWT:    token,
	}
	h.store.Sessions.Set(sessID, session)

	client := h.getOrCreateClient(r)
	client.LastActiveSessionID = &sessID
	client.UpdatedAt = now
	h.rebuildClientSessions(&client)
	h.store.Clients.Set(client.ID, client)

	h.setSessionCookies(w, client.ID, token)
	h.respondWithClient(w, client)
}

// --- End Session ---

// EndSession handles DELETE /v1/client/sessions/{id}.
func (h *Handler) EndSession(w http.ResponseWriter, r *http.Request) {
	sessID := chi.URLParam(r, "id")

	session, ok := h.store.Sessions.Get(sessID)
	if !ok {
		clerkError(w, http.StatusNotFound, "resource_not_found",
			"Session not found.", fmt.Sprintf("No session with id %s.", sessID))
		return
	}

	session.Status = "ended"
	session.UpdatedAt = store.Now()
	h.store.Sessions.Set(sessID, session)

	client := h.getOrCreateClient(r)
	h.rebuildClientSessions(&client)

	// If this was the last active session, clear the reference
	if client.LastActiveSessionID != nil && *client.LastActiveSessionID == sessID {
		client.LastActiveSessionID = nil
	}
	client.UpdatedAt = store.Now()
	h.store.Clients.Set(client.ID, client)

	if len(client.Sessions) == 0 {
		clearSessionCookies(w)
	} else {
		h.setSessionCookies(w, client.ID, h.activeSessionJWT(client))
	}
	h.respondWithClient(w, client)
}

// --- Handshake ---

// Handshake handles GET /v1/client/handshake.
// This is used by Clerk's middleware to refresh cookies via a redirect.
// The twin simplifies this: it sets fresh cookies and redirects back.
func (h *Handler) Handshake(w http.ResponseWriter, r *http.Request) {
	redirectURL := r.URL.Query().Get("redirect_url")
	if redirectURL == "" {
		redirectURL = "/"
	}

	client := h.getOrCreateClient(r)
	h.rebuildClientSessions(&client)
	h.store.Clients.Set(client.ID, client)

	jwt := h.activeSessionJWT(client)
	h.setSessionCookies(w, client.ID, jwt)

	http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
}

// respondWithClient writes the full client state as JSON response.
func (h *Handler) respondWithClient(w http.ResponseWriter, client store.Client) {
	twincore.JSON(w, http.StatusOK, client)
}

// maskEmail returns a partially masked email (e.g., "a***@example.com").
func maskEmail(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return "***"
	}
	local := parts[0]
	if len(local) <= 1 {
		return local + "***@" + parts[1]
	}
	return string(local[0]) + "***@" + parts[1]
}
