package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/bantamhq/ephemeral/internal/store"
)

func generateSessionID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// AuthConfigResponse describes available authentication methods.
type AuthConfigResponse struct {
	AuthMethods []string `json:"auth_methods"`
	WebAuthURL  string   `json:"web_auth_url,omitempty"`
}

func (s *Server) handleAuthConfig(w http.ResponseWriter, r *http.Request) {
	methods := []string{"token"}

	webAuthEnabled := s.authOpts.WebAuthURL != ""
	if webAuthEnabled {
		methods = append(methods, "web_auth")
	}

	response := AuthConfigResponse{
		AuthMethods: methods,
	}
	if webAuthEnabled {
		response.WebAuthURL = s.authOpts.WebAuthURL
	}

	JSON(w, http.StatusOK, response)
}

type createAuthSessionRequest struct {
	ExpiresInSeconds int `json:"expires_in_seconds"`
}

type authSessionResponse struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Token     string    `json:"token,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Server) handleCreateAuthSession(w http.ResponseWriter, r *http.Request) {
	var req createAuthSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	expiresIn := req.ExpiresInSeconds
	if expiresIn <= 0 || expiresIn > 600 {
		expiresIn = 300
	}

	sessionID, err := generateSessionID()
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to generate session ID")
		return
	}

	now := time.Now()
	session := &store.AuthSession{
		ID:        sessionID,
		Status:    "pending",
		CreatedAt: now,
		ExpiresAt: now.Add(time.Duration(expiresIn) * time.Second),
	}

	if err := s.store.CreateAuthSession(session); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to create auth session")
		return
	}

	JSON(w, http.StatusCreated, authSessionResponse{
		ID:        session.ID,
		Status:    session.Status,
		ExpiresAt: session.ExpiresAt,
	})
}

func (s *Server) handleGetAuthSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")

	session, err := s.store.GetAuthSession(sessionID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get auth session")
		return
	}
	if session == nil {
		JSONError(w, http.StatusNotFound, "Session not found")
		return
	}

	if time.Now().After(session.ExpiresAt) {
		s.store.DeleteAuthSession(sessionID)
		JSONError(w, http.StatusNotFound, "Session expired")
		return
	}

	resp := authSessionResponse{
		ID:        session.ID,
		Status:    session.Status,
		ExpiresAt: session.ExpiresAt,
	}

	if session.Status == "completed" && session.Token != nil {
		resp.Token = *session.Token
		if err := s.store.DeleteAuthSession(sessionID); err != nil {
			fmt.Printf("Warning: failed to delete completed auth session: %v\n", err)
		}
	}

	JSON(w, http.StatusOK, resp)
}

type completeAuthSessionRequest struct {
	UserID string `json:"user_id"`
}

func (s *Server) handleCompleteAuthSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")

	var req completeAuthSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.UserID == "" {
		JSONError(w, http.StatusBadRequest, "user_id is required")
		return
	}

	session, err := s.store.GetAuthSession(sessionID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get auth session")
		return
	}
	if session == nil {
		JSONError(w, http.StatusNotFound, "Session not found")
		return
	}

	if time.Now().After(session.ExpiresAt) {
		s.store.DeleteAuthSession(sessionID)
		JSONError(w, http.StatusNotFound, "Session expired")
		return
	}

	if session.Status != "pending" {
		JSONError(w, http.StatusConflict, "Session already completed")
		return
	}

	user, err := s.store.GetUser(req.UserID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get user")
		return
	}
	if user == nil {
		JSONError(w, http.StatusNotFound, "User not found")
		return
	}

	rawToken, _, err := s.store.GenerateUserToken(req.UserID, nil)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	if err := s.store.CompleteAuthSession(sessionID, req.UserID, rawToken); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to complete auth session")
		return
	}

	JSON(w, http.StatusOK, authSessionResponse{
		ID:        sessionID,
		Status:    "completed",
		ExpiresAt: session.ExpiresAt,
	})
}
