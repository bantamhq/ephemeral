package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/bantamhq/ephemeral/internal/store"
)

func newAdminCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Server administration commands",
		Long:  `Administrative commands for managing users and namespaces. Requires access to the server's data directory.`,
	}

	cmd.AddCommand(
		newAdminInitCmd(),
		newAdminUserCmd(),
		newAdminNamespaceCmd(),
	)

	return cmd
}

func newAdminInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize the server (first-time setup)",
		RunE:  runAdminInit,
	}
}

func runAdminInit(cmd *cobra.Command, args []string) error {
	cfg, _, err := loadConfig("server.toml")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	st, err := initStore(cfg.Storage.DataDir)
	if err != nil {
		return err
	}
	defer st.Close()

	hasAdmin, err := st.HasAdminToken()
	if err != nil {
		return fmt.Errorf("check admin token: %w", err)
	}

	if hasAdmin {
		fmt.Println("Server is already initialized.")
		fmt.Println("Run 'eph serve' to start the server.")
		return nil
	}

	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("interactive terminal required for setup wizard")
	}

	wizard := NewSetupWizard(st, cfg.Storage.DataDir)
	if _, err := wizard.Run(); err != nil {
		return fmt.Errorf("setup wizard: %w", err)
	}

	return nil
}

type adminContext struct {
	store   store.Store
	dataDir string
}

func loadAdminContext() (*adminContext, error) {
	cfg, _, err := loadConfig("server.toml")
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	tokenPath := filepath.Join(cfg.Storage.DataDir, "admin-token")
	tokenBytes, err := os.ReadFile(tokenPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("server not initialized - run 'eph admin init' first")
		}
		return nil, fmt.Errorf("read admin token: %w", err)
	}

	if strings.TrimSpace(string(tokenBytes)) == "" {
		return nil, fmt.Errorf("admin token file is empty")
	}

	st, err := initStore(cfg.Storage.DataDir)
	if err != nil {
		return nil, err
	}

	return &adminContext{
		store:   st,
		dataDir: cfg.Storage.DataDir,
	}, nil
}

func (c *adminContext) Close() error {
	return c.store.Close()
}
