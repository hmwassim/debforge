package self

import (
	"path/filepath"

	"github.com/hmwassim/debforge/internal/deploy"
)

const (
	DefaultRepoURL  = "https://github.com/hmwassim/debforge"
	DefaultBranch   = "main"
	DefaultGoBinary = "go"
)

// Config holds every filesystem location debforge's self-management
// commands (--self-update, --self-remove) and bootstrap care about. All
// derived paths (state file, lock file, build/source/cache dirs) are
// computed here, once, with filepath.Join - so no other package needs to
// re-derive a path under RootDir by hand.
type Config struct {
	RootDir   string
	SourceDir string
	BinDir    string
	GoPath    string
	GoCache   string
	LinkPath  string
	RepoURL   string
	Branch    string
	GoBinary  string

	// PkgsDir is the directory searched for YAML package definitions
	// at startup. All .yaml files under it are preloaded into the
	// package registry so bare names (e.g. "steam") resolve without
	// needing a file path. Each subdirectory under PkgsDir is treated
	// as a type namespace (apt/, deb/, source/, config/).
	PkgsDir string

	// LockPath is the file used to serialize debforge operations
	// (install/remove/update/self-update/self-remove).
	LockPath string
	// StatePath is the JSON file recording installed packages.
	StatePath string
}

func DefaultConfig() *Config {
	root := deploy.DefaultRootDir
	varDir := filepath.Join(root, "var")
	goPath := filepath.Join(varDir, "gopath")
	sourceDir := filepath.Join(root, "src")
	return &Config{
		RootDir:   root,
		SourceDir: sourceDir,
		BinDir:    filepath.Join(root, "bin"),
		GoPath:    goPath,
		GoCache:   filepath.Join(goPath, "buildcache"),
		LinkPath:  deploy.DefaultLinkPath,
		RepoURL:   DefaultRepoURL,
		Branch:    DefaultBranch,
		GoBinary:  DefaultGoBinary,

		PkgsDir:   filepath.Join(sourceDir, "repo", "packages"),
		LockPath:  filepath.Join(varDir, "lock"),
		StatePath: filepath.Join(varDir, "states", "state.json"),
	}
}
