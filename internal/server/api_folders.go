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

type folderTreeNode struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	ParentID  *string          `json:"parent_id,omitempty"`
	CreatedAt time.Time        `json:"created_at"`
	Children  []folderTreeNode `json:"children,omitempty"`
}

func (s *Server) handleGetFolderTree(w http.ResponseWriter, r *http.Request) {
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

	tree := buildFolderTree(folders)
	JSON(w, http.StatusOK, tree)
}

func buildFolderTree(folders []store.Folder) []folderTreeNode {
	nodeMap := make(map[string]*folderTreeNode)
	childrenMap := make(map[string][]string)

	for _, f := range folders {
		nodeMap[f.ID] = &folderTreeNode{
			ID:        f.ID,
			Name:      f.Name,
			ParentID:  f.ParentID,
			CreatedAt: f.CreatedAt,
		}
		if f.ParentID != nil {
			childrenMap[*f.ParentID] = append(childrenMap[*f.ParentID], f.ID)
		}
	}

	var buildNode func(id string) folderTreeNode
	buildNode = func(id string) folderTreeNode {
		node := nodeMap[id]
		result := folderTreeNode{
			ID:        node.ID,
			Name:      node.Name,
			ParentID:  node.ParentID,
			CreatedAt: node.CreatedAt,
			Children:  []folderTreeNode{},
		}
		for _, childID := range childrenMap[id] {
			result.Children = append(result.Children, buildNode(childID))
		}
		return result
	}

	var roots []folderTreeNode
	for _, f := range folders {
		if f.ParentID == nil {
			roots = append(roots, buildNode(f.ID))
		}
	}
	return roots
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
