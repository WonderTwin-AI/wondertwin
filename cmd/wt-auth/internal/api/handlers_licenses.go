package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/cmd/wt-auth/internal/store"
)

// --- Internal: Create License ---

type createLicenseRequest struct {
	LicenseKey    string `json:"license_key"`
	OrgID         string `json:"org_id"`
	OrgName       string `json:"org_name"`
	Tier          string `json:"tier"`
	TwinName      string `json:"twin_name"`
	TwinClass     string `json:"twin_class"`
	BillingPeriod string `json:"billing_period"`
	ExpiresAt     *int64 `json:"expires_at,omitempty"`
}

func (h *Handler) handleCreateLicense(w http.ResponseWriter, r *http.Request) {
	var req createLicenseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	// Ensure org exists, create if needed.
	_, err := h.licenses.GetOrg(req.OrgID)
	if err != nil {
		org := &store.Org{
			ID:        req.OrgID,
			Name:      req.OrgName,
			Tier:      req.Tier,
			CreatedAt: time.Now(),
			Active:    true,
		}
		if req.OrgName == "" {
			org.Name = req.OrgID
		}
		if err := h.licenses.CreateOrg(org); err != nil {
			writeError(w, http.StatusInternalServerError, "org_creation_failed")
			return
		}
	}

	lic := &store.License{
		ID:            req.LicenseKey,
		OrgID:         req.OrgID,
		Tier:          req.Tier,
		TwinName:      req.TwinName,
		TwinClass:     req.TwinClass,
		BillingPeriod: req.BillingPeriod,
		Active:        true,
		IssuedAt:      time.Now(),
	}
	if req.ExpiresAt != nil {
		t := time.Unix(*req.ExpiresAt, 0)
		lic.ExpiresAt = &t
	}

	if err := h.licenses.CreateLicense(lic); err != nil {
		writeError(w, http.StatusInternalServerError, "license_creation_failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, lic)
}

// --- Internal: Revoke License ---

func (h *Handler) handleRevokeLicense(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Reason == "" {
		body.Reason = "revoked via API"
	}

	if err := h.licenses.RevokeLicense(id, body.Reason); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// --- Internal: List Licenses ---

func (h *Handler) handleListLicenses(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")
	licenses, err := h.licenses.ListLicenses(orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"licenses": licenses})
}

// --- Internal Auth Middleware ---

func (h *Handler) internalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "missing authorization")
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		if token != h.internalKey {
			writeError(w, http.StatusUnauthorized, "invalid internal key")
			return
		}
		next.ServeHTTP(w, r)
	})
}
