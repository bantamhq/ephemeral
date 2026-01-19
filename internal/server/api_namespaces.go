package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"ephemeral/internal/store"
)

func (s *Server) handleGetCurrentNamespace(w http.ResponseWriter, r *http.Request) {
	token := s.requireAuth(w, r)
	if token == nil {
		return
	}

	ns, err := s.store.GetNamespace(token.NamespaceID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get namespace")
		return
	}
	if ns == nil {
		JSONError(w, http.StatusNotFound, "Namespace not found")
		return
	}

	JSON(w, http.StatusOK, ns)
}

func (s *Server) handleListNamespaces(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}

	cursor := r.URL.Query().Get("cursor")
	namespaces, err := s.store.ListNamespaces(cursor, defaultPageSize+1)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list namespaces")
		return
	}

	hasMore := len(namespaces) > defaultPageSize
	if hasMore {
		namespaces = namespaces[:defaultPageSize]
	}

	var nextCursor *string
	if hasMore && len(namespaces) > 0 {
		c := namespaces[len(namespaces)-1].ID
		nextCursor = &c
	}

	JSONList(w, namespaces, nextCursor, hasMore)
}

type createNamespaceRequest struct {
	Name              string `json:"name"`
	RepoLimit         *int   `json:"repo_limit,omitempty"`
	StorageLimitBytes *int   `json:"storage_limit_bytes,omitempty"`
}

func (s *Server) handleCreateNamespace(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}

	var req createNamespaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := ValidateName(req.Name); err != nil {
		JSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	existing, err := s.store.GetNamespaceByName(req.Name)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check existing namespace")
		return
	}
	if existing != nil {
		JSONError(w, http.StatusConflict, "Namespace already exists")
		return
	}

	ns := &store.Namespace{
		ID:                uuid.New().String(),
		Name:              req.Name,
		CreatedAt:         time.Now(),
		RepoLimit:         req.RepoLimit,
		StorageLimitBytes: req.StorageLimitBytes,
	}

	if err := s.store.CreateNamespace(ns); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to create namespace")
		return
	}

	JSON(w, http.StatusCreated, ns)
}

func (s *Server) handleGetNamespace(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}

	id := chi.URLParam(r, "id")
	ns, err := s.store.GetNamespace(id)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get namespace")
		return
	}
	if ns == nil {
		JSONError(w, http.StatusNotFound, "Namespace not found")
		return
	}

	JSON(w, http.StatusOK, ns)
}

func (s *Server) handleDeleteNamespace(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}

	id := chi.URLParam(r, "id")
	ns, err := s.store.GetNamespace(id)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get namespace")
		return
	}
	if ns == nil {
		JSONError(w, http.StatusNotFound, "Namespace not found")
		return
	}

	reposPath, err := SafeNamespacePath(s.dataDir, ns.ID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to resolve namespace path")
		return
	}

	if err := s.store.DeleteNamespace(id); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to delete namespace")
		return
	}

	if err := os.RemoveAll(reposPath); err != nil {
		fmt.Printf("Warning: failed to remove namespace directory %s: %v\n", reposPath, err)
	}

	w.WriteHeader(http.StatusNoContent)
}
