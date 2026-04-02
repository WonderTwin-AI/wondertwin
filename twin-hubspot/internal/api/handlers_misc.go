package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-hubspot/internal/store"
)

// --- Owners ---

// ListOwners handles GET /crm/v3/owners
func (h *Handler) ListOwners(w http.ResponseWriter, r *http.Request) {
	owners := h.store.Owners.List()
	if owners == nil {
		owners = []store.Owner{}
	}
	hsJSON(w, 200, map[string]any{"results": owners})
}

// GetOwner handles GET /crm/v3/owners/{ownerId}
func (h *Handler) GetOwner(w http.ResponseWriter, r *http.Request) {
	ownerID := chi.URLParam(r, "ownerId")
	owner, ok := h.store.Owners.Get(ownerID)
	if !ok {
		hsError(w, 404, "OBJECT_NOT_FOUND", "Owner "+ownerID+" not found")
		return
	}
	hsJSON(w, 200, owner)
}

// --- Lists ---

// ListLists handles GET /crm/v3/lists
func (h *Handler) ListLists(w http.ResponseWriter, r *http.Request) {
	lists := h.store.Lists.List()
	if lists == nil {
		lists = []store.List{}
	}
	hsJSON(w, 200, map[string]any{"lists": lists})
}

// CreateList handles POST /crm/v3/lists
func (h *Handler) CreateList(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name           string `json:"name"`
		ObjectTypeID   string `json:"objectTypeId"`
		ProcessingType string `json:"processingType"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body")
		return
	}
	if body.Name == "" {
		hsError(w, 400, "VALIDATION_ERROR", "List name is required")
		return
	}
	if body.ProcessingType == "" {
		body.ProcessingType = "MANUAL"
	}

	id := h.store.NextID()
	list := store.List{
		ID:             id,
		Name:           body.Name,
		ObjectTypeID:   body.ObjectTypeID,
		ProcessingType: body.ProcessingType,
		MemberIDs:      []string{},
	}
	h.store.Lists.Set(id, list)
	hsJSON(w, 201, list)
}

// GetList handles GET /crm/v3/lists/{listId}
func (h *Handler) GetList(w http.ResponseWriter, r *http.Request) {
	listID := chi.URLParam(r, "listId")
	list, ok := h.store.Lists.Get(listID)
	if !ok {
		hsError(w, 404, "OBJECT_NOT_FOUND", "List "+listID+" not found")
		return
	}
	hsJSON(w, 200, list)
}

// DeleteList handles DELETE /crm/v3/lists/{listId}
func (h *Handler) DeleteList(w http.ResponseWriter, r *http.Request) {
	listID := chi.URLParam(r, "listId")
	if !h.store.Lists.Delete(listID) {
		hsError(w, 404, "OBJECT_NOT_FOUND", "List "+listID+" not found")
		return
	}
	w.WriteHeader(204)
}

// GetListMemberships handles GET /crm/v3/lists/{listId}/memberships
func (h *Handler) GetListMemberships(w http.ResponseWriter, r *http.Request) {
	listID := chi.URLParam(r, "listId")
	list, ok := h.store.Lists.Get(listID)
	if !ok {
		hsError(w, 404, "OBJECT_NOT_FOUND", "List "+listID+" not found")
		return
	}
	memberIDs := list.MemberIDs
	if memberIDs == nil {
		memberIDs = []string{}
	}
	hsJSON(w, 200, map[string]any{"results": memberIDs})
}

// AddListMembers handles PUT /crm/v3/lists/{listId}/memberships/add
func (h *Handler) AddListMembers(w http.ResponseWriter, r *http.Request) {
	listID := chi.URLParam(r, "listId")
	list, ok := h.store.Lists.Get(listID)
	if !ok {
		hsError(w, 404, "OBJECT_NOT_FOUND", "List "+listID+" not found")
		return
	}

	var body []string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body: expected array of IDs")
		return
	}

	existing := make(map[string]bool, len(list.MemberIDs))
	for _, id := range list.MemberIDs {
		existing[id] = true
	}
	for _, id := range body {
		if !existing[id] {
			list.MemberIDs = append(list.MemberIDs, id)
			existing[id] = true
		}
	}
	h.store.Lists.Set(listID, list)
	w.WriteHeader(204)
}

// RemoveListMembers handles PUT /crm/v3/lists/{listId}/memberships/remove
func (h *Handler) RemoveListMembers(w http.ResponseWriter, r *http.Request) {
	listID := chi.URLParam(r, "listId")
	list, ok := h.store.Lists.Get(listID)
	if !ok {
		hsError(w, 404, "OBJECT_NOT_FOUND", "List "+listID+" not found")
		return
	}

	var body []string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body: expected array of IDs")
		return
	}

	toRemove := make(map[string]bool, len(body))
	for _, id := range body {
		toRemove[id] = true
	}
	filtered := make([]string, 0, len(list.MemberIDs))
	for _, id := range list.MemberIDs {
		if !toRemove[id] {
			filtered = append(filtered, id)
		}
	}
	list.MemberIDs = filtered
	h.store.Lists.Set(listID, list)
	w.WriteHeader(204)
}

// --- Webhooks ---

// ListWebhookSubscriptions handles GET /webhooks/v3/{appId}/subscriptions
func (h *Handler) ListWebhookSubscriptions(w http.ResponseWriter, r *http.Request) {
	webhooks := h.store.Webhooks.List()
	if webhooks == nil {
		webhooks = []store.Webhook{}
	}
	hsJSON(w, 200, map[string]any{"results": webhooks})
}

// CreateWebhookSubscription handles POST /webhooks/v3/{appId}/subscriptions
func (h *Handler) CreateWebhookSubscription(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SubscriptionType string `json:"subscriptionType"`
		PropertyName     string `json:"propertyName,omitempty"`
		Active           bool   `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body")
		return
	}
	if body.SubscriptionType == "" {
		hsError(w, 400, "VALIDATION_ERROR", "subscriptionType is required")
		return
	}

	id := h.store.NextID()
	wh := store.Webhook{
		ID:               id,
		SubscriptionType: body.SubscriptionType,
		PropertyName:     body.PropertyName,
		Active:           body.Active,
		CreatedAt:        h.store.Now(),
	}
	h.store.Webhooks.Set(id, wh)
	hsJSON(w, 201, wh)
}

