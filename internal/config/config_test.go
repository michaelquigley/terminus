package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadExplicitConfigDoesNotRequireDefaultFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cwd := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(previous)
	})

	configPath := filepath.Join(t.TempDir(), "terminus.yaml")
	if err := os.WriteFile(configPath, []byte(`canon_path: /tmp/canon
reviewer:
  name: dummy
  impl: dummy
`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConfigPath != configPath {
		t.Fatalf("ConfigPath = %q, expected %q", cfg.ConfigPath, configPath)
	}
}
