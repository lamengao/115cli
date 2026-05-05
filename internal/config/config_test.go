package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadConfigDefaultsRootID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	err := Save(path, &Config{RefreshToken: "rt"})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RefreshToken != "rt" {
		t.Fatalf("refresh token = %q", cfg.RefreshToken)
	}
	if cfg.RootID != "0" {
		t.Fatalf("root id = %q", cfg.RootID)
	}
}

func TestDefaultPathUsesDotConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg"))

	got, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".config", "115cli", "config.json")
	if got != want {
		t.Fatalf("DefaultPath() = %q, want %q", got, want)
	}

	if _, err := os.Stat(filepath.Dir(got)); !os.IsNotExist(err) {
		t.Fatalf("DefaultPath should not create directories, stat err = %v", err)
	}
}
