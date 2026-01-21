package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"ephemeral/internal/config"
)

func newCredentialCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "credential",
		Short:  "Git credential helper commands",
		Hidden: true,
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "get",
			Short: "Get credentials for git",
			RunE:  runCredentialGet,
		},
		&cobra.Command{
			Use:   "store",
			Short: "Store credentials (no-op)",
			RunE:  runCredentialStore,
		},
		&cobra.Command{
			Use:   "erase",
			Short: "Erase credentials (no-op)",
			RunE:  runCredentialErase,
		},
	)

	return cmd
}

func runCredentialGet(cmd *cobra.Command, args []string) error {
	input := parseCredentialInput(os.Stdin)

	host := input["host"]
	if host == "" {
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return nil
	}

	for _, ctx := range cfg.Contexts {
		if hostMatches(ctx.Server, host) {
			fmt.Printf("username=x-token\n")
			fmt.Printf("password=%s\n", ctx.Token)
			return nil
		}
	}

	return nil
}

func runCredentialStore(cmd *cobra.Command, args []string) error {
	return nil
}

func runCredentialErase(cmd *cobra.Command, args []string) error {
	return nil
}
