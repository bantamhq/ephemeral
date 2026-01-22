package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"

	"github.com/bantamhq/ephemeral/internal/store"
)

// namespaceGrantResponse represents a namespace grant in API responses.
type namespaceGrantResponse struct {
	NamespaceID string   `json:"namespace_id"`
	Allow       []string `json:"allow"`
	Deny        []string `json:"deny,omitempty"`
	IsPrimary   bool     `json:"is_primary"`
}

// namespaceListResponse represents a namespace with its grant for the current token.
type namespaceListResponse struct {
	store.Namespace
	Allow     []string `json:"allow"`
	Deny      []string `json:"deny,omitempty"`
	IsPrimary bool     `json:"is_primary"`
}

// handleListNamespaces lists all namespaces the current user token has access to.
func (s *Server) handleListNamespaces(w http.ResponseWriter, r *http.Request) {
	token := s.requireUserToken(w, r)
	if token == nil {
		return
	}

	grants, err := s.store.ListTokenNamespaceGrants(token.ID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list namespaces")
		return
	}

	var result []namespaceListResponse
	for _, g := range grants {
		ns, err := s.store.GetNamespace(g.NamespaceID)
		if err != nil || ns == nil {
			continue
		}
		result = append(result, namespaceListResponse{
			Namespace: *ns,
			Allow:     g.AllowBits.ToStrings(),
			Deny:      g.DenyBits.ToStrings(),
			IsPrimary: g.IsPrimary,
		})
	}

	JSON(w, http.StatusOK, result)
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

type updateNamespaceRequest struct {
	Name              *string `json:"name,omitempty"`
	RepoLimit         *int    `json:"repo_limit,omitempty"`
	StorageLimitBytes *int    `json:"storage_limit_bytes,omitempty"`
}

// handleUpdateNamespace updates a namespace (requires namespace:admin).
func (s *Server) handleUpdateNamespace(w http.ResponseWriter, r *http.Request) {
	token := s.requireUserToken(w, r)
	if token == nil {
		return
	}

	nsID := chi.URLParam(r, "id")
	ns, err := s.store.GetNamespace(nsID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get namespace")
		return
	}
	if ns == nil {
		JSONError(w, http.StatusNotFound, "Namespace not found")
		return
	}

	if !s.requireNamespacePermission(w, token, nsID, store.PermNamespaceAdmin) {
		return
	}

	var req updateNamespaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name != nil {
		if err := ValidateName(*req.Name); err != nil {
			JSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		ns.Name = *req.Name
	}

	if req.RepoLimit != nil {
		ns.RepoLimit = req.RepoLimit
	}

	if req.StorageLimitBytes != nil {
		ns.StorageLimitBytes = req.StorageLimitBytes
	}

	if err := s.store.UpdateNamespace(ns); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to update namespace")
		return
	}

	JSON(w, http.StatusOK, ns)
}

// handleDeleteNamespaceScoped deletes a namespace (requires namespace:admin).
func (s *Server) handleDeleteNamespaceScoped(w http.ResponseWriter, r *http.Request) {
	token := s.requireUserToken(w, r)
	if token == nil {
		return
	}

	nsID := chi.URLParam(r, "id")
	ns, err := s.store.GetNamespace(nsID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get namespace")
		return
	}
	if ns == nil {
		JSONError(w, http.StatusNotFound, "Namespace not found")
		return
	}

	if !s.requireNamespacePermission(w, token, nsID, store.PermNamespaceAdmin) {
		return
	}

	repos, err := s.store.ListRepos(nsID, "", 1)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check repos")
		return
	}
	if len(repos) > 0 {
		JSONError(w, http.StatusConflict, "Cannot delete namespace with existing repos")
		return
	}

	tokenCount, err := s.store.CountNamespaceTokens(nsID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check tokens")
		return
	}
	if tokenCount > 1 {
		JSONError(w, http.StatusConflict, "Cannot delete namespace with other tokens having access")
		return
	}

	reposPath, err := SafeNamespacePath(s.dataDir, ns.ID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to resolve namespace path")
		return
	}

	if err := s.store.DeleteNamespace(nsID); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to delete namespace")
		return
	}

	if err := os.RemoveAll(reposPath); err != nil {
		fmt.Printf("Warning: failed to remove namespace directory %s: %v\n", reposPath, err)
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleListNamespaceGrants returns the current token's grant for a namespace (requires namespace:admin).
func (s *Server) handleListNamespaceGrants(w http.ResponseWriter, r *http.Request) {
	token := s.requireUserToken(w, r)
	if token == nil {
		return
	}

	nsID := chi.URLParam(r, "id")
	ns, err := s.store.GetNamespace(nsID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get namespace")
		return
	}
	if ns == nil {
		JSONError(w, http.StatusNotFound, "Namespace not found")
		return
	}

	if !s.requireNamespacePermission(w, token, nsID, store.PermNamespaceAdmin) {
		return
	}

	grant, err := s.store.GetNamespaceGrant(token.ID, nsID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get grant")
		return
	}

	var result []namespaceGrantResponse
	if grant != nil {
		result = append(result, namespaceGrantResponse{
			NamespaceID: grant.NamespaceID,
			Allow:       grant.AllowBits.ToStrings(),
			Deny:        grant.DenyBits.ToStrings(),
			IsPrimary:   grant.IsPrimary,
		})
	}

	JSON(w, http.StatusOK, result)
}
