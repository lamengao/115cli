package cmd

import (
	"time"

	"github.com/yibing/115cli/internal/config"
	"github.com/yibing/115cli/internal/open115"
)

func newClient() (*open115.Client, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	if cfg.Cookie != "" {
		return open115.NewCookie(cfg.Cookie, cfg.RootID), nil
	}
	return open115.New(cfg.RefreshToken, cfg.AccessToken, cfg.RootID, func(accessToken, refreshToken string) {
		cfg.AccessToken = accessToken
		cfg.RefreshToken = refreshToken
		cfg.LastTokenRefresh = time.Now()
		_ = config.Save(configPath, cfg)
	}), nil
}
