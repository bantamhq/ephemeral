package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
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
	limit := parseLimit(r.URL.Query().Get("limit"), defaultPageSize)
	expand := r.URL.Query().Get("expand")

	if expand == "folders" {
		repos, err := s.store.ListReposWithFolders(token.NamespaceID, cursor, limit+1)
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
			c := repos[len(repos)-1].Name
			nextCursor = &c
		}

		JSONList(w, repos, nextCursor, hasMore)
		return
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
		c := repos[len(repos)-1].Name
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
	req.Name = strings.ToLower(req.Name)

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
	Name   *string `json:"name,omitempty"`
	Public *bool   `json:"public,omitempty"`
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
	nameChanged := req.Name != nil && strings.ToLower(*req.Name) != oldName

	if nameChanged {
		if err := ValidateName(*req.Name); err != nil {
			JSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		lowered := strings.ToLower(*req.Name)
		req.Name = &lowered

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

	if nameChanged {
		if err := s.renameRepoOnDisk(repo.NamespaceID, oldName, *req.Name); err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to rename repository on disk")
			return
		}

		if err := s.store.UpdateRepo(repo); err != nil {
			if rollbackErr := s.renameRepoOnDisk(repo.NamespaceID, *req.Name, oldName); rollbackErr != nil {
				fmt.Printf("CRITICAL: failed to rollback repo rename from %s to %s: %v\n", *req.Name, oldName, rollbackErr)
			}
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

func (s *Server) handleListRepoFolders(w http.ResponseWriter, r *http.Request) {
	token := s.requireAuth(w, r)
	if token == nil {
		return
	}

	repo := s.requireRepoAccess(w, r, token)
	if repo == nil {
		return
	}

	folders, err := s.store.ListRepoFolders(repo.ID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list repo folders")
		return
	}

	JSON(w, http.StatusOK, folders)
}

type repoFoldersRequest struct {
	FolderIDs []string `json:"folder_ids"`
}

// validateFolderIDs verifies all folder IDs exist and belong to the token's namespace.
func (s *Server) validateFolderIDs(w http.ResponseWriter, folderIDs []string, namespaceID string) bool {
	for _, folderID := range folderIDs {
		folder, err := s.store.GetFolderByID(folderID)
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to get folder")
			return false
		}
		if folder == nil {
			JSONError(w, http.StatusBadRequest, "Folder not found: "+folderID)
			return false
		}
		if folder.NamespaceID != namespaceID {
			JSONError(w, http.StatusForbidden, "Folder not in namespace: "+folderID)
			return false
		}
	}
	return true
}

func (s *Server) handleAddRepoFolders(w http.ResponseWriter, r *http.Request) {
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

	var req repoFoldersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if !s.validateFolderIDs(w, req.FolderIDs, token.NamespaceID) {
		return
	}

	if err := s.store.AddRepoFolders(repo.ID, req.FolderIDs); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to add folders")
		return
	}

	folders, err := s.store.ListRepoFolders(repo.ID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list repo folders")
		return
	}

	JSON(w, http.StatusOK, folders)
}

func (s *Server) handleSetRepoFolders(w http.ResponseWriter, r *http.Request) {
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

	var req repoFoldersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if !s.validateFolderIDs(w, req.FolderIDs, token.NamespaceID) {
		return
	}

	if err := s.store.SetRepoFolders(repo.ID, req.FolderIDs); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to set repo folders")
		return
	}

	folders, err := s.store.ListRepoFolders(repo.ID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list repo folders")
		return
	}

	JSON(w, http.StatusOK, folders)
}

func (s *Server) handleRemoveRepoFolder(w http.ResponseWriter, r *http.Request) {
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

	folderID := chi.URLParam(r, "folderID")

	if err := s.store.RemoveRepoFolder(repo.ID, folderID); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to remove folder")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
