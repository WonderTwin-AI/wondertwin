package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-hubspot/internal/store"
)

// ListPipelines handles GET /crm/v3/pipelines/{objectType}
func (h *Handler) ListPipelines(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")
	pipelines := h.store.ListPipelinesByType(objectType)
	if pipelines == nil {
		pipelines = []store.Pipeline{}
	}
	hsJSON(w, 200, map[string]any{"results": pipelines})
}

// CreatePipeline handles POST /crm/v3/pipelines/{objectType}
func (h *Handler) CreatePipeline(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")

	var body struct {
		Label  string               `json:"label"`
		Stages []store.PipelineStage `json:"stages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body")
		return
	}
	if body.Label == "" {
		hsError(w, 400, "VALIDATION_ERROR", "Pipeline label is required")
		return
	}

	now := h.store.Now()
	id := h.store.NextID()

	// Assign IDs to stages if missing
	for i := range body.Stages {
		if body.Stages[i].ID == "" {
			body.Stages[i].ID = h.store.NextID()
		}
	}

	pipeline := store.Pipeline{
		ID:         id,
		Label:      body.Label,
		Stages:     body.Stages,
		ObjectType: objectType,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	h.store.Pipelines.Set(id, pipeline)
	hsJSON(w, 201, pipeline)
}

// GetPipeline handles GET /crm/v3/pipelines/{objectType}/{pipelineId}
func (h *Handler) GetPipeline(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")
	pipelineID := chi.URLParam(r, "pipelineId")

	pipeline, ok := h.store.Pipelines.Get(pipelineID)
	if !ok || pipeline.ObjectType != objectType {
		hsError(w, 404, "OBJECT_NOT_FOUND", "Pipeline "+pipelineID+" not found")
		return
	}
	hsJSON(w, 200, pipeline)
}

// UpdatePipeline handles PATCH /crm/v3/pipelines/{objectType}/{pipelineId}
func (h *Handler) UpdatePipeline(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")
	pipelineID := chi.URLParam(r, "pipelineId")

	pipeline, ok := h.store.Pipelines.Get(pipelineID)
	if !ok || pipeline.ObjectType != objectType {
		hsError(w, 404, "OBJECT_NOT_FOUND", "Pipeline "+pipelineID+" not found")
		return
	}

	var body struct {
		Label  *string               `json:"label,omitempty"`
		Stages []store.PipelineStage `json:"stages,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body")
		return
	}
	if body.Label != nil {
		pipeline.Label = *body.Label
	}
	if body.Stages != nil {
		for i := range body.Stages {
			if body.Stages[i].ID == "" {
				body.Stages[i].ID = h.store.NextID()
			}
		}
		pipeline.Stages = body.Stages
	}
	pipeline.UpdatedAt = h.store.Now()
	h.store.Pipelines.Set(pipelineID, pipeline)
	hsJSON(w, 200, pipeline)
}

// DeletePipeline handles DELETE /crm/v3/pipelines/{objectType}/{pipelineId}
func (h *Handler) DeletePipeline(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")
	pipelineID := chi.URLParam(r, "pipelineId")

	pipeline, ok := h.store.Pipelines.Get(pipelineID)
	if !ok || pipeline.ObjectType != objectType {
		hsError(w, 404, "OBJECT_NOT_FOUND", "Pipeline "+pipelineID+" not found")
		return
	}
	h.store.Pipelines.Delete(pipelineID)
	w.WriteHeader(204)
}

// ListPipelineStages handles GET /crm/v3/pipelines/{objectType}/{pipelineId}/stages
func (h *Handler) ListPipelineStages(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")
	pipelineID := chi.URLParam(r, "pipelineId")

	pipeline, ok := h.store.Pipelines.Get(pipelineID)
	if !ok || pipeline.ObjectType != objectType {
		hsError(w, 404, "OBJECT_NOT_FOUND", "Pipeline "+pipelineID+" not found")
		return
	}

	stages := pipeline.Stages
	if stages == nil {
		stages = []store.PipelineStage{}
	}
	hsJSON(w, 200, map[string]any{"results": stages})
}

