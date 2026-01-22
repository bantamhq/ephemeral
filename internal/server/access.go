package server

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/bantamhq/ephemeral/internal/store"
)

// requireAuth returns the authenticated token or writes an error response.
func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) *store.Token {
	token := GetTokenFromContext(r.Context())
	if token == nil {
		JSONError(w, http.StatusUnauthorized, "Authentication required")
		return nil
	}
	return token
}

// requireUserToken returns a non-admin token or writes an error response.
func (s *Server) requireUserToken(w http.ResponseWriter, r *http.Request) *store.Token {
	token := s.requireAuth(w, r)
	if token == nil {
		return nil
	}
	if token.IsAdmin {
		JSONError(w, http.StatusForbidden, "Admin token cannot be used for this operation")
		return nil
	}
	return token
}

// requireAdminToken returns an admin token or writes an error response.
func (s *Server) requireAdminToken(w http.ResponseWriter, r *http.Request) *store.Token {
	token := s.requireAuth(w, r)
	if token == nil {
		return nil
	}
	if !token.IsAdmin {
		JSONError(w, http.StatusForbidden, "Admin access required")
		return nil
	}
	return token
}

// requireNamespacePermission checks if the token has the required permission for a namespace.
func (s *Server) requireNamespacePermission(w http.ResponseWriter, token *store.Token, namespaceID string, required store.Permission) bool {
	hasPermission, err := s.permissions.CheckNamespacePermission(token.ID, namespaceID, required)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check permissions")
		return false
	}
	if !hasPermission {
		JSONError(w, http.StatusForbidden, "Insufficient permissions")
		return false
	}
	return true
}

// requireRepoPermission checks if the token has the required permission for a repo.
func (s *Server) requireRepoPermission(w http.ResponseWriter, token *store.Token, repo *store.Repo, required store.Permission) bool {
	hasPermission, err := s.permissions.CheckRepoPermission(token.ID, repo, required)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check permissions")
		return false
	}
	if !hasPermission {
		JSONError(w, http.StatusForbidden, "Insufficient permissions")
		return false
	}
	return true
}

// resolveNamespaceID resolves the namespace ID from X-Namespace header or token's primary.
// This only resolves the namespace without enforcing permissions.
func (s *Server) resolveNamespaceID(w http.ResponseWriter, r *http.Request, token *store.Token) string {
	if nsID := GetNamespaceIDFromContext(r.Context()); nsID != "" {
		return nsID
	}

	if nsName := r.Header.Get("X-Namespace"); nsName != "" {
		ns, err := s.store.GetNamespaceByName(nsName)
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to resolve namespace")
			return ""
		}
		if ns == nil {
			JSONError(w, http.StatusNotFound, "Namespace not found")
			return ""
		}
		return ns.ID
	}

	primaryNS, err := s.store.GetTokenPrimaryNamespace(token.ID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get primary namespace")
		return ""
	}
	if primaryNS == nil {
		JSONError(w, http.StatusBadRequest, "No namespace specified and no primary namespace configured")
		return ""
	}

	return primaryNS.ID
}

// getActiveNamespaceID resolves namespace and validates the token can access it.
// Tokens can access a namespace if they have namespace grants OR repo grants in it.
func (s *Server) getActiveNamespaceID(w http.ResponseWriter, r *http.Request, token *store.Token) string {
	nsID := s.resolveNamespaceID(w, r, token)
	if nsID == "" {
		return ""
	}

	canAccess, err := s.permissions.CanAccessNamespace(token.ID, nsID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check namespace access")
		return ""
	}
	if !canAccess {
		JSONError(w, http.StatusForbidden, "Access denied to namespace")
		return ""
	}

	return nsID
}

// getNamespaceIDWithPermission resolves namespace and requires specific permission.
// This should be used for operations requiring namespace-level permissions (like list folders).
func (s *Server) getNamespaceIDWithPermission(w http.ResponseWriter, r *http.Request, token *store.Token, required store.Permission) string {
	nsID := s.resolveNamespaceID(w, r, token)
	if nsID == "" {
		return ""
	}

	if !s.requireNamespacePermission(w, token, nsID, required) {
		return ""
	}

	return nsID
}

// requireRepoAccess returns the repo if the token has read access, or writes an error.
func (s *Server) requireRepoAccess(w http.ResponseWriter, r *http.Request, token *store.Token) *store.Repo {
	return s.requireRepoAccessWithPermission(w, r, token, store.PermRepoRead)
}

// requireRepoAccessWithPermission returns the repo if the token has the required permission.
func (s *Server) requireRepoAccessWithPermission(w http.ResponseWriter, r *http.Request, token *store.Token, required store.Permission) *store.Repo {
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

	if !s.requireRepoPermission(w, token, repo, required) {
		return nil
	}

	return repo
}

// requireFolderAccess returns the folder if the token has namespace read access, or writes an error.
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

	if !s.requireNamespacePermission(w, token, folder.NamespaceID, store.PermNamespaceRead) {
		return nil
	}

	return folder
}

// requireFolderAccessWithPermission returns the folder if the token has the required namespace permission.
func (s *Server) requireFolderAccessWithPermission(w http.ResponseWriter, r *http.Request, token *store.Token, required store.Permission) *store.Folder {
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

	if !s.requireNamespacePermission(w, token, folder.NamespaceID, required) {
		return nil
	}

	return folder
}

// GetNamespaceIDFromContext retrieves the namespace ID from the request context.
func GetNamespaceIDFromContext(ctx context.Context) string {
	nsID, _ := ctx.Value(namespaceContextKey).(string)
	return nsID
}
