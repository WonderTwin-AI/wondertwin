package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// authURL is the wt-auth service URL for token verification.
var authURL string

func init() {
	authURL = os.Getenv("WT_AUTH_URL")
}

// authMiddleware validates Bearer tokens. If WT_AUTH_URL is set, tokens are
// verified against the wt-auth service. Otherwise, falls back to structural
// license key validation (development mode).
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, `{"error":"missing or invalid Authorization header"}`, http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(auth, "Bearer ")

		if authURL != "" {
			// Production mode: verify JIT token against wt-auth.
			twinName := extractTwinName(r)
			if err := verifyTokenWithAuth(token, twinName); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusUnauthorized)
				return
			}
		} else {
			// Development mode: structural license key validation only.
			if !isValidLicenseKey(token) {
				http.Error(w, `{"error":"invalid license key"}`, http.StatusUnauthorized)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// verifyTokenWithAuth calls POST /v1/tokens/verify on wt-auth.
func verifyTokenWithAuth(token, twinName string) error {
	body, _ := json.Marshal(map[string]string{
		"token":     token,
		"twin_name": twinName,
	})

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(authURL+"/v1/tokens/verify", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("auth service unavailable: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Valid bool   `json:"valid"`
		Error string `json:"error,omitempty"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if !result.Valid {
		if result.Error != "" {
			return fmt.Errorf("%s", result.Error)
		}
		return fmt.Errorf("token verification failed")
	}

	return nil
}

// extractTwinName pulls the twin name from the request path.
// Path format: /v1/content/{twin}@{version}
func extractTwinName(r *http.Request) string {
	spec := strings.TrimPrefix(r.URL.Path, "/v1/content/")
	if i := strings.LastIndex(spec, "@"); i > 0 {
		return spec[:i]
	}
	return spec
}

// isValidLicenseKey validates the structural format of a license key:
// wt_{tier}_{org}_{random}_{check} where check = sum of payload bytes mod 256 as 2-char hex.
// Used in development mode when WT_AUTH_URL is not set.
func isValidLicenseKey(key string) bool {
	parts := strings.Split(key, "_")
	if len(parts) < 5 {
		return false
	}

	if parts[0] != "wt" {
		return false
	}

	tier := parts[1]
	if tier != "com" && tier != "ent" {
		return false
	}

	if parts[2] == "" {
		return false
	}

	check := parts[len(parts)-1]
	if len(check) != 2 {
		return false
	}

	random := strings.Join(parts[3:len(parts)-1], "_")
	if len(random) < 6 {
		return false
	}

	payload := strings.Join(parts[:len(parts)-1], "_")
	var sum byte
	for _, b := range []byte(payload) {
		sum += b
	}
	expected := fmt.Sprintf("%02x", sum)
	return check == expected
}
