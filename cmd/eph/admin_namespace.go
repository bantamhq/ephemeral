package main

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/bantamhq/ephemeral/internal/store"
)

func newAdminNamespaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "namespace",
		Short: "Manage namespaces",
	}

	cmd.AddCommand(
		newAdminNamespaceAddCmd(),
		newAdminNamespaceListCmd(),
		newAdminNamespaceDeleteCmd(),
		newAdminNamespaceGrantCmd(),
		newAdminNamespaceRevokeCmd(),
	)

	return cmd
}

func newAdminNamespaceAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <name>",
		Short: "Create a namespace",
		Args:  cobra.ExactArgs(1),
		RunE:  runAdminNamespaceAdd,
	}
}

func newAdminNamespaceListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all namespaces",
		RunE:  runAdminNamespaceList,
	}
}

func newAdminNamespaceDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a namespace",
		Args:  cobra.ExactArgs(1),
		RunE:  runAdminNamespaceDelete,
	}
}

func newAdminNamespaceGrantCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "grant <namespace> <username>",
		Short: "Grant a user access to a namespace",
		Args:  cobra.ExactArgs(2),
		RunE:  runAdminNamespaceGrant,
	}
}

func newAdminNamespaceRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <namespace> <username>",
		Short: "Revoke a user's access to a namespace",
		Args:  cobra.ExactArgs(2),
		RunE:  runAdminNamespaceRevoke,
	}
}

func runAdminNamespaceAdd(cmd *cobra.Command, args []string) error {
	name := args[0]

	if err := validateNamespaceName(name); err != nil {
		return fmt.Errorf("invalid namespace name: %w", err)
	}

	ctx, err := loadAdminContext()
	if err != nil {
		return err
	}
	defer ctx.Close()

	existing, err := ctx.store.GetNamespaceByName(name)
	if err != nil {
		return fmt.Errorf("check namespace: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("namespace %q already exists", name)
	}

	ns := &store.Namespace{
		ID:        uuid.New().String(),
		Name:      name,
		CreatedAt: time.Now(),
	}
	if err := ctx.store.CreateNamespace(ns); err != nil {
		return fmt.Errorf("create namespace: %w", err)
	}

	fmt.Printf("Created namespace %q\n", name)

	return nil
}

func runAdminNamespaceList(cmd *cobra.Command, args []string) error {
	ctx, err := loadAdminContext()
	if err != nil {
		return err
	}
	defer ctx.Close()

	namespaces, err := ctx.store.ListNamespaces("", 1000)
	if err != nil {
		return fmt.Errorf("list namespaces: %w", err)
	}

	if len(namespaces) == 0 {
		fmt.Println("No namespaces found")
		return nil
	}

	for _, ns := range namespaces {
		fmt.Println(ns.Name)
	}

	return nil
}

func runAdminNamespaceDelete(cmd *cobra.Command, args []string) error {
	name := args[0]

	ctx, err := loadAdminContext()
	if err != nil {
		return err
	}
	defer ctx.Close()

	ns, err := ctx.store.GetNamespaceByName(name)
	if err != nil {
		return fmt.Errorf("get namespace: %w", err)
	}
	if ns == nil {
		return fmt.Errorf("namespace %q not found", name)
	}

	users, err := ctx.store.ListUsers("", 1000)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}

	for _, user := range users {
		if user.PrimaryNamespaceID == ns.ID {
			return fmt.Errorf("cannot delete namespace %q: it is the primary namespace for a user (use 'eph admin user delete' instead)", name)
		}
	}

	if err := ctx.store.DeleteNamespace(ns.ID); err != nil {
		return fmt.Errorf("delete namespace: %w", err)
	}

	fmt.Printf("Deleted namespace %q\n", name)

	return nil
}

func runAdminNamespaceGrant(cmd *cobra.Command, args []string) error {
	namespaceName := args[0]
	username := args[1]

	ctx, err := loadAdminContext()
	if err != nil {
		return err
	}
	defer ctx.Close()

	ns, err := ctx.store.GetNamespaceByName(namespaceName)
	if err != nil {
		return fmt.Errorf("get namespace: %w", err)
	}
	if ns == nil {
		return fmt.Errorf("namespace %q not found", namespaceName)
	}

	user, err := findUserByUsername(ctx.store, username)
	if err != nil {
		return err
	}

	now := time.Now()
	grant := &store.NamespaceGrant{
		UserID:      user.ID,
		NamespaceID: ns.ID,
		AllowBits:   store.DefaultNamespaceGrant(),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := ctx.store.UpsertNamespaceGrant(grant); err != nil {
		return fmt.Errorf("create grant: %w", err)
	}

	fmt.Printf("Granted %q access to namespace %q\n", username, namespaceName)

	return nil
}

func runAdminNamespaceRevoke(cmd *cobra.Command, args []string) error {
	namespaceName := args[0]
	username := args[1]

	ctx, err := loadAdminContext()
	if err != nil {
		return err
	}
	defer ctx.Close()

	ns, err := ctx.store.GetNamespaceByName(namespaceName)
	if err != nil {
		return fmt.Errorf("get namespace: %w", err)
	}
	if ns == nil {
		return fmt.Errorf("namespace %q not found", namespaceName)
	}

	user, err := findUserByUsername(ctx.store, username)
	if err != nil {
		return err
	}

	if user.PrimaryNamespaceID == ns.ID {
		return fmt.Errorf("cannot revoke access to user's primary namespace")
	}

	if err := ctx.store.DeleteNamespaceGrant(user.ID, ns.ID); err != nil {
		return fmt.Errorf("delete grant: %w", err)
	}

	fmt.Printf("Revoked %q access to namespace %q\n", username, namespaceName)

	return nil
}

func findUserByUsername(st store.Store, username string) (*store.User, error) {
	ns, err := st.GetNamespaceByName(username)
	if err != nil {
		return nil, fmt.Errorf("get namespace: %w", err)
	}
	if ns == nil {
		return nil, fmt.Errorf("user %q not found", username)
	}

	users, err := st.ListUsers("", 1000)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	for _, u := range users {
		if u.PrimaryNamespaceID == ns.ID {
			return &u, nil
		}
	}

	return nil, fmt.Errorf("user %q not found", username)
}
