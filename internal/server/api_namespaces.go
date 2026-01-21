package server

import (
	"net/http"
)

// handleListNamespaces lists all namespaces the current user token has access to.
func (s *Server) handleListNamespaces(w http.ResponseWriter, r *http.Request) {
	token := s.requireUserToken(w, r)
	if token == nil {
		return
	}

	namespaces, err := s.store.ListTokenNamespaces(token.ID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list namespaces")
		return
	}

	JSON(w, http.StatusOK, namespaces)
}

// handleGetCurrentNamespace returns the currently active namespace based on X-Namespace header
// or the token's primary namespace.
func (s *Server) handleGetCurrentNamespace(w http.ResponseWriter, r *http.Request) {
	token := s.requireUserToken(w, r)
	if token == nil {
		return
	}

	nsID := s.getActiveNamespaceID(w, r, token)
	if nsID == "" {
		return
	}

	ns, err := s.store.GetNamespace(nsID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get namespace")
		return
	}
	if ns == nil {
		JSONError(w, http.StatusNotFound, "Namespace not found")
		return
	}

	JSON(w, http.StatusOK, ns)
}
