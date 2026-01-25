package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bantamhq/ephemeral/internal/client"
	"github.com/bantamhq/ephemeral/internal/config"
)

func newNamespacesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "namespaces",
		Short: "List accessible namespaces",
		RunE:  runNamespaceList,
	}
}

func runNamespaceList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return errNotLoggedIn
	}

	if !cfg.IsConfigured() {
		return errNotLoggedIn
	}

	c := client.New(cfg.Server, cfg.Token)

	namespaces, err := c.ListNamespaces(context.Background())
	if err != nil {
		return formatAPIError("list namespaces", err)
	}

	if len(namespaces) == 0 {
		fmt.Println("No namespaces available.")
		return nil
	}

	for _, ns := range namespaces {
		marker := "  "
		if ns.IsPrimary {
			marker = "* "
		}
		if cfg.DefaultNamespace == ns.Name {
			marker = "> "
		}
		fmt.Printf("%s%s\n", marker, ns.Name)
	}

	fmt.Println()
	fmt.Println("* = primary namespace")
	fmt.Println("> = default namespace")

	return nil
}
