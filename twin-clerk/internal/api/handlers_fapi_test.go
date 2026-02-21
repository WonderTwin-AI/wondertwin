package api_test

import (
	"net/http"
	"testing"
)

// --- Environment Tests ---

func TestFAPIGetEnvironment(t *testing.T) {
	_, tc := setupClerk(t)

	resp := tc.Get("/v1/environment")
	resp.AssertStatus(200)

	m := resp.JSONMap()
	if m["object"] != "environment" {
		t.Errorf("expected object=environment, got %v", m["object"])
	}

	authConfig, ok := m["auth_config"].(map[string]any)
	if !ok {
		t.Fatal("expected auth_config object")
	}
	if authConfig["object"] != "auth_config" {
		t.Errorf("expected auth_config.object=auth_config, got %v", authConfig["object"])
	}

	displayConfig, ok := m["display_config"].(map[string]any)
	if !ok {
		t.Fatal("expected display_config object")
	}
	if displayConfig["instance_environment_type"] != "development" {
		t.Errorf("expected development instance, got %v", displayConfig["instance_environment_type"])
	}
	if displayConfig["preferred_sign_in_strategy"] != "password" {
		t.Errorf("expected password strategy, got %v", displayConfig["preferred_sign_in_strategy"])
	}

	userSettings, ok := m["user_settings"].(map[string]any)
	if !ok {
		t.Fatal("expected user_settings object")
	}
	attrs, ok := userSettings["attributes"].(map[string]any)
	if !ok {
		t.Fatal("expected attributes in user_settings")
	}
	emailAttr, ok := attrs["email_address"].(map[string]any)
	if !ok {
		t.Fatal("expected email_address attribute")
	}
	if emailAttr["enabled"] != true {
		t.Error("expected email_address.enabled=true")
	}

	orgSettings, ok := m["organization_settings"].(map[string]any)
	if !ok {
		t.Fatal("expected organization_settings")
	}
	if orgSettings["enabled"] != true {
		t.Error("expected organization_settings.enabled=true")
	}
}

// --- Client Tests ---

func TestFAPIGetClient(t *testing.T) {
	_, tc := setupClerk(t)

	resp := tc.Get("/v1/client")
	resp.AssertStatus(200)

	m := resp.JSONMap()
	if m["object"] != "client" {
		t.Errorf("expected object=client, got %v", m["object"])
	}
	sessions, ok := m["sessions"].([]any)
	if !ok {
		t.Fatal("expected sessions array")
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions on new client, got %d", len(sessions))
	}
}

func TestFAPICreateClient(t *testing.T) {
	_, tc := setupClerk(t)

	resp := tc.Post("/v1/client", nil)
	resp.AssertStatus(200)

	m := resp.JSONMap()
	if m["object"] != "client" {
		t.Errorf("expected object=client, got %v", m["object"])
	}
	id, ok := m["id"].(string)
	if !ok || id == "" {
		t.Fatal("expected client id")
	}
}

// --- Sign-In Tests ---

func TestFAPISignInWithPassword(t *testing.T) {
	_, tc := setupClerk(t)

	// Create a user with password
	clerkPost(tc, "/v1/users", map[string]any{
		"email_address": []string{"merchant@example.com"},
		"first_name":    "Merchant",
		"last_name":     "User",
		"password":      "test-password-123",
	}).AssertStatus(200)

	// Sign in with correct password
	resp := tc.Post("/v1/client/sign_ins", map[string]any{
		"strategy":   "password",
		"identifier": "merchant@example.com",
		"password":   "test-password-123",
	})
	resp.AssertStatus(200)

	m := resp.JSONMap()
	if m["object"] != "client" {
		t.Errorf("expected client response, got %v", m["object"])
	}

	// Client should have a session
	sessions, ok := m["sessions"].([]any)
	if !ok || len(sessions) == 0 {
		t.Fatal("expected at least 1 session after sign-in")
	}

	sess := sessions[0].(map[string]any)
	if sess["status"] != "active" {
		t.Errorf("expected session status=active, got %v", sess["status"])
	}

	// Session should have embedded user
	user, ok := sess["user"].(map[string]any)
	if !ok {
		t.Fatal("expected user in session")
	}
	if user["first_name"] != "Merchant" {
		t.Errorf("expected first_name=Merchant, got %v", user["first_name"])
	}

	// Last active token should have JWT
	lastToken, ok := sess["last_active_token"].(map[string]any)
	if !ok {
		t.Fatal("expected last_active_token in session")
	}
	jwt, ok := lastToken["jwt"].(string)
	if !ok || jwt == "" {
		t.Fatal("expected JWT in last_active_token")
	}
}

