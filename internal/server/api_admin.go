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

	"github.com/bantamhq/ephemeral/internal/store"
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

	name := chi.URLParam(r, "name")
	ns, err := s.store.GetNamespaceByName(name)
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

	name := chi.URLParam(r, "name")
	ns, err := s.store.GetNamespaceByName(name)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get namespace")
		return
	}
	if ns == nil {
		JSONError(w, http.StatusNotFound, "Namespace not found")
		return
	}

	repos, err := s.store.ListRepos(ns.ID, "", 1)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check repos")
		return
	}
	if len(repos) > 0 {
		JSONError(w, http.StatusConflict, "Cannot delete namespace with existing repos")
		return
	}

	userCount, err := s.store.CountNamespaceUsers(ns.ID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check users")
		return
	}
	if userCount > 0 {
		JSONError(w, http.StatusConflict, "Cannot delete namespace with user access")
		return
	}

	reposPath, err := SafeNamespacePath(s.dataDir, ns.ID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to resolve namespace path")
		return
	}

	if err := s.store.DeleteNamespace(ns.ID); err != nil {
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
	IsAdmin         bool                        `json:"is_admin"`
	UserID          *string                     `json:"user_id,omitempty"`
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
}

type repoGrantAPIResponse struct {
	RepoID string   `json:"repo_id"`
	Allow  []string `json:"allow"`
	Deny   []string `json:"deny,omitempty"`
}

