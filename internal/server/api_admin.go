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
	ID              string                      `json:"id"`
	Name            *string                     `json:"name,omitempty"`
	IsAdmin         bool                        `json:"is_admin"`
	CreatedAt       time.Time                   `json:"created_at"`
	ExpiresAt       *time.Time                  `json:"expires_at,omitempty"`
	LastUsedAt      *time.Time                  `json:"last_used_at,omitempty"`
	NamespaceGrants []namespaceGrantAPIResponse `json:"namespace_grants,omitempty"`
	RepoGrants      []repoGrantAPIResponse      `json:"repo_grants,omitempty"`
}

type namespaceGrantAPIResponse struct {
	NamespaceID string   `json:"namespace_id"`
	Allow       []string `json:"allow"`
	Deny        []string `json:"deny,omitempty"`
	IsPrimary   bool     `json:"is_primary"`
}

type repoGrantAPIResponse struct {
	RepoID string   `json:"repo_id"`
	Allow  []string `json:"allow"`
	Deny   []string `json:"deny,omitempty"`
}

func (s *Server) adminTokenToResponse(t store.Token) adminTokenResponse {
	resp := adminTokenResponse{
		ID:         t.ID,
		Name:       t.Name,
		IsAdmin:    t.IsAdmin,
		CreatedAt:  t.CreatedAt,
		ExpiresAt:  t.ExpiresAt,
		LastUsedAt: t.LastUsedAt,
	}

	if !t.IsAdmin {
		nsGrants, err := s.store.ListTokenNamespaceGrants(t.ID)
		if err == nil {
			for _, g := range nsGrants {
				resp.NamespaceGrants = append(resp.NamespaceGrants, namespaceGrantAPIResponse{
					NamespaceID: g.NamespaceID,
					Allow:       g.AllowBits.ToStrings(),
					Deny:        g.DenyBits.ToStrings(),
					IsPrimary:   g.IsPrimary,
				})
			}
		}

		repoGrants, err := s.store.ListTokenRepoGrants(t.ID)
		if err == nil {
			for _, g := range repoGrants {
				resp.RepoGrants = append(resp.RepoGrants, repoGrantAPIResponse{
					RepoID: g.RepoID,
					Allow:  g.AllowBits.ToStrings(),
					Deny:   g.DenyBits.ToStrings(),
				})
			}
		}
	}

	return resp
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
		resp[i] = s.adminTokenToResponse(t)
	}

	JSONList(w, resp, nextCursor, hasMore)
}

type namespaceGrantRequest struct {
	NamespaceID string   `json:"namespace_id"`
	Allow       []string `json:"allow"`
	Deny        []string `json:"deny,omitempty"`
	IsPrimary   bool     `json:"is_primary"`
}

type repoGrantRequest struct {
	RepoID string   `json:"repo_id"`
	Allow  []string `json:"allow"`
	Deny   []string `json:"deny,omitempty"`
}

