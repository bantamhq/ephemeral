package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/bantamhq/ephemeral/internal/store"
)

const defaultPageSize = 20

func (s *Server) handleListRepos(w http.ResponseWriter, r *http.Request) {
	token := s.requireUserToken(w, r)
	if token == nil {
		return
	}

	if token.UserID == nil {
		JSONError(w, http.StatusForbidden, "Token has no associated user")
		return
	}

	nsFilter := r.URL.Query().Get("namespace")
	cursor := r.URL.Query().Get("cursor")
	limit := parseLimit(r.URL.Query().Get("limit"), defaultPageSize)
	expand := r.URL.Query().Get("expand")

	if nsFilter == "" {
		repos, err := s.store.ListAllUserAccessibleRepos(*token.UserID)
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to list repos")
			return
		}

		JSONList(w, repos, nil, false)
		return
	}

	ns, err := s.store.GetNamespaceByName(nsFilter)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get namespace")
		return
	}
	if ns == nil {
		JSONError(w, http.StatusNotFound, "Namespace not found")
		return
	}
	nsID := ns.ID

	canAccess, err := s.permissions.CanAccessNamespace(token.ID, nsID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check namespace access")
		return
	}
	if !canAccess {
		JSONError(w, http.StatusForbidden, "Access denied to namespace")
		return
	}

	hasNSRead, err := s.permissions.CheckNamespacePermission(token.ID, nsID, store.PermNamespaceRead)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check permissions")
		return
	}

	if hasNSRead {
		if expand == "folders" {
			repos, err := s.store.ListReposWithFolders(nsID, cursor, limit+1)
			if err != nil {
				JSONError(w, http.StatusInternalServerError, "Failed to list repos")
				return
			}

			repos, nextCursor, hasMore := paginateSlice(repos, limit, func(r store.RepoWithFolders) string {
				return r.Name
			})

			JSONList(w, repos, nextCursor, hasMore)
			return
		}

		repos, err := s.store.ListRepos(nsID, cursor, limit+1)
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to list repos")
			return
		}

		repos, nextCursor, hasMore := paginateSlice(repos, limit, func(r store.Repo) string {
			return r.Name
		})

		JSONList(w, repos, nextCursor, hasMore)
		return
	}

	repos, err := s.store.ListUserReposWithGrants(*token.UserID, nsID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list repos")
		return
	}

	JSONList(w, repos, nil, false)
}

type createRepoRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Public      bool    `json:"public"`
	Namespace   *string `json:"namespace,omitempty"`
}

func (s *Server) handleCreateRepo(w http.ResponseWriter, r *http.Request) {
	token := s.requireUserToken(w, r)
	if token == nil {
		return
	}

	var req createRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	nsID := s.resolveNamespaceID(w, token, req.Namespace)
	if nsID == "" {
		return
	}

	if !s.requireNamespacePermission(w, token, nsID, store.PermNamespaceWrite) {
		return
	}

	if err := ValidateName(req.Name); err != nil {
		JSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Name = strings.ToLower(req.Name)

	if req.Description != nil && len(*req.Description) > 512 {
		JSONError(w, http.StatusBadRequest, "Description must be 512 characters or less")
		return
	}

	existing, err := s.store.GetRepo(nsID, req.Name)
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
		NamespaceID: nsID,
		Name:        req.Name,
		Description: req.Description,
		Public:      req.Public,
		SizeBytes:   0,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	repoPath, err := SafeRepoPath(s.dataDir, nsID, req.Name)
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
	token := s.requireUserToken(w, r)
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
	token := s.requireUserToken(w, r)
	if token == nil {
		return
	}

	repo := s.requireRepoAccessWithPermission(w, r, token, store.PermRepoAdmin)
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
		slog.Warn("failed to remove repo directory", "path", repoPath, "error", err)
	}

	w.WriteHeader(http.StatusNoContent)
}

type updateRepoRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Public      *bool   `json:"public,omitempty"`
}

func (s *Server) handleUpdateRepo(w http.ResponseWriter, r *http.Request) {
	token := s.requireUserToken(w, r)
	if token == nil {
		return
	}

	repo := s.requireRepoAccessWithPermission(w, r, token, store.PermRepoAdmin)
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

	if req.Description != nil {
		if len(*req.Description) > 512 {
			JSONError(w, http.StatusBadRequest, "Description must be 512 characters or less")
			return
		}
		repo.Description = req.Description
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
				slog.Error("failed to rollback repo rename", "from", *req.Name, "to", oldName, "error", rollbackErr)
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
	token := s.requireUserToken(w, r)
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
	token := s.requireUserToken(w, r)
	if token == nil {
		return
	}

	repo := s.requireRepoAccessWithPermission(w, r, token, store.PermRepoAdmin)
	if repo == nil {
		return
	}

	var req repoFoldersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if !s.validateFolderIDs(w, req.FolderIDs, repo.NamespaceID) {
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
	token := s.requireUserToken(w, r)
	if token == nil {
		return
	}

	repo := s.requireRepoAccessWithPermission(w, r, token, store.PermRepoAdmin)
	if repo == nil {
		return
	}

	var req repoFoldersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if !s.validateFolderIDs(w, req.FolderIDs, repo.NamespaceID) {
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
	token := s.requireUserToken(w, r)
	if token == nil {
		return
	}

	repo := s.requireRepoAccessWithPermission(w, r, token, store.PermRepoAdmin)
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