func (s *Server) adminTokenToResponse(t store.Token) adminTokenResponse {
	resp := adminTokenResponse{
		ID:         t.ID,
		IsAdmin:    t.IsAdmin,
		UserID:     t.UserID,
		CreatedAt:  t.CreatedAt,
		ExpiresAt:  t.ExpiresAt,
		LastUsedAt: t.LastUsedAt,
	}

	if t.IsAdmin || t.UserID == nil {
		return resp
	}

	nsGrants, err := s.store.ListUserNamespaceGrants(*t.UserID)
	if err == nil {
		for _, g := range nsGrants {
			resp.NamespaceGrants = append(resp.NamespaceGrants, namespaceGrantToResponse(g))
		}
	}

	repoGrants, err := s.store.ListUserRepoGrants(*t.UserID)
	if err == nil {
		for _, g := range repoGrants {
			resp.RepoGrants = append(resp.RepoGrants, repoGrantToResponse(g))
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

func (s *Server) handleAdminGetToken(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminToken(w, r) == nil {
		return
	}

	token := s.getTokenByID(w, r)
	if token == nil {
		return
	}

	JSON(w, http.StatusOK, s.adminTokenToResponse(*token))
}

func (s *Server) handleAdminDeleteToken(w http.ResponseWriter, r *http.Request) {
	adminToken := s.requireAdminToken(w, r)
	if adminToken == nil {
		return
	}

	token := s.getTokenByID(w, r)
	if token == nil {
		return
	}

	if token.ID == adminToken.ID {
		JSONError(w, http.StatusBadRequest, "Cannot delete current token")
		return
	}

	if err := s.store.DeleteToken(token.ID); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to delete token")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type adminUserResponse struct {
	ID                 string    `json:"id"`
	PrimaryNamespaceID string    `json:"primary_namespace_id"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func userToResponse(u store.User) adminUserResponse {
	return adminUserResponse{
		ID:                 u.ID,
		PrimaryNamespaceID: u.PrimaryNamespaceID,
		CreatedAt:          u.CreatedAt,
		UpdatedAt:          u.UpdatedAt,
	}
}

func namespaceGrantToResponse(g store.NamespaceGrant) namespaceGrantAPIResponse {
	return namespaceGrantAPIResponse{
		NamespaceID: g.NamespaceID,
		Allow:       g.AllowBits.ToStrings(),
		Deny:        g.DenyBits.ToStrings(),
	}
}

func repoGrantToResponse(g store.RepoGrant) repoGrantAPIResponse {
	return repoGrantAPIResponse{
		RepoID: g.RepoID,
		Allow:  g.AllowBits.ToStrings(),
		Deny:   g.DenyBits.ToStrings(),
	}
}

func (s *Server) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminToken(w, r) == nil {
		return
	}

	cursor := r.URL.Query().Get("cursor")
	users, err := s.store.ListUsers(cursor, defaultPageSize+1)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list users")
		return
	}

	hasMore := len(users) > defaultPageSize
	if hasMore {
		users = users[:defaultPageSize]
	}

	var nextCursor *string
	if hasMore && len(users) > 0 {
		c := users[len(users)-1].ID
		nextCursor = &c
	}

	resp := make([]adminUserResponse, len(users))
	for i, u := range users {
		resp[i] = userToResponse(u)
	}

	JSONList(w, resp, nextCursor, hasMore)
}

type adminCreateUserRequest struct {
	NamespaceID string `json:"namespace_id"`
}

func (s *Server) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminToken(w, r) == nil {
		return
	}

	var req adminCreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.NamespaceID == "" {
		JSONError(w, http.StatusBadRequest, "namespace_id is required")
		return
	}

	ns, err := s.store.GetNamespace(req.NamespaceID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check namespace")
		return
	}
	if ns == nil {
		JSONError(w, http.StatusNotFound, "Namespace not found")
		return
	}

	now := time.Now()
	user := &store.User{
		ID:                 uuid.New().String(),
		PrimaryNamespaceID: req.NamespaceID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := s.store.CreateUser(user); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to create user")
		return
	}

	grant := &store.NamespaceGrant{
		UserID:      user.ID,
		NamespaceID: req.NamespaceID,
		AllowBits:   store.DefaultNamespaceGrant(),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.store.UpsertNamespaceGrant(grant); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to create grant")
		return
	}

	JSON(w, http.StatusCreated, userToResponse(*user))
}

func (s *Server) handleAdminGetUser(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminToken(w, r) == nil {
		return
	}

	user := s.getUserByID(w, r)
	if user == nil {
		return
	}

	JSON(w, http.StatusOK, userToResponse(*user))
}

func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminToken(w, r) == nil {
		return
	}

	user := s.getUserByID(w, r)
	if user == nil {
		return
	}

	if err := s.store.DeleteUser(user.ID); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to delete user")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminListUserTokens(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminToken(w, r) == nil {
		return
	}

	user := s.getUserByID(w, r)
	if user == nil {
		return
	}

	tokens, err := s.store.ListUserTokens(user.ID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list user tokens")
		return
	}

	resp := make([]adminTokenResponse, len(tokens))
	for i, t := range tokens {
		resp[i] = s.adminTokenToResponse(t)
	}

	JSON(w, http.StatusOK, resp)
}

type adminCreateUserTokenRequest struct {
	ExpiresIn *int `json:"expires_in_seconds,omitempty"`
}

type adminCreateUserTokenResponse struct {
	Token    string             `json:"token"`
	Metadata adminTokenResponse `json:"metadata"`
}

func (s *Server) handleAdminCreateUserToken(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminToken(w, r) == nil {
		return
	}

	user := s.getUserByID(w, r)
	if user == nil {
		return
	}

	var req adminCreateUserTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.ExpiresIn != nil && *req.ExpiresIn < 0 {
		JSONError(w, http.StatusBadRequest, "expires_in_seconds cannot be negative")
		return
	}

	var expiresAt *time.Time
	if req.ExpiresIn != nil {
		exp := time.Now().Add(time.Duration(*req.ExpiresIn) * time.Second)
		expiresAt = &exp
	}

	rawToken, token, err := s.store.GenerateUserToken(user.ID, expiresAt)
	if err != nil {
		if errors.Is(err, store.ErrTokenLookupCollision) {
			JSONError(w, http.StatusInternalServerError, "Failed to create token after retries")
			return
		}
		JSONError(w, http.StatusInternalServerError, "Failed to create token")
		return
	}

	resp := adminCreateUserTokenResponse{
		Token:    rawToken,
		Metadata: s.adminTokenToResponse(*token),
	}

	JSON(w, http.StatusCreated, resp)
}

type userNamespaceGrantRequest struct {
	NamespaceID string   `json:"namespace_id"`
	Allow       []string `json:"allow"`
	Deny        []string `json:"deny,omitempty"`
}

func (s *Server) handleAdminCreateUserNamespaceGrant(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminToken(w, r) == nil {
		return
	}

	user := s.getUserByID(w, r)
	if user == nil {
		return
	}

	var req userNamespaceGrantRequest
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
		UserID:      user.ID,
		NamespaceID: req.NamespaceID,
		AllowBits:   allowBits,
		DenyBits:    denyBits,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.store.UpsertNamespaceGrant(grant); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to create grant")
		return
	}

	grants, err := s.store.ListUserNamespaceGrants(user.ID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list grants")
		return
	}

	resp := make([]namespaceGrantAPIResponse, len(grants))
	for i, g := range grants {
		resp[i] = namespaceGrantToResponse(g)
	}

	JSON(w, http.StatusOK, resp)
}

func (s *Server) handleAdminListUserNamespaceGrants(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminToken(w, r) == nil {
		return
	}

	user := s.getUserByID(w, r)
	if user == nil {
		return
	}

	grants, err := s.store.ListUserNamespaceGrants(user.ID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list grants")
		return
	}

	resp := make([]namespaceGrantAPIResponse, len(grants))
	for i, g := range grants {
		resp[i] = namespaceGrantToResponse(g)
	}

	JSON(w, http.StatusOK, resp)
}

func (s *Server) handleAdminGetUserNamespaceGrant(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminToken(w, r) == nil {
		return
	}

	user := s.getUserByID(w, r)
	if user == nil {
		return
	}
	nsID := chi.URLParam(r, "nsID")

	grant, err := s.store.GetNamespaceGrant(user.ID, nsID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get grant")
		return
	}
	if grant == nil {
		JSONError(w, http.StatusNotFound, "Grant not found")
		return
	}

	JSON(w, http.StatusOK, namespaceGrantToResponse(*grant))
}

func (s *Server) handleAdminDeleteUserNamespaceGrant(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminToken(w, r) == nil {
		return
	}

	user := s.getUserByID(w, r)
	if user == nil {
		return
	}
	nsID := chi.URLParam(r, "nsID")

	grant, err := s.store.GetNamespaceGrant(user.ID, nsID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check grant")
		return
	}
	if grant == nil {
		JSONError(w, http.StatusNotFound, "Grant not found")
		return
	}

	if err := s.store.DeleteNamespaceGrant(user.ID, nsID); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to delete grant")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type userRepoGrantRequest struct {
	RepoID string   `json:"repo_id"`
	Allow  []string `json:"allow"`
	Deny   []string `json:"deny,omitempty"`
}

func (s *Server) handleAdminCreateUserRepoGrant(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminToken(w, r) == nil {
		return
	}

	user := s.getUserByID(w, r)
	if user == nil {
		return
	}

	var req userRepoGrantRequest
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
		UserID:    user.ID,
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

	grants, err := s.store.ListUserRepoGrants(user.ID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list grants")
		return
	}

	resp := make([]repoGrantAPIResponse, len(grants))
	for i, g := range grants {
		resp[i] = repoGrantToResponse(g)
	}

	JSON(w, http.StatusOK, resp)
}

func (s *Server) handleAdminListUserRepoGrants(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminToken(w, r) == nil {
		return
	}

	user := s.getUserByID(w, r)
	if user == nil {
		return
	}

	grants, err := s.store.ListUserRepoGrants(user.ID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to list grants")
		return
	}

	resp := make([]repoGrantAPIResponse, len(grants))
	for i, g := range grants {
		resp[i] = repoGrantToResponse(g)
	}

	JSON(w, http.StatusOK, resp)
}

func (s *Server) handleAdminGetUserRepoGrant(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminToken(w, r) == nil {
		return
	}

	user := s.getUserByID(w, r)
	if user == nil {
		return
	}
	repoID := chi.URLParam(r, "repoID")

	grant, err := s.store.GetRepoGrant(user.ID, repoID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get grant")
		return
	}
	if grant == nil {
		JSONError(w, http.StatusNotFound, "Grant not found")
		return
	}

	JSON(w, http.StatusOK, repoGrantToResponse(*grant))
}

func (s *Server) handleAdminDeleteUserRepoGrant(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminToken(w, r) == nil {
		return
	}

	user := s.getUserByID(w, r)
	if user == nil {
		return
	}
	repoID := chi.URLParam(r, "repoID")

	grant, err := s.store.GetRepoGrant(user.ID, repoID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check grant")
		return
	}
	if grant == nil {
		JSONError(w, http.StatusNotFound, "Grant not found")
		return
	}

	if err := s.store.DeleteRepoGrant(user.ID, repoID); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to delete grant")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
