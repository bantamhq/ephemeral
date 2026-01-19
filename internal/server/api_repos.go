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

const defaultPageSize = 20

func (s *Server) handleListRepos(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if token == nil {
		JSONError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	cursor := r.URL.Query().Get("cursor")
	repos, err := s.store.ListRepos(token.NamespaceID, cursor, defaultPageSize+1)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list repos")
		return
	}

	hasMore := len(repos) > defaultPageSize
	if hasMore {
		repos = repos[:defaultPageSize]
	}

	var nextCursor *string
	if hasMore && len(repos) > 0 {
		c := repos[len(repos)-1].ID
		nextCursor = &c
	}

	JSONList(w, repos, nextCursor, hasMore)
}

type createRepoRequest struct {
	Name   string `json:"name"`
	Public bool   `json:"public"`
}

func (s *Server) handleCreateRepo(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if !HasScope(token, store.ScopeRepos) {
		JSONError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	var req createRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name == "" {
		JSONError(w, http.StatusBadRequest, "Name is required")
		return
	}

	existing, err := s.store.GetRepo(token.NamespaceID, req.Name)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check existing repo")
		return
	}
	if existing != nil {
		JSONError(w, http.StatusConflict, "Repository already exists")
		return
	}

	now := time.Now()
	repo := &store.Repo{
		ID:          uuid.New().String(),
		NamespaceID: token.NamespaceID,
		Name:        req.Name,
		Public:      req.Public,
		SizeBytes:   0,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.store.CreateRepo(repo); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to create repo")
		return
	}

	repoPath := filepath.Join(s.dataDir, "repos", token.NamespaceID, req.Name+".git")
	if err := initBareRepo(repoPath); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to init bare repo")
		return
	}

	JSON(w, http.StatusCreated, repo)
}

func (s *Server) handleGetRepo(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if token == nil {
		JSONError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	repo, err := s.store.GetRepoByID(id)
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

	JSON(w, http.StatusOK, repo)
}

func (s *Server) handleDeleteRepo(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if !HasScope(token, store.ScopeRepos) {
		JSONError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	id := chi.URLParam(r, "id")
	repo, err := s.store.GetRepoByID(id)
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

	if err := s.store.DeleteRepo(id); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to delete repo")
		return
	}

	repoPath := filepath.Join(s.dataDir, "repos", repo.NamespaceID, repo.Name+".git")
	os.RemoveAll(repoPath)

	w.WriteHeader(http.StatusNoContent)
}

type updateRepoRequest struct {
	Name     *string `json:"name,omitempty"`
	Public   *bool   `json:"public,omitempty"`
	FolderID *string `json:"folder_id,omitempty"`
}

func (s *Server) handleUpdateRepo(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if !HasScope(token, store.ScopeRepos) {
		JSONError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	id := chi.URLParam(r, "id")
	repo, err := s.store.GetRepoByID(id)
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

	var req updateRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	oldName := repo.Name
	if req.Name != nil {
		repo.Name = *req.Name
	}
	if req.Public != nil {
		repo.Public = *req.Public
	}
	if req.FolderID != nil {
		repo.FolderID = req.FolderID
	}

	if err := s.store.UpdateRepo(repo); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to update repo")
		return
	}

	if req.Name != nil && oldName != *req.Name {
		oldPath := filepath.Join(s.dataDir, "repos", repo.NamespaceID, oldName+".git")
		newPath := filepath.Join(s.dataDir, "repos", repo.NamespaceID, *req.Name+".git")
		os.Rename(oldPath, newPath)
	}

	JSON(w, http.StatusOK, repo)
}
