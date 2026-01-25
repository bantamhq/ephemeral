package server

import (
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
	return s.requireFolderAccessWithPermission(w, r, token, store.PermNamespaceRead)
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

// getTokenByID retrieves a token by ID from the URL parameter, writing an error response if not found.
func (s *Server) getTokenByID(w http.ResponseWriter, r *http.Request) *store.Token {
	id := chi.URLParam(r, "id")
	token, err := s.store.GetTokenByID(id)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get token")
		return nil
	}
	if token == nil {
		JSONError(w, http.StatusNotFound, "Token not found")
		return nil
	}
	return token
}

// getUserByID retrieves a user by ID from the URL parameter, writing an error response if not found.
func (s *Server) getUserByID(w http.ResponseWriter, r *http.Request) *store.User {
	id := chi.URLParam(r, "id")
	user, err := s.store.GetUser(id)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get user")
		return nil
	}
	if user == nil {
		JSONError(w, http.StatusNotFound, "User not found")
		return nil
	}
	return user
}

// resolveNamespaceID resolves a namespace ID from an explicit name or falls back to the user's primary namespace.
// Returns empty string and writes error response on failure.
func (s *Server) resolveNamespaceID(w http.ResponseWriter, token *store.Token, namespaceName *string) string {
	if namespaceName != nil && *namespaceName != "" {
		ns, err := s.store.GetNamespaceByName(*namespaceName)
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to get namespace")
			return ""
		}
		if ns == nil {
			JSONError(w, http.StatusNotFound, "Namespace not found")
			return ""
		}
		return ns.ID
	}

	if token.UserID == nil {
		JSONError(w, http.StatusForbidden, "Token has no associated user")
		return ""
	}

	user, err := s.store.GetUser(*token.UserID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get user")
		return ""
	}
	if user == nil {
		JSONError(w, http.StatusInternalServerError, "User not found")
		return ""
	}
	return user.PrimaryNamespaceID
}
