package api

import (
	"cmp"
	"encoding/base64"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// ListDomains handles GET /domains, newest first; every domain when limit is
// absent (quirk pagination-002). Items carry no object and no records (probe
// m03 of 2026-11-08).
func (h *Handler) ListDomains(w http.ResponseWriter, r *http.Request) {
	pq, ok := readPageQuery(w, r)
	if !ok {
		return
	}
	domains := h.store.Domains.List()
	items := make([]listedDomain, len(domains))
	for i, d := range domains {
		items[i] = listedDomain{
			ID: d.ID, Name: d.Name, Status: d.Status, CreatedAt: d.CreatedAt, Region: d.Region,
			Capabilities: d.Capabilities, OpenTracking: d.OpenTracking, ClickTracking: d.ClickTracking,
		}
	}
	twincore.JSON(w, http.StatusOK, paginate(items, pq, 0, func(d listedDomain) string { return d.ID }))
}

// listedDomain is a domain as GET /domains lists it.
type listedDomain struct {
	ID            string                   `json:"id"`
	Name          string                   `json:"name"`
	Status        string                   `json:"status"`
	CreatedAt     string                   `json:"created_at"`
	Region        string                   `json:"region"`
	Capabilities  store.DomainCapabilities `json:"capabilities"`
	OpenTracking  bool                     `json:"open_tracking"`
	ClickTracking bool                     `json:"click_tracking"`
}

// GetDomain handles GET /domains/{id}: the domain with its records and
// tracking switches (probe m02 of 2026-11-08).
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

// domainRegions are the regions POST /domains accepts, in Resend's order.
var domainRegions = []string{"us-east-1", "eu-west-1", "sa-east-1", "ap-northeast-1"}

const domainRegionMessage = "The `region` field must be one of \"us-east-1\", \"eu-west-1\", \"sa-east-1\", or \"ap-northeast-1\"."

// CreateDomain handles POST /domains. It answers the domain without its
// tracking switches, which only GET reports (probes m01, m05 to m07 of
// 2026-11-08). open_tracking, click_tracking and capabilities in the request
// are kept as given; that create takes them is from the SDK types, not a
// recording.
func (h *Handler) CreateDomain(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name          string                   `json:"name"`
		Region        string                   `json:"region"`
		OpenTracking  bool                     `json:"open_tracking"`
		ClickTracking bool                     `json:"click_tracking"`
		Capabilities  store.DomainCapabilities `json:"capabilities"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Name == "" {
		missingRequiredField(w, "name")
		return
	}
	if req.Region == "" {
		req.Region = "us-east-1"
	}
	if !slices.Contains(domainRegions, req.Region) {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", domainRegionMessage)
		return
	}
	for _, d := range h.store.Domains.List() {
		if strings.EqualFold(d.Name, req.Name) {
			writeError(w, http.StatusForbidden, "validation_error",
				fmt.Sprintf("The %s domain has been registered already.", req.Name))
			return
		}
	}

	id := h.resendID()
	domain := store.Domain{
		ID:     id,
		Object: "domain",
		Name:   req.Name,
		Status: "not_started",
		Region: req.Region,
		Capabilities: store.DomainCapabilities{
			Sending:   cmp.Or(req.Capabilities.Sending, "enabled"),
			Receiving: cmp.Or(req.Capabilities.Receiving, "disabled"),
		},
		OpenTracking:  req.OpenTracking,
		ClickTracking: req.ClickTracking,
		Records: []store.DomainRecord{
			{Record: "DKIM", Name: "resend._domainkey", Type: "TXT", TTL: "Auto", Status: "not_started", Value: "p=" + h.dkimPublicKey()},
			{Record: "SPF", Name: "send", Type: "MX", TTL: "Auto", Status: "not_started", Value: fmt.Sprintf("feedback-smtp.%s.amazonses.com", req.Region), Priority: intPtr(10)},
			{Record: "SPF", Name: "send", Type: "TXT", TTL: "Auto", Status: "not_started", Value: "v=spf1 include:amazonses.com ~all"},
			{Record: "SPF", Name: "rsend", Type: "CNAME", TTL: "Auto", Status: "not_started", Value: "send.forge.rmta.net"},
		},
		CreatedAt: h.store.Clock.Now().UTC().Format(pgTimestamp),
	}

	h.store.Domains.Set(id, domain)
	twincore.JSON(w, http.StatusCreated, map[string]any{
		"object": domain.Object, "id": domain.ID, "name": domain.Name, "created_at": domain.CreatedAt,
		"status": domain.Status, "capabilities": domain.Capabilities, "records": domain.Records,
		"region": domain.Region,
	})
}

// rsaKeyHeader and rsaKeyTrailer wrap a 1024-bit modulus as the DER
// SubjectPublicKeyInfo Resend publishes in its DKIM record, exponent 65537.
var (
	rsaKeyHeader  = []byte{0x30, 0x81, 0x9f, 0x30, 0x0d, 0x06, 0x09, 0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x01, 0x01, 0x05, 0x00, 0x03, 0x81, 0x8d, 0x00, 0x30, 0x81, 0x89, 0x02, 0x81, 0x81, 0x00}
	rsaKeyTrailer = []byte{0x02, 0x03, 0x01, 0x00, 0x01}
)

// dkimPublicKey returns a base64 RSA public key of Resend's form, its
// modulus drawn from the store seed. It is shaped like a key, not usable as
// one.
func (h *Handler) dkimPublicKey() string {
	modulus := make([]byte, 128)
	h.store.RandBytes(modulus)
	modulus[0] |= 0x80
	modulus[127] |= 0x01
	der := slices.Concat(rsaKeyHeader, modulus, rsaKeyTrailer)
	return base64.StdEncoding.EncodeToString(der)
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
