package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bantamhq/ephemeral/internal/client"
	"github.com/bantamhq/ephemeral/internal/config"
)

func newNamespaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "namespace",
		Short: "Manage namespaces",
		Long:  `List and switch between namespaces.`,
		RunE:  runNamespaceList,
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "use <name>",
			Short: "Switch to a different namespace",
			Args:  cobra.ExactArgs(1),
			RunE:  runNamespaceUse,
		},
	)

	return cmd
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

	namespaces, err := c.ListNamespaces()
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

func runNamespaceUse(cmd *cobra.Command, args []string) error {
	name := args[0]

	cfg, err := config.Load()
	if err != nil {
		return errNotLoggedIn
	}

	if !cfg.IsConfigured() {
		return errNotLoggedIn
	}

	c := client.New(cfg.Server, cfg.Token)

	namespaces, err := c.ListNamespaces()
	if err != nil {
		return formatAPIError("list namespaces", err)
	}

	if !hasNamespace(namespaces, name) {
		return fmt.Errorf("namespace %q not found or no access", name)
	}

	cfg.DefaultNamespace = name

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("Default namespace set to %q\n", name)
	return nil
}

func hasNamespace(namespaces []client.NamespaceWithAccess, name string) bool {
	for _, ns := range namespaces {
		if ns.Name == name {
			return true
		}
	}
	return false
}
