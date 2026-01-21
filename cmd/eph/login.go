package main

import (
	"fmt"
	"os"
	"strings"

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

func runLogin(cmd *cobra.Command, args []string) error {
	serverURL := "http://localhost:8080"
	if len(args) > 0 {
		serverURL = args[0]
	}

	if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
		serverURL = "http://" + serverURL
	}

	fmt.Printf("Authenticating with %s\n", serverURL)
	fmt.Print("Token: ")

	token, err := readToken()
	if err != nil {
		return fmt.Errorf("read token: %w", err)
	}
	fmt.Println()
	if token == "" {
		return fmt.Errorf("token cannot be empty")
	}

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

	cfg, err := config.Load()
	if err != nil {
		cfg = &config.ClientConfig{
			Contexts: make(map[string]config.Context),
		}
	}

	contextName := generateContextName(serverURL)
	cfg.Contexts[contextName] = config.Context{
		Server:    serverURL,
		Token:     token,
		Namespace: primaryNs,
	}
	cfg.CurrentContext = contextName

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	if err := configureGitHelper(serverURL); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to configure git credential helper: %v\n", err)
	}

	fmt.Printf("Logged in to %s as namespace %q\n", serverURL, primaryNs)
	fmt.Printf("Context %q saved and set as current.\n", contextName)
	fmt.Printf("Git credential helper configured for %s\n", serverURL)
	if len(namespaces) > 1 {
		fmt.Printf("You have access to %d namespaces. Use 'eph namespace' to list them.\n", len(namespaces))
	}

	return nil
}
