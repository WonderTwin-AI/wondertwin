package api

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-twilio/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// ListVerifyServices handles GET /v2/Services
func (h *Handler) ListVerifyServices(w http.ResponseWriter, r *http.Request) {
	services := h.store.VerifyServices.List()
	twincore.JSON(w, http.StatusOK, map[string]any{
		"services": services,
		"meta": map[string]any{
			"page":           0,
			"page_size":      50,
			"first_page_url": "/v2/Services?PageSize=50&Page=0",
			"url":            "/v2/Services?PageSize=50&Page=0",
		},
	})
}

// CreateVerifyService handles POST /v2/Services
func (h *Handler) CreateVerifyService(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		twilioError(w, http.StatusBadRequest, 60200, "Unable to parse form data")
		return
	}

	friendlyName := r.FormValue("FriendlyName")
	if friendlyName == "" {
		twilioError(w, http.StatusBadRequest, 60200, "FriendlyName is required")
		return
	}

	now := h.store.Clock.Now()
	sid := h.store.VerifyServices.NextID()

	svc := store.VerifyService{
		SID:                      sid,
		AccountSID:               "AC_sim_test",
		FriendlyName:             friendlyName,
		CodeLength:               6,
		LookupEnabled:            false,
		SkipSMSToLandlines:       false,
		DTMFInputRequired:        true,
		DoNotShareWarningEnabled: false,
		DateCreated:              now.Format("Mon, 02 Jan 2006 15:04:05 +0000"),
		DateUpdated:              now.Format("Mon, 02 Jan 2006 15:04:05 +0000"),
		URL:                      fmt.Sprintf("/v2/Services/%s", sid),
	}

	h.store.VerifyServices.Set(sid, svc)
	twincore.JSON(w, http.StatusCreated, svc)
}

// GetVerifyService handles GET /v2/Services/{ServiceSid}
func (h *Handler) GetVerifyService(w http.ResponseWriter, r *http.Request) {
	sid := chi.URLParam(r, "ServiceSid")
	svc, ok := h.store.VerifyServices.Get(sid)
	if !ok {
		twilioError(w, http.StatusNotFound, 20404, "Service not found")
		return
	}
	twincore.JSON(w, http.StatusOK, svc)
}

// UpdateVerifyService handles POST /v2/Services/{ServiceSid}
func (h *Handler) UpdateVerifyService(w http.ResponseWriter, r *http.Request) {
	sid := chi.URLParam(r, "ServiceSid")
	svc, ok := h.store.VerifyServices.Get(sid)
	if !ok {
		twilioError(w, http.StatusNotFound, 20404, "Service not found")
		return
	}

	if err := r.ParseForm(); err == nil {
		if name := r.FormValue("FriendlyName"); name != "" {
			svc.FriendlyName = name
		}
		if cl := r.FormValue("CodeLength"); cl != "" {
			// simplified — just accept 4-10
			if len(cl) == 1 && cl[0] >= '4' && cl[0] <= '9' {
				svc.CodeLength = int(cl[0] - '0')
			}
		}
	}

	svc.DateUpdated = h.store.Clock.Now().Format("Mon, 02 Jan 2006 15:04:05 +0000")
	h.store.VerifyServices.Set(sid, svc)
	twincore.JSON(w, http.StatusOK, svc)
}

// DeleteVerifyService handles DELETE /v2/Services/{ServiceSid}
func (h *Handler) DeleteVerifyService(w http.ResponseWriter, r *http.Request) {
	sid := chi.URLParam(r, "ServiceSid")
	if _, ok := h.store.VerifyServices.Get(sid); !ok {
		twilioError(w, http.StatusNotFound, 20404, "Service not found")
		return
	}
	h.store.VerifyServices.Delete(sid)
	w.WriteHeader(http.StatusNoContent)
}

// UpdateVerification handles POST /v2/Services/{ServiceSid}/Verifications/{Sid}
// Used to cancel a pending verification.
func (h *Handler) UpdateVerification(w http.ResponseWriter, r *http.Request) {
	sid := chi.URLParam(r, "Sid")
	v, ok := h.store.Verifications.Get(sid)
	if !ok {
		twilioError(w, http.StatusNotFound, 20404, "Verification not found")
		return
	}

	if err := r.ParseForm(); err == nil {
		if status := r.FormValue("Status"); status == "canceled" {
			if v.Status == store.VerificationStatusPending {
				v.Status = store.VerificationStatusCanceled
				v.UpdatedAt = h.store.Clock.Now().Format("2006-01-02T15:04:05Z")
				h.store.Verifications.Set(sid, v)
			}
		}
	}

	twincore.JSON(w, http.StatusOK, verificationResponse(v))
}