// GetWebhookSubscription handles GET /webhooks/v3/{appId}/subscriptions/{subscriptionId}
func (h *Handler) GetWebhookSubscription(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionId")
	wh, ok := h.store.Webhooks.Get(subID)
	if !ok {
		hsError(w, 404, "OBJECT_NOT_FOUND", "Subscription "+subID+" not found")
		return
	}
	hsJSON(w, 200, wh)
}

// UpdateWebhookSubscription handles PATCH /webhooks/v3/{appId}/subscriptions/{subscriptionId}
func (h *Handler) UpdateWebhookSubscription(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionId")
	wh, ok := h.store.Webhooks.Get(subID)
	if !ok {
		hsError(w, 404, "OBJECT_NOT_FOUND", "Subscription "+subID+" not found")
		return
	}

	var body struct {
		Active *bool `json:"active,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body")
		return
	}
	if body.Active != nil {
		wh.Active = *body.Active
	}
	h.store.Webhooks.Set(subID, wh)
	hsJSON(w, 200, wh)
}

// DeleteWebhookSubscription handles DELETE /webhooks/v3/{appId}/subscriptions/{subscriptionId}
func (h *Handler) DeleteWebhookSubscription(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionId")
	if !h.store.Webhooks.Delete(subID) {
		hsError(w, 404, "OBJECT_NOT_FOUND", "Subscription "+subID+" not found")
		return
	}
	w.WriteHeader(204)
}

// --- Forms ---

// ListForms handles GET /marketing/v3/forms
func (h *Handler) ListForms(w http.ResponseWriter, r *http.Request) {
	forms := h.store.Forms.List()
	if forms == nil {
		forms = []store.Form{}
	}
	hsJSON(w, 200, map[string]any{"results": forms})
}

// CreateForm handles POST /marketing/v3/forms
func (h *Handler) CreateForm(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		FormType string `json:"formType"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body")
		return
	}
	if body.Name == "" {
		hsError(w, 400, "VALIDATION_ERROR", "Form name is required")
		return
	}
	if body.FormType == "" {
		body.FormType = "hubspot"
	}

	now := h.store.Now()
	id := h.store.NextID()
	form := store.Form{
		ID:        id,
		Name:      body.Name,
		FormType:  body.FormType,
		CreatedAt: now,
		UpdatedAt: now,
	}
	h.store.Forms.Set(id, form)
	hsJSON(w, 201, form)
}