type adminCreateTokenRequest struct {
	Name            string                  `json:"name"`
	IsAdmin         bool                    `json:"is_admin"`
	NamespaceID     *string                 `json:"namespace_id,omitempty"`
	NamespaceGrants []namespaceGrantRequest `json:"namespace_grants,omitempty"`
	RepoGrants      []repoGrantRequest      `json:"repo_grants,omitempty"`
	ExpiresIn       *int                    `json:"expires_in_seconds,omitempty"`
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
		if req.NamespaceID != nil || len(req.NamespaceGrants) > 0 || len(req.RepoGrants) > 0 {
			JSONError(w, http.StatusBadRequest, "Admin tokens cannot have grants")
			return
		}
	} else {
		isSimpleMode := req.NamespaceID != nil
		isFullMode := len(req.NamespaceGrants) > 0 || len(req.RepoGrants) > 0

		if isSimpleMode && isFullMode {
			JSONError(w, http.StatusBadRequest, "Cannot use both namespace_id (simple mode) and grants (full mode)")
			return
		}

		if !isSimpleMode && !isFullMode {
			JSONError(w, http.StatusBadRequest, "User tokens require either namespace_id or grants")
			return
		}

		if isSimpleMode {
			ns, err := s.store.GetNamespace(*req.NamespaceID)
			if err != nil {
				JSONError(w, http.StatusInternalServerError, "Failed to check namespace")
				return
			}
			if ns == nil {
				JSONError(w, http.StatusBadRequest, "Namespace not found")
				return
			}

			req.NamespaceGrants = []namespaceGrantRequest{{
				NamespaceID: *req.NamespaceID,
				Allow:       []string{"namespace:write", "repo:admin"},
				IsPrimary:   true,
			}}
		}
	}

	if req.ExpiresIn != nil && *req.ExpiresIn < 0 {
		JSONError(w, http.StatusBadRequest, "expires_in_seconds cannot be negative")
		return
	}

	var nsGrants []store.NamespaceGrant
	for _, g := range req.NamespaceGrants {
		ns, err := s.store.GetNamespace(g.NamespaceID)
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to check namespace")
			return
		}
		if ns == nil {
			JSONError(w, http.StatusBadRequest, "Namespace not found: "+g.NamespaceID)
			return
		}

		allowBits, err := store.ParsePermissions(g.Allow)
		if err != nil {
			JSONError(w, http.StatusBadRequest, "Invalid permission: "+err.Error())
			return
		}

		var denyBits store.Permission
		if len(g.Deny) > 0 {
			denyBits, err = store.ParsePermissions(g.Deny)
			if err != nil {
				JSONError(w, http.StatusBadRequest, "Invalid permission: "+err.Error())
				return
			}
		}

		nsGrants = append(nsGrants, store.NamespaceGrant{
			NamespaceID: g.NamespaceID,
			AllowBits:   allowBits,
			DenyBits:    denyBits,
			IsPrimary:   g.IsPrimary,
		})
	}

	var repoGrants []store.RepoGrant
	for _, g := range req.RepoGrants {
		repo, err := s.store.GetRepoByID(g.RepoID)
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to check repo")
			return
		}
		if repo == nil {
			JSONError(w, http.StatusBadRequest, "Repository not found: "+g.RepoID)
			return
		}

		allowBits, err := store.ParsePermissions(g.Allow)
		if err != nil {
			JSONError(w, http.StatusBadRequest, "Invalid permission: "+err.Error())
			return
		}

		var denyBits store.Permission
		if len(g.Deny) > 0 {
			denyBits, err = store.ParsePermissions(g.Deny)
			if err != nil {
				JSONError(w, http.StatusBadRequest, "Invalid permission: "+err.Error())
				return
			}
		}

		repoGrants = append(repoGrants, store.RepoGrant{
			RepoID:    g.RepoID,
			AllowBits: allowBits,
			DenyBits:  denyBits,
		})
	}

	if req.IsAdmin {
		rawToken, newToken, err := s.createAdminToken(&req.Name, req.ExpiresIn)
		if err != nil {
			if errors.Is(err, store.ErrTokenLookupCollision) {
				JSONError(w, http.StatusInternalServerError, "Failed to create token after retries")
				return
			}
			JSONError(w, http.StatusInternalServerError, "Failed to create token")
			return
		}

		resp := adminCreateTokenResponse{
			adminTokenResponse: s.adminTokenToResponse(*newToken),
			Token:              rawToken,
		}

		JSON(w, http.StatusCreated, resp)
		return
	}

	var expiresAt *time.Time
	if req.ExpiresIn != nil {
		exp := time.Now().Add(time.Duration(*req.ExpiresIn) * time.Second)
		expiresAt = &exp
	}

	rawToken, newToken, err := s.store.GenerateUserTokenWithGrants(&req.Name, expiresAt, nsGrants, repoGrants)
	if err != nil {
		if errors.Is(err, store.ErrTokenLookupCollision) {
			JSONError(w, http.StatusInternalServerError, "Failed to create token after retries")
			return
		}
		JSONError(w, http.StatusInternalServerError, "Failed to create token")
		return
	}

	resp := adminCreateTokenResponse{
		adminTokenResponse: s.adminTokenToResponse(*newToken),
		Token:              rawToken,
	}

	JSON(w, http.StatusCreated, resp)
}

func (s *Server) createAdminToken(name *string, expiresIn *int) (string, *store.Token, error) {
	const tokenCreateAttempts = 5

	for attempt := 0; attempt < tokenCreateAttempts; attempt++ {
		tokenID := uuid.New().String()
		tokenLookup := tokenID[:8]

		secret, err := core.GenerateTokenSecret(24)
		if err != nil {
			return "", nil, fmt.Errorf("generate token secret: %w", err)
		}

		rawToken := core.BuildToken(tokenLookup, secret)

		tokenHash, err := core.HashToken(rawToken)
		if err != nil {
			return "", nil, fmt.Errorf("hash token: %w", err)
		}

		now := time.Now()
		candidate := &store.Token{
			ID:          tokenID,
			TokenHash:   tokenHash,
			TokenLookup: tokenLookup,
			Name:        name,
			IsAdmin:     true,
			CreatedAt:   now,
		}

		if expiresIn != nil {
			exp := now.Add(time.Duration(*expiresIn) * time.Second)
			candidate.ExpiresAt = &exp
		}

		if err := s.store.CreateToken(candidate); err != nil {
			if errors.Is(err, store.ErrTokenLookupCollision) {
				continue
			}
			return "", nil, err
		}

		return rawToken, candidate, nil
	}

	return "", nil, store.ErrTokenLookupCollision
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

	JSON(w, http.StatusOK, s.adminTokenToResponse(*token))
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

func (s *Server) handleAdminCreateNamespaceGrant(w http.ResponseWriter, r *http.Request) {
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
		JSONError(w, http.StatusBadRequest, "Admin tokens cannot have grants")
		return
	}

	var req namespaceGrantRequest
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

	allowBits, err := store.ParsePermissions(req.Allow)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid permission: "+err.Error())
		return
	}

	var denyBits store.Permission
	if len(req.Deny) > 0 {
		denyBits, err = store.ParsePermissions(req.Deny)
		if err != nil {
			JSONError(w, http.StatusBadRequest, "Invalid permission: "+err.Error())
			return
		}
	}

	now := time.Now()
	grant := &store.NamespaceGrant{
		TokenID:     tokenID,
		NamespaceID: req.NamespaceID,
		AllowBits:   allowBits,
		DenyBits:    denyBits,
		IsPrimary:   req.IsPrimary,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.store.UpsertNamespaceGrant(grant); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to create grant")
		return
	}

	grants, err := s.store.ListTokenNamespaceGrants(tokenID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list grants")
		return
	}

	var resp []namespaceGrantAPIResponse
	for _, g := range grants {
		resp = append(resp, namespaceGrantAPIResponse{
			NamespaceID: g.NamespaceID,
			Allow:       g.AllowBits.ToStrings(),
			Deny:        g.DenyBits.ToStrings(),
			IsPrimary:   g.IsPrimary,
		})
	}

	JSON(w, http.StatusOK, resp)
}

