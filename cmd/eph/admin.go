package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bantamhq/ephemeral/internal/client"
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
		return formatAPIError("list namespaces", err)
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
		return formatAPIError("create namespace", err)
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
		return formatAPIError("delete namespace", err)
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
	createCmd.MarkFlagRequired("namespace")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all tokens",
		RunE:  runAdminTokenList,
	}

	showCmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show token details with grants",
		Args:  cobra.ExactArgs(1),
		RunE:  runAdminTokenShow,
	}

	deleteCmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a token",
		Args:  cobra.ExactArgs(1),
		RunE:  runAdminTokenDelete,
	}

	grantCmd := &cobra.Command{
		Use:   "grant",
		Short: "Grant permissions to a token",
	}

	grantNSCmd := &cobra.Command{
		Use:   "namespace <token-id> <namespace>",
		Short: "Grant namespace permissions to a token",
		Args:  cobra.ExactArgs(2),
		RunE:  runAdminTokenGrantNamespace,
	}
	grantNSCmd.Flags().StringSlice("allow", []string{"namespace:read"}, "Permissions to allow")
	grantNSCmd.Flags().StringSlice("deny", nil, "Permissions to deny")
	grantNSCmd.Flags().Bool("primary", false, "Mark as primary namespace")

	grantRepoCmd := &cobra.Command{
		Use:   "repo <token-id> <repo>",
		Short: "Grant repo permissions to a token",
		Long: `Grant repo permissions to a token.

The repo can be specified as:
  - A repo ID (UUID)
  - A namespace/repo-name path (e.g., "myns/myrepo")`,
		Args: cobra.ExactArgs(2),
		RunE: runAdminTokenGrantRepo,
	}
	grantRepoCmd.Flags().StringSlice("allow", []string{"repo:read"}, "Permissions to allow")
	grantRepoCmd.Flags().StringSlice("deny", nil, "Permissions to deny")

	grantCmd.AddCommand(grantNSCmd, grantRepoCmd)

	revokeCmd := &cobra.Command{
		Use:   "revoke",
		Short: "Revoke permissions from a token",
	}

	revokeNSCmd := &cobra.Command{
		Use:   "namespace <token-id> <namespace>",
		Short: "Revoke namespace permissions from a token",
		Args:  cobra.ExactArgs(2),
		RunE:  runAdminTokenRevokeNamespace,
	}

	revokeRepoCmd := &cobra.Command{
		Use:   "repo <token-id> <repo>",
		Short: "Revoke repo permissions from a token",
		Long: `Revoke repo permissions from a token.

The repo can be specified as:
  - A repo ID (UUID)
  - A namespace/repo-name path (e.g., "myns/myrepo")`,
		Args: cobra.ExactArgs(2),
		RunE: runAdminTokenRevokeRepo,
	}

	revokeCmd.AddCommand(revokeNSCmd, revokeRepoCmd)

	cmd.AddCommand(createCmd, listCmd, showCmd, deleteCmd, grantCmd, revokeCmd)

	return cmd
}

func runAdminTokenCreate(cmd *cobra.Command, args []string) error {
	c, err := getAdminClient(cmd)
	if err != nil {
		return err
	}

	nsFlag, _ := cmd.Flags().GetString("namespace")
	nameFlag, _ := cmd.Flags().GetString("name")

	namespaceID, err := resolveNamespaceID(c, nsFlag)
	if err != nil {
		return err
	}

	var name *string
	if nameFlag != "" {
		name = &nameFlag
	}

	token, err := c.AdminCreateToken(namespaceID, name)
	if err != nil {
		return formatAPIError("create token", err)
	}

	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("TOKEN CREATED (save this, it won't be shown again):")
	fmt.Println(token.Token)
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()
	fmt.Printf("ID: %s\n", token.ID)
	fmt.Println("Permissions: namespace:write, repo:admin")
	if token.Name != nil {
		fmt.Printf("Name: %s\n", *token.Name)
	}

	return nil
}

func runAdminTokenList(cmd *cobra.Command, args []string) error {
	c, err := getAdminClient(cmd)
	if err != nil {
		return err
	}

	tokens, err := c.AdminListTokens()
	if err != nil {
		return formatAPIError("list tokens", err)
	}

	if len(tokens) == 0 {
		fmt.Println("No tokens.")
		return nil
	}

	fmt.Printf("%-36s  %-20s  %-6s  %s\n", "ID", "NAME", "TYPE", "CREATED")
	for _, t := range tokens {
		name := "(unnamed)"
		if t.Name != nil {
			name = *t.Name
		}

		tokenType := "user"
		if t.IsAdmin {
			tokenType = "admin"
		}

		fmt.Printf("%-36s  %-20s  %-6s  %s\n", t.ID, name, tokenType, t.CreatedAt.Format("2006-01-02"))
	}

	return nil
}

func runAdminTokenDelete(cmd *cobra.Command, args []string) error {
	c, err := getAdminClient(cmd)
	if err != nil {
		return err
	}

	if err := c.AdminDeleteToken(args[0]); err != nil {
		return formatAPIError("delete token", err)
	}

	fmt.Println("Token deleted.")
	return nil
}

