package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/bantamhq/ephemeral/internal/client"
	"github.com/bantamhq/ephemeral/internal/config"
)

func newLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login [server]",
		Short: "Authenticate with an Ephemeral server",
		Long: `Authenticate with an Ephemeral server and save the credentials.

If no server is specified, defaults to http://localhost:8080.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runLogin,
	}
}

type authConfigResponse struct {
	AuthMethod   string `json:"auth_method"`
	ServerURL    string `json:"server_url,omitempty"`
	AuthEndpoint string `json:"auth_endpoint,omitempty"`
}

type createSessionRequest struct {
	ExpiresInSeconds int `json:"expires_in_seconds"`
}

type authSessionResponse struct {
	Data struct {
		ID        string    `json:"id"`
		Status    string    `json:"status"`
		Token     string    `json:"token,omitempty"`
		ExpiresAt time.Time `json:"expires_at"`
	} `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func runLogin(cmd *cobra.Command, args []string) error {
	cfg, _ := config.Load()
	if cfg != nil && cfg.IsConfigured() {
		return fmt.Errorf("already logged in to %s. Run 'eph logout' first to switch servers", cfg.Server)
	}

	serverURL := "http://localhost:8080"
	if len(args) > 0 {
		serverURL = args[0]
	}

	if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
		serverURL = "http://" + serverURL
	}

	var authConfig *authConfigResponse
	var sessionID string
	var targetServer string

	err := runSpinner(
		"Connecting to "+serverURL+"...",
		"Connected to "+serverURL,
		func() error {
			var fetchErr error
			authConfig, fetchErr = fetchAuthConfig(serverURL)
			if fetchErr != nil {
				return fetchErr
			}

			targetServer = serverURL
			if authConfig.ServerURL != "" {
				targetServer = authConfig.ServerURL
				if !strings.HasPrefix(targetServer, "http://") && !strings.HasPrefix(targetServer, "https://") {
					targetServer = "https://" + targetServer
				}
			}

			if authConfig.AuthMethod == "web" && authConfig.AuthEndpoint != "" {
				sessionID, fetchErr = createAuthSession(serverURL)
				return fetchErr
			}

			return nil
		},
	)
	if err != nil {
		if isConnectionError(err) {
			return fmt.Errorf("could not connect to Ephemeral server at %s", serverURL)
		}
		return loginWithToken(serverURL)
	}

	if authConfig.AuthMethod == "web" && authConfig.AuthEndpoint != "" {
		return loginWithWebAuth(serverURL, targetServer, authConfig.AuthEndpoint, sessionID)
	}

	return loginWithToken(targetServer)
}

func fetchAuthConfig(serverURL string) (*authConfigResponse, error) {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Get(serverURL + "/.well-known/ephemeral-auth")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var config authConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

func loginWithToken(serverURL string) error {
	fmt.Print("Token: ")

	token, err := readToken()
	if err != nil {
		return fmt.Errorf("read token: %w", err)
	}
	fmt.Println()

	if token == "" {
		return fmt.Errorf("token cannot be empty")
	}

	return completeLogin(serverURL, serverURL, token)
}

func loginWithWebAuth(connectedServer, targetServer, authEndpoint, sessionID string) error {
	authURL := fmt.Sprintf("%s?session=%s&server=%s",
		authEndpoint, sessionID, url.QueryEscape(connectedServer))

	fmt.Println()
	fmt.Println("Open this URL to authenticate:")
	fmt.Printf("  %s\n", authURL)
	fmt.Println()

	var token string
	err := runSpinner("Waiting for authentication...", "Authenticated", func() error {
		var pollErr error
		token, pollErr = pollForToken(connectedServer, sessionID, 5*time.Minute)
		return pollErr
	})
	if err != nil {
		return err
	}

	return completeLogin(targetServer, connectedServer, token)
}

func createAuthSession(serverURL string) (string, error) {
	reqBody := createSessionRequest{
		ExpiresInSeconds: 300,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Post(
		serverURL+"/api/v1/auth/sessions",
		"application/json",
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return "", fmt.Errorf("create session request: %w", err)
	}
	defer resp.Body.Close()

	var sessionResp authSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sessionResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		if sessionResp.Error != nil {
			return "", fmt.Errorf("create session failed (%s): %s", sessionResp.Error.Code, sessionResp.Error.Message)
		}
		return "", fmt.Errorf("create session failed: status %d", resp.StatusCode)
	}

	return sessionResp.Data.ID, nil
}

func pollForToken(serverURL, sessionID string, timeout time.Duration) (string, error) {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	deadline := time.Now().Add(timeout)
	pollInterval := 2 * time.Second

	for time.Now().Before(deadline) {
		resp, err := httpClient.Get(serverURL + "/api/v1/auth/sessions/" + sessionID)
		if err != nil {
			time.Sleep(pollInterval)
			continue
		}

		var sessionResp authSessionResponse
		if err := json.NewDecoder(resp.Body).Decode(&sessionResp); err != nil {
			resp.Body.Close()
			time.Sleep(pollInterval)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("session expired or not found")
		}

		if resp.StatusCode != http.StatusOK {
			time.Sleep(pollInterval)
			continue
		}

		if sessionResp.Data.Status == "completed" && sessionResp.Data.Token != "" {
			return sessionResp.Data.Token, nil
		}

		time.Sleep(pollInterval)
	}

	return "", fmt.Errorf("authentication timed out")
}

func completeLogin(serverURL, displayURL, token string) error {
	c := client.New(serverURL, token)

	var namespaces []client.NamespaceWithAccess
	err := runSpinner("Validating token...", "Token validated", func() error {
		var validateErr error
		namespaces, validateErr = c.ListNamespaces(context.Background())
		return validateErr
	})
	if err != nil {
		return formatLoginError(err)
	}

	if len(namespaces) == 0 {
		return fmt.Errorf("token has no namespace access")
	}

	var primaryNs string
	for _, ns := range namespaces {
		if ns.IsPrimary {
			primaryNs = ns.Name
			break
		}
	}
	if primaryNs == "" {
		primaryNs = namespaces[0].Name
	}

	return saveLoginAndConfigure(serverURL, displayURL, token, primaryNs, len(namespaces))
}

func formatLoginError(err error) error {
	errStr := err.Error()

	if strings.Contains(errStr, "invalid header field value") {
		return errors.New("invalid token format")
	}

	if strings.Contains(errStr, "401") || strings.Contains(errStr, "unauthorized") {
		return errors.New("invalid or expired token")
	}

	return formatAPIError("authentication failed", err)
}

func saveLoginAndConfigure(serverURL, displayURL, token, namespace string, namespaceCount int) error {
	cfg := &config.ClientConfig{
		Server:           serverURL,
		Token:            token,
		DefaultNamespace: namespace,
	}

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	if err := configureGitHelper(serverURL); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to configure git credential helper: %v\n", err)
	}

	fmt.Printf("Logged in to %s\n", displayURL)
	fmt.Printf("Default namespace: %s\n", namespace)
	if namespaceCount > 1 {
		fmt.Printf("You have access to %d namespaces. Use 'eph namespace' to list them.\n", namespaceCount)
	}

	return nil
}
