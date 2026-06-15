package settings

import (
	"os"
	"os/exec"

	"github.com/hmwassim/debforge/pkg/executil"
)

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
	for _, d := range []string{c.BinDir(), c.SourceDir()} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	for _, d := range []string{c.StateDir(), c.CacheDir(), c.GoPathDir(), c.GoCacheDir()} {
		if err := os.MkdirAll(d, 0700); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) GoCacheClean() error {
	cmd := exec.Command("go", "clean", "-cache")
	cmd.Env = []string{
		"PATH=/usr/local/go/bin:/usr/bin:/bin",
		"GOPATH=" + c.GoPathDir(),
		"GOMODCACHE=" + c.GoPathDir() + "/mod",
		"GOCACHE=" + c.GoCacheDir(),
	}
	return executil.Run(cmd)
}
