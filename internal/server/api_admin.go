package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"ephemeral/internal/core"
	"ephemeral/internal/store"
)

var validUserScopes = map[string]bool{
	store.ScopeReadOnly: true,
	store.ScopeRepos:    true,
	store.ScopeFull:     true,
}

func (s *Server) handleAdminListNamespaces(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminToken(w, r) == nil {
		return
	}

	cursor := r.URL.Query().Get("cursor")
	namespaces, err := s.store.ListNamespaces(cursor, defaultPageSize+1)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list namespaces")
		return
	}

	hasMore := len(namespaces) > defaultPageSize
	if hasMore {
		namespaces = namespaces[:defaultPageSize]
	}

	var nextCursor *string
	if hasMore && len(namespaces) > 0 {
		c := namespaces[len(namespaces)-1].ID
		nextCursor = &c
	}

	JSONList(w, namespaces, nextCursor, hasMore)
}

type adminCreateNamespaceRequest struct {
	Name              string `json:"name"`
	RepoLimit         *int   `json:"repo_limit,omitempty"`
	StorageLimitBytes *int   `json:"storage_limit_bytes,omitempty"`
}

func (s *Server) handleAdminCreateNamespace(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminToken(w, r) == nil {
		return
	}

	var req adminCreateNamespaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := ValidateName(req.Name); err != nil {
		JSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	existing, err := s.store.GetNamespaceByName(req.Name)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check existing namespace")
		return
	}
	if existing != nil {
		JSONError(w, http.StatusConflict, "Namespace already exists")
		return
	}

	ns := &store.Namespace{
		ID:                uuid.New().String(),
		Name:              req.Name,
		CreatedAt:         time.Now(),
		RepoLimit:         req.RepoLimit,
		StorageLimitBytes: req.StorageLimitBytes,
	}

	if err := s.store.CreateNamespace(ns); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to create namespace")
		return
	}

	JSON(w, http.StatusCreated, ns)
}

func (s *Server) handleAdminGetNamespace(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminToken(w, r) == nil {
		return
	}

	id := chi.URLParam(r, "id")
	ns, err := s.store.GetNamespace(id)
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

func (s *Server) handleAdminDeleteNamespace(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminToken(w, r) == nil {
		return
	}

	id := chi.URLParam(r, "id")
	ns, err := s.store.GetNamespace(id)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get namespace")
		return
	}
	if ns == nil {
		JSONError(w, http.StatusNotFound, "Namespace not found")
		return
	}

	repos, err := s.store.ListRepos(id, "", 1)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check repos")
		return
	}
	if len(repos) > 0 {
		JSONError(w, http.StatusConflict, "Cannot delete namespace with existing repos")
		return
	}

	tokenCount, err := s.store.CountNamespaceTokens(id)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check tokens")
		return
	}
	if tokenCount > 0 {
		JSONError(w, http.StatusConflict, "Cannot delete namespace with token access")
		return
	}

	reposPath, err := SafeNamespacePath(s.dataDir, ns.ID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to resolve namespace path")
		return
	}

	if err := s.store.DeleteNamespace(id); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to delete namespace")
		return
	}

	if err := os.RemoveAll(reposPath); err != nil {
		fmt.Printf("Warning: failed to remove namespace directory %s: %v\n", reposPath, err)
	}

	w.WriteHeader(http.StatusNoContent)
}

