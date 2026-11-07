package api

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// ListDomains handles GET /domains
func (h *Handler) ListDomains(w http.ResponseWriter, r *http.Request) {
	domains := h.store.Domains.Filter(func(_ string, _ store.Domain) bool { return true })
	twincore.JSON(w, http.StatusOK, listBody(domains))
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
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Name == "" {
		missingField(w, "name")
		return
	}
	if req.Region == "" {
		req.Region = "us-east-1"
	}

	id := h.resendID()
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
		CreatedAt: h.store.Clock.Now().UTC().Format(pgTimestamp),
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
	// Verification lands at once on the outcome /admin/config sets:
	// verified by default, or pending or failed (architecture decision 7,
	// quirk lifecycle-006).
	outcome := h.verifyOutcome()
	domain.Status = outcome
	for i := range domain.Records {
		domain.Records[i].Status = outcome
	}
	h.store.Domains.Set(id, domain)
	twincore.JSON(w, http.StatusOK, map[string]any{"object": "domain", "id": id})
}

// Domain verification outcomes /admin/config can set.
const (
	domainStatusPending = "pending"
	domainStatusFailed  = "failed"
)

func validVerifyOutcome(s string) bool {
	switch s {
	case store.DomainStatusVerified, domainStatusPending, domainStatusFailed:
		return true
	}
	return false
}

// verifyOutcome is the status POST /domains/{id}/verify gives a domain.
func (h *Handler) verifyOutcome() string {
	if s, ok := h.verifyMode.Load().(string); ok && s != "" {
		return s
	}
	return store.DomainStatusVerified
}

// resendID returns a UUIDv4 for a new resource other than an email.
func (h *Handler) resendID() string {
	return h.store.NewID()
}

func intPtr(i int) *int {
	return &i
}
