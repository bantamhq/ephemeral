package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"ephemeral/internal/store"
)

// requireAuth ensures the request has a valid token.
// Returns the token if valid, or writes an error response and returns nil.
func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) *store.Token {
	token := GetTokenFromContext(r.Context())
	if token == nil {
		JSONError(w, http.StatusUnauthorized, "Authentication required")
		return nil
	}
	return token
}

// requireScope ensures the token has at least the specified scope level.
// Returns true if authorized, or writes an error response and returns false.
func (s *Server) requireScope(w http.ResponseWriter, token *store.Token, scope string) bool {
	if !HasScope(token, scope) {
		JSONError(w, http.StatusForbidden, "Insufficient permissions")
		return false
	}
	return true
}

// requireAdmin ensures the request has a valid token with admin scope.
// Returns the token if authorized, or writes an error response and returns nil.
func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) *store.Token {
	token := GetTokenFromContext(r.Context())
	if token == nil {
		JSONError(w, http.StatusUnauthorized, "Authentication required")
		return nil
	}
	if !HasScope(token, store.ScopeAdmin) {
		JSONError(w, http.StatusForbidden, "Admin access required")
		return nil
	}
	return token
}

// requireRepoAccess retrieves a repo by ID and verifies the token has access.
// Returns the repo if access is granted, or writes an error response and returns nil.
func (s *Server) requireRepoAccess(w http.ResponseWriter, r *http.Request, token *store.Token) *store.Repo {
	id := chi.URLParam(r, "id")
	repo, err := s.store.GetRepoByID(id)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get repo")
		return nil
	}
	if repo == nil {
		JSONError(w, http.StatusNotFound, "Repository not found")
		return nil
	}

	if repo.NamespaceID != token.NamespaceID && !HasScope(token, store.ScopeAdmin) {
		JSONError(w, http.StatusForbidden, "Access denied")
		return nil
	}

	return repo
}

// requireFolderAccess retrieves a folder by ID and verifies the token has access.
// Returns the folder if access is granted, or writes an error response and returns nil.
func (s *Server) requireFolderAccess(w http.ResponseWriter, r *http.Request, token *store.Token) *store.Folder {
	id := chi.URLParam(r, "id")
	folder, err := s.store.GetFolderByID(id)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get folder")
		return nil
	}
	if folder == nil {
		JSONError(w, http.StatusNotFound, "Folder not found")
		return nil
	}

	if folder.NamespaceID != token.NamespaceID && !HasScope(token, store.ScopeAdmin) {
		JSONError(w, http.StatusForbidden, "Access denied")
		return nil
	}

	return folder
}
