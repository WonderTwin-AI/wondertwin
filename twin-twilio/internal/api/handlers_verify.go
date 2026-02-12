package api

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/pkg/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-twilio/internal/store"
)

// CreateVerification handles POST /v2/Services/{ServiceSid}/Verifications
// Sends a verification code to the given phone number.
func (h *Handler) CreateVerification(w http.ResponseWriter, r *http.Request) {
	serviceSID := chi.URLParam(r, "ServiceSid")

	if err := r.ParseForm(); err != nil {
		twilioError(w, http.StatusBadRequest, 60200, "Unable to parse form data: "+err.Error())
		return
	}

	to := r.FormValue("To")
	channel := r.FormValue("Channel")
	if to == "" {
		twilioError(w, http.StatusBadRequest, 60200, "A 'To' phone number or email is required.")
		return
	}
	if channel == "" {
		channel = "sms"
	}

	now := h.store.Clock.Now()
	sid := h.store.Verifications.NextID()
	code := generateCode(6)
	ttl := time.Duration(h.store.OTPTTLSeconds) * time.Second

	v := store.Verification{
		SID:        sid,
		ServiceSID: serviceSID,
		To:         to,
		Channel:    channel,
		Status:     store.VerificationStatusPending,
		Code:       code,
		Valid:      false,
		CreatedAt:  now.Format(time.RFC3339),
		UpdatedAt:  now.Format(time.RFC3339),
		ExpiresAt:  now.Add(ttl).Format(time.RFC3339),
		URL:        fmt.Sprintf("/v2/Services/%s/Verifications/%s", serviceSID, sid),
	}

	h.store.Verifications.Set(sid, v)

	twincore.JSON(w, http.StatusCreated, verificationResponse(v))
}

// CheckVerification handles POST /v2/Services/{ServiceSid}/VerificationCheck
// Checks a verification code against the stored verification.
func (h *Handler) CheckVerification(w http.ResponseWriter, r *http.Request) {
	serviceSID := chi.URLParam(r, "ServiceSid")

	if err := r.ParseForm(); err != nil {
		twilioError(w, http.StatusBadRequest, 60200, "Unable to parse form data: "+err.Error())
		return
	}

	to := r.FormValue("To")
	code := r.FormValue("Code")
	if to == "" {
		twilioError(w, http.StatusBadRequest, 60200, "A 'To' phone number or email is required.")
		return
	}
	if code == "" {
		twilioError(w, http.StatusBadRequest, 60200, "A 'Code' is required.")
		return
	}

	now := h.store.Clock.Now()

	// Find the most recent pending verification for this number + service
	verifications := h.store.Verifications.List()
	var found *store.Verification
	for i := len(verifications) - 1; i >= 0; i-- {
		v := verifications[i]
		if v.To == to && v.ServiceSID == serviceSID && v.Status == store.VerificationStatusPending {
			found = &v
			break
		}
	}

	if found == nil {
		twilioError(w, http.StatusNotFound, 60200, "Verification not found or already consumed.")
		return
	}

	// Check expiry
	expiresAt, _ := time.Parse(time.RFC3339, found.ExpiresAt)
	if now.After(expiresAt) {
		found.Status = store.VerificationStatusExpired
		found.UpdatedAt = now.Format(time.RFC3339)
		h.store.Verifications.Set(found.SID, *found)
		twilioError(w, http.StatusNotFound, 60202, "Verification code has expired.")
		return
	}

	// Check code
	if found.Code == code {
		found.Status = store.VerificationStatusApproved
		found.Valid = true
	} else {
		// Twilio returns the verification with valid=false, not an error
		found.Valid = false
	}
	found.UpdatedAt = now.Format(time.RFC3339)
	h.store.Verifications.Set(found.SID, *found)

	twincore.JSON(w, http.StatusOK, verificationResponse(*found))
}

// GetVerification handles GET /v2/Services/{ServiceSid}/Verifications/{Sid}
func (h *Handler) GetVerification(w http.ResponseWriter, r *http.Request) {
	sid := chi.URLParam(r, "Sid")

	v, ok := h.store.Verifications.Get(sid)
	if !ok {
		twilioError(w, http.StatusNotFound, 20404, "Verification not found.")
		return
	}

	// Check if expired
	now := h.store.Clock.Now()
	expiresAt, _ := time.Parse(time.RFC3339, v.ExpiresAt)
	if v.Status == store.VerificationStatusPending && now.After(expiresAt) {
		v.Status = store.VerificationStatusExpired
		v.UpdatedAt = now.Format(time.RFC3339)
		h.store.Verifications.Set(v.SID, v)
	}

	twincore.JSON(w, http.StatusOK, verificationResponse(v))
}

// AdminListVerifications handles GET /admin/verifications
// Supports ?to={phone} query parameter for filtering.
func (h *Handler) AdminListVerifications(w http.ResponseWriter, r *http.Request) {
	to := r.URL.Query().Get("to")

	verifications := h.store.Verifications.List()

	if to != "" {
		var filtered []store.Verification
		for _, v := range verifications {
			if v.To == to {
				filtered = append(filtered, v)
			}
		}
		verifications = filtered
	}

	// Include codes in admin response
	type adminVerification struct {
		store.Verification
		Code string `json:"code"`
	}
	result := make([]adminVerification, len(verifications))
	for i, v := range verifications {
		result[i] = adminVerification{Verification: v, Code: v.Code}
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"verifications": result,
		"total":         len(result),
	})
}

// AdminExpireVerification handles POST /admin/verifications/{sid}/expire
func (h *Handler) AdminExpireVerification(w http.ResponseWriter, r *http.Request) {
	sid := chi.URLParam(r, "sid")

	v, ok := h.store.Verifications.Get(sid)
	if !ok {
		twincore.Error(w, http.StatusNotFound, "Verification not found: "+sid)
		return
	}

	if v.Status != store.VerificationStatusPending {
		twincore.Error(w, http.StatusBadRequest, "Verification is not pending, current status: "+v.Status)
		return
	}

	v.Status = store.VerificationStatusCanceled
	v.UpdatedAt = h.store.Clock.Now().Format(time.RFC3339)
	h.store.Verifications.Set(sid, v)

	twincore.JSON(w, http.StatusOK, verificationResponse(v))
}

// verificationResponse creates a Twilio-compatible verification response.
// The code is never included in API responses.
func verificationResponse(v store.Verification) map[string]any {
	return map[string]any{
		"sid":          v.SID,
		"service_sid":  v.ServiceSID,
		"account_sid":  "AC_sim_test",
		"to":           v.To,
		"channel":      v.Channel,
		"status":       v.Status,
		"valid":        v.Valid,
		"date_created": v.CreatedAt,
		"date_updated": v.UpdatedAt,
		"url":          v.URL,
	}
}

// twilioError writes a Twilio-style error response.
func twilioError(w http.ResponseWriter, status int, code int, message string) {
	resp := map[string]any{
		"code":    code,
		"message": message,
		"status":  status,
	}
	data, _ := json.Marshal(resp)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(data)
}

// generateCode generates a random numeric code of the given length.
func generateCode(length int) string {
	code := ""
	for i := 0; i < length; i++ {
		n, _ := rand.Int(rand.Reader, big.NewInt(10))
		code += fmt.Sprintf("%d", n.Int64())
	}
	return code
}