func runAdminTokenShow(cmd *cobra.Command, args []string) error {
	c, err := getAdminClient(cmd)
	if err != nil {
		return err
	}

	token, err := c.AdminGetToken(args[0])
	if err != nil {
		return formatAPIError("get token", err)
	}

	name := "(unnamed)"
	if token.Name != nil {
		name = *token.Name
	}

	tokenType := "user"
	if token.IsAdmin {
		tokenType = "admin"
	}

	fmt.Printf("Token: %s\n", token.ID)
	fmt.Printf("Name:  %s\n", name)
	fmt.Printf("Type:  %s\n", tokenType)
	fmt.Printf("Created: %s\n", token.CreatedAt.Format("2006-01-02"))
	if token.ExpiresAt != nil {
		fmt.Printf("Expires: %s\n", token.ExpiresAt.Format("2006-01-02"))
	}
	if token.LastUsedAt != nil {
		fmt.Printf("Last Used: %s\n", token.LastUsedAt.Format("2006-01-02 15:04"))
	}

	if !token.IsAdmin {
		fmt.Println()
		if len(token.NamespaceGrants) > 0 {
			fmt.Println("Namespace Grants:")
			fmt.Printf("  %-36s  %-40s  %s\n", "NAMESPACE", "ALLOW", "PRIMARY")
			for _, g := range token.NamespaceGrants {
				allow := strings.Join(g.Allow, ", ")
				primary := ""
				if g.IsPrimary {
					primary = "yes"
				}
				fmt.Printf("  %-36s  %-40s  %s\n", g.NamespaceID, allow, primary)
			}
		}

		if len(token.RepoGrants) > 0 {
			fmt.Println()
			fmt.Println("Repo Grants:")
			fmt.Printf("  %-36s  %s\n", "REPO", "ALLOW")
			for _, g := range token.RepoGrants {
				allow := strings.Join(g.Allow, ", ")
				fmt.Printf("  %-36s  %s\n", g.RepoID, allow)
			}
		}
	}

	return nil
}

func resolveNamespaceID(c *client.Client, nsArg string) (string, error) {
	namespaces, err := c.AdminListNamespaces()
	if err != nil {
		return "", fmt.Errorf("list namespaces: %w", err)
	}

	for _, ns := range namespaces {
		if ns.ID == nsArg || ns.Name == nsArg {
			return ns.ID, nil
		}
	}

	return "", fmt.Errorf("namespace %q not found", nsArg)
}

func resolveRepoID(c *client.Client, repoArg string) (string, error) {
	if !strings.Contains(repoArg, "/") {
		return repoArg, nil
	}

	parts := strings.SplitN(repoArg, "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid repo format %q, expected namespace/repo", repoArg)
	}

	nsID, err := resolveNamespaceID(c, parts[0])
	if err != nil {
		return "", err
	}

	nsClient := c.WithNamespace(nsID)
	repos, _, err := nsClient.ListRepos("", 0)
	if err != nil {
		return "", fmt.Errorf("list repos: %w", err)
	}

	for _, repo := range repos {
		if repo.Name == parts[1] {
			return repo.ID, nil
		}
	}

	return "", fmt.Errorf("repo %q not found in namespace %q", parts[1], parts[0])
}

func runAdminTokenGrantNamespace(cmd *cobra.Command, args []string) error {
	c, err := getAdminClient(cmd)
	if err != nil {
		return err
	}

	tokenID := args[0]
	nsArg := args[1]

	nsID, err := resolveNamespaceID(c, nsArg)
	if err != nil {
		return err
	}

	allow, _ := cmd.Flags().GetStringSlice("allow")
	deny, _ := cmd.Flags().GetStringSlice("deny")
	isPrimary, _ := cmd.Flags().GetBool("primary")

	if err := c.AdminUpsertNamespaceGrant(tokenID, nsID, allow, deny, isPrimary); err != nil {
		return formatAPIError("grant namespace", err)
	}

	fmt.Printf("Granted namespace %q to token.\n", nsArg)
	return nil
}

func runAdminTokenGrantRepo(cmd *cobra.Command, args []string) error {
	c, err := getAdminClient(cmd)
	if err != nil {
		return err
	}

	tokenID := args[0]
	repoArg := args[1]

	repoID, err := resolveRepoID(c, repoArg)
	if err != nil {
		return err
	}

	allow, _ := cmd.Flags().GetStringSlice("allow")
	deny, _ := cmd.Flags().GetStringSlice("deny")

	if err := c.AdminUpsertRepoGrant(tokenID, repoID, allow, deny); err != nil {
		return formatAPIError("grant repo", err)
	}

	fmt.Printf("Granted repo %q to token.\n", repoArg)
	return nil
}

func runAdminTokenRevokeNamespace(cmd *cobra.Command, args []string) error {
	c, err := getAdminClient(cmd)
	if err != nil {
		return err
	}

	tokenID := args[0]
	nsArg := args[1]

	nsID, err := resolveNamespaceID(c, nsArg)
	if err != nil {
		return err
	}

	if err := c.AdminDeleteNamespaceGrant(tokenID, nsID); err != nil {
		return formatAPIError("revoke namespace", err)
	}

	fmt.Printf("Revoked namespace %q from token.\n", nsArg)
	return nil
}

func runAdminTokenRevokeRepo(cmd *cobra.Command, args []string) error {
	c, err := getAdminClient(cmd)
	if err != nil {
		return err
	}

	tokenID := args[0]
	repoArg := args[1]

	repoID, err := resolveRepoID(c, repoArg)
	if err != nil {
		return err
	}

	if err := c.AdminDeleteRepoGrant(tokenID, repoID); err != nil {
		return formatAPIError("revoke repo", err)
	}

	fmt.Printf("Revoked repo %q from token.\n", repoArg)
	return nil
}
