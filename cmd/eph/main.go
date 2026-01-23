package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/bantamhq/ephemeral/internal/client"
	"github.com/bantamhq/ephemeral/internal/config"
	"github.com/bantamhq/ephemeral/internal/server"
	"github.com/bantamhq/ephemeral/internal/store"
	"github.com/bantamhq/ephemeral/internal/tui"
)

type Config struct {
	Server struct {
		Port int    `toml:"port"`
		Host string `toml:"host"`
	} `toml:"server"`
	Storage struct {
		DataDir string `toml:"data_dir"`
	} `toml:"storage"`
	Auth struct {
		WebAuthURL            string `toml:"web_auth_url"`
		ExchangeValidationURL string `toml:"exchange_validation_url"`
		ExchangeSecret        string `toml:"exchange_secret"`
	} `toml:"auth"`
	LFS struct {
		Enabled     bool   `toml:"enabled"`
		MaxFileSize int64  `toml:"max_file_size"`
		BaseURL     string `toml:"base_url"`
	} `toml:"lfs"`
}

var version = "dev"

func main() {
	rootCmd := &cobra.Command{
		Use:           "eph",
		Short:         "A minimal, terminal-native git hosting service",
		Long:          `Ephemeral is a minimal git hosting service with a terminal-first approach.`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runTUI,
	}

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Ephemeral server",
		RunE:  runServe,
	}

	rootCmd.AddCommand(
		serveCmd,
		newLoginCmd(),
		newLogoutCmd(),
		newWhoamiCmd(),
		newCredentialCmd(),
		newNamespaceCmd(),
		newAdminCmd(),
		newNewCmd(),
		newCloneCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func runTUI(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return errNotLoggedIn
	}

	if !cfg.IsConfigured() {
		return errNotLoggedIn
	}

	c := client.New(cfg.Server, cfg.Token)
	if cfg.DefaultNamespace != "" {
		c = c.WithNamespace(cfg.DefaultNamespace)
	}

	return tui.Run(c, cfg.DefaultNamespace, cfg.Server)
}

func runServe(cmd *cobra.Command, args []string) error {
	cfg, loadedFromFile, err := loadConfig("server.toml")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if loadedFromFile {
		fmt.Println("Loaded configuration from server.toml")
	} else {
		fmt.Println("No server.toml found, using defaults")
	}

	if err := os.MkdirAll(cfg.Storage.DataDir, 0755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	dbPath := filepath.Join(cfg.Storage.DataDir, "ephemeral.db")

	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		return fmt.Errorf("initialize database: %w", err)
	}
	defer st.Close()

	fmt.Println("Initializing database...")
	if err := st.Initialize(); err != nil {
		return fmt.Errorf("initialize schema: %w", err)
	}

	isFirstRun, err := checkFirstRun(st)
	if err != nil {
		return fmt.Errorf("check first run: %w", err)
	}

	if isFirstRun && term.IsTerminal(int(os.Stdout.Fd())) {
		wizard := NewSetupWizard(st, cfg.Storage.DataDir)
		if _, err := wizard.Run(); err != nil {
			return fmt.Errorf("setup wizard: %w", err)
		}
	} else {
		token, err := st.GenerateAdminToken()
		if err != nil {
			return fmt.Errorf("generate admin token: %w", err)
		}

		if token != "" {
			tokenPath := filepath.Join(cfg.Storage.DataDir, "admin-token")
			if err := os.WriteFile(tokenPath, []byte(token), 0600); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save admin token to file: %v\n", err)
			}

			fmt.Println("\n" + strings.Repeat("=", 60))
			fmt.Println("ADMIN TOKEN GENERATED")
			fmt.Println("Saved to: " + tokenPath)
			fmt.Println(strings.Repeat("=", 60))
			fmt.Println(token)
			fmt.Println(strings.Repeat("=", 60) + "\n")
		}
	}

	authOpts := server.AuthOptions{
		WebAuthURL:            cfg.Auth.WebAuthURL,
		ExchangeValidationURL: cfg.Auth.ExchangeValidationURL,
		ExchangeSecret:        cfg.Auth.ExchangeSecret,
	}

	lfsBaseURL := cfg.LFS.BaseURL
	if lfsBaseURL == "" {
		lfsBaseURL = fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port)
	}

	lfsOpts := server.LFSOptions{
		Enabled:     cfg.LFS.Enabled,
		MaxFileSize: cfg.LFS.MaxFileSize,
		BaseURL:     lfsBaseURL,
	}

	srv := server.NewServer(st, cfg.Storage.DataDir, authOpts, lfsOpts)

	fmt.Printf("Starting Ephemeral server on %s:%d\n", cfg.Server.Host, cfg.Server.Port)
	fmt.Printf("Data directory: %s\n", cfg.Storage.DataDir)
	fmt.Println("\nServer is ready to accept connections.")
	fmt.Println("Example: git clone http://x-token:<token>@localhost:8080/git/<namespace>/myrepo.git")

	return srv.Start(cfg.Server.Host, cfg.Server.Port)
}

func checkFirstRun(st store.Store) (bool, error) {
	namespaces, err := st.ListNamespaces("", 1)
	if err != nil {
		return false, err
	}
	return len(namespaces) == 0, nil
}

func loadConfig(path string) (*Config, bool, error) {
	config := Config{
		Server: struct {
			Port int    `toml:"port"`
			Host string `toml:"host"`
		}{Port: 8080, Host: "0.0.0.0"},
		Storage: struct {
			DataDir string `toml:"data_dir"`
		}{DataDir: "./data"},
	}

	if _, err := os.Stat(path); err == nil {
		if _, err := toml.DecodeFile(path, &config); err != nil {
			return nil, false, fmt.Errorf("decode config: %w", err)
		}
		return &config, true, nil
	}

	return &config, false, nil
}
