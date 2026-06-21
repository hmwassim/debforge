package self

import "path/filepath"

const (
	DefaultRootDir  = "/opt/debforge"
	DefaultRepoURL  = "https://github.com/hmwassim/debforge"
	DefaultBranch   = "main"
	DefaultGoBinary = "go"
	DefaultLinkPath = "/usr/local/bin/debforge"
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

	// LockPath is the file used to serialize debforge operations
	// (install/remove/update/self-update/self-remove).
	LockPath string
	// StatePath is the JSON file recording installed packages.
	StatePath string
}

func DefaultConfig() *Config {
	root := DefaultRootDir
	varDir := filepath.Join(root, "var")
	goPath := filepath.Join(varDir, "gopath")
	return &Config{
		RootDir:   root,
		SourceDir: filepath.Join(root, "src"),
		BinDir:    filepath.Join(root, "bin"),
		GoPath:    goPath,
		GoCache:   filepath.Join(goPath, "buildcache"),
		LinkPath:  DefaultLinkPath,
		RepoURL:   DefaultRepoURL,
		Branch:    DefaultBranch,
		GoBinary:  DefaultGoBinary,

		LockPath:  filepath.Join(varDir, "lock"),
		StatePath: filepath.Join(varDir, "states", "state.json"),
	}
}
