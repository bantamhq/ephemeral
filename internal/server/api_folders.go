package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"ephemeral/internal/store"
)

func (s *Server) handleListFolders(w http.ResponseWriter, r *http.Request) {
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

	folders, err := s.store.ListFolders(token.NamespaceID, cursor, fetchLimit)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list folders")
		return
	}

	var hasMore bool
	var nextCursor *string

	if limit > 0 {
		hasMore = len(folders) > limit
		if hasMore {
			folders = folders[:limit]
		}
		if hasMore && len(folders) > 0 {
			c := folders[len(folders)-1].Name
			nextCursor = &c
		}
	}

	JSONList(w, folders, nextCursor, hasMore)
}

type createFolderRequest struct {
	Name     string  `json:"name"`
	ParentID *string `json:"parent_id,omitempty"`
}

func (s *Server) checkFolderNameConflict(w http.ResponseWriter, namespaceID, name string, parentID *string, excludeID string) bool {
	existing, err := s.store.GetFolderByName(namespaceID, name, parentID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check existing folder")
		return true
	}
	if existing != nil && existing.ID != excludeID {
		JSONError(w, http.StatusConflict, "Folder with that name already exists in this location")
		return true
	}
	return false
}

func (s *Server) handleCreateFolder(w http.ResponseWriter, r *http.Request) {
	token := s.requireAuth(w, r)
	if token == nil {
		return
	}
	if !s.requireScope(w, token, store.ScopeRepos) {
		return
	}

	var req createFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := ValidateName(req.Name); err != nil {
		JSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.ParentID != nil {
		parent, err := s.store.GetFolderByID(*req.ParentID)
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to check parent folder")
			return
		}
		if parent == nil {
			JSONError(w, http.StatusBadRequest, "Parent folder not found")
			return
		}
		if parent.NamespaceID != token.NamespaceID {
			JSONError(w, http.StatusForbidden, "Parent folder not in namespace")
			return
		}
	}

	if s.checkFolderNameConflict(w, token.NamespaceID, req.Name, req.ParentID, "") {
		return
	}

	folder := &store.Folder{
		ID:          uuid.New().String(),
		NamespaceID: token.NamespaceID,
		Name:        req.Name,
		ParentID:    req.ParentID,
		CreatedAt:   time.Now(),
	}

	if err := s.store.CreateFolder(folder); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to create folder")
		return
	}

	JSON(w, http.StatusCreated, folder)
}

func (s *Server) handleGetFolder(w http.ResponseWriter, r *http.Request) {
	token := s.requireAuth(w, r)
	if token == nil {
		return
	}

	folder := s.requireFolderAccess(w, r, token)
	if folder == nil {
		return
	}

	JSON(w, http.StatusOK, folder)
}

type updateFolderRequest struct {
	Name     *string `json:"name,omitempty"`
	ParentID *string `json:"parent_id,omitempty"`
}

func (s *Server) handleUpdateFolder(w http.ResponseWriter, r *http.Request) {
	token := s.requireAuth(w, r)
	if token == nil {
		return
	}
	if !s.requireScope(w, token, store.ScopeRepos) {
		return
	}

	folder := s.requireFolderAccess(w, r, token)
	if folder == nil {
		return
	}

	var req updateFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name != nil {
		if err := ValidateName(*req.Name); err != nil {
			JSONError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	newParentID := folder.ParentID
	if req.ParentID != nil {
		if *req.ParentID == "" {
			newParentID = nil
		} else if *req.ParentID == folder.ID {
			JSONError(w, http.StatusBadRequest, "Folder cannot be its own parent")
			return
		} else {
			parent, err := s.store.GetFolderByID(*req.ParentID)
			if err != nil {
				JSONError(w, http.StatusInternalServerError, "Failed to check parent folder")
				return
			}
			if parent == nil {
				JSONError(w, http.StatusBadRequest, "Parent folder not found")
				return
			}
			if parent.NamespaceID != folder.NamespaceID {
				JSONError(w, http.StatusBadRequest, "Parent folder not in same namespace")
				return
			}

			if s.isFolderDescendant(folder.ID, *req.ParentID) {
				JSONError(w, http.StatusBadRequest, "Cannot set parent to a descendant folder (would create cycle)")
				return
			}

			newParentID = req.ParentID
		}
	}

	newName := folder.Name
	if req.Name != nil {
		newName = *req.Name
	}

	nameChanged := newName != folder.Name
	parentChanged := (newParentID == nil) != (folder.ParentID == nil) ||
		(newParentID != nil && folder.ParentID != nil && *newParentID != *folder.ParentID)

	if nameChanged || parentChanged {
		if s.checkFolderNameConflict(w, folder.NamespaceID, newName, newParentID, folder.ID) {
			return
		}
	}

	folder.Name = newName
	folder.ParentID = newParentID

	if err := s.store.UpdateFolder(folder); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to update folder")
		return
	}

	JSON(w, http.StatusOK, folder)
}

func (s *Server) isFolderDescendant(ancestorID, potentialDescendantID string) bool {
	visited := make(map[string]bool)

	for current := potentialDescendantID; current != ""; {
		if visited[current] {
			return false
		}
		visited[current] = true

		folder, err := s.store.GetFolderByID(current)
		if err != nil || folder == nil || folder.ParentID == nil {
			return false
		}

		if *folder.ParentID == ancestorID {
			return true
		}
		current = *folder.ParentID
	}

	return false
}

func (s *Server) handleDeleteFolder(w http.ResponseWriter, r *http.Request) {
	token := s.requireAuth(w, r)
	if token == nil {
		return
	}
	if !s.requireScope(w, token, store.ScopeRepos) {
		return
	}

	folder := s.requireFolderAccess(w, r, token)
	if folder == nil {
		return
	}

	force := r.URL.Query().Get("force") == "true"
	if !force {
		repos, subfolders, err := s.store.CountFolderContents(folder.ID)
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to check folder contents")
			return
		}
		if repos > 0 || subfolders > 0 {
			JSONError(w, http.StatusConflict, "Folder is not empty. Use ?force=true to delete anyway")
			return
		}
	}

	if err := s.store.DeleteFolder(folder.ID); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to delete folder")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
