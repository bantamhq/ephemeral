package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/bantamhq/ephemeral/internal/store"
)

// AuthConfigResponse describes available authentication methods.
type AuthConfigResponse struct {
	AuthMethods []string `json:"auth_methods"`
	WebAuthURL  string   `json:"web_auth_url,omitempty"`
}

// ExchangeRequest is the request body for token exchange.
type ExchangeRequest struct {
	Code         string `json:"code"`
	CodeVerifier string `json:"code_verifier"`
}

// ExchangeResponse is the response for successful token exchange.
type ExchangeResponse struct {
	Token     string   `json:"token"`
	Namespace string   `json:"namespace"`
	Allow     []string `json:"allow"`
	Deny      []string `json:"deny,omitempty"`
}

// ExchangeErrorResponse is the error response for token exchange.
type ExchangeErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// PlatformValidationRequest is sent to the platform to validate an exchange code.
type PlatformValidationRequest struct {
	Code         string `json:"code"`
	CodeVerifier string `json:"code_verifier"`
}

// PlatformValidationResponse is the response from the platform validation endpoint.
type PlatformValidationResponse struct {
	Valid       bool     `json:"valid"`
	NamespaceID string   `json:"namespace_id,omitempty"`
	Allow       []string `json:"allow,omitempty"`
	Deny        []string `json:"deny,omitempty"`
	UserID      string   `json:"user_id,omitempty"`
	Error       string   `json:"error,omitempty"`
}

func (s *Server) handleAuthConfig(w http.ResponseWriter, r *http.Request) {
	methods := []string{"token"}

	webAuthEnabled := s.authOpts.WebAuthURL != "" && s.authOpts.ExchangeValidationURL != ""
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

func (s *Server) handleAuthExchange(w http.ResponseWriter, r *http.Request) {
	var req ExchangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeExchangeError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}

	if req.Code == "" {
		writeExchangeError(w, http.StatusBadRequest, "invalid_request", "Code is required")
		return
	}

	if req.CodeVerifier == "" {
		writeExchangeError(w, http.StatusBadRequest, "invalid_request", "Code verifier is required")
		return
	}

	if s.authOpts.ExchangeValidationURL == "" {
		writeExchangeError(w, http.StatusNotImplemented, "not_configured", "Web authentication is not configured")
		return
	}

	validationResp, err := s.validateWithPlatform(req.Code, req.CodeVerifier)
	if err != nil {
		writeExchangeError(w, http.StatusBadGateway, "validation_failed", "Failed to validate with platform")
		return
	}

	if !validationResp.Valid {
		code := validationResp.Error
		if code == "" {
			code = "invalid_code"
		}
		writeExchangeError(w, http.StatusBadRequest, code, errorMessageForCode(code))
		return
	}

	ns, err := s.store.GetNamespace(validationResp.NamespaceID)
	if err != nil {
		writeExchangeError(w, http.StatusInternalServerError, "internal_error", "Failed to look up namespace")
		return
	}

	if ns == nil {
		writeExchangeError(w, http.StatusNotFound, "namespace_not_found", "Namespace does not exist on this server")
		return
	}

	// Use grants from platform response, or default to namespace:write + repo:admin
	allowPerms := validationResp.Allow
	denyPerms := validationResp.Deny
	if len(allowPerms) == 0 {
		allowPerms = []string{"namespace:write", "repo:admin"}
	}

	allowBits := store.PermissionsFromStrings(allowPerms)
	denyBits := store.PermissionsFromStrings(denyPerms)

	grant := store.NamespaceGrant{
		NamespaceID: ns.ID,
		AllowBits:   allowBits,
		DenyBits:    denyBits,
		IsPrimary:   true,
	}

	rawToken, _, err := s.store.GenerateUserTokenWithGrants(nil, nil, []store.NamespaceGrant{grant}, nil)
	if err != nil {
		writeExchangeError(w, http.StatusInternalServerError, "internal_error", "Failed to generate token")
		return
	}

	JSON(w, http.StatusOK, ExchangeResponse{
		Token:     rawToken,
		Namespace: ns.Name,
		Allow:     allowBits.ToStrings(),
		Deny:      denyBits.ToStrings(),
	})
}

func (s *Server) validateWithPlatform(code, codeVerifier string) (*PlatformValidationResponse, error) {
	reqBody := PlatformValidationRequest{
		Code:         code,
		CodeVerifier: codeVerifier,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, s.authOpts.ExchangeValidationURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if s.authOpts.ExchangeSecret != "" {
		req.Header.Set("Authorization", "Bearer "+s.authOpts.ExchangeSecret)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("platform returned status %d", resp.StatusCode)
	}

	var validationResp PlatformValidationResponse
	if err := json.NewDecoder(resp.Body).Decode(&validationResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &validationResp, nil
}

func writeExchangeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(struct {
		Error ExchangeErrorResponse `json:"error"`
	}{
		Error: ExchangeErrorResponse{
			Code:    code,
			Message: message,
		},
	})
}

func errorMessageForCode(code string) string {
	switch code {
	case "invalid_code", "code_expired":
		return "Code is invalid or has expired"
	case "code_used":
		return "Code has already been used"
	case "invalid_verifier":
		return "PKCE verification failed"
	default:
		return "Authentication failed"
	}
}