// GetForm handles GET /marketing/v3/forms/{formId}
func (h *Handler) GetForm(w http.ResponseWriter, r *http.Request) {
	formID := chi.URLParam(r, "formId")
	form, ok := h.store.Forms.Get(formID)
	if !ok {
		hsError(w, 404, "OBJECT_NOT_FOUND", "Form "+formID+" not found")
		return
	}
	hsJSON(w, 200, form)
}

// UpdateForm handles PATCH /marketing/v3/forms/{formId}
func (h *Handler) UpdateForm(w http.ResponseWriter, r *http.Request) {
	formID := chi.URLParam(r, "formId")
	form, ok := h.store.Forms.Get(formID)
	if !ok {
		hsError(w, 404, "OBJECT_NOT_FOUND", "Form "+formID+" not found")
		return
	}

	var body struct {
		Name     *string `json:"name,omitempty"`
		FormType *string `json:"formType,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body")
		return
	}
	if body.Name != nil {
		form.Name = *body.Name
	}
	if body.FormType != nil {
		form.FormType = *body.FormType
	}
	form.UpdatedAt = h.store.Now()
	h.store.Forms.Set(formID, form)
	hsJSON(w, 200, form)
}

// DeleteForm handles DELETE /marketing/v3/forms/{formId}
func (h *Handler) DeleteForm(w http.ResponseWriter, r *http.Request) {
	formID := chi.URLParam(r, "formId")
	if !h.store.Forms.Delete(formID) {
		hsError(w, 404, "OBJECT_NOT_FOUND", "Form "+formID+" not found")
		return
	}
	w.WriteHeader(204)
}

// --- OAuth ---

// OAuthToken handles POST /oauth/v1/token
func (h *Handler) OAuthToken(w http.ResponseWriter, r *http.Request) {
	hsJSON(w, 200, map[string]any{
		"access_token":  "twin-hubspot-access-token-" + h.store.NextID(),
		"refresh_token": "twin-hubspot-refresh-token-" + h.store.NextID(),
		"expires_in":    21600,
		"token_type":    "bearer",
	})
}

// --- GDPR ---

// GDPRDelete handles POST /crm/v3/objects/contacts/gdpr-delete
func (h *Handler) GDPRDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ObjectID   string `json:"objectId"`
		IDProperty string `json:"idProperty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	if body.ObjectID != "" {
		obj, ok := h.store.GetObjectByID("contacts", body.ObjectID)
		if ok {
			obj.Archived = true
			obj.UpdatedAt = h.store.Now()
			h.store.Objects.Set(body.ObjectID, *obj)
		}
	} else if body.IDProperty != "" {
		// Search by email or other identifying property
		objs := h.store.ListObjectsByType("contacts")
		for _, obj := range objs {
			if obj.Properties["email"] == body.IDProperty {
				obj.Archived = true
				obj.UpdatedAt = h.store.Now()
				h.store.Objects.Set(obj.ID, obj)
				break
			}
		}
	}
	w.WriteHeader(204)
}
