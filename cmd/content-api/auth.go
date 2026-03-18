package main

import (
	"fmt"
	"net/http"
	"strings"
)

// authMiddleware validates Bearer tokens using the same structural checksum
// logic as the CLI's ParseLicenseKey. Phase 1 — no DB lookup, format only.
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, `{"error":"missing or invalid Authorization header"}`, http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		if !isValidLicenseKey(token) {
			http.Error(w, `{"error":"invalid license key"}`, http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// isValidLicenseKey validates the structural format of a license key:
// wt_{tier}_{org}_{random}_{check} where check = sum of payload bytes mod 256 as 2-char hex.
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
