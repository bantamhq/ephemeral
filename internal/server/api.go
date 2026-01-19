package server

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"ephemeral/internal/store"
)

const defaultPageSize = 20

// Repo API handlers

func (s *Server) handleListRepos(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if token == nil {
		JSONError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	cursor := r.URL.Query().Get("cursor")
	repos, err := s.store.ListRepos(token.NamespaceID, cursor, defaultPageSize+1)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list repos")
		return
	}

	hasMore := len(repos) > defaultPageSize
	if hasMore {
		repos = repos[:defaultPageSize]
	}

	var nextCursor *string
	if hasMore && len(repos) > 0 {
		c := repos[len(repos)-1].ID
		nextCursor = &c
	}

	JSONList(w, repos, nextCursor, hasMore)
}

type createRepoRequest struct {
	Name   string `json:"name"`
	Public bool   `json:"public"`
}

func (s *Server) handleCreateRepo(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if !HasScope(token, store.ScopeRepos) {
		JSONError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	var req createRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name == "" {
		JSONError(w, http.StatusBadRequest, "Name is required")
		return
	}

	existing, err := s.store.GetRepo(token.NamespaceID, req.Name)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check existing repo")
		return
	}
	if existing != nil {
		JSONError(w, http.StatusConflict, "Repository already exists")
		return
	}

	now := time.Now()
	repo := &store.Repo{
		ID:          uuid.New().String(),
		NamespaceID: token.NamespaceID,
		Name:        req.Name,
		Public:      req.Public,
		SizeBytes:   0,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.store.CreateRepo(repo); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to create repo")
		return
	}

	repoPath := filepath.Join(s.dataDir, "repos", token.NamespaceID, req.Name+".git")
	if err := initBareRepo(repoPath); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to init bare repo")
		return
	}

	JSON(w, http.StatusCreated, repo)
}

func (s *Server) handleGetRepo(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if token == nil {
		JSONError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	repo, err := s.store.GetRepoByID(id)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get repo")
		return
	}
	if repo == nil {
		JSONError(w, http.StatusNotFound, "Repository not found")
		return
	}

	if repo.NamespaceID != token.NamespaceID && !HasScope(token, store.ScopeAdmin) {
		JSONError(w, http.StatusForbidden, "Access denied")
		return
	}

	JSON(w, http.StatusOK, repo)
}

func (s *Server) handleDeleteRepo(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if !HasScope(token, store.ScopeRepos) {
		JSONError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	id := chi.URLParam(r, "id")
	repo, err := s.store.GetRepoByID(id)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get repo")
		return
	}
	if repo == nil {
		JSONError(w, http.StatusNotFound, "Repository not found")
		return
	}

	if repo.NamespaceID != token.NamespaceID && !HasScope(token, store.ScopeAdmin) {
		JSONError(w, http.StatusForbidden, "Access denied")
		return
	}

	if err := s.store.DeleteRepo(id); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to delete repo")
		return
	}

	repoPath := filepath.Join(s.dataDir, "repos", repo.NamespaceID, repo.Name+".git")
	os.RemoveAll(repoPath)

	w.WriteHeader(http.StatusNoContent)
}

type updateRepoRequest struct {
	Name   *string `json:"name,omitempty"`
	Public *bool   `json:"public,omitempty"`
}

func (s *Server) handleUpdateRepo(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if !HasScope(token, store.ScopeRepos) {
		JSONError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	id := chi.URLParam(r, "id")
	repo, err := s.store.GetRepoByID(id)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get repo")
		return
	}
	if repo == nil {
		JSONError(w, http.StatusNotFound, "Repository not found")
		return
	}

	if repo.NamespaceID != token.NamespaceID && !HasScope(token, store.ScopeAdmin) {
		JSONError(w, http.StatusForbidden, "Access denied")
		return
	}

	var req updateRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	oldName := repo.Name
	if req.Name != nil {
		repo.Name = *req.Name
	}
	if req.Public != nil {
		repo.Public = *req.Public
	}

	if err := s.store.UpdateRepo(repo); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to update repo")
		return
	}

	if req.Name != nil && oldName != *req.Name {
		oldPath := filepath.Join(s.dataDir, "repos", repo.NamespaceID, oldName+".git")
		newPath := filepath.Join(s.dataDir, "repos", repo.NamespaceID, *req.Name+".git")
		os.Rename(oldPath, newPath)
	}

	JSON(w, http.StatusOK, repo)
}

// Token API handlers

