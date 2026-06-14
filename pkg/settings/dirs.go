package settings

import "os"

type Config struct {
	RootDir    string
	BinaryPath string
	RepoURL    string
	Branch     string
}

var Default = &Config{
	RootDir:    "/opt/debforge",
	BinaryPath: "/usr/local/bin/debforge",
	RepoURL:    "https://github.com/hmwassim/debforge",
	Branch:     "main",
}

func (c *Config) BinDir() string      { return c.RootDir + "/bin" }
func (c *Config) SourceDir() string   { return c.RootDir + "/src" }
func (c *Config) StateDir() string    { return c.RootDir + "/var" }
func (c *Config) StateFile() string   { return c.StateDir() + "/state.json" }
func (c *Config) CacheDir() string    { return c.StateDir() + "/cache" }
func (c *Config) GoPathDir() string   { return c.StateDir() + "/gopath" }
func (c *Config) GoCacheDir() string  { return c.GoPathDir() + "/buildcache" }
func (c *Config) LockFile() string    { return c.StateDir() + "/.lock" }

func (c *Config) EnsureDirsExist() error {
	for _, d := range []string{c.BinDir(), c.SourceDir(), c.StateDir(), c.CacheDir(), c.GoPathDir(), c.GoCacheDir()} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	return nil
}
