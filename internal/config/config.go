package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	AURHelper    string `toml:"aur_helper"`    // paru or yay
	MirrorHelper string `toml:"mirror_helper"` // rate-mirrors or reflector
	Theme        string `toml:"theme"`         // default, dracula, nord
}

type CustomTheme struct {
	TitleBg   string `toml:"title_bg"`
	TitleFg   string `toml:"title_fg"`
	Border    string `toml:"border"`
	InfoTitle string `toml:"info_title"`
	InfoKey   string `toml:"info_key"`
	Error     string `toml:"error"`
	StatusBar string `toml:"status_bar"`
}

type ThemesConfig struct {
	Themes map[string]CustomTheme `toml:"themes"`
}

var DefaultConfig = Config{
	AURHelper:    "paru",
	MirrorHelper: "rate-mirrors",
	Theme:        "ayu-dark",
}

const DefaultConfigTOML = `# paruz configuration

# AUR helper to use (paru or yay)
aur_helper = "paru"

# Mirror helper to use (rate-mirrors or reflector)
mirror_helper = "rate-mirrors"

# Theme to use (default, dracula, nord, ayu-dark, etc.)
# You can add custom themes in themes.toml
theme = "ayu-dark"
`

func GetConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	configDir := filepath.Join(home, ".config", "paruz")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", err
	}
	return configDir, nil
}

func LoadConfig() Config {
	configDir, err := GetConfigDir()
	if err != nil {
		return DefaultConfig
	}
	configPath := filepath.Join(configDir, "config.toml")
	oldConfigPath := filepath.Join(configDir, "config.json")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Migration check
		if _, err := os.Stat(oldConfigPath); err == nil {
			data, err := os.ReadFile(oldConfigPath)
			if err == nil {
				var cfg Config
				if err := json.Unmarshal(data, &cfg); err == nil {
					// Ensure we don't have empty fields from old config
					if cfg.AURHelper == "" {
						cfg.AURHelper = DefaultConfig.AURHelper
					}
					if cfg.MirrorHelper == "" {
						cfg.MirrorHelper = DefaultConfig.MirrorHelper
					}
					if cfg.Theme == "" {
						cfg.Theme = DefaultConfig.Theme
					}
					_ = SaveConfig(cfg)
					_ = os.Remove(oldConfigPath)
					return cfg
				}
			}
		}
		// Create new config with comments
		_ = os.WriteFile(configPath, []byte(DefaultConfigTOML), 0644)
		return DefaultConfig
	}

	var cfg Config
	if _, err := toml.DecodeFile(configPath, &cfg); err != nil {
		return DefaultConfig
	}

	// Double check for empty fields after decoding
	if cfg.AURHelper == "" {
		cfg.AURHelper = DefaultConfig.AURHelper
	}
	if cfg.MirrorHelper == "" {
		cfg.MirrorHelper = DefaultConfig.MirrorHelper
	}
	if cfg.Theme == "" {
		cfg.Theme = DefaultConfig.Theme
	}

	return cfg
}

func SaveConfig(cfg Config) error {
	configDir, err := GetConfigDir()
	if err != nil {
		return err
	}

	configPath := filepath.Join(configDir, "config.toml")
	f, err := os.Create(configPath)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := toml.NewEncoder(f).Encode(cfg); err != nil {
		return err
	}

	return nil
}

var ExampleThemesConfig = ThemesConfig{
	Themes: map[string]CustomTheme{
		"dracula-pro": {
			TitleBg:   "#bd93f9",
			TitleFg:   "#282a36",
			Border:    "#6272a4",
			InfoTitle: "#ff79c6",
			InfoKey:   "#8be9fd",
			Error:     "#ff5555",
			StatusBar: "#f8f8f2",
		},
	},
}

func LoadCustomThemes() map[string]CustomTheme {
	configDir, err := GetConfigDir()
	if err != nil {
		return nil
	}
	themesPath := filepath.Join(configDir, "themes.toml")

	if _, err := os.Stat(themesPath); os.IsNotExist(err) {
		// Create example themes file
		f, err := os.Create(themesPath)
		if err == nil {
			defer f.Close()
			_ = toml.NewEncoder(f).Encode(ExampleThemesConfig)
		}
		return ExampleThemesConfig.Themes
	}

	var themesCfg ThemesConfig
	if _, err := toml.DecodeFile(themesPath, &themesCfg); err != nil {
		return nil
	}

	return themesCfg.Themes
}