type adminTokenResponse struct {
	ID         string     `json:"id"`
	Name       *string    `json:"name,omitempty"`
	IsAdmin    bool       `json:"is_admin"`
	Scope      string     `json:"scope"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

func adminTokenToResponse(t store.Token) adminTokenResponse {
	return adminTokenResponse{
		ID:         t.ID,
		Name:       t.Name,
		IsAdmin:    t.IsAdmin,
		Scope:      t.Scope,
		CreatedAt:  t.CreatedAt,
		ExpiresAt:  t.ExpiresAt,
		LastUsedAt: t.LastUsedAt,
	}
}

func (s *Server) handleAdminListTokens(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminToken(w, r) == nil {
		return
	}

	cursor := r.URL.Query().Get("cursor")
	tokens, err := s.store.ListTokens(cursor, defaultPageSize+1)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list tokens")
		return
	}

	hasMore := len(tokens) > defaultPageSize
	if hasMore {
		tokens = tokens[:defaultPageSize]
	}

	var nextCursor *string
	if hasMore && len(tokens) > 0 {
		c := tokens[len(tokens)-1].ID
		nextCursor = &c
	}

	resp := make([]adminTokenResponse, len(tokens))
	for i, t := range tokens {
		resp[i] = adminTokenToResponse(t)
	}

	JSONList(w, resp, nextCursor, hasMore)
}

type adminCreateTokenRequest struct {
	Name        string  `json:"name"`
	IsAdmin     bool    `json:"is_admin"`
	NamespaceID *string `json:"namespace_id,omitempty"`
	Scope       string  `json:"scope"`
	ExpiresIn   *int    `json:"expires_in_seconds,omitempty"`
}

type adminCreateTokenResponse struct {
	adminTokenResponse
	Token string `json:"token"`
}

func (s *Server) handleAdminCreateToken(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminToken(w, r) == nil {
		return
	}

	var req adminCreateTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.IsAdmin {
		if req.NamespaceID != nil {
			JSONError(w, http.StatusBadRequest, "Admin tokens cannot have a namespace_id")
			return
		}
		req.Scope = store.ScopeFull
	} else {
		if req.NamespaceID == nil {
			JSONError(w, http.StatusBadRequest, "User tokens require a namespace_id for primary namespace")
			return
		}
		if req.Scope == "" {
			req.Scope = store.ScopeFull
		}
		if !validUserScopes[req.Scope] {
			JSONError(w, http.StatusBadRequest, "Invalid scope: must be full, repos, or read-only")
			return
		}

		ns, err := s.store.GetNamespace(*req.NamespaceID)
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to check namespace")
			return
		}
		if ns == nil {
			JSONError(w, http.StatusBadRequest, "Namespace not found")
			return
		}
	}

	if req.ExpiresIn != nil && *req.ExpiresIn < 0 {
		JSONError(w, http.StatusBadRequest, "expires_in_seconds cannot be negative")
		return
	}

	const tokenCreateAttempts = 5

	var rawToken string
	var newToken *store.Token

	for attempt := 0; attempt < tokenCreateAttempts; attempt++ {
		tokenID := uuid.New().String()
		tokenLookup := tokenID[:8]

		secret, err := core.GenerateTokenSecret(24)
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to generate token")
			return
		}

		rawToken = core.BuildToken(tokenLookup, secret)

		tokenHash, err := core.HashToken(rawToken)
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to generate token")
			return
		}

		now := time.Now()
		candidate := &store.Token{
			ID:          tokenID,
			TokenHash:   tokenHash,
			TokenLookup: tokenLookup,
			Name:        &req.Name,
			IsAdmin:     req.IsAdmin,
			Scope:       req.Scope,
			CreatedAt:   now,
		}

		if req.ExpiresIn != nil {
			exp := now.Add(time.Duration(*req.ExpiresIn) * time.Second)
			candidate.ExpiresAt = &exp
		}

		if err := s.store.CreateToken(candidate); err != nil {
			if errors.Is(err, store.ErrTokenLookupCollision) {
				continue
			}
			JSONError(w, http.StatusInternalServerError, "Failed to create token")
			return
		}

		newToken = candidate
		break
	}

	if newToken == nil {
		JSONError(w, http.StatusInternalServerError, "Failed to create token after retries")
		return
	}

	if !req.IsAdmin {
		access := &store.TokenNamespaceAccess{
			TokenID:     newToken.ID,
			NamespaceID: *req.NamespaceID,
			IsPrimary:   true,
			CreatedAt:   time.Now(),
		}
		if err := s.store.GrantTokenNamespaceAccess(access); err != nil {
			s.store.DeleteToken(newToken.ID)
			JSONError(w, http.StatusInternalServerError, "Failed to grant namespace access")
			return
		}
	}

	resp := adminCreateTokenResponse{
		adminTokenResponse: adminTokenToResponse(*newToken),
		Token:              rawToken,
	}

	JSON(w, http.StatusCreated, resp)
}

func (s *Server) handleAdminGetToken(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminToken(w, r) == nil {
		return
	}

	id := chi.URLParam(r, "id")
	token, err := s.store.GetTokenByID(id)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get token")
		return
	}
	if token == nil {
		JSONError(w, http.StatusNotFound, "Token not found")
		return
	}

	JSON(w, http.StatusOK, adminTokenToResponse(*token))
}

func (s *Server) handleAdminDeleteToken(w http.ResponseWriter, r *http.Request) {
	adminToken := s.requireAdminToken(w, r)
	if adminToken == nil {
		return
	}

	id := chi.URLParam(r, "id")
	token, err := s.store.GetTokenByID(id)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get token")
		return
	}
	if token == nil {
		JSONError(w, http.StatusNotFound, "Token not found")
		return
	}

	if token.ID == adminToken.ID {
		JSONError(w, http.StatusBadRequest, "Cannot delete current token")
		return
	}

	if err := s.store.DeleteToken(id); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to delete token")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type grantNamespaceRequest struct {
	NamespaceID string `json:"namespace_id"`
	IsPrimary   bool   `json:"is_primary"`
}

func (s *Server) handleAdminGrantTokenNamespace(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminToken(w, r) == nil {
		return
	}

	tokenID := chi.URLParam(r, "id")
	token, err := s.store.GetTokenByID(tokenID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get token")
		return
	}
	if token == nil {
		JSONError(w, http.StatusNotFound, "Token not found")
		return
	}

	if token.IsAdmin {
		JSONError(w, http.StatusBadRequest, "Admin tokens cannot have namespace access")
		return
	}

	var req grantNamespaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	ns, err := s.store.GetNamespace(req.NamespaceID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get namespace")
		return
	}
	if ns == nil {
		JSONError(w, http.StatusNotFound, "Namespace not found")
		return
	}

	access := &store.TokenNamespaceAccess{
		TokenID:     tokenID,
		NamespaceID: req.NamespaceID,
		IsPrimary:   req.IsPrimary,
		CreatedAt:   time.Now(),
	}

	if err := s.store.GrantTokenNamespaceAccess(access); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to grant namespace access")
		return
	}

	namespaces, err := s.store.ListTokenNamespaces(tokenID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list namespaces")
		return
	}

	JSON(w, http.StatusOK, namespaces)
}

func (s *Server) handleAdminRevokeTokenNamespace(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminToken(w, r) == nil {
		return
	}

	tokenID := chi.URLParam(r, "id")
	nsID := chi.URLParam(r, "nsID")

	token, err := s.store.GetTokenByID(tokenID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get token")
		return
	}
	if token == nil {
		JSONError(w, http.StatusNotFound, "Token not found")
		return
	}

	access, err := s.store.GetTokenNamespaceAccess(tokenID, nsID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check access")
		return
	}
	if access == nil {
		JSONError(w, http.StatusNotFound, "Token does not have access to this namespace")
		return
	}

	if access.IsPrimary {
		JSONError(w, http.StatusBadRequest, "Cannot revoke access to primary namespace")
		return
	}

	if err := s.store.RevokeTokenNamespaceAccess(tokenID, nsID); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to revoke namespace access")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
