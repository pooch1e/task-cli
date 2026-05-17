package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/BurntSushi/toml"
)

type LLMConfig struct {
	Provider    string `toml:"provider"`
	Model       string `toml:"model"`
	BaseURL     string `toml:"base_url"`
	APIKey      string `toml:"api_key"`
	MaxTokens   int    `toml:"max_tokens"`
	TimeoutSecs int    `toml:"timeout_secs"`
}

type StorageConfig struct {
	DBPath string `toml:"db_path"`
}

type ProjectConfig struct {
	AutoDetect bool `toml:"auto_detect"`
}

type Config struct {
	LLM     LLMConfig     `toml:"llm"`
	Storage StorageConfig `toml:"storage"`
	Project ProjectConfig `toml:"project"`
}

// Dir returns ~/.task
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".task"), nil
}

// Path returns ~/.task/config.toml
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// Load reads the config file. Returns ErrNotFound if the file doesn't exist.
var ErrNotFound = errors.New("config not found")

func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, ErrNotFound
	}

	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	cfg.applyDefaults()

	// env var overrides api_key
	if key := os.Getenv("TASK_API_KEY"); key != "" {
		cfg.LLM.APIKey = key
	}

	return &cfg, nil
}

// Save writes config to ~/.task/config.toml with mode 0600.
func Save(cfg *Config) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	path := filepath.Join(dir, "config.toml")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	return toml.NewEncoder(f).Encode(cfg)
}

// Default returns a config with sensible defaults (no API key).
func Default() *Config {
	dir, _ := Dir()
	cfg := &Config{
		LLM: LLMConfig{
			Provider:    "deepseek",
			Model:       "deepseek-chat",
			BaseURL:     "https://api.deepseek.com/v1",
			MaxTokens:   1024,
			TimeoutSecs: 30,
		},
		Storage: StorageConfig{
			DBPath: filepath.Join(dir, "tasks.db"),
		},
		Project: ProjectConfig{
			AutoDetect: true,
		},
	}
	return cfg
}

func (c *Config) applyDefaults() {
	d := Default()
	if c.LLM.Provider == "" {
		c.LLM.Provider = d.LLM.Provider
	}
	if c.LLM.Model == "" {
		c.LLM.Model = d.LLM.Model
	}
	if c.LLM.BaseURL == "" {
		c.LLM.BaseURL = d.LLM.BaseURL
	}
	if c.LLM.MaxTokens == 0 {
		c.LLM.MaxTokens = d.LLM.MaxTokens
	}
	if c.LLM.TimeoutSecs == 0 {
		c.LLM.TimeoutSecs = d.LLM.TimeoutSecs
	}
	if c.Storage.DBPath == "" {
		c.Storage.DBPath = d.Storage.DBPath
	}
}

// LoadOrDefault loads config, returning Default() if the file does not exist.
func LoadOrDefault() (*Config, error) {
	cfg, err := Load()
	if err == ErrNotFound {
		return Default(), nil
	}
	return cfg, err
}

// Validate returns an error if required fields are missing.
func (c *Config) Validate() error {
	if c.LLM.Provider != "pi" && c.LLM.Provider != "opencode" && c.LLM.APIKey == "" {
		if os.Getenv("TASK_API_KEY") == "" {
			return fmt.Errorf("no API key set — add api_key to config or set TASK_API_KEY env var")
		}
	}
	return nil
}