func (s *Server) handleAdminListNamespaceGrants(w http.ResponseWriter, r *http.Request) {
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

	grants, err := s.store.ListTokenNamespaceGrants(tokenID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list grants")
		return
	}

	var resp []namespaceGrantAPIResponse
	for _, g := range grants {
		resp = append(resp, namespaceGrantAPIResponse{
			NamespaceID: g.NamespaceID,
			Allow:       g.AllowBits.ToStrings(),
			Deny:        g.DenyBits.ToStrings(),
			IsPrimary:   g.IsPrimary,
		})
	}

	JSON(w, http.StatusOK, resp)
}

func (s *Server) handleAdminDeleteNamespaceGrant(w http.ResponseWriter, r *http.Request) {
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

	grant, err := s.store.GetNamespaceGrant(tokenID, nsID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check grant")
		return
	}
	if grant == nil {
		JSONError(w, http.StatusNotFound, "Grant not found")
		return
	}

	if grant.IsPrimary {
		JSONError(w, http.StatusBadRequest, "Cannot delete primary namespace grant")
		return
	}

	if err := s.store.DeleteNamespaceGrant(tokenID, nsID); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to delete grant")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminCreateRepoGrant(w http.ResponseWriter, r *http.Request) {
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
		JSONError(w, http.StatusBadRequest, "Admin tokens cannot have grants")
		return
	}

	var req repoGrantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	repo, err := s.store.GetRepoByID(req.RepoID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get repo")
		return
	}
	if repo == nil {
		JSONError(w, http.StatusNotFound, "Repository not found")
		return
	}

	allowBits, err := store.ParsePermissions(req.Allow)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid permission: "+err.Error())
		return
	}

	var denyBits store.Permission
	if len(req.Deny) > 0 {
		denyBits, err = store.ParsePermissions(req.Deny)
		if err != nil {
			JSONError(w, http.StatusBadRequest, "Invalid permission: "+err.Error())
			return
		}
	}

	now := time.Now()
	grant := &store.RepoGrant{
		TokenID:   tokenID,
		RepoID:    req.RepoID,
		AllowBits: allowBits,
		DenyBits:  denyBits,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.store.UpsertRepoGrant(grant); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to create grant")
		return
	}

	grants, err := s.store.ListTokenRepoGrants(tokenID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list grants")
		return
	}

	var resp []repoGrantAPIResponse
	for _, g := range grants {
		resp = append(resp, repoGrantAPIResponse{
			RepoID: g.RepoID,
			Allow:  g.AllowBits.ToStrings(),
			Deny:   g.DenyBits.ToStrings(),
		})
	}

	JSON(w, http.StatusOK, resp)
}

func (s *Server) handleAdminListRepoGrants(w http.ResponseWriter, r *http.Request) {
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

	grants, err := s.store.ListTokenRepoGrants(tokenID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list grants")
		return
	}

	var resp []repoGrantAPIResponse
	for _, g := range grants {
		resp = append(resp, repoGrantAPIResponse{
			RepoID: g.RepoID,
			Allow:  g.AllowBits.ToStrings(),
			Deny:   g.DenyBits.ToStrings(),
		})
	}

	JSON(w, http.StatusOK, resp)
}

func (s *Server) handleAdminDeleteRepoGrant(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminToken(w, r) == nil {
		return
	}

	tokenID := chi.URLParam(r, "id")
	repoID := chi.URLParam(r, "repoID")

	token, err := s.store.GetTokenByID(tokenID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get token")
		return
	}
	if token == nil {
		JSONError(w, http.StatusNotFound, "Token not found")
		return
	}

	grant, err := s.store.GetRepoGrant(tokenID, repoID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check grant")
		return
	}
	if grant == nil {
		JSONError(w, http.StatusNotFound, "Grant not found")
		return
	}

	if err := s.store.DeleteRepoGrant(tokenID, repoID); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to delete grant")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
