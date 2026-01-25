package main

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/bantamhq/ephemeral/internal/store"
)

func newAdminUserCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage users",
	}

	cmd.AddCommand(
		newAdminUserAddCmd(),
		newAdminUserListCmd(),
		newAdminUserDeleteCmd(),
		newAdminUserTokenCmd(),
	)

	return cmd
}

func newAdminUserAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <username>",
		Short: "Create a user with their primary namespace",
		Args:  cobra.ExactArgs(1),
		RunE:  runAdminUserAdd,
	}
}

func newAdminUserListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all users",
		RunE:  runAdminUserList,
	}
}

func newAdminUserDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <username>",
		Short: "Delete a user and their primary namespace",
		Args:  cobra.ExactArgs(1),
		RunE:  runAdminUserDelete,
	}
}

func newAdminUserTokenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "token <username>",
		Short: "Generate a new token for a user",
		Args:  cobra.ExactArgs(1),
		RunE:  runAdminUserToken,
	}
}

func runAdminUserAdd(cmd *cobra.Command, args []string) error {
	username := args[0]

	if err := validateNamespaceName(username); err != nil {
		return fmt.Errorf("invalid username: %w", err)
	}

	ctx, err := loadAdminContext()
	if err != nil {
		return err
	}
	defer ctx.Close()

	existing, err := ctx.store.GetNamespaceByName(username)
	if err != nil {
		return fmt.Errorf("check namespace: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("namespace %q already exists", username)
	}

	now := time.Now()

	ns := &store.Namespace{
		ID:        uuid.New().String(),
		Name:      username,
		CreatedAt: now,
	}
	if err := ctx.store.CreateNamespace(ns); err != nil {
		return fmt.Errorf("create namespace: %w", err)
	}

	user := &store.User{
		ID:                 uuid.New().String(),
		PrimaryNamespaceID: ns.ID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := ctx.store.CreateUser(user); err != nil {
		return fmt.Errorf("create user: %w", err)
	}

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

	token, _, err := ctx.store.GenerateUserToken(user.ID, nil)
	if err != nil {
		return fmt.Errorf("generate token: %w", err)
	}

	fmt.Printf("Created user %q\n", username)
	fmt.Printf("Token: %s\n", token)

	return nil
}

func runAdminUserList(cmd *cobra.Command, args []string) error {
	ctx, err := loadAdminContext()
	if err != nil {
		return err
	}
	defer ctx.Close()

	users, err := ctx.store.ListUsers("", 1000)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}

	if len(users) == 0 {
		fmt.Println("No users found")
		return nil
	}

	for _, user := range users {
		ns, err := ctx.store.GetNamespace(user.PrimaryNamespaceID)
		if err != nil {
			return fmt.Errorf("get namespace: %w", err)
		}

		name := user.ID
		if ns != nil {
			name = ns.Name
		}
		fmt.Println(name)
	}

	return nil
}

func runAdminUserDelete(cmd *cobra.Command, args []string) error {
	username := args[0]

	ctx, err := loadAdminContext()
	if err != nil {
		return err
	}
	defer ctx.Close()

	ns, err := ctx.store.GetNamespaceByName(username)
	if err != nil {
		return fmt.Errorf("get namespace: %w", err)
	}
	if ns == nil {
		return fmt.Errorf("user %q not found", username)
	}

	users, err := ctx.store.ListUsers("", 1000)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}

	var user *store.User
	for _, u := range users {
		if u.PrimaryNamespaceID == ns.ID {
			user = &u
			break
		}
	}

	if user == nil {
		return fmt.Errorf("user %q not found", username)
	}

	if err := ctx.store.DeleteUser(user.ID); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	if err := ctx.store.DeleteNamespace(ns.ID); err != nil {
		return fmt.Errorf("delete namespace: %w", err)
	}

	fmt.Printf("Deleted user %q\n", username)

	return nil
}

func runAdminUserToken(cmd *cobra.Command, args []string) error {
	username := args[0]

	ctx, err := loadAdminContext()
	if err != nil {
		return err
	}
	defer ctx.Close()

	ns, err := ctx.store.GetNamespaceByName(username)
	if err != nil {
		return fmt.Errorf("get namespace: %w", err)
	}
	if ns == nil {
		return fmt.Errorf("user %q not found", username)
	}

	users, err := ctx.store.ListUsers("", 1000)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}

	var user *store.User
	for _, u := range users {
		if u.PrimaryNamespaceID == ns.ID {
			user = &u
			break
		}
	}

	if user == nil {
		return fmt.Errorf("user %q not found", username)
	}

	token, _, err := ctx.store.GenerateUserToken(user.ID, nil)
	if err != nil {
		return fmt.Errorf("generate token: %w", err)
	}

	fmt.Printf("Token: %s\n", token)

	return nil
}
