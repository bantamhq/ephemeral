package main

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/bantamhq/ephemeral/internal/store"
)

func newServeNamespaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "namespace",
		Short: "Manage server namespaces",
	}

	cmd.AddCommand(
		newServeNamespaceAddCmd(),
		newServeNamespaceListCmd(),
		newServeNamespaceDeleteCmd(),
	)

	return cmd
}

func newServeNamespaceAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <name>",
		Short: "Create a namespace and grant access to user tokens",
		Args:  cobra.ExactArgs(1),
		RunE:  runServeNamespaceAdd,
	}
}

func newServeNamespaceListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all namespaces",
		RunE:  runServeNamespaceList,
	}
}

func newServeNamespaceDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a namespace",
		Args:  cobra.ExactArgs(1),
		RunE:  runServeNamespaceDelete,
	}
}

func runServeNamespaceAdd(cmd *cobra.Command, args []string) error {
	name := args[0]

	cfg, _, err := loadConfig("server.toml")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	st, err := initStore(cfg.Storage.DataDir)
	if err != nil {
		return err
	}
	defer st.Close()

	// Check if namespace already exists
	existing, err := st.GetNamespaceByName(name)
	if err != nil {
		return fmt.Errorf("check namespace: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("namespace %q already exists", name)
	}

	// Create namespace
	ns := &store.Namespace{
		ID:        uuid.New().String(),
		Name:      name,
		CreatedAt: time.Now(),
	}
	if err := st.CreateNamespace(ns); err != nil {
		return fmt.Errorf("create namespace: %w", err)
	}

	// Find all non-admin tokens and grant access
	tokens, err := st.ListTokens("", 100)
	if err != nil {
		return fmt.Errorf("list tokens: %w", err)
	}

	grantedCount := 0
	for _, token := range tokens {
		if token.IsAdmin {
			continue
		}

		grant := &store.NamespaceGrant{
			TokenID:     token.ID,
			NamespaceID: ns.ID,
			AllowBits:   store.DefaultNamespaceGrant(),
			IsPrimary:   false,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		if err := st.UpsertNamespaceGrant(grant); err != nil {
			return fmt.Errorf("grant access to token %s: %w", token.ID, err)
		}
		grantedCount++
	}

	fmt.Printf("Created namespace %q\n", name)
	if grantedCount > 0 {
		fmt.Printf("Granted access to %d user token(s)\n", grantedCount)
	}

	return nil
}

func runServeNamespaceList(cmd *cobra.Command, args []string) error {
	cfg, _, err := loadConfig("server.toml")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	st, err := initStore(cfg.Storage.DataDir)
	if err != nil {
		return err
	}
	defer st.Close()

	namespaces, err := st.ListNamespaces("", 100)
	if err != nil {
		return fmt.Errorf("list namespaces: %w", err)
	}

	if len(namespaces) == 0 {
		fmt.Println("No namespaces found")
		return nil
	}

	for _, ns := range namespaces {
		fmt.Printf("%s\n", ns.Name)
	}

	return nil
}

func runServeNamespaceDelete(cmd *cobra.Command, args []string) error {
	name := args[0]

	cfg, _, err := loadConfig("server.toml")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	st, err := initStore(cfg.Storage.DataDir)
	if err != nil {
		return err
	}
	defer st.Close()

	ns, err := st.GetNamespaceByName(name)
	if err != nil {
		return fmt.Errorf("get namespace: %w", err)
	}
	if ns == nil {
		return fmt.Errorf("namespace %q not found", name)
	}

	if err := st.DeleteNamespace(ns.ID); err != nil {
		return fmt.Errorf("delete namespace: %w", err)
	}

	fmt.Printf("Deleted namespace %q\n", name)
	return nil
}
