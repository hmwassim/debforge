package self

const (
	DefaultRootDir  = "/opt/debforge"
	DefaultRepoURL  = "https://github.com/hmwassim/debforge"
	DefaultBranch   = "main"
	DefaultGoBinary = "go"
	DefaultLinkPath = "/usr/local/bin/debforge"
)

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
}

func DefaultConfig() *Config {
	root := DefaultRootDir
	return &Config{
		RootDir:   root,
		SourceDir: root + "/src",
		BinDir:    root + "/bin",
		GoPath:    root + "/var/gopath",
		GoCache:   root + "/var/gopath/buildcache",
		LinkPath:  DefaultLinkPath,
		RepoURL:   DefaultRepoURL,
		Branch:    DefaultBranch,
		GoBinary:  DefaultGoBinary,
	}
}
