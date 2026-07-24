package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "patchwork.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadMissingFileError(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil {
		t.Fatal("expected error for missing config")
	}
	if !strings.Contains(err.Error(), "patchwork.yaml.example") {
		t.Errorf("error should point at the example file, got: %v", err)
	}
}

func TestSMTPPassFromEnv(t *testing.T) {
	t.Setenv("PATCHWORK_SMTP_PASS", "env-secret")
	cfg, err := Load(writeConfig(t, `
instance:
  name: "Test"
smtp:
  host: "smtp.example.com"
  pass: "yaml-secret"
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SMTP.Pass != "env-secret" {
		t.Errorf("SMTP.Pass = %q, want env override", cfg.SMTP.Pass)
	}
}

func TestPortAndDBPathFromEnv(t *testing.T) {
	t.Setenv("PATCHWORK_PORT", "8190")
	t.Setenv("PATCHWORK_DB_PATH", "data/e2e/patchwork.db")
	cfg, err := Load(writeConfig(t, `
instance:
  name: "Test"
server:
  port: "8080"
database:
  path: "data/patchwork.db"
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != "8190" {
		t.Errorf("Server.Port = %q, want env override", cfg.Server.Port)
	}
	if cfg.Database.Path != "data/e2e/patchwork.db" {
		t.Errorf("Database.Path = %q, want env override", cfg.Database.Path)
	}
}

func TestPortAndDBPathKeepYAMLWhenEnvUnset(t *testing.T) {
	cfg, err := Load(writeConfig(t, `
instance:
  name: "Test"
server:
  port: "8080"
database:
  path: "data/patchwork.db"
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != "8080" || cfg.Database.Path != "data/patchwork.db" {
		t.Errorf("empty env should leave YAML values, got port %q db %q", cfg.Server.Port, cfg.Database.Path)
	}
}

func TestWarningsForExampleDomain(t *testing.T) {
	cfg, err := Load(writeConfig(t, `
instance:
  name: "Test"
  domain: "patchwork.example.com"
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Warnings()) == 0 {
		t.Error("expected a warning for the example domain")
	}
}

func TestWarningsForLocalhostFederation(t *testing.T) {
	cfg, err := Load(writeConfig(t, `
instance:
  name: "Test"
  domain: "localhost:8080"
federation:
  enabled: true
`))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range cfg.Warnings() {
		if strings.Contains(w, "federation") {
			found = true
		}
	}
	if !found {
		t.Error("expected a federation warning for localhost domain")
	}
}

func TestNoWarningsForRealConfig(t *testing.T) {
	cfg, err := Load(writeConfig(t, `
instance:
  name: "Test"
  domain: "quilt.example.org"
federation:
  enabled: true
`))
	if err != nil {
		t.Fatal(err)
	}
	if ws := cfg.Warnings(); len(ws) != 0 {
		t.Errorf("expected no warnings, got %v", ws)
	}
}
