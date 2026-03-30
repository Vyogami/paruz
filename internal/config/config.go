package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	AURHelper    string `json:"aur_helper"`    // paru or yay
	MirrorHelper string `json:"mirror_helper"` // rate-mirrors or reflector
	Theme        string `json:"theme"`         // default, dracula, nord
}

var DefaultConfig = Config{
	AURHelper:    "paru",
	MirrorHelper: "rate-mirrors",
	Theme:        "default",
}

func LoadConfig() Config {
	home, err := os.UserHomeDir()
	if err != nil {
		return DefaultConfig
	}

	configDir := filepath.Join(home, ".config", "paruz")
	configPath := filepath.Join(configDir, "config.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		// Create default config if it doesn't exist
		SaveConfig(DefaultConfig)
		return DefaultConfig
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig
	}

	return cfg
}

func SaveConfig(cfg Config) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	configDir := filepath.Join(home, ".config", "paruz")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	configPath := filepath.Join(configDir, "config.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}
