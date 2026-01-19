package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"ephemeral/internal/store"
)

func (s *Server) handleListFolders(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if token == nil {
		JSONError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	folders, err := s.store.ListFolders(token.NamespaceID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list folders")
		return
	}

	JSON(w, http.StatusOK, folders)
}

type createFolderRequest struct {
	Name     string  `json:"name"`
	ParentID *string `json:"parent_id,omitempty"`
}

func (s *Server) handleCreateFolder(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if !HasScope(token, store.ScopeRepos) {
		JSONError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	var req createFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name == "" {
		JSONError(w, http.StatusBadRequest, "Name is required")
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
	token := GetTokenFromContext(r.Context())
	if token == nil {
		JSONError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	folder, err := s.store.GetFolderByID(id)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get folder")
		return
	}
	if folder == nil {
		JSONError(w, http.StatusNotFound, "Folder not found")
		return
	}

	if folder.NamespaceID != token.NamespaceID && !HasScope(token, store.ScopeAdmin) {
		JSONError(w, http.StatusForbidden, "Access denied")
		return
	}

	JSON(w, http.StatusOK, folder)
}

type updateFolderRequest struct {
	Name     *string `json:"name,omitempty"`
	ParentID *string `json:"parent_id,omitempty"`
}

func (s *Server) handleUpdateFolder(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if !HasScope(token, store.ScopeRepos) {
		JSONError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	id := chi.URLParam(r, "id")
	folder, err := s.store.GetFolderByID(id)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get folder")
		return
	}
	if folder == nil {
		JSONError(w, http.StatusNotFound, "Folder not found")
		return
	}

	if folder.NamespaceID != token.NamespaceID && !HasScope(token, store.ScopeAdmin) {
		JSONError(w, http.StatusForbidden, "Access denied")
		return
	}

	var req updateFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name != nil {
		folder.Name = *req.Name
	}
	if req.ParentID != nil {
		if *req.ParentID == folder.ID {
			JSONError(w, http.StatusBadRequest, "Folder cannot be its own parent")
			return
		}
		folder.ParentID = req.ParentID
	}

	if err := s.store.UpdateFolder(folder); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to update folder")
		return
	}

	JSON(w, http.StatusOK, folder)
}

func (s *Server) handleDeleteFolder(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if !HasScope(token, store.ScopeRepos) {
		JSONError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	id := chi.URLParam(r, "id")
	folder, err := s.store.GetFolderByID(id)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get folder")
		return
	}
	if folder == nil {
		JSONError(w, http.StatusNotFound, "Folder not found")
		return
	}

	if folder.NamespaceID != token.NamespaceID && !HasScope(token, store.ScopeAdmin) {
		JSONError(w, http.StatusForbidden, "Access denied")
		return
	}

	force := r.URL.Query().Get("force") == "true"
	if !force {
		repos, subfolders, err := s.store.CountFolderContents(id)
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to check folder contents")
			return
		}
		if repos > 0 || subfolders > 0 {
			JSONError(w, http.StatusConflict, "Folder is not empty. Use ?force=true to delete anyway")
			return
		}
	}

	if err := s.store.DeleteFolder(id); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to delete folder")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
