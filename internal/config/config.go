// Package config provides configuration loading and validation for the TSDNS server.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	errListenRequired    = errors.New("config: tsdns.listen is required")
	errCacheRequired     = errors.New("config: tsdns.cache_refresh_interval is required")
	errStorageRequired   = errors.New("config: storage.dsn is required")
	errAPIMethodRequired = errors.New("config: at least one API listen method (api.listen or api.socket) is required")
)

// Config represents the root configuration structure.
type Config struct {
	TSDNS   TSDNSConfig   `yaml:"tsdns"`
	API     APIConfig     `yaml:"api"`
	Log     LogConfig     `yaml:"log"`
	Storage StorageConfig `yaml:"storage"`
}

// TSDNSConfig contains settings for the TSDNS protocol server.
type TSDNSConfig struct {
	// Listen is the TCP listen address for the TSDNS protocol.
	// Example: "0.0.0.0:41144".
	Listen string `yaml:"listen"`

	// CacheRefreshInterval controls how often the in-memory cache is refreshed from the repository.
	// Use a duration string like "30s". Use "0" to disable periodic refresh.
	CacheRefreshInterval string `yaml:"cache_refresh_interval"`
}

// APIConfig contains settings for the admin HTTP API.
type APIConfig struct {
	// Listen is the HTTP listen address for the admin API.
	// Example: "127.0.0.1:8080". Use empty string to disable the API.
	Listen string `yaml:"listen"`

	// Socket is the path to the Unix domain socket for local management.
	// Example: "/var/run/tsdns.sock". Use empty string to disable.
	Socket string `yaml:"socket"`

	// URL is the base URL used by the CLI when talking to the admin API.
	// Example: "http://127.0.0.1:8080".
	URL string `yaml:"url"`

	// Token is the shared token required by the admin API.
	// When empty, the API is unauthenticated.
	Token string `yaml:"token"`
}

// LogConfig contains logging settings.
type LogConfig struct {
	// Level is the log level (debug, info, warn, error).
	Level string `yaml:"level"`
	// Format is the log format (text, json).
	Format string `yaml:"format"`
}

// StorageConfig contains settings for the storage backend.
type StorageConfig struct {
	// DSN selects the storage backend and its configuration.
	//
	// Supported schemes:
	//   - sqlite:   sqlite:./data/tsdns.sqlite or sqlite:///:memory:
	//   - postgres: postgres://user:pass@host:5432/db?sslmode=disable
	//   - mysql:    mysql://user:pass@host:3306/db?parseTime=true (or mysql:<go-sql-driver dsn>)
	//   - mariadb:  mariadb://user:pass@host:3306/db?parseTime=true
	//   - redis:    redis://:password@host:6379/0
	DSN string `yaml:"dsn"`
}

// Default returns a Config initialized with default values.
func Default() Config {
	return Config{
		TSDNS: TSDNSConfig{
			Listen:               "0.0.0.0:41144",
			CacheRefreshInterval: "30s",
		},
		API: APIConfig{
			Listen: "127.0.0.1:8080",
			Socket: "", // Default to disabled to avoid permission issues in non-container environments
			URL:    "http://127.0.0.1:8080",
			Token:  "",
		},
		Log: LogConfig{
			Level:  "info",
			Format: "text",
		},
		Storage: StorageConfig{
			DSN: "sqlite:./data/tsdns.sqlite",
		},
	}
}

// Load reads a YAML configuration file from the given path and applies environment variable overrides.
func Load(path string) (Config, error) {
	cfg := Default()

	if path != "" {
		// Clean path to mitigate potential G304 (File Inclusion).
		cleanPath := filepath.Clean(path)
		b, err := os.ReadFile(cleanPath)
		if err != nil {
			return Config{}, fmt.Errorf("read config file: %w", err)
		}
		err = yaml.Unmarshal(b, &cfg)
		if err != nil {
			return Config{}, fmt.Errorf("parse config file: %w", err)
		}
	}

	cfg.ApplyEnv()

	err := cfg.Validate()
	if err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// ApplyEnv overrides configuration values with environment variables if they are set.
func (c *Config) ApplyEnv() {
	if v := os.Getenv("TSDNS_LISTEN"); v != "" {
		c.TSDNS.Listen = v
	}
	if v := os.Getenv("TSDNS_CACHE_REFRESH_INTERVAL"); v != "" {
		c.TSDNS.CacheRefreshInterval = v
	}

	if v := os.Getenv("TSDNS_API_LISTEN"); v != "" {
		c.API.Listen = v
	}
	if v := os.Getenv("TSDNS_API_SOCKET"); v != "" {
		c.API.Socket = v
	}
	if v := os.Getenv("TSDNS_API_URL"); v != "" {
		c.API.URL = v
	}
	if v := os.Getenv("TSDNS_API_TOKEN"); v != "" {
		c.API.Token = v
	}

	if v := os.Getenv("TSDNS_LOG_LEVEL"); v != "" {
		c.Log.Level = v
	}
	if v := os.Getenv("TSDNS_LOG_FORMAT"); v != "" {
		c.Log.Format = v
	}

	if v := os.Getenv("TSDNS_STORAGE_DSN"); v != "" {
		c.Storage.DSN = v
	}
}

// Validate checks if the configuration contains all required fields and is valid.
func (c *Config) Validate() error {
	if strings.TrimSpace(c.TSDNS.Listen) == "" {
		return errListenRequired
	}
	if strings.TrimSpace(c.TSDNS.CacheRefreshInterval) == "" {
		return errCacheRequired
	}
	if strings.TrimSpace(c.Storage.DSN) == "" {
		return errStorageRequired
	}

	if strings.TrimSpace(c.API.Listen) == "" && strings.TrimSpace(c.API.Socket) == "" {
		return errAPIMethodRequired
	}

	return nil
}
