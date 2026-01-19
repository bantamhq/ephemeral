package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"

	"ephemeral/internal/client"
	"ephemeral/internal/config"
	"ephemeral/internal/server"
	"ephemeral/internal/store"
	"ephemeral/internal/tui"
)

type Config struct {
	Server struct {
		Port int    `toml:"port"`
		Host string `toml:"host"`
	} `toml:"server"`
	Storage struct {
		DataDir string `toml:"data_dir"`
	} `toml:"storage"`
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "ephemeral",
		Short: "A minimal, terminal-native git hosting service",
		Long:  `Ephemeral is a minimal git hosting service with a terminal-first approach.`,
		RunE:  runTUI,
	}

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Ephemeral server",
		RunE:  runServe,
	}

	rootCmd.AddCommand(serveCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runTUI(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Configuration not found.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Create a config file at ~/.config/ephemeral/config.toml:")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, `  current_context = "default"`)
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  [contexts.default]")
		fmt.Fprintln(os.Stderr, `  server = "http://localhost:8080"`)
		fmt.Fprintln(os.Stderr, `  token = "eph_xxx"  # your token from 'ephemeral serve'`)
		fmt.Fprintln(os.Stderr, `  namespace = "default"`)
		return fmt.Errorf("config not found: %w", err)
	}

	ctx := cfg.Current()
	if ctx == nil {
		return fmt.Errorf("no current context configured")
	}

	if ctx.Server == "" {
		return fmt.Errorf("server not configured in context")
	}

	if ctx.Token == "" {
		return fmt.Errorf("token not configured in context")
	}

	c := client.New(ctx.Server, ctx.Token)

	return tui.Run(c, ctx.Namespace, ctx.Server)
}

func runServe(cmd *cobra.Command, args []string) error {
	config, err := loadConfig("config.toml")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := os.MkdirAll(config.Storage.DataDir, 0755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	dbPath := filepath.Join(config.Storage.DataDir, "ephemeral.db")

	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		return fmt.Errorf("initialize database: %w", err)
	}
	defer st.Close()

	fmt.Println("Initializing database...")
	if err := st.Initialize(); err != nil {
		return fmt.Errorf("initialize schema: %w", err)
	}

	token, err := st.GenerateRootToken()
	if err != nil {
		return fmt.Errorf("generate root token: %w", err)
	}

	if token != "" {
		fmt.Println("\n" + strings.Repeat("=", 60))
		fmt.Println("ROOT TOKEN GENERATED (save this, it won't be shown again):")
		fmt.Println(token)
		fmt.Println(strings.Repeat("=", 60) + "\n")
	}

	srv := server.NewServer(st, config.Storage.DataDir)

	fmt.Printf("Starting Ephemeral server on %s:%d\n", config.Server.Host, config.Server.Port)
	fmt.Printf("Data directory: %s\n", config.Storage.DataDir)
	fmt.Println("\nServer is ready to accept connections.")
	fmt.Println("Example: git clone http://x-token:<token>@localhost:8080/git/default/myrepo.git")

	return srv.Start(config.Server.Host, config.Server.Port)
}

func loadConfig(path string) (*Config, error) {
	config := Config{
		Server:  struct{ Port int `toml:"port"`; Host string `toml:"host"` }{Port: 8080, Host: "0.0.0.0"},
		Storage: struct{ DataDir string `toml:"data_dir"` }{DataDir: "./data"},
	}

	if _, err := os.Stat(path); err == nil {
		if _, err := toml.DecodeFile(path, &config); err != nil {
			return nil, fmt.Errorf("decode config: %w", err)
		}
	}

	return &config, nil
}