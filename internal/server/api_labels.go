package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"ephemeral/internal/store"
)

func (s *Server) handleListLabels(w http.ResponseWriter, r *http.Request) {
	token := s.requireAuth(w, r)
	if token == nil {
		return
	}

	cursor := r.URL.Query().Get("cursor")
	limit := parseLimit(r.URL.Query().Get("limit"), defaultPageSize)

	fetchLimit := limit
	if limit > 0 {
		fetchLimit = limit + 1
	}

	labels, err := s.store.ListLabels(token.NamespaceID, cursor, fetchLimit)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list labels")
		return
	}

	var hasMore bool
	var nextCursor *string

	if limit > 0 {
		hasMore = len(labels) > limit
		if hasMore {
			labels = labels[:limit]
		}
		if hasMore && len(labels) > 0 {
			c := labels[len(labels)-1].Name
			nextCursor = &c
		}
	}

	JSONList(w, labels, nextCursor, hasMore)
}

type createLabelRequest struct {
	Name  string  `json:"name"`
	Color *string `json:"color,omitempty"`
}

func (s *Server) handleCreateLabel(w http.ResponseWriter, r *http.Request) {
	token := s.requireAuth(w, r)
	if token == nil {
		return
	}
	if !s.requireScope(w, token, store.ScopeRepos) {
		return
	}

	var req createLabelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name == "" {
		JSONError(w, http.StatusBadRequest, "Name is required")
		return
	}

	existing, err := s.store.GetLabelByName(token.NamespaceID, req.Name)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check existing label")
		return
	}
	if existing != nil {
		JSONError(w, http.StatusConflict, "Label already exists")
		return
	}

	label := &store.Label{
		ID:          uuid.New().String(),
		NamespaceID: token.NamespaceID,
		Name:        req.Name,
		Color:       req.Color,
		CreatedAt:   time.Now(),
	}

	if err := s.store.CreateLabel(label); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to create label")
		return
	}

	JSON(w, http.StatusCreated, label)
}

func (s *Server) handleGetLabel(w http.ResponseWriter, r *http.Request) {
	token := s.requireAuth(w, r)
	if token == nil {
		return
	}

	label := s.requireLabelAccess(w, r, token)
	if label == nil {
		return
	}

	JSON(w, http.StatusOK, label)
}

type updateLabelRequest struct {
	Name  *string `json:"name,omitempty"`
	Color *string `json:"color,omitempty"`
}

func (s *Server) handleUpdateLabel(w http.ResponseWriter, r *http.Request) {
	token := s.requireAuth(w, r)
	if token == nil {
		return
	}
	if !s.requireScope(w, token, store.ScopeRepos) {
		return
	}

	label := s.requireLabelAccess(w, r, token)
	if label == nil {
		return
	}

	var req updateLabelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name != nil {
		label.Name = *req.Name
	}
	if req.Color != nil {
		label.Color = req.Color
	}

	if err := s.store.UpdateLabel(label); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to update label")
		return
	}

	JSON(w, http.StatusOK, label)
}

func (s *Server) handleDeleteLabel(w http.ResponseWriter, r *http.Request) {
	token := s.requireAuth(w, r)
	if token == nil {
		return
	}
	if !s.requireScope(w, token, store.ScopeRepos) {
		return
	}

	label := s.requireLabelAccess(w, r, token)
	if label == nil {
		return
	}

	if err := s.store.DeleteLabel(label.ID); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to delete label")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type addRepoLabelsRequest struct {
	LabelIDs []string `json:"label_ids"`
}

func (s *Server) handleAddRepoLabels(w http.ResponseWriter, r *http.Request) {
	token := s.requireAuth(w, r)
	if token == nil {
		return
	}
	if !s.requireScope(w, token, store.ScopeRepos) {
		return
	}

	repo := s.requireRepoAccess(w, r, token)
	if repo == nil {
		return
	}

	var req addRepoLabelsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	for _, labelID := range req.LabelIDs {
		label, err := s.store.GetLabelByID(labelID)
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to get label")
			return
		}
		if label == nil {
			JSONError(w, http.StatusBadRequest, "Label not found: "+labelID)
			return
		}
		if label.NamespaceID != token.NamespaceID {
			JSONError(w, http.StatusForbidden, "Label not in namespace: "+labelID)
			return
		}

		if err := s.store.AddRepoLabel(repo.ID, labelID); err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to add label")
			return
		}
	}

	labels, err := s.store.ListRepoLabels(repo.ID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list repo labels")
		return
	}

	JSON(w, http.StatusOK, labels)
}

func (s *Server) handleRemoveRepoLabel(w http.ResponseWriter, r *http.Request) {
	token := s.requireAuth(w, r)
	if token == nil {
		return
	}
	if !s.requireScope(w, token, store.ScopeRepos) {
		return
	}

	repo := s.requireRepoAccess(w, r, token)
	if repo == nil {
		return
	}

	labelID := chi.URLParam(r, "labelID")

	if err := s.store.RemoveRepoLabel(repo.ID, labelID); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to remove label")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListRepoLabels(w http.ResponseWriter, r *http.Request) {
	token := s.requireAuth(w, r)
	if token == nil {
		return
	}

	repo := s.requireRepoAccess(w, r, token)
	if repo == nil {
		return
	}

	labels, err := s.store.ListRepoLabels(repo.ID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list repo labels")
		return
	}

	JSON(w, http.StatusOK, labels)
}
