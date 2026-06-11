package settings

import "os"

const (
	RootDir    = "/opt/debforge"
	BinaryPath = "/usr/local/bin/debforge"
	BinDir     = RootDir + "/bin"
	SourceDir  = RootDir + "/src"
	StateDir   = RootDir + "/var"
	StateFile  = StateDir + "/state.json"
	CacheDir   = StateDir + "/cache"
	GoPathDir  = StateDir + "/gopath"

	RepoURL  = "https://github.com/hmwassim/debforge"
	Branch   = "main"
	RepoName = "hmwassim/debforge"
)

func EnsureDirsExist() error {
	for _, d := range []string{BinDir, SourceDir, StateDir, CacheDir, GoPathDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	return nil
}
