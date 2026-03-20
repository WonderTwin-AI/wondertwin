package twincore

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// TokenConfig defines a registered token.
type TokenConfig struct {
	Token        string    `json:"token"`
	Scopes       []string  `json:"scopes,omitempty"`
	UserID       string    `json:"user_id,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
}

// TokenRegistry manages registered tokens.
// If no tokens are registered, all bearer tokens are accepted (open mode).
// Once any token is registered, only registered tokens are accepted.
type TokenRegistry struct {
	mu     sync.RWMutex
	tokens map[string]*TokenConfig // access token → config
}

// NewTokenRegistry creates a new token registry.
func NewTokenRegistry() *TokenRegistry {
	return &TokenRegistry{
		tokens: make(map[string]*TokenConfig),
	}
}

// Register adds a token to the registry.
func (tr *TokenRegistry) Register(cfg TokenConfig) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.tokens[cfg.Token] = &cfg
}

// Revoke removes a token from the registry.
func (tr *TokenRegistry) Revoke(token string) bool {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	_, ok := tr.tokens[token]
	delete(tr.tokens, token)
	return ok
}

// Validate checks if a token is valid and not expired.
// Returns the token config if valid, nil otherwise.
// In open mode (no tokens registered), returns a default config.
func (tr *TokenRegistry) Validate(token string) *TokenConfig {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	// Open mode: no tokens registered = accept any.
	if len(tr.tokens) == 0 {
		return &TokenConfig{Token: token}
	}

	cfg, ok := tr.tokens[token]
	if !ok {
		return nil
	}

	// Check expiry.
	if !cfg.ExpiresAt.IsZero() && time.Now().After(cfg.ExpiresAt) {
		return nil
	}

	return cfg
}

// HasScope checks if a token has a specific scope.
func (tr *TokenRegistry) HasScope(token, scope string) bool {
	cfg := tr.Validate(token)
	if cfg == nil {
		return false
	}
	if len(cfg.Scopes) == 0 {
		return true // no scopes = full access
	}
	for _, s := range cfg.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// FindByRefreshToken finds a token config by its refresh token.
func (tr *TokenRegistry) FindByRefreshToken(refreshToken string) *TokenConfig {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	for _, cfg := range tr.tokens {
		if cfg.RefreshToken == refreshToken {
			return cfg
		}
	}
	return nil
}

// List returns all registered tokens (without exposing the token values directly).
func (tr *TokenRegistry) List() []TokenConfig {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	result := make([]TokenConfig, 0, len(tr.tokens))
	for _, cfg := range tr.tokens {
		result = append(result, *cfg)
	}
	return result
}

// Reset clears all tokens.
func (tr *TokenRegistry) Reset() {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.tokens = make(map[string]*TokenConfig)
}

// Count returns the number of registered tokens.
func (tr *TokenRegistry) Count() int {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	return len(tr.tokens)
}

// TokenAuth returns middleware that validates tokens against the registry.
// Extracts token from Authorization: Bearer <token> header.
// In open mode (no tokens registered), any non-empty bearer token is accepted.
func (tr *TokenRegistry) TokenAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			// Also check X-Api-Key header.
			apiKey := r.Header.Get("X-Api-Key")
			if apiKey == "" {
				Error(w, http.StatusUnauthorized, "Authorization required")
				return
			}
			auth = "Bearer " + apiKey
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		if token == "" {
			Error(w, http.StatusUnauthorized, "Authorization required")
			return
		}

		cfg := tr.Validate(token)
		if cfg == nil {
			Error(w, http.StatusUnauthorized, "Invalid or expired token")
			return
		}

		next.ServeHTTP(w, r)
	})
}
