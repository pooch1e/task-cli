package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/BurntSushi/toml"
)

// Provider constants — single source of truth. Defined here so both config
// validation and the llm package can use them without a circular import.
const (
	ProviderPi       = "pi"
	ProviderOpencode = "opencode"
	ProviderGemini   = "gemini"
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

type ObsidianConfig struct {
	VaultPath string `toml:"vault_path"`
}

type Config struct {
	LLM      LLMConfig      `toml:"llm"`
	Storage  StorageConfig  `toml:"storage"`
	Project  ProjectConfig  `toml:"project"`
	Obsidian ObsidianConfig `toml:"obsidian"`
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

// ErrNotFound is returned by Load when the config file does not exist.
var ErrNotFound = errors.New("config not found")

// Load reads the config file. Returns ErrNotFound if the file doesn't exist.
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
	cfg.applyEnvOverrides()

	return &cfg, nil
}

// LoadOrDefault loads config, returning Default() if the file does not exist.
func LoadOrDefault() (*Config, error) {
	cfg, err := Load()
	if err == ErrNotFound {
		return Default(), nil
	}
	return cfg, err
}

// Save writes cfg to ~/.task/config.toml with mode 0600.
// O_NOFOLLOW prevents symlink-based attacks on the config path.
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

// Default returns a config with sensible defaults and no API key.
// If the home directory cannot be resolved, the DB path falls back to the
// OS temp directory rather than producing a silently invalid empty path.
func Default() *Config {
	dir, err := Dir()
	if err != nil {
		dir = os.TempDir()
	}
	return &Config{
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
}

// applyDefaults fills zero-value fields using KnownProviders metadata where
// available, falling back to hardcoded deepseek defaults for unknown providers.
func (c *Config) applyDefaults() {
	if meta := LookupProvider(c.LLM.Provider); meta != nil {
		if c.LLM.Model == "" {
			c.LLM.Model = meta.DefaultModel
		}
		if c.LLM.BaseURL == "" {
			c.LLM.BaseURL = meta.DefaultBaseURL
		}
	} else {
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
	}
	if c.LLM.MaxTokens == 0 {
		c.LLM.MaxTokens = 1024
	}
	if c.LLM.TimeoutSecs == 0 {
		c.LLM.TimeoutSecs = 30
	}
	if c.Storage.DBPath == "" {
		dir, err := Dir()
		if err != nil {
			dir = os.TempDir()
		}
		c.Storage.DBPath = filepath.Join(dir, "tasks.db")
	}
}

// applyEnvOverrides applies environment variable overrides. Env vars take
// precedence over the config file. Supported vars:
//
//	TASK_API_KEY   — overrides llm.api_key
//	TASK_PROVIDER  — overrides llm.provider
//	TASK_MODEL     — overrides llm.model
//	TASK_BASE_URL  — overrides llm.base_url
func (c *Config) applyEnvOverrides() {
	if v := os.Getenv("TASK_API_KEY"); v != "" {
		c.LLM.APIKey = v
	}
	// Provider-specific key env var — checked only if TASK_API_KEY is not set.
	if c.LLM.APIKey == "" {
		if v := getAPIKeyFromEnv(c.LLM.Provider); v != "" {
			c.LLM.APIKey = v
		}
	}
	if v := os.Getenv("TASK_PROVIDER"); v != "" {
		c.LLM.Provider = v
	}
	if v := os.Getenv("TASK_MODEL"); v != "" {
		c.LLM.Model = v
	}
	if v := os.Getenv("TASK_BASE_URL"); v != "" {
		c.LLM.BaseURL = v
	}
}

// getAPIKeyFromEnv returns the value of the provider-specific API key env var,
// or empty string if the provider has no key var or the var is unset.
func getAPIKeyFromEnv(provider string) string {
	meta := LookupProvider(provider)
	if meta == nil || meta.KeyEnvVar == "" || meta.KeyEnvVar == "TASK_API_KEY" {
		return ""
	}
	return os.Getenv(meta.KeyEnvVar)
}

// Validate returns an error if required fields are missing for the chosen provider.
func (c *Config) Validate() error {
	// Subprocess providers use their own credential stores — no key needed.
	if c.LLM.Provider == ProviderPi || c.LLM.Provider == ProviderOpencode {
		return nil
	}
	if c.LLM.APIKey == "" {
		return fmt.Errorf("no API key set — add api_key to config or set TASK_API_KEY env var")
	}
	return nil
}
