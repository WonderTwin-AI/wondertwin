package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
)

// jwtSecret signs every token the emulator issues. Xero signs with a key the
// emulator does not have, and xero-python never verifies the signature.
var jwtSecret = []byte("twin-xero")

// signJWT returns a compact HS256 JWT over the claims.
func signJWT(claims map[string]any) string {
	enc := base64.RawURLEncoding
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT", "kid": "twin-xero"})
	payload, _ := json.Marshal(claims)
	signing := enc.EncodeToString(header) + "." + enc.EncodeToString(payload)
	mac := hmac.New(sha256.New, jwtSecret)
	mac.Write([]byte(signing))
	return signing + "." + enc.EncodeToString(mac.Sum(nil))
}
