package main

import (
	"bytes"
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
	Data struct {
		AuthMethods []string `json:"auth_methods"`
		WebAuthURL  string   `json:"web_auth_url,omitempty"`
	} `json:"data"`
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

	fmt.Printf("Connecting to %s...\n", serverURL)

	authConfig, err := fetchAuthConfig(serverURL)
	if err != nil {
		return loginWithToken(serverURL)
	}

	if containsWebAuth(authConfig.Data.AuthMethods) && authConfig.Data.WebAuthURL != "" {
		return loginWithWebAuth(serverURL, authConfig.Data.WebAuthURL)
	}

	return loginWithToken(serverURL)
}

func fetchAuthConfig(serverURL string) (*authConfigResponse, error) {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Get(serverURL + "/api/v1/auth/config")
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

func containsWebAuth(methods []string) bool {
	for _, m := range methods {
		if m == "web_auth" {
			return true
		}
	}
	return false
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

	return completeLogin(serverURL, token)
}

func loginWithWebAuth(serverURL, webAuthURL string) error {
	sessionID, err := createAuthSession(serverURL)
	if err != nil {
		return fmt.Errorf("create auth session: %w", err)
	}

	authURL := fmt.Sprintf("%s?session=%s&server=%s",
		webAuthURL, sessionID, url.QueryEscape(serverURL))

	fmt.Println()
	fmt.Println("Open this URL to authenticate:")
	fmt.Printf("  %s\n", authURL)
	fmt.Println()
	fmt.Println("Waiting for authentication...")

	token, err := pollForToken(serverURL, sessionID, 5*time.Minute)
	if err != nil {
		return err
	}

	return completeLogin(serverURL, token)
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

func completeLogin(serverURL, token string) error {
	c := client.New(serverURL, token)
	namespaces, err := c.ListNamespaces()
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

	return saveLoginAndConfigure(serverURL, token, primaryNs, len(namespaces))
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

func saveLoginAndConfigure(serverURL, token, namespace string, namespaceCount int) error {
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

	fmt.Printf("Logged in to %s\n", serverURL)
	fmt.Printf("Default namespace: %s\n", namespace)
	if namespaceCount > 1 {
		fmt.Printf("You have access to %d namespaces. Use 'eph namespace' to list them.\n", namespaceCount)
	}

	return nil
}
