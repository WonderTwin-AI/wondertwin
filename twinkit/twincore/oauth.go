package twincore

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// OAuthConfig configures the simulated OAuth flow.
type OAuthConfig struct {
	AuthorizePath string        `json:"authorize_path"` // e.g., "/oauth/authorize"
	TokenPath     string        `json:"token_path"`     // e.g., "/oauth/token"
	ClientID      string        `json:"client_id"`
	ClientSecret  string        `json:"client_secret"`
	TokenExpiry   time.Duration `json:"token_expiry"` // default: 1 hour
}

// OAuthServer provides simulated OAuth 2.0 endpoints.
type OAuthServer struct {
	mu     sync.Mutex
	config OAuthConfig
	codes  map[string]*authCode // code → auth code data
	tokens *TokenRegistry
}

type authCode struct {
	Code        string
	ClientID    string
	RedirectURI string
	Scopes      []string
	CreatedAt   time.Time
}

// NewOAuthServer creates a new OAuth server.
func NewOAuthServer(cfg OAuthConfig, tokens *TokenRegistry) *OAuthServer {
	if cfg.TokenExpiry == 0 {
		cfg.TokenExpiry = time.Hour
	}
	return &OAuthServer{
		config: cfg,
		codes:  make(map[string]*authCode),
		tokens: tokens,
	}
}

// Routes mounts the OAuth endpoints on the given router.
// Call this from the twin's main.go to enable OAuth simulation.
func (s *OAuthServer) Routes(mux interface {
	HandleFunc(string, func(http.ResponseWriter, *http.Request))
}) {
	if s.config.AuthorizePath != "" {
		mux.HandleFunc(s.config.AuthorizePath, s.handleAuthorize)
	}
	if s.config.TokenPath != "" {
		mux.HandleFunc(s.config.TokenPath, s.handleToken)
	}
}

// handleAuthorize simulates the authorization endpoint.
// GET: returns a redirect with an authorization code.
func (s *OAuthServer) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	scope := r.URL.Query().Get("scope")
	state := r.URL.Query().Get("state")

	if clientID == "" {
		http.Error(w, "client_id is required", http.StatusBadRequest)
		return
	}
	if redirectURI == "" {
		http.Error(w, "redirect_uri is required", http.StatusBadRequest)
		return
	}

	// Generate authorization code.
	code := generateCode()
	var scopes []string
	if scope != "" {
		scopes = strings.Split(scope, " ")
	}

	s.mu.Lock()
	s.codes[code] = &authCode{
		Code:        code,
		ClientID:    clientID,
		RedirectURI: redirectURI,
		Scopes:      scopes,
		CreatedAt:   time.Now(),
	}
	s.mu.Unlock()

	// Redirect with code.
	sep := "?"
	if strings.Contains(redirectURI, "?") {
		sep = "&"
	}
	location := fmt.Sprintf("%s%scode=%s", redirectURI, sep, code)
	if state != "" {
		location += "&state=" + state
	}
	http.Redirect(w, r, location, http.StatusFound)
}

// handleToken exchanges an authorization code for an access token,
// or refreshes an existing token.
func (s *OAuthServer) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	r.ParseForm()
	grantType := r.FormValue("grant_type")

	switch grantType {
	case "authorization_code":
		s.handleAuthCodeExchange(w, r)
	case "refresh_token":
		s.handleRefreshToken(w, r)
	default:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "unsupported_grant_type",
			"error_description": "Supported: authorization_code, refresh_token",
		})
	}
}

func (s *OAuthServer) handleAuthCodeExchange(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")

	if code == "" || clientID == "" || clientSecret == "" {
		oauthError(w, "invalid_request", "code, client_id, and client_secret are required")
		return
	}

	// Validate client credentials.
	if s.config.ClientID != "" && clientID != s.config.ClientID {
		oauthError(w, "invalid_client", "Invalid client_id")
		return
	}
	if s.config.ClientSecret != "" && clientSecret != s.config.ClientSecret {
		oauthError(w, "invalid_client", "Invalid client_secret")
		return
	}

	// Validate and consume authorization code.
	s.mu.Lock()
	ac, ok := s.codes[code]
	if ok {
		delete(s.codes, code)
	}
	s.mu.Unlock()

	if !ok {
		oauthError(w, "invalid_grant", "Authorization code not found or already used")
		return
	}

	// Generate access token and refresh token.
	accessToken := "wt_access_" + generateCode()
	refreshToken := "wt_refresh_" + generateCode()

	// Register tokens.
	s.tokens.Register(TokenConfig{
		Token:        accessToken,
		Scopes:       ac.Scopes,
		ExpiresAt:    time.Now().Add(s.config.TokenExpiry),
		RefreshToken: refreshToken,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"access_token":  accessToken,
		"token_type":    "Bearer",
		"expires_in":    int(s.config.TokenExpiry.Seconds()),
		"refresh_token": refreshToken,
		"scope":         strings.Join(ac.Scopes, " "),
	})
}

func (s *OAuthServer) handleRefreshToken(w http.ResponseWriter, r *http.Request) {
	refreshToken := r.FormValue("refresh_token")
	if refreshToken == "" {
		oauthError(w, "invalid_request", "refresh_token is required")
		return
	}

	// Find token by refresh token.
	existing := s.tokens.FindByRefreshToken(refreshToken)
	if existing == nil {
		oauthError(w, "invalid_grant", "Invalid refresh token")
		return
	}

	// Revoke old token, issue new one.
	s.tokens.Revoke(existing.Token)

	newAccessToken := "wt_access_" + generateCode()
	newRefreshToken := "wt_refresh_" + generateCode()

	s.tokens.Register(TokenConfig{
		Token:        newAccessToken,
		Scopes:       existing.Scopes,
		ExpiresAt:    time.Now().Add(s.config.TokenExpiry),
		RefreshToken: newRefreshToken,
		UserID:       existing.UserID,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"access_token":  newAccessToken,
		"token_type":    "Bearer",
		"expires_in":    int(s.config.TokenExpiry.Seconds()),
		"refresh_token": newRefreshToken,
		"scope":         strings.Join(existing.Scopes, " "),
	})
}

func oauthError(w http.ResponseWriter, errCode, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{
		"error":             errCode,
		"error_description": description,
	})
}

func generateCode() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