func TestFAPISignInWrongPassword(t *testing.T) {
	_, tc := setupClerk(t)

	clerkPost(tc, "/v1/users", map[string]any{
		"email_address": []string{"user@example.com"},
		"password":      "correct-password",
	}).AssertStatus(200)

	resp := tc.Post("/v1/client/sign_ins", map[string]any{
		"strategy":   "password",
		"identifier": "user@example.com",
		"password":   "wrong-password",
	})
	resp.AssertStatus(422)
	resp.AssertBodyContains("form_password_incorrect")
}

func TestFAPISignInUserNotFound(t *testing.T) {
	_, tc := setupClerk(t)

	resp := tc.Post("/v1/client/sign_ins", map[string]any{
		"strategy":   "password",
		"identifier": "nobody@example.com",
		"password":   "any-password",
	})
	resp.AssertStatus(422)
	resp.AssertBodyContains("form_identifier_not_found")
}

func TestFAPISignInNeedsFirstFactor(t *testing.T) {
	_, tc := setupClerk(t)

	clerkPost(tc, "/v1/users", map[string]any{
		"email_address": []string{"needs-factor@example.com"},
		"password":      "my-password",
	}).AssertStatus(200)

	// Sign in without password — should return needs_first_factor
	resp := tc.Post("/v1/client/sign_ins", map[string]any{
		"identifier": "needs-factor@example.com",
	})
	resp.AssertStatus(200)

	m := resp.JSONMap()
	// Response is a client with sign_in populated
	signIn, ok := m["sign_in"].(map[string]any)
	if !ok {
		t.Fatal("expected sign_in in client response")
	}
	if signIn["status"] != "needs_first_factor" {
		t.Errorf("expected needs_first_factor, got %v", signIn["status"])
	}
}

func TestFAPIAttemptFirstFactor(t *testing.T) {
	_, tc := setupClerk(t)

	clerkPost(tc, "/v1/users", map[string]any{
		"email_address": []string{"attempt@example.com"},
		"password":      "my-password",
	}).AssertStatus(200)

	// Start sign-in without password
	resp := tc.Post("/v1/client/sign_ins", map[string]any{
		"identifier": "attempt@example.com",
	})
	resp.AssertStatus(200)
	client := resp.JSONMap()
	signIn := client["sign_in"].(map[string]any)
	signInID := signIn["id"].(string)

	// Attempt first factor with correct password
	resp = tc.Post("/v1/client/sign_ins/"+signInID+"/attempt_first_factor", map[string]any{
		"strategy": "password",
		"password": "my-password",
	})
	resp.AssertStatus(200)

	m := resp.JSONMap()
	sessions, ok := m["sessions"].([]any)
	if !ok || len(sessions) == 0 {
		t.Fatal("expected session after successful first factor")
	}
}

func TestFAPIAttemptFirstFactorWrongPassword(t *testing.T) {
	_, tc := setupClerk(t)

	clerkPost(tc, "/v1/users", map[string]any{
		"email_address": []string{"wrong@example.com"},
		"password":      "correct",
	}).AssertStatus(200)

	resp := tc.Post("/v1/client/sign_ins", map[string]any{
		"identifier": "wrong@example.com",
	})
	signIn := resp.JSONMap()["sign_in"].(map[string]any)
	signInID := signIn["id"].(string)

	resp = tc.Post("/v1/client/sign_ins/"+signInID+"/attempt_first_factor", map[string]any{
		"strategy": "password",
		"password": "incorrect",
	})
	resp.AssertStatus(422)
	resp.AssertBodyContains("form_password_incorrect")
}

// --- Session Token Tests ---

func TestFAPIGetSessionToken(t *testing.T) {
	_, tc := setupClerk(t)

	// Create user, create session via admin
	resp := clerkPost(tc, "/v1/users", map[string]any{
		"email_address": []string{"token-test@example.com"},
		"first_name":    "Token",
		"password":      "password",
	})
	userID := resp.JSONMap()["id"].(string)

	resp = tc.Post("/admin/sessions", map[string]any{"user_id": userID})
	sessID := resp.JSONMap()["id"].(string)

	// Get token
	resp = tc.Post("/v1/client/sessions/"+sessID+"/tokens", nil)
	resp.AssertStatus(200)

	m := resp.JSONMap()
	if m["object"] != "token" {
		t.Errorf("expected object=token, got %v", m["object"])
	}
	jwt, ok := m["jwt"].(string)
	if !ok || jwt == "" {
		t.Fatal("expected JWT in token response")
	}
}

