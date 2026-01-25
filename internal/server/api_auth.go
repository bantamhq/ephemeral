package server

import (
	"net/http"
)

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
