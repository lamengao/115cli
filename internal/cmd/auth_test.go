package cmd

import (
	"path/filepath"
	"testing"

	"github.com/yibing/115cli/internal/config"
)

func TestAuthCookieCommandSavesPositionalCookie(t *testing.T) {
	oldConfigPath := configPath
	oldAuthRootID := authRootID
	t.Cleanup(func() {
		configPath = oldConfigPath
		authRootID = oldAuthRootID
	})

	configPath = filepath.Join(t.TempDir(), "config.json")
	authRootID = "123"
	cookie := "UID=test; CID=test; SEID=test; KID=test"

	if err := authCookieCmd.RunE(authCookieCmd, []string{cookie}); err != nil {
		t.Fatalf("auth cookie failed: %v", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Cookie != cookie {
		t.Fatalf("cookie = %q, want %q", cfg.Cookie, cookie)
	}
	if cfg.RootID != "123" {
		t.Fatalf("root id = %q, want 123", cfg.RootID)
	}
}

func TestAuthCookieCommandRequiresPositionalCookie(t *testing.T) {
	if err := authCookieCmd.Args(authCookieCmd, nil); err == nil {
		t.Fatal("expected missing cookie arg to fail")
	}
	if flag := authCookieCmd.Flags().Lookup("cookie"); flag != nil {
		t.Fatal("did not expect --cookie flag to be registered")
	}
}
