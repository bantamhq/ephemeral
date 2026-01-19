package server

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"ephemeral/internal/store"
)

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
	Name      string `json:"name"`
	Scope     string `json:"scope"`
	ExpiresIn *int   `json:"expires_in_seconds,omitempty"`
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
