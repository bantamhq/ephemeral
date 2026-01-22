package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/bantamhq/ephemeral/internal/config"
)

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the current context",
		Long:  `Remove the current context and its credentials from the configuration.`,
		RunE:  runLogout,
	}
}

func runLogout(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("not logged in")
	}

	if !cfg.IsConfigured() {
		return fmt.Errorf("not logged in")
	}

	serverURL := cfg.Server

	if err := config.Delete(); err != nil {
		return fmt.Errorf("delete config: %w", err)
	}

	if err := unconfigureGitHelper(serverURL); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to remove git credential helper: %v\n", err)
	}

	fmt.Printf("Logged out of %s\n", serverURL)

	return nil
}
