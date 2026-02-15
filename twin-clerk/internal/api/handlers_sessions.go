package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-clerk/internal/store"
)

// adminCreateSessionRequest is the JSON body for POST /admin/sessions.
type adminCreateSessionRequest struct {
	UserID string `json:"user_id"`
}

// AdminCreateSession creates a session for a user (admin/test endpoint).
func (h *Handler) AdminCreateSession(w http.ResponseWriter, r *http.Request) {
	var req adminCreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		clerkError(w, http.StatusBadRequest, "form_param_invalid",
			"Invalid request body.", err.Error())
		return
	}

	if req.UserID == "" {
		clerkError(w, http.StatusBadRequest, "form_param_missing",
			"user_id is required.", "You must provide a user_id to create a session.")
		return
	}

	// Verify user exists
	if _, ok := h.store.Users.Get(req.UserID); !ok {
		clerkError(w, http.StatusNotFound, "resource_not_found",
			"User not found.",
			fmt.Sprintf("No user was found with id %s.", req.UserID))
		return
	}

	id := h.store.Sessions.NextID()
	now := store.Now()

	// Session expires in 7 days, abandoned in 30 days
	expireAt := now + 7*24*60*60*1000
	abandonAt := now + 30*24*60*60*1000

	session := store.Session{
		ID:           id,
		Object:       "session",
		UserID:       req.UserID,
		ClientID:     fmt.Sprintf("client_%s", id),
		Status:       "active",
		LastActiveAt: now,
		ExpireAt:     expireAt,
		AbandonAt:    abandonAt,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	h.store.Sessions.Set(id, session)
	twincore.JSON(w, http.StatusOK, session)
}

// GetSession handles GET /v1/sessions/{id}.
func (h *Handler) GetSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	session, ok := h.store.Sessions.Get(id)
	if !ok {
		clerkError(w, http.StatusNotFound, "resource_not_found",
			"Session not found.",
			fmt.Sprintf("No session was found with id %s.", id))
		return
	}

	twincore.JSON(w, http.StatusOK, session)
}

// ListSessions handles GET /v1/sessions.
func (h *Handler) ListSessions(w http.ResponseWriter, r *http.Request) {
	limit := 10
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		fmt.Sscanf(o, "%d", &offset)
	}

	// Filter by user_id and status
	userID := r.URL.Query().Get("user_id")
	status := r.URL.Query().Get("status")

	allSessions := h.store.Sessions.List()

	var filtered []store.Session
	for _, s := range allSessions {
		if userID != "" && s.UserID != userID {
			continue
		}
		if status != "" && s.Status != status {
			continue
		}
		filtered = append(filtered, s)
	}

	total := len(filtered)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := filtered[offset:end]

	if page == nil {
		page = []store.Session{}
	}

	twincore.JSON(w, http.StatusOK, store.ClerkList[store.Session]{
		Data:       page,
		TotalCount: total,
	})
}

// verifySessionRequest is the JSON body for POST /v1/sessions/{id}/verify.
type verifySessionRequest struct {
	Token string `json:"token"`
}

// VerifySession handles POST /v1/sessions/{id}/verify.
// In the real Clerk API, this verifies a session token. For the twin,
// we just check the session exists and is active.
func (h *Handler) VerifySession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	session, ok := h.store.Sessions.Get(id)
	if !ok {
		clerkError(w, http.StatusNotFound, "resource_not_found",
			"Session not found.",
			fmt.Sprintf("No session was found with id %s.", id))
		return
	}

	if session.Status != "active" {
		clerkError(w, http.StatusUnauthorized, "session_not_active",
			"Session is not active.",
			fmt.Sprintf("Session %s has status %s.", id, session.Status))
		return
	}

	// Generate a fresh JWT for this session
	token, err := h.jwtMgr.GenerateToken(session.UserID, session.ID, nil)
	if err != nil {
		clerkError(w, http.StatusInternalServerError, "internal_error",
			"Failed to generate session token.", err.Error())
		return
	}

	// Update last active
	now := store.Now()
	session.LastActiveAt = now
	session.UpdatedAt = now
	session.LastActiveToken = &store.TokenResponse{
		Object: "token",
		JWT:    token,
	}
	h.store.Sessions.Set(id, session)

	twincore.JSON(w, http.StatusOK, session)
}

// RevokeSession handles POST /v1/sessions/{id}/revoke.
func (h *Handler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	session, ok := h.store.Sessions.Get(id)
	if !ok {
		clerkError(w, http.StatusNotFound, "resource_not_found",
			"Session not found.",
			fmt.Sprintf("No session was found with id %s.", id))
		return
	}

	session.Status = "revoked"
	session.UpdatedAt = store.Now()
	h.store.Sessions.Set(id, session)

	twincore.JSON(w, http.StatusOK, session)
}