func TestFAPIGetSessionTokenWithOrg(t *testing.T) {
	_, tc := setupClerk(t)

	resp := clerkPost(tc, "/v1/users", map[string]any{
		"email_address": []string{"org-token@example.com"},
		"password":      "password",
	})
	userID := resp.JSONMap()["id"].(string)

	resp = tc.Post("/admin/sessions", map[string]any{"user_id": userID})
	sessID := resp.JSONMap()["id"].(string)

	// Get token with organization_id
	resp = tc.Post("/v1/client/sessions/"+sessID+"/tokens", map[string]any{
		"organization_id": "org_test123",
	})
	resp.AssertStatus(200)

	m := resp.JSONMap()
	jwt, ok := m["jwt"].(string)
	if !ok || jwt == "" {
		t.Fatal("expected JWT")
	}
	// JWT should contain org_id claim (we verify it's non-empty; full JWT parsing is out of scope)
	if len(jwt) < 50 {
		t.Error("JWT seems too short")
	}
}

// --- Session Touch Tests ---

func TestFAPITouchSession(t *testing.T) {
	_, tc := setupClerk(t)

	resp := clerkPost(tc, "/v1/users", map[string]any{
		"email_address": []string{"touch@example.com"},
		"password":      "password",
	})
	userID := resp.JSONMap()["id"].(string)

	resp = tc.Post("/admin/sessions", map[string]any{"user_id": userID})
	sessID := resp.JSONMap()["id"].(string)

	resp = tc.Post("/v1/client/sessions/"+sessID+"/touch", nil)
	resp.AssertStatus(200)

	m := resp.JSONMap()
	if m["object"] != "client" {
		t.Errorf("expected client response, got %v", m["object"])
	}
}

// --- End Session Tests ---

func TestFAPIEndSession(t *testing.T) {
	_, tc := setupClerk(t)

	// Sign in to create a session
	clerkPost(tc, "/v1/users", map[string]any{
		"email_address": []string{"end-sess@example.com"},
		"password":      "password",
	}).AssertStatus(200)

	resp := tc.Post("/v1/client/sign_ins", map[string]any{
		"strategy":   "password",
		"identifier": "end-sess@example.com",
		"password":   "password",
	})
	resp.AssertStatus(200)

	client := resp.JSONMap()
	sessions := client["sessions"].([]any)
	sessID := sessions[0].(map[string]any)["id"].(string)

	// End the session
	resp = tc.DoWithHeaders("DELETE", "/v1/client/sessions/"+sessID, nil, nil)
	resp.AssertStatus(200)

	m := resp.JSONMap()
	endSessions, ok := m["sessions"].([]any)
	if !ok {
		t.Fatal("expected sessions array")
	}
	if len(endSessions) != 0 {
		t.Errorf("expected 0 sessions after ending, got %d", len(endSessions))
	}
}

// --- Handshake Tests ---

func TestFAPIHandshake(t *testing.T) {
	srv, _ := setupClerk(t)

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // don't follow redirects
		},
	}

	resp, err := client.Get(srv.URL + "/v1/client/handshake?redirect_url=http://localhost:3000/dashboard")
	if err != nil {
		t.Fatalf("handshake request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Errorf("expected 307, got %d", resp.StatusCode)
	}

	loc := resp.Header.Get("Location")
	if loc != "http://localhost:3000/dashboard" {
		t.Errorf("expected redirect to dashboard, got %s", loc)
	}
}

// --- Clerk-JS CDN Redirect ---

func TestClerkJSCDNRedirect(t *testing.T) {
	srv, _ := setupClerk(t)

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get(srv.URL + "/npm/@clerk/clerk-js@5/dist/clerk.browser.js")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Errorf("expected 307, got %d", resp.StatusCode)
	}

	loc := resp.Header.Get("Location")
	expected := "https://cdn.jsdelivr.net/npm/@clerk/clerk-js@5/dist/clerk.browser.js"
	if loc != expected {
		t.Errorf("expected redirect to %s, got %s", expected, loc)
	}
}

// --- Destroy Client Tests ---

func TestFAPIDestroyClient(t *testing.T) {
	_, tc := setupClerk(t)

	// Create a client and sign in
	clerkPost(tc, "/v1/users", map[string]any{
		"email_address": []string{"destroy@example.com"},
		"password":      "password",
	}).AssertStatus(200)

	tc.Post("/v1/client/sign_ins", map[string]any{
		"strategy":   "password",
		"identifier": "destroy@example.com",
		"password":   "password",
	}).AssertStatus(200)

	// Destroy the client
	resp := tc.DoWithHeaders("DELETE", "/v1/client", nil, nil)
	resp.AssertStatus(200)

	m := resp.JSONMap()
	sessions, ok := m["sessions"].([]any)
	if !ok {
		t.Fatal("expected sessions array")
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions after destroy, got %d", len(sessions))
	}
}
