package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"

	"ephemeral/internal/store"
)

const defaultPageSize = 20

func (s *Server) handleListRepos(w http.ResponseWriter, r *http.Request) {
	token := s.requireAuth(w, r)
	if token == nil {
		return
	}

	cursor := r.URL.Query().Get("cursor")
	limitStr := r.URL.Query().Get("limit")

	limit := defaultPageSize
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l >= 1 && l <= 100 {
			limit = l
		}
	}

	repos, err := s.store.ListRepos(token.NamespaceID, cursor, limit+1)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list repos")
		return
	}

	hasMore := len(repos) > limit
	if hasMore {
		repos = repos[:limit]
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
	token := s.requireAuth(w, r)
	if token == nil {
		return
	}
	if !s.requireScope(w, token, store.ScopeRepos) {
		return
	}

	var req createRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := ValidateName(req.Name); err != nil {
		JSONError(w, http.StatusBadRequest, err.Error())
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

	repoPath, err := SafeRepoPath(s.dataDir, token.NamespaceID, req.Name)
	if err != nil {
		JSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.store.CreateRepo(repo); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to create repo")
		return
	}

	if err := initBareRepo(repoPath); err != nil {
		s.store.DeleteRepo(repo.ID)
		JSONError(w, http.StatusInternalServerError, "Failed to init bare repo")
		return
	}

	JSON(w, http.StatusCreated, repo)
}

func (s *Server) handleGetRepo(w http.ResponseWriter, r *http.Request) {
	token := s.requireAuth(w, r)
	if token == nil {
		return
	}

	repo := s.requireRepoAccess(w, r, token)
	if repo == nil {
		return
	}

	JSON(w, http.StatusOK, repo)
}

func (s *Server) handleDeleteRepo(w http.ResponseWriter, r *http.Request) {
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

	repoPath, err := SafeRepoPath(s.dataDir, repo.NamespaceID, repo.Name)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to resolve repo path")
		return
	}

	if err := s.store.DeleteRepo(repo.ID); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to delete repo")
		return
	}

	if err := os.RemoveAll(repoPath); err != nil {
		fmt.Printf("Warning: failed to remove repo directory %s: %v\n", repoPath, err)
	}

	w.WriteHeader(http.StatusNoContent)
}

type updateRepoRequest struct {
	Name     *string `json:"name,omitempty"`
	Public   *bool   `json:"public,omitempty"`
	FolderID *string `json:"folder_id,omitempty"`
}

func (s *Server) handleUpdateRepo(w http.ResponseWriter, r *http.Request) {
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

	var req updateRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	oldName := repo.Name
	nameChanged := req.Name != nil && *req.Name != oldName

	if nameChanged {
		if err := ValidateName(*req.Name); err != nil {
			JSONError(w, http.StatusBadRequest, err.Error())
			return
		}

		existing, err := s.store.GetRepo(repo.NamespaceID, *req.Name)
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to check existing repo")
			return
		}
		if existing != nil {
			JSONError(w, http.StatusConflict, "Repository with that name already exists")
			return
		}

		repo.Name = *req.Name
	}

	if req.Public != nil {
		repo.Public = *req.Public
	}
	if req.FolderID != nil {
		repo.FolderID = req.FolderID
	}

	if nameChanged {
		if err := s.renameRepoOnDisk(repo.NamespaceID, oldName, *req.Name); err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to rename repository on disk")
			return
		}

		if err := s.store.UpdateRepo(repo); err != nil {
			s.renameRepoOnDisk(repo.NamespaceID, *req.Name, oldName)
			JSONError(w, http.StatusInternalServerError, "Failed to update repo")
			return
		}
	} else {
		if err := s.store.UpdateRepo(repo); err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to update repo")
			return
		}
	}

	JSON(w, http.StatusOK, repo)
}

func (s *Server) renameRepoOnDisk(namespaceID, oldName, newName string) error {
	oldPath, err := SafeRepoPath(s.dataDir, namespaceID, oldName)
	if err != nil {
		return err
	}
	newPath, err := SafeRepoPath(s.dataDir, namespaceID, newName)
	if err != nil {
		return err
	}
	return os.Rename(oldPath, newPath)
}
