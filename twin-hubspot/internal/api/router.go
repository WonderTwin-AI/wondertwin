// Package api implements the HubSpot CRM API-compatible handlers for the twin.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-hubspot/internal/store"
)

type Handler struct {
	store *store.MemoryStore
	mw    *twincore.Middleware
}

func NewHandler(s *store.MemoryStore, mw *twincore.Middleware) *Handler {
	return &Handler{store: s, mw: mw}
}

func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.bearerAuthMiddleware)
		r.Use(h.mw.FaultInjection)
		r.Use(h.rateLimitHeaders)

		// Generic CRM Objects (contacts, companies, deals, tickets, notes, calls, emails, meetings, tasks, etc.)
		r.Get("/crm/v3/objects/{objectType}", h.ListObjects)
		r.Post("/crm/v3/objects/{objectType}", h.CreateObject)
		r.Get("/crm/v3/objects/{objectType}/{objectId}", h.GetObject)
		r.Patch("/crm/v3/objects/{objectType}/{objectId}", h.UpdateObject)
		r.Delete("/crm/v3/objects/{objectType}/{objectId}", h.ArchiveObject)

		// Batch operations
		r.Post("/crm/v3/objects/{objectType}/batch/create", h.BatchCreateObjects)
		r.Post("/crm/v3/objects/{objectType}/batch/read", h.BatchReadObjects)
		r.Post("/crm/v3/objects/{objectType}/batch/update", h.BatchUpdateObjects)
		r.Post("/crm/v3/objects/{objectType}/batch/upsert", h.BatchUpsertObjects)
		r.Post("/crm/v3/objects/{objectType}/batch/archive", h.BatchArchiveObjects)

		// Search
		r.Post("/crm/v3/objects/{objectType}/search", h.SearchObjects)

		// Properties
		r.Get("/crm/v3/properties/{objectType}", h.ListProperties)
		r.Post("/crm/v3/properties/{objectType}", h.CreateProperty)
		r.Get("/crm/v3/properties/{objectType}/{propertyName}", h.GetProperty)
		r.Patch("/crm/v3/properties/{objectType}/{propertyName}", h.UpdateProperty)
		r.Delete("/crm/v3/properties/{objectType}/{propertyName}", h.DeleteProperty)

		// Property Groups
		r.Get("/crm/v3/properties/{objectType}/groups", h.ListPropertyGroups)
		r.Post("/crm/v3/properties/{objectType}/groups", h.CreatePropertyGroup)
		r.Get("/crm/v3/properties/{objectType}/groups/{groupName}", h.GetPropertyGroup)
		r.Patch("/crm/v3/properties/{objectType}/groups/{groupName}", h.UpdatePropertyGroup)
		r.Delete("/crm/v3/properties/{objectType}/groups/{groupName}", h.DeletePropertyGroup)

		// Pipelines
		r.Get("/crm/v3/pipelines/{objectType}", h.ListPipelines)
		r.Post("/crm/v3/pipelines/{objectType}", h.CreatePipeline)
		r.Get("/crm/v3/pipelines/{objectType}/{pipelineId}", h.GetPipeline)
		r.Patch("/crm/v3/pipelines/{objectType}/{pipelineId}", h.UpdatePipeline)
		r.Delete("/crm/v3/pipelines/{objectType}/{pipelineId}", h.DeletePipeline)

		// Pipeline Stages
		r.Get("/crm/v3/pipelines/{objectType}/{pipelineId}/stages", h.ListPipelineStages)
		r.Post("/crm/v3/pipelines/{objectType}/{pipelineId}/stages", h.CreatePipelineStage)
		r.Get("/crm/v3/pipelines/{objectType}/{pipelineId}/stages/{stageId}", h.GetPipelineStage)
		r.Patch("/crm/v3/pipelines/{objectType}/{pipelineId}/stages/{stageId}", h.UpdatePipelineStage)
		r.Delete("/crm/v3/pipelines/{objectType}/{pipelineId}/stages/{stageId}", h.DeletePipelineStage)

		// Associations v4
		r.Get("/crm/v4/objects/{fromObjectType}/{objectId}/associations/{toObjectType}", h.ListAssociations)
		r.Put("/crm/v4/objects/{fromObjectType}/{objectId}/associations/default/{toObjectType}/{toObjectId}", h.CreateDefaultAssociation)
		r.Delete("/crm/v4/objects/{fromObjectType}/{objectId}/associations/{toObjectType}/{toObjectId}", h.DeleteAssociation)
		r.Post("/crm/v4/associations/{fromObjectType}/{toObjectType}/batch/create", h.BatchCreateAssociations)
		r.Post("/crm/v4/associations/{fromObjectType}/{toObjectType}/batch/read", h.BatchReadAssociations)
		r.Post("/crm/v4/associations/{fromObjectType}/{toObjectType}/batch/archive", h.BatchArchiveAssociations)
		r.Get("/crm/v4/associations/{fromObjectType}/{toObjectType}/labels", h.ListAssociationLabels)

		// Owners
		r.Get("/crm/v3/owners", h.ListOwners)
		r.Get("/crm/v3/owners/{ownerId}", h.GetOwner)

		// Lists
		r.Get("/crm/v3/lists", h.ListLists)
		r.Post("/crm/v3/lists", h.CreateList)
		r.Get("/crm/v3/lists/{listId}", h.GetList)
		r.Delete("/crm/v3/lists/{listId}", h.DeleteList)
		r.Get("/crm/v3/lists/{listId}/memberships", h.GetListMemberships)
		r.Put("/crm/v3/lists/{listId}/memberships/add", h.AddListMembers)
		r.Put("/crm/v3/lists/{listId}/memberships/remove", h.RemoveListMembers)

		// Webhooks
		r.Get("/webhooks/v3/{appId}/subscriptions", h.ListWebhookSubscriptions)
		r.Post("/webhooks/v3/{appId}/subscriptions", h.CreateWebhookSubscription)
		r.Get("/webhooks/v3/{appId}/subscriptions/{subscriptionId}", h.GetWebhookSubscription)
		r.Patch("/webhooks/v3/{appId}/subscriptions/{subscriptionId}", h.UpdateWebhookSubscription)
		r.Delete("/webhooks/v3/{appId}/subscriptions/{subscriptionId}", h.DeleteWebhookSubscription)

		// Forms
		r.Get("/marketing/v3/forms", h.ListForms)
		r.Post("/marketing/v3/forms", h.CreateForm)
		r.Get("/marketing/v3/forms/{formId}", h.GetForm)
		r.Patch("/marketing/v3/forms/{formId}", h.UpdateForm)
		r.Delete("/marketing/v3/forms/{formId}", h.DeleteForm)

		// OAuth
		r.Post("/oauth/v1/token", h.OAuthToken)

		// GDPR
		r.Post("/crm/v3/objects/contacts/gdpr-delete", h.GDPRDelete)
	})

	// Admin
	r.Get("/admin/objects", h.AdminListObjects)
}

func (h *Handler) bearerAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			hsError(w, 401, "INVALID_AUTHENTICATION", "Authentication credentials not found. This API supports OAuth 2.0 authentication and target_name authentication")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) rateLimitHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-HubSpot-RateLimit-Max", "190")
		w.Header().Set("X-HubSpot-RateLimit-Remaining", "189")
		w.Header().Set("X-HubSpot-RateLimit-Interval-Milliseconds", "10000")
		next.ServeHTTP(w, r)
	})
}

func hsJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func hsError(w http.ResponseWriter, status int, category, message string) {
	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"status":        "error",
		"message":       message,
		"correlationId": "twin-" + store.New().NextID(),
		"category":      category,
	})
}
