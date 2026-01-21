package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"ephemeral/internal/config"
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
		return fmt.Errorf("load config: %w", err)
	}

	if cfg.CurrentContext == "" {
		return fmt.Errorf("no current context set")
	}

	ctx, ok := cfg.Contexts[cfg.CurrentContext]
	if !ok {
		return fmt.Errorf("current context %q not found", cfg.CurrentContext)
	}

	serverURL := ctx.Server
	contextName := cfg.CurrentContext

	delete(cfg.Contexts, cfg.CurrentContext)

	cfg.CurrentContext = ""
	for name := range cfg.Contexts {
		cfg.CurrentContext = name
		break
	}

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	if err := unconfigureGitHelper(serverURL); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to remove git credential helper: %v\n", err)
	}

	fmt.Printf("Logged out of %q\n", contextName)
	if cfg.CurrentContext != "" {
		fmt.Printf("Switched to context %q\n", cfg.CurrentContext)
	}

	return nil
}
