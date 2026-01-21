package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"ephemeral/internal/client"
	"ephemeral/internal/config"
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

type exchangeRequest struct {
	Code         string `json:"code"`
	CodeVerifier string `json:"code_verifier"`
}

type exchangeResponse struct {
	Data struct {
		Token     string `json:"token"`
		Namespace string `json:"namespace"`
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
	sessionID, err := generateSessionID()
	if err != nil {
		return fmt.Errorf("generate session: %w", err)
	}

	codeVerifier, err := generateCodeVerifier()
	if err != nil {
		return fmt.Errorf("generate verifier: %w", err)
	}

	codeChallenge := generateCodeChallenge(codeVerifier)

	authURL := fmt.Sprintf("%s/%s?code_challenge=%s", webAuthURL, sessionID, codeChallenge)

	fmt.Println()
	fmt.Println("Open this URL to authenticate:")
	fmt.Printf("  %s\n", authURL)
	fmt.Println()
	fmt.Print("Enter code: ")

	code, err := readLine()
	if err != nil {
		return fmt.Errorf("read code: %w", err)
	}

	if code == "" {
		return fmt.Errorf("code cannot be empty")
	}

	token, exchangeNamespace, err := exchangeCode(serverURL, code, codeVerifier)
	if err != nil {
		return err
	}

	c := client.New(serverURL, token)
	namespaces, err := c.ListNamespaces()
	if err != nil {
		return fmt.Errorf("validate token: %w", err)
	}

	if len(namespaces) == 0 {
		return fmt.Errorf("token has no namespace access")
	}

	defaultNs := exchangeNamespace
	nsFound := false
	for _, ns := range namespaces {
		if ns.Name == exchangeNamespace {
			nsFound = true
			break
		}
	}

	if !nsFound {
		for _, ns := range namespaces {
			if ns.IsPrimary {
				defaultNs = ns.Name
				break
			}
		}
		if defaultNs == exchangeNamespace {
			defaultNs = namespaces[0].Name
		}
	}

	return saveLoginAndConfigure(serverURL, token, defaultNs, len(namespaces))
}

func exchangeCode(serverURL, code, codeVerifier string) (string, string, error) {
	reqBody := exchangeRequest{
		Code:         code,
		CodeVerifier: codeVerifier,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", "", fmt.Errorf("marshal request: %w", err)
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Post(
		serverURL+"/api/v1/auth/exchange",
		"application/json",
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return "", "", fmt.Errorf("exchange request: %w", err)
	}
	defer resp.Body.Close()

	var exchangeResp exchangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&exchangeResp); err != nil {
		return "", "", fmt.Errorf("decode response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if exchangeResp.Error != nil {
			return "", "", fmt.Errorf("exchange failed (%s): %s", exchangeResp.Error.Code, exchangeResp.Error.Message)
		}
		return "", "", fmt.Errorf("exchange failed: status %d", resp.StatusCode)
	}

	return exchangeResp.Data.Token, exchangeResp.Data.Namespace, nil
}

func completeLogin(serverURL, token string) error {
	c := client.New(serverURL, token)
	namespaces, err := c.ListNamespaces()
	if err != nil {
		return fmt.Errorf("validate token: %w", err)
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
