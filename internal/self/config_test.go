package self

import (
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.RootDir != DefaultRootDir {
		t.Errorf("RootDir = %q, want %q", cfg.RootDir, DefaultRootDir)
	}
	if cfg.RepoURL != DefaultRepoURL {
		t.Errorf("RepoURL = %q, want %q", cfg.RepoURL, DefaultRepoURL)
	}
	if cfg.Branch != DefaultBranch {
		t.Errorf("Branch = %q, want %q", cfg.Branch, DefaultBranch)
	}
	if cfg.GoBinary != DefaultGoBinary {
		t.Errorf("GoBinary = %q, want %q", cfg.GoBinary, DefaultGoBinary)
	}
	if cfg.LinkPath != DefaultLinkPath {
		t.Errorf("LinkPath = %q, want %q", cfg.LinkPath, DefaultLinkPath)
	}

	wantBinDir := filepath.Join(DefaultRootDir, "bin")
	if cfg.BinDir != wantBinDir {
		t.Errorf("BinDir = %q, want %q", cfg.BinDir, wantBinDir)
	}

	wantSourceDir := filepath.Join(DefaultRootDir, "src")
	if cfg.SourceDir != wantSourceDir {
		t.Errorf("SourceDir = %q, want %q", cfg.SourceDir, wantSourceDir)
	}

	wantGoPath := filepath.Join(DefaultRootDir, "var", "gopath")
	if cfg.GoPath != wantGoPath {
		t.Errorf("GoPath = %q, want %q", cfg.GoPath, wantGoPath)
	}

	wantGoCache := filepath.Join(wantGoPath, "buildcache")
	if cfg.GoCache != wantGoCache {
		t.Errorf("GoCache = %q, want %q", cfg.GoCache, wantGoCache)
	}

	wantPkgsDir := filepath.Join(wantSourceDir, "repo", "packages")
	if cfg.PkgsDir != wantPkgsDir {
		t.Errorf("PkgsDir = %q, want %q", cfg.PkgsDir, wantPkgsDir)
	}

	wantLockPath := filepath.Join(DefaultRootDir, "var", "lock")
	if cfg.LockPath != wantLockPath {
		t.Errorf("LockPath = %q, want %q", cfg.LockPath, wantLockPath)
	}

	wantStatePath := filepath.Join(DefaultRootDir, "var", "states", "packages.state.json")
	if cfg.StatePath != wantStatePath {
		t.Errorf("StatePath = %q, want %q", cfg.StatePath, wantStatePath)
	}
}
