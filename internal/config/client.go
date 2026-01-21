package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type ClientConfig struct {
	Server           string `toml:"server"`
	Token            string `toml:"token"`
	DefaultNamespace string `toml:"default_namespace"`
}

const globalConfigPath = ".config/ephemeral/config.toml"

func configPath() (string, error) {
	// Check EPHEMERAL_CONFIG env var first
	if envPath := os.Getenv("EPHEMERAL_CONFIG"); envPath != "" {
		return envPath, nil
	}

	// Default to global config
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}

	return filepath.Join(home, globalConfigPath), nil
}

func Load() (*ClientConfig, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found at %s", path)
		}
		return nil, fmt.Errorf("access config: %w", err)
	}

	config := &ClientConfig{}

	if _, err := toml.DecodeFile(path, config); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	return config, nil
}

func (c *ClientConfig) Save() error {
	path, err := configPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create config file: %w", err)
	}
	defer f.Close()

	if err := f.Chmod(0600); err != nil {
		return fmt.Errorf("set config permissions: %w", err)
	}

	if err := toml.NewEncoder(f).Encode(c); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	return nil
}

func (c *ClientConfig) IsConfigured() bool {
	return c.Server != "" && c.Token != ""
}

func Delete() error {
	path, err := configPath()
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove config: %w", err)
	}

	return nil
}
