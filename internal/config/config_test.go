package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault_Valid(t *testing.T) {
	cfg := Default()
	err := cfg.Validate()
	if err != nil {
		t.Fatalf("default config should be valid, got: %v", err)
	}
}

func TestValidate_MissingStorageDSN(t *testing.T) {
	cfg := Default()
	cfg.Storage.DSN = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatalf("expected Validate to fail for missing storage.dsn")
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("TSDNS_STORAGE_DSN", "postgres://user:pass@127.0.0.1:5432/db?sslmode=disable")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Storage.DSN == "" {
		t.Fatalf("expected storage.dsn to be set")
	}
}

func TestValidate_MissingAPI(t *testing.T) {
	cfg := Default()
	cfg.API.Listen = ""
	cfg.API.Socket = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatalf("expected Validate to fail for missing api.listen and api.socket")
	}
}

func TestLoad_YAMLFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := []byte(`
tsdns:
  listen: "127.0.0.1:41144"
  cache_refresh_interval: "0"
api:
  listen: "127.0.0.1:8080"
storage:
  dsn: "file:` + filepath.Join(dir, "records.msgpack") + `"
`)
	// Use 0o600 for test files to satisfy G306.
	err := os.WriteFile(path, content, 0o600)
	if err != nil {
		t.Fatalf("write config file failed: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.TSDNS.Listen != "127.0.0.1:41144" {
		t.Fatalf("unexpected tsdns.listen: %q", cfg.TSDNS.Listen)
	}
	if cfg.TSDNS.CacheRefreshInterval != "0" {
		t.Fatalf("unexpected tsdns.cache_refresh_interval: %q", cfg.TSDNS.CacheRefreshInterval)
	}
	if cfg.Storage.DSN == "" {
		t.Fatalf("expected storage.dsn to be set")
	}
}
