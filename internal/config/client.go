package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type ClientConfig struct {
	CurrentContext string             `toml:"current_context"`
	Contexts       map[string]Context `toml:"contexts"`
}

type Context struct {
	Server    string `toml:"server"`
	Token     string `toml:"token"`
	Namespace string `toml:"namespace"`
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".config", "ephemeral", "config.toml"), nil
}

func Load() (*ClientConfig, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	config := &ClientConfig{
		Contexts: make(map[string]Context),
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found at %s", path)
	}

	if _, err := toml.DecodeFile(path, config); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	config.applyEnvOverrides()

	return config, nil
}

func (c *ClientConfig) applyEnvOverrides() {
	ctx, ok := c.Contexts[c.CurrentContext]
	if !ok {
		return
	}

	if server := os.Getenv("EPHEMERAL_SERVER"); server != "" {
		ctx.Server = server
	}

	if token := os.Getenv("EPHEMERAL_TOKEN"); token != "" {
		ctx.Token = token
	}

	c.Contexts[c.CurrentContext] = ctx
}

func (c *ClientConfig) Save() error {
	path, err := configPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create config file: %w", err)
	}
	defer f.Close()

	if err := toml.NewEncoder(f).Encode(c); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	return nil
}

func (c *ClientConfig) Current() *Context {
	if c.CurrentContext == "" {
		return nil
	}

	ctx, ok := c.Contexts[c.CurrentContext]
	if !ok {
		return nil
	}

	return &ctx
}