// CreatePipelineStage handles POST /crm/v3/pipelines/{objectType}/{pipelineId}/stages
func (h *Handler) CreatePipelineStage(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")
	pipelineID := chi.URLParam(r, "pipelineId")

	pipeline, ok := h.store.Pipelines.Get(pipelineID)
	if !ok || pipeline.ObjectType != objectType {
		hsError(w, 404, "OBJECT_NOT_FOUND", "Pipeline "+pipelineID+" not found")
		return
	}

	var stage store.PipelineStage
	if err := json.NewDecoder(r.Body).Decode(&stage); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body")
		return
	}
	if stage.ID == "" {
		stage.ID = h.store.NextID()
	}

	pipeline.Stages = append(pipeline.Stages, stage)
	pipeline.UpdatedAt = h.store.Now()
	h.store.Pipelines.Set(pipelineID, pipeline)
	hsJSON(w, 201, stage)
}

// GetPipelineStage handles GET /crm/v3/pipelines/{objectType}/{pipelineId}/stages/{stageId}
func (h *Handler) GetPipelineStage(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")
	pipelineID := chi.URLParam(r, "pipelineId")
	stageID := chi.URLParam(r, "stageId")

	pipeline, ok := h.store.Pipelines.Get(pipelineID)
	if !ok || pipeline.ObjectType != objectType {
		hsError(w, 404, "OBJECT_NOT_FOUND", "Pipeline "+pipelineID+" not found")
		return
	}

	for _, stage := range pipeline.Stages {
		if stage.ID == stageID {
			hsJSON(w, 200, stage)
			return
		}
	}
	hsError(w, 404, "OBJECT_NOT_FOUND", "Stage "+stageID+" not found in pipeline "+pipelineID)
}

// UpdatePipelineStage handles PATCH /crm/v3/pipelines/{objectType}/{pipelineId}/stages/{stageId}
func (h *Handler) UpdatePipelineStage(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")
	pipelineID := chi.URLParam(r, "pipelineId")
	stageID := chi.URLParam(r, "stageId")

	pipeline, ok := h.store.Pipelines.Get(pipelineID)
	if !ok || pipeline.ObjectType != objectType {
		hsError(w, 404, "OBJECT_NOT_FOUND", "Pipeline "+pipelineID+" not found")
		return
	}

	var body struct {
		Label        *string        `json:"label,omitempty"`
		DisplayOrder *int           `json:"displayOrder,omitempty"`
		Metadata     map[string]any `json:"metadata,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	for i, stage := range pipeline.Stages {
		if stage.ID == stageID {
			if body.Label != nil {
				pipeline.Stages[i].Label = *body.Label
			}
			if body.DisplayOrder != nil {
				pipeline.Stages[i].DisplayOrder = *body.DisplayOrder
			}
			if body.Metadata != nil {
				pipeline.Stages[i].Metadata = body.Metadata
			}
			pipeline.UpdatedAt = h.store.Now()
			h.store.Pipelines.Set(pipelineID, pipeline)
			hsJSON(w, 200, pipeline.Stages[i])
			return
		}
	}
	hsError(w, 404, "OBJECT_NOT_FOUND", "Stage "+stageID+" not found in pipeline "+pipelineID)
}

// DeletePipelineStage handles DELETE /crm/v3/pipelines/{objectType}/{pipelineId}/stages/{stageId}
func (h *Handler) DeletePipelineStage(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")
	pipelineID := chi.URLParam(r, "pipelineId")
	stageID := chi.URLParam(r, "stageId")

	pipeline, ok := h.store.Pipelines.Get(pipelineID)
	if !ok || pipeline.ObjectType != objectType {
		hsError(w, 404, "OBJECT_NOT_FOUND", "Pipeline "+pipelineID+" not found")
		return
	}

	found := false
	stages := make([]store.PipelineStage, 0, len(pipeline.Stages))
	for _, stage := range pipeline.Stages {
		if stage.ID == stageID {
			found = true
			continue
		}
		stages = append(stages, stage)
	}
	if !found {
		hsError(w, 404, "OBJECT_NOT_FOUND", "Stage "+stageID+" not found in pipeline "+pipelineID)
		return
	}

	pipeline.Stages = stages
	pipeline.UpdatedAt = h.store.Now()
	h.store.Pipelines.Set(pipelineID, pipeline)
	w.WriteHeader(204)
}
