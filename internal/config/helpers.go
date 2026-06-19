package config

import (
	"context"
	"os"

	"github.com/hmwassim/debforge/internal/ports"
)

func EnsureDirsExist(cfg *Config, fs ports.FileSystem) error {
	for _, d := range []string{cfg.BinDir(), cfg.SourceDir()} {
		if err := fs.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	for _, d := range []string{cfg.StateDir(), cfg.StatesDir, cfg.CacheDir(), cfg.GoPathDir(), cfg.GoCacheDir()} {
		if err := fs.MkdirAll(d, 0700); err != nil {
			return err
		}
	}
	return nil
}

func GoCacheClean(ctx context.Context, cfg *Config, runner ports.CommandRunner) error {
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"GOPATH=" + cfg.GoPathDir(),
		"GOMODCACHE=" + cfg.GoPathDir() + "/mod",
		"GOCACHE=" + cfg.GoCacheDir(),
	}
	_, _, err := runner.RunWithEnv(ctx, "", env, cfg.GoBinaryPath, "clean", "-cache")
	return err
}