type tokenResponse struct {
	ID          string     `json:"id"`
	Name        *string    `json:"name,omitempty"`
	NamespaceID string     `json:"namespace_id"`
	Scope       string     `json:"scope"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}

func tokenToResponse(t store.Token) tokenResponse {
	return tokenResponse{
		ID:          t.ID,
		Name:        t.Name,
		NamespaceID: t.NamespaceID,
		Scope:       t.Scope,
		CreatedAt:   t.CreatedAt,
		ExpiresAt:   t.ExpiresAt,
		LastUsedAt:  t.LastUsedAt,
	}
}

func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if !HasScope(token, store.ScopeFull) {
		JSONError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	cursor := r.URL.Query().Get("cursor")
	tokens, err := s.store.ListTokens(token.NamespaceID, cursor, defaultPageSize+1)
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

	resp := make([]tokenResponse, len(tokens))
	for i, t := range tokens {
		resp[i] = tokenToResponse(t)
	}

	JSONList(w, resp, nextCursor, hasMore)
}

type createTokenRequest struct {
	Name      string  `json:"name"`
	Scope     string  `json:"scope"`
	ExpiresIn *int    `json:"expires_in_seconds,omitempty"`
}

type createTokenResponse struct {
	tokenResponse
	Token string `json:"token"`
}

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if !HasScope(token, store.ScopeFull) {
		JSONError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	var req createTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Scope == "" {
		req.Scope = store.ScopeReadOnly
	}

	validScopes := map[string]bool{
		store.ScopeReadOnly: true,
		store.ScopeRepos:    true,
		store.ScopeFull:     true,
		store.ScopeAdmin:    true,
	}
	if !validScopes[req.Scope] {
		JSONError(w, http.StatusBadRequest, "Invalid scope")
		return
	}

	if scopeLevel(req.Scope) > scopeLevel(token.Scope) {
		JSONError(w, http.StatusForbidden, "Cannot create token with higher scope")
		return
	}

	nsPrefix := token.NamespaceID
	if len(nsPrefix) > 8 {
		nsPrefix = nsPrefix[:8]
	}
	rawToken := fmt.Sprintf("eph_%s_%s", nsPrefix, uuid.New().String()[:16])
	hasher := sha256.New()
	hasher.Write([]byte(rawToken))
	tokenHash := fmt.Sprintf("%x", hasher.Sum(nil))

	now := time.Now()
	newToken := &store.Token{
		ID:          uuid.New().String(),
		TokenHash:   tokenHash,
		Name:        &req.Name,
		NamespaceID: token.NamespaceID,
		Scope:       req.Scope,
		CreatedAt:   now,
	}

	if req.ExpiresIn != nil {
		exp := now.Add(time.Duration(*req.ExpiresIn) * time.Second)
		newToken.ExpiresAt = &exp
	}

	if err := s.store.CreateToken(newToken); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to create token")
		return
	}

	resp := createTokenResponse{
		tokenResponse: tokenToResponse(*newToken),
		Token:         rawToken,
	}

	JSON(w, http.StatusCreated, resp)
}

func (s *Server) handleDeleteToken(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if !HasScope(token, store.ScopeFull) {
		JSONError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	id := chi.URLParam(r, "id")
	targetToken, err := s.store.GetTokenByID(id)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get token")
		return
	}
	if targetToken == nil {
		JSONError(w, http.StatusNotFound, "Token not found")
		return
	}

	if targetToken.NamespaceID != token.NamespaceID && !HasScope(token, store.ScopeAdmin) {
		JSONError(w, http.StatusForbidden, "Access denied")
		return
	}

	if targetToken.ID == token.ID {
		JSONError(w, http.StatusBadRequest, "Cannot delete current token")
		return
	}

	if err := s.store.DeleteToken(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			JSONError(w, http.StatusNotFound, "Token not found")
			return
		}
		JSONError(w, http.StatusInternalServerError, "Failed to delete token")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Namespace API handlers

func (s *Server) handleListNamespaces(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if !HasScope(token, store.ScopeAdmin) {
		JSONError(w, http.StatusForbidden, "Admin access required")
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

type createNamespaceRequest struct {
	Name              string `json:"name"`
	RepoLimit         *int   `json:"repo_limit,omitempty"`
	StorageLimitBytes *int   `json:"storage_limit_bytes,omitempty"`
}

func (s *Server) handleCreateNamespace(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if !HasScope(token, store.ScopeAdmin) {
		JSONError(w, http.StatusForbidden, "Admin access required")
		return
	}

	var req createNamespaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name == "" {
		JSONError(w, http.StatusBadRequest, "Name is required")
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

func (s *Server) handleGetNamespace(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if !HasScope(token, store.ScopeAdmin) {
		JSONError(w, http.StatusForbidden, "Admin access required")
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

func (s *Server) handleDeleteNamespace(w http.ResponseWriter, r *http.Request) {
	token := GetTokenFromContext(r.Context())
	if !HasScope(token, store.ScopeAdmin) {
		JSONError(w, http.StatusForbidden, "Admin access required")
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

	if err := s.store.DeleteNamespace(id); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to delete namespace")
		return
	}

	reposPath := filepath.Join(s.dataDir, "repos", ns.Name)
	os.RemoveAll(reposPath)

	w.WriteHeader(http.StatusNoContent)
}
