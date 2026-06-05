package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"time"

	"github.com/wondertwin-ai/wondertwin/cmd/wt-auth/internal/store"
	"github.com/wondertwin-ai/wondertwin/internal/config"
)

const tokenTTL = 5 * time.Minute

// --- Token Issuance ---

type issueTokenRequest struct {
	LicenseKey string `json:"license_key"`
	TwinName   string `json:"twin_name"`
}

type issueTokenResponse struct {
	Token             string `json:"token"`
	TwinName          string `json:"twin_name"`
	OrgID             string `json:"org_id"`
	ExpiresAt         int64  `json:"expires_at"`
	TelemetryRequired bool   `json:"telemetry_required"`
}

func (h *Handler) handleIssueToken(w http.ResponseWriter, r *http.Request) {
	var req issueTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	if req.LicenseKey == "" || req.TwinName == "" {
		writeError(w, http.StatusBadRequest, "license_key and twin_name are required")
		return
	}

	// Parse and structurally validate the license key.
	info := config.ParseLicenseKey(req.LicenseKey)
	if info == nil {
		writeError(w, http.StatusUnauthorized, "invalid_license")
		return
	}

	// Look up the license in the database.
	lic, err := h.licenses.FindLicense(info.Org, req.TwinName)
	if err != nil {
		writeError(w, http.StatusForbidden, "twin_not_licensed")
		return
	}

	// Check the full license key matches. ConstantTimeCompare avoids
	// the timing oracle that `lic.ID != req.LicenseKey` would create
	// on the byte-by-byte equality check. Audit 2026-06-04, section 3.
	if subtle.ConstantTimeCompare([]byte(lic.ID), []byte(req.LicenseKey)) != 1 {
		writeError(w, http.StatusUnauthorized, "invalid_license")
		return
	}

	// Check revocation.
	if lic.IsRevoked() {
		writeError(w, http.StatusForbidden, "license_revoked")
		return
	}

	// Check expiry — hard stop, no graceful degradation.
	if lic.IsExpired() {
		writeError(w, http.StatusForbidden, "license_expired")
		return
	}

	// Issue JIT token.
	telemetryRequired := true // always required for licensed twins
	tokenStr, claims, err := h.signer.Issue(info.Org, req.TwinName, tokenTTL, telemetryRequired)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_generation_failed")
		return
	}

	// Record the token in the database.
	dbToken := &store.Token{
		ID:        claims.ID,
		LicenseID: lic.ID,
		OrgID:     info.Org,
		TwinName:  req.TwinName,
		IssuedAt:  time.Unix(claims.IssuedAt, 0),
		ExpiresAt: time.Unix(claims.ExpiresAt, 0),
	}
	if err := h.tokens.CreateToken(dbToken); err != nil {
		writeError(w, http.StatusInternalServerError, "token_storage_failed")
		return
	}

	writeJSON(w, http.StatusOK, issueTokenResponse{
		Token:             tokenStr,
		TwinName:          req.TwinName,
		OrgID:             info.Org,
		ExpiresAt:         claims.ExpiresAt,
		TelemetryRequired: telemetryRequired,
	})
}

// --- Token Verification ---

type verifyTokenRequest struct {
	Token    string `json:"token"`
	TwinName string `json:"twin_name"`
}

type verifyTokenResponse struct {
	Valid             bool   `json:"valid"`
	OrgID             string `json:"org_id,omitempty"`
	TwinName          string `json:"twin_name,omitempty"`
	TelemetryRequired bool   `json:"telemetry_required,omitempty"`
	Error             string `json:"error,omitempty"`
}

func (h *Handler) handleVerifyToken(w http.ResponseWriter, r *http.Request) {
	var req verifyTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	if req.Token == "" || req.TwinName == "" {
		writeError(w, http.StatusBadRequest, "token and twin_name are required")
		return
	}

	// Verify the token signature and expiry.
	claims, err := h.signer.Verify(req.Token)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, verifyTokenResponse{
			Valid: false,
			Error: err.Error(),
		})
		return
	}

	// Check twin name matches.
	if claims.Twin != req.TwinName {
		writeJSON(w, http.StatusUnauthorized, verifyTokenResponse{
			Valid: false,
			Error: "token not valid for this twin",
		})
		return
	}

	writeJSON(w, http.StatusOK, verifyTokenResponse{
		Valid:             true,
		OrgID:             claims.Org,
		TwinName:          claims.Twin,
		TelemetryRequired: claims.Telemetry,
	})
}
