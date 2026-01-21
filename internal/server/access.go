package server

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"ephemeral/internal/store"
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

// requireScope returns true if the token has sufficient scope, or writes an error.
func (s *Server) requireScope(w http.ResponseWriter, token *store.Token, scope string) bool {
	if !HasScope(token, scope) {
		JSONError(w, http.StatusForbidden, "Insufficient permissions")
		return false
	}
	return true
}

// getActiveNamespaceID resolves the namespace from X-Namespace header or token's primary.
func (s *Server) getActiveNamespaceID(w http.ResponseWriter, r *http.Request, token *store.Token) string {
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

		hasAccess, err := s.store.HasTokenNamespaceAccess(token.ID, ns.ID)
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to check namespace access")
			return ""
		}
		if !hasAccess {
			JSONError(w, http.StatusForbidden, "Access denied to namespace")
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

// hasNamespaceAccess checks if a user token can access the given namespace.
func (s *Server) hasNamespaceAccess(token *store.Token, namespaceID string) (bool, error) {
	if token.IsAdmin {
		return false, nil
	}
	return s.store.HasTokenNamespaceAccess(token.ID, namespaceID)
}

// requireRepoAccess returns the repo if the token has access, or writes an error.
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

	hasAccess, err := s.hasNamespaceAccess(token, repo.NamespaceID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check access")
		return nil
	}
	if !hasAccess {
		JSONError(w, http.StatusForbidden, "Access denied")
		return nil
	}

	return repo
}

// requireFolderAccess returns the folder if the token has access, or writes an error.
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

	hasAccess, err := s.hasNamespaceAccess(token, folder.NamespaceID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check access")
		return nil
	}
	if !hasAccess {
		JSONError(w, http.StatusForbidden, "Access denied")
		return nil
	}

	return folder
}

// GetNamespaceIDFromContext retrieves the namespace ID from the request context.
func GetNamespaceIDFromContext(ctx context.Context) string {
	nsID, _ := ctx.Value(namespaceContextKey).(string)
	return nsID
}
