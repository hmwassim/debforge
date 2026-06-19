package config

import (
	"os"
	"path/filepath"
)

const (
	DefaultSourcesListPath    = "/etc/apt/sources.list"
	DefaultExtrepoConfigPath  = "/etc/extrepo/config.yaml"
	DefaultResolvConfPath     = "/etc/resolv.conf"
	DefaultStubResolvConfPath = "/run/systemd/resolve/stub-resolv.conf"
	DefaultFontDir            = "/usr/local/share/fonts"
	DefaultFontURL            = "https://codeberg.org/hmwassim/fonts/raw/branch/main/fonts.tar.gz"
	DefaultRepoURL            = "https://github.com/hmwassim/debforge"
	DefaultBranch             = "main"
	DefaultBinaryPath         = "/usr/local/bin/debforge"
	DefaultGoBinaryPath       = "go"
)

type Config struct {
	RootDir            string
	DataDir            string
	PackagesDir        string
	ConfigsDir         string
	StatesDir          string
	StateFile          string
	LogFile            string
	SourcesListPath    string
	ExtrepoConfigPath  string
	ResolvConfPath     string
	StubResolvConfPath string
	FontDir            string
	FontURL            string
	RepoURL            string
	Branch             string
	BinaryPath         string
	GoBinaryPath       string
	TempDir            string
}

func NewDefaultConfig() *Config {
	root := os.Getenv("DEBFORGE_ROOT")
	if root == "" {
		root = "/opt/debforge"
	}
	return &Config{
		RootDir:            root,
		DataDir:            filepath.Join(root, "data"),
		PackagesDir:        filepath.Join(root, "data", "packages"),
		ConfigsDir:         filepath.Join(root, "data", "configs"),
		StatesDir:          filepath.Join(root, "var", "states"),
		StateFile:          filepath.Join(root, "state.json"),
		LogFile:            filepath.Join(root, "debforge.log"),
		SourcesListPath:    DefaultSourcesListPath,
		ExtrepoConfigPath:  DefaultExtrepoConfigPath,
		ResolvConfPath:     DefaultResolvConfPath,
		StubResolvConfPath: DefaultStubResolvConfPath,
		FontDir:            DefaultFontDir,
		FontURL:            DefaultFontURL,
		RepoURL:            DefaultRepoURL,
		Branch:             DefaultBranch,
		BinaryPath:         DefaultBinaryPath,
		GoBinaryPath:       DefaultGoBinaryPath,
		TempDir:            "",
	}
}

func (c *Config) BinDir() string     { return filepath.Join(c.RootDir, "bin") }
func (c *Config) SourceDir() string  { return filepath.Join(c.RootDir, "src") }
func (c *Config) StateDir() string   { return filepath.Join(c.RootDir, "var") }
func (c *Config) CacheDir() string   { return filepath.Join(c.StateDir(), "cache") }
func (c *Config) GoPathDir() string  { return filepath.Join(c.StateDir(), "gopath") }
func (c *Config) GoCacheDir() string { return filepath.Join(c.GoPathDir(), "buildcache") }
func (c *Config) LockFile() string   { return filepath.Join(c.StateDir(), ".lock") }
