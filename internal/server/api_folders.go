package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/bantamhq/ephemeral/internal/store"
)

func (s *Server) handleListFolders(w http.ResponseWriter, r *http.Request) {
	token := s.requireUserToken(w, r)
	if token == nil {
		return
	}

	nsFilter := r.URL.Query().Get("namespace")
	if nsFilter == "" {
		nsFilter = r.Header.Get("X-Namespace")
	}
	cursor := r.URL.Query().Get("cursor")
	limit := parseLimit(r.URL.Query().Get("limit"), defaultPageSize)

	var nsName *string
	if nsFilter != "" {
		nsName = &nsFilter
	}

	nsID := s.resolveNamespaceID(w, token, nsName)
	if nsID == "" {
		return
	}

	if !s.requireNamespacePermission(w, token, nsID, store.PermNamespaceRead) {
		return
	}

	folders, err := s.store.ListFolders(nsID, cursor, limit+1)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list folders")
		return
	}

	hasMore := len(folders) > limit
	if hasMore {
		folders = folders[:limit]
	}

	var nextCursor *string
	if hasMore && len(folders) > 0 {
		c := folders[len(folders)-1].Name
		nextCursor = &c
	}

	JSONList(w, folders, nextCursor, hasMore)
}

type createFolderRequest struct {
	Name      string  `json:"name"`
	Color     *string `json:"color,omitempty"`
	Namespace *string `json:"namespace,omitempty"`
}

func (s *Server) handleCreateFolder(w http.ResponseWriter, r *http.Request) {
	token := s.requireUserToken(w, r)
	if token == nil {
		return
	}

	var req createFolderRequest
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

	existing, err := s.store.GetFolderByName(nsID, req.Name)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check existing folder")
		return
	}
	if existing != nil {
		JSONError(w, http.StatusConflict, "Folder with that name already exists")
		return
	}

	folder := &store.Folder{
		ID:          uuid.New().String(),
		NamespaceID: nsID,
		Name:        req.Name,
		Color:       req.Color,
		CreatedAt:   time.Now(),
	}

	if err := s.store.CreateFolder(folder); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to create folder")
		return
	}

	JSON(w, http.StatusCreated, folder)
}

func (s *Server) handleGetFolder(w http.ResponseWriter, r *http.Request) {
	token := s.requireUserToken(w, r)
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
	Name  *string `json:"name,omitempty"`
	Color *string `json:"color,omitempty"`
}

func (s *Server) handleUpdateFolder(w http.ResponseWriter, r *http.Request) {
	token := s.requireUserToken(w, r)
	if token == nil {
		return
	}

	folder := s.requireFolderAccessWithPermission(w, r, token, store.PermNamespaceWrite)
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
		lowered := strings.ToLower(*req.Name)
		req.Name = &lowered

		if *req.Name != folder.Name {
			existing, err := s.store.GetFolderByName(folder.NamespaceID, *req.Name)
			if err != nil {
				JSONError(w, http.StatusInternalServerError, "Failed to check existing folder")
				return
			}
			if existing != nil {
				JSONError(w, http.StatusConflict, "Folder with that name already exists")
				return
			}
		}

		folder.Name = *req.Name
	}

	if req.Color != nil {
		if *req.Color == "" {
			folder.Color = nil
		} else {
			folder.Color = req.Color
		}
	}

	if err := s.store.UpdateFolder(folder); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to update folder")
		return
	}

	JSON(w, http.StatusOK, folder)
}

func (s *Server) handleDeleteFolder(w http.ResponseWriter, r *http.Request) {
	token := s.requireUserToken(w, r)
	if token == nil {
		return
	}

	folder := s.requireFolderAccessWithPermission(w, r, token, store.PermNamespaceWrite)
	if folder == nil {
		return
	}

	force := r.URL.Query().Get("force") == "true"
	if !force {
		count, err := s.store.CountFolderRepos(folder.ID)
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to check folder contents")
			return
		}
		if count > 0 {
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
