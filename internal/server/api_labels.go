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
	token := GetTokenFromContext(r.Context())
	if token == nil {
		JSONError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	labels, err := s.store.ListLabels(token.NamespaceID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list labels")
		return
	}

	JSON(w, http.StatusOK, labels)
}

type createLabelRequest struct {
	Name  string  `json:"name"`
	Color *string `json:"color,omitempty"`
}

func (s *Server) handleCreateLabel(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if !HasScope(token, store.ScopeRepos) {
		JSONError(w, http.StatusForbidden, "Insufficient permissions")
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
	token := GetTokenFromContext(r.Context())
	if token == nil {
		JSONError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	label, err := s.store.GetLabelByID(id)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get label")
		return
	}
	if label == nil {
		JSONError(w, http.StatusNotFound, "Label not found")
		return
	}

	if label.NamespaceID != token.NamespaceID && !HasScope(token, store.ScopeAdmin) {
		JSONError(w, http.StatusForbidden, "Access denied")
		return
	}

	JSON(w, http.StatusOK, label)
}

type updateLabelRequest struct {
	Name  *string `json:"name,omitempty"`
	Color *string `json:"color,omitempty"`
}

func (s *Server) handleUpdateLabel(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if !HasScope(token, store.ScopeRepos) {
		JSONError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	id := chi.URLParam(r, "id")
	label, err := s.store.GetLabelByID(id)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get label")
		return
	}
	if label == nil {
		JSONError(w, http.StatusNotFound, "Label not found")
		return
	}

	if label.NamespaceID != token.NamespaceID && !HasScope(token, store.ScopeAdmin) {
		JSONError(w, http.StatusForbidden, "Access denied")
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
	token := GetTokenFromContext(r.Context())
	if !HasScope(token, store.ScopeRepos) {
		JSONError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	id := chi.URLParam(r, "id")
	label, err := s.store.GetLabelByID(id)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get label")
		return
	}
	if label == nil {
		JSONError(w, http.StatusNotFound, "Label not found")
		return
	}

	if label.NamespaceID != token.NamespaceID && !HasScope(token, store.ScopeAdmin) {
		JSONError(w, http.StatusForbidden, "Access denied")
		return
	}

	if err := s.store.DeleteLabel(id); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to delete label")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type addRepoLabelsRequest struct {
	LabelIDs []string `json:"label_ids"`
}

func (s *Server) handleAddRepoLabels(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if !HasScope(token, store.ScopeRepos) {
		JSONError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	repoID := chi.URLParam(r, "id")
	repo, err := s.store.GetRepoByID(repoID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get repo")
		return
	}
	if repo == nil {
		JSONError(w, http.StatusNotFound, "Repository not found")
		return
	}

	if repo.NamespaceID != token.NamespaceID && !HasScope(token, store.ScopeAdmin) {
		JSONError(w, http.StatusForbidden, "Access denied")
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

		if err := s.store.AddRepoLabel(repoID, labelID); err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to add label")
			return
		}
	}

	labels, err := s.store.ListRepoLabels(repoID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list repo labels")
		return
	}

	JSON(w, http.StatusOK, labels)
}

func (s *Server) handleRemoveRepoLabel(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if !HasScope(token, store.ScopeRepos) {
		JSONError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	repoID := chi.URLParam(r, "id")
	labelID := chi.URLParam(r, "labelID")

	repo, err := s.store.GetRepoByID(repoID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get repo")
		return
	}
	if repo == nil {
		JSONError(w, http.StatusNotFound, "Repository not found")
		return
	}

	if repo.NamespaceID != token.NamespaceID && !HasScope(token, store.ScopeAdmin) {
		JSONError(w, http.StatusForbidden, "Access denied")
		return
	}

	if err := s.store.RemoveRepoLabel(repoID, labelID); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to remove label")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListRepoLabels(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if token == nil {
		JSONError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	repoID := chi.URLParam(r, "id")
	repo, err := s.store.GetRepoByID(repoID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get repo")
		return
	}
	if repo == nil {
		JSONError(w, http.StatusNotFound, "Repository not found")
		return
	}

	if repo.NamespaceID != token.NamespaceID && !HasScope(token, store.ScopeAdmin) {
		JSONError(w, http.StatusForbidden, "Access denied")
		return
	}

	labels, err := s.store.ListRepoLabels(repoID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list repo labels")
		return
	}

	JSON(w, http.StatusOK, labels)
}
