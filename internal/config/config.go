package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	appName        = "115cli"
	configFileName = "config.json"
)

type Config struct {
	Cookie           string    `json:"cookie,omitempty"`
	RefreshToken     string    `json:"refresh_token,omitempty"`
	AccessToken      string    `json:"access_token,omitempty"`
	ClientID         string    `json:"client_id,omitempty"`
	RootID           string    `json:"root_id"`
	LastTokenRefresh time.Time `json:"last_token_refresh,omitempty"`
}

func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", appName, configFileName), nil
}

func Load(path string) (*Config, error) {
	if path == "" {
		var err error
		path, err = DefaultPath()
		if err != nil {
			return nil, err
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("config not found; run `115cli auth cookie --cookie '<cookie>'` first")
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.RootID == "" {
		cfg.RootID = "0"
	}
	if cfg.Cookie == "" && cfg.RefreshToken == "" {
		return nil, fmt.Errorf("missing credentials; run `115cli auth cookie --cookie '<cookie>'`")
	}
	return &cfg, nil
}

func Save(path string, cfg *Config) error {
	if path == "" {
		var err error
		path, err = DefaultPath()
		if err != nil {
			return err
		}
	}
	if cfg.RootID == "" {
		cfg.RootID = "0"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}
