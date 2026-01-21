package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"ephemeral/internal/client"
)

const adminTokenEnv = "EPH_ADMIN_TOKEN"

func newAdminCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Admin operations",
		Long: `Administrative operations for managing namespaces and tokens.

Token is resolved in order:
  1. --token flag
  2. EPH_ADMIN_TOKEN environment variable
  3. data/admin-token file (for local server)

The admin token is generated when the server starts for the first time.`,
	}

	cmd.PersistentFlags().String("server", "http://localhost:8080", "Server URL")
	cmd.PersistentFlags().String("token", "", "Admin token (overrides env and file)")
	cmd.PersistentFlags().String("data-dir", "./data", "Data directory (for token file discovery)")

	cmd.AddCommand(
		newAdminNamespaceCmd(),
		newAdminTokenCmd(),
	)

	return cmd
}

func getAdminClient(cmd *cobra.Command) (*client.Client, error) {
	token, _ := cmd.Flags().GetString("token")

	if token == "" {
		token = os.Getenv(adminTokenEnv)
	}

	if token == "" {
		dataDir, _ := cmd.Flags().GetString("data-dir")
		tokenPath := dataDir + "/admin-token"
		if data, err := os.ReadFile(tokenPath); err == nil {
			token = strings.TrimSpace(string(data))
		}
	}

	if token == "" {
		return nil, fmt.Errorf("admin token required: use --token, set %s, or ensure data/admin-token exists", adminTokenEnv)
	}

	server, _ := cmd.Flags().GetString("server")
	if !strings.HasPrefix(server, "http://") && !strings.HasPrefix(server, "https://") {
		server = "http://" + server
	}

	return client.New(server, token), nil
}

func newAdminNamespaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "namespace",
		Short: "Manage namespaces",
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List all namespaces",
			RunE:  runAdminNamespaceList,
		},
		&cobra.Command{
			Use:   "create <name>",
			Short: "Create a namespace",
			Args:  cobra.ExactArgs(1),
			RunE:  runAdminNamespaceCreate,
		},
		&cobra.Command{
			Use:   "delete <id>",
			Short: "Delete a namespace",
			Args:  cobra.ExactArgs(1),
			RunE:  runAdminNamespaceDelete,
		},
	)

	return cmd
}

func runAdminNamespaceList(cmd *cobra.Command, args []string) error {
	c, err := getAdminClient(cmd)
	if err != nil {
		return err
	}

	namespaces, err := c.AdminListNamespaces()
	if err != nil {
		return fmt.Errorf("list namespaces: %w", err)
	}

	if len(namespaces) == 0 {
		fmt.Println("No namespaces.")
		return nil
	}

	fmt.Printf("%-36s  %s\n", "ID", "NAME")
	for _, ns := range namespaces {
		fmt.Printf("%-36s  %s\n", ns.ID, ns.Name)
	}

	return nil
}

func runAdminNamespaceCreate(cmd *cobra.Command, args []string) error {
	c, err := getAdminClient(cmd)
	if err != nil {
		return err
	}

	ns, err := c.AdminCreateNamespace(args[0])
	if err != nil {
		return fmt.Errorf("create namespace: %w", err)
	}

	fmt.Printf("Created namespace %q (ID: %s)\n", ns.Name, ns.ID)
	return nil
}

func runAdminNamespaceDelete(cmd *cobra.Command, args []string) error {
	c, err := getAdminClient(cmd)
	if err != nil {
		return err
	}

	if err := c.AdminDeleteNamespace(args[0]); err != nil {
		return fmt.Errorf("delete namespace: %w", err)
	}

	fmt.Println("Namespace deleted.")
	return nil
}

func newAdminTokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Manage tokens",
	}

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a user token",
		RunE:  runAdminTokenCreate,
	}
	createCmd.Flags().String("namespace", "", "Namespace name or ID (required)")
	createCmd.Flags().String("name", "", "Token name/label")
	createCmd.Flags().String("scope", "full", "Token scope (full, repos, or read-only)")
	createCmd.MarkFlagRequired("namespace")

	cmd.AddCommand(createCmd)

	return cmd
}

func runAdminTokenCreate(cmd *cobra.Command, args []string) error {
	c, err := getAdminClient(cmd)
	if err != nil {
		return err
	}

	nsFlag, _ := cmd.Flags().GetString("namespace")
	nameFlag, _ := cmd.Flags().GetString("name")
	scope, _ := cmd.Flags().GetString("scope")

	namespaces, err := c.AdminListNamespaces()
	if err != nil {
		return fmt.Errorf("list namespaces: %w", err)
	}

	var namespaceID string
	for _, ns := range namespaces {
		if ns.ID == nsFlag || ns.Name == nsFlag {
			namespaceID = ns.ID
			break
		}
	}

	if namespaceID == "" {
		return fmt.Errorf("namespace %q not found", nsFlag)
	}

	var name *string
	if nameFlag != "" {
		name = &nameFlag
	}

	token, err := c.AdminCreateToken(namespaceID, name, scope)
	if err != nil {
		return fmt.Errorf("create token: %w", err)
	}

	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("TOKEN CREATED (save this, it won't be shown again):")
	fmt.Println(token.Token)
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()
	fmt.Printf("ID: %s\n", token.ID)
	fmt.Printf("Scope: %s\n", token.Scope)
	if token.Name != nil {
		fmt.Printf("Name: %s\n", *token.Name)
	}

	return nil
}
