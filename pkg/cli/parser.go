package cli

import (
	"fmt"
	"io"
	"os"
)

var Version = "0.1.0-dev"

type Operation string

const (
	OpSelfUpdate Operation = "self-update"
	OpSelfRemove Operation = "self-remove"
	OpVersion    Operation = "version"
	OpHelp       Operation = "help"
	OpCore       Operation = "core"
	OpInstall    Operation = "install"
	OpRemove     Operation = "remove"
	OpList       Operation = "list"
)

type ParseResult struct {
	Op   Operation
	Args []string
}

func Parse() (*ParseResult, error) {
	args := os.Args[1:]
	if len(args) == 0 {
		return &ParseResult{Op: OpHelp}, nil
	}

	switch op := args[0]; op {
	case "--self-update":
		return &ParseResult{Op: OpSelfUpdate}, nil
	case "--self-remove":
		return &ParseResult{Op: OpSelfRemove}, nil
	case "--version", "-V":
		return &ParseResult{Op: OpVersion}, nil
	case "--help", "-h":
		return &ParseResult{Op: OpHelp}, nil
	case "core":
		if len(args) < 2 {
			return nil, fmt.Errorf("core requires a subcommand: setup, list")
		}
		return &ParseResult{Op: OpCore, Args: args[1:]}, nil
	case "install":
		if len(args) < 2 {
			return nil, fmt.Errorf("install requires a package name")
		}
		return &ParseResult{Op: OpInstall, Args: args[1:]}, nil
	case "remove":
		if len(args) < 2 {
			return nil, fmt.Errorf("remove requires a package name")
		}
		return &ParseResult{Op: OpRemove, Args: args[1:]}, nil
	case "list":
		return &ParseResult{Op: OpList}, nil
	default:
		return nil, fmt.Errorf("unknown command: %s", op)
	}
}

func PrintUsage(w io.Writer) {
	name := "debforge"
	fmt.Fprintf(w, `%s – Manage packages outside Debian repositories

Usage:
  %[1]s install <package>...     Install packages from external repos
  %[1]s remove <package>...      Remove packages and repo sources
  %[1]s list                     List managed packages
  %[1]s core setup               Set up core packages and configs
  %[1]s core setup -f, --force   Force re-apply all core packages and configs
  %[1]s core list                List core packages and status
  %[1]s --self-update            Install or update debforge
  %[1]s --self-remove            Remove debforge and all data
  %[1]s --version, -V            Show version
  %[1]s --help, -h               Show this help message
`,
		name,
	)
}

func PrintVersion() {
	fmt.Printf("debforge %s\n", Version)
}
