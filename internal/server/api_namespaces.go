package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"ephemeral/internal/store"
)

func (s *Server) handleListNamespaces(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if !HasScope(token, store.ScopeAdmin) {
		JSONError(w, http.StatusForbidden, "Admin access required")
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
	token := GetTokenFromContext(r.Context())
	if !HasScope(token, store.ScopeAdmin) {
		JSONError(w, http.StatusForbidden, "Admin access required")
		return
	}

	var req createNamespaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name == "" {
		JSONError(w, http.StatusBadRequest, "Name is required")
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
	token := GetTokenFromContext(r.Context())
	if !HasScope(token, store.ScopeAdmin) {
		JSONError(w, http.StatusForbidden, "Admin access required")
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
	token := GetTokenFromContext(r.Context())
	if !HasScope(token, store.ScopeAdmin) {
		JSONError(w, http.StatusForbidden, "Admin access required")
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

	if err := s.store.DeleteNamespace(id); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to delete namespace")
		return
	}

	reposPath := filepath.Join(s.dataDir, "repos", ns.Name)
	os.RemoveAll(reposPath)

	w.WriteHeader(http.StatusNoContent)
}
