package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"ephemeral/internal/client"
	"ephemeral/internal/config"
)

func newWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show current context info",
		Long:  `Display the current server, namespace, and accessible namespaces.`,
		Args:  cobra.NoArgs,
		RunE:  runWhoami,
	}
}

func runWhoami(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return errNotLoggedIn
	}

	if !cfg.IsConfigured() {
		return errNotLoggedIn
	}

	fmt.Printf("Server:    %s\n", cfg.Server)
	fmt.Printf("Namespace: %s\n", cfg.DefaultNamespace)

	c := client.New(cfg.Server, cfg.Token)
	namespaces, err := c.ListNamespaces()
	if err != nil {
		return formatAPIError("list namespaces", err)
	}

	if len(namespaces) > 1 {
		fmt.Printf("\nAccessible namespaces:\n")
		for _, ns := range namespaces {
			marker := "  "
			if ns.IsPrimary {
				marker = "* "
			}
			if cfg.DefaultNamespace == ns.Name {
				marker = "> "
			}
			fmt.Printf("  %s%s\n", marker, ns.Name)
		}
		fmt.Println()
		fmt.Println("  * = primary, > = current")
	}

	return nil
}
