package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// ListDomains handles GET /domains
func (h *Handler) ListDomains(w http.ResponseWriter, r *http.Request) {
	domains := h.store.Domains.Filter(func(_ string, _ store.Domain) bool { return true })
	twincore.JSON(w, http.StatusOK, map[string]any{"data": domains})
}

// GetDomain handles GET /domains/{id}
func (h *Handler) GetDomain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	domain, ok := h.store.Domains.Get(id)
	if !ok {
		twincore.JSON(w, http.StatusNotFound, map[string]any{
			"statusCode": 404, "message": "Domain not found", "name": "not_found",
		})
		return
	}
	twincore.JSON(w, http.StatusOK, domain)
}

// CreateDomain handles POST /domains
func (h *Handler) CreateDomain(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string `json:"name"`
		Region string `json:"region"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		twincore.JSON(w, http.StatusBadRequest, map[string]any{
			"statusCode": 400, "message": "name is required", "name": "validation_error",
		})
		return
	}
	if req.Region == "" {
		req.Region = "us-east-1"
	}

	id := resendID()
	domain := store.Domain{
		ID:     id,
		Object: "domain",
		Name:   req.Name,
		Status: "not_started",
		Region: req.Region,
		Records: []store.DomainRecord{
			{Record: "SPF", Name: req.Name, Type: "MX", TTL: "Auto", Status: "not_started", Value: fmt.Sprintf("feedback-smtp.%s.amazonses.com", req.Region), Priority: intPtr(10)},
			{Record: "SPF", Name: req.Name, Type: "TXT", TTL: "Auto", Status: "not_started", Value: "v=spf1 include:amazonses.com ~all"},
			{Record: "DKIM", Name: fmt.Sprintf("resend._domainkey.%s", req.Name), Type: "CNAME", TTL: "Auto", Status: "not_started", Value: fmt.Sprintf("%s.dkim.amazonses.com", id[:8])},
		},
		CreatedAt: h.store.Clock.Now().Format("2006-01-02T15:04:05.000Z"),
	}

	h.store.Domains.Set(id, domain)
	twincore.JSON(w, http.StatusCreated, domain)
}

// UpdateDomain handles PATCH /domains/{id}
func (h *Handler) UpdateDomain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	domain, ok := h.store.Domains.Get(id)
	if !ok {
		twincore.JSON(w, http.StatusNotFound, map[string]any{
			"statusCode": 404, "message": "Domain not found", "name": "not_found",
		})
		return
	}

	// Resend PATCH /domains only supports click/open tracking toggles
	// For the twin, just acknowledge the update
	h.store.Domains.Set(id, domain)
	twincore.JSON(w, http.StatusOK, domain)
}

// DeleteDomain handles DELETE /domains/{id}
func (h *Handler) DeleteDomain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, ok := h.store.Domains.Get(id); !ok {
		twincore.JSON(w, http.StatusNotFound, map[string]any{
			"statusCode": 404, "message": "Domain not found", "name": "not_found",
		})
		return
	}
	h.store.Domains.Delete(id)
	twincore.JSON(w, http.StatusOK, map[string]any{"object": "domain", "id": id, "deleted": true})
}

// VerifyDomain handles POST /domains/{id}/verify
func (h *Handler) VerifyDomain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	domain, ok := h.store.Domains.Get(id)
	if !ok {
		twincore.JSON(w, http.StatusNotFound, map[string]any{
			"statusCode": 404, "message": "Domain not found", "name": "not_found",
		})
		return
	}
	// In sim mode, verification always succeeds
	domain.Status = "verified"
	for i := range domain.Records {
		domain.Records[i].Status = "verified"
	}
	h.store.Domains.Set(id, domain)
	twincore.JSON(w, http.StatusOK, map[string]any{"object": "domain", "id": id})
}

func resendID() string {
	b := make([]byte, 12)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func intPtr(i int) *int {
	return &i
}
