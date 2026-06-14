package cli

import (
	"fmt"
	"io"
	"os"
)

const Version = "0.1.0-dev"

type Operation string

const (
	OpSelfUpdate Operation = "self-update"
	OpSelfRemove Operation = "self-remove"
	OpVersion    Operation = "version"
	OpHelp       Operation = "help"
	OpCore       Operation = "core"
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
			return nil, fmt.Errorf("core requires a subcommand: update, repair, list")
		}
		return &ParseResult{Op: OpCore, Args: args[1:]}, nil
	default:
		return nil, fmt.Errorf("unknown flag: %s", op)
	}
}

func PrintUsage(w io.Writer) {
	name := "debforge"
	fmt.Fprintf(w, `%s – Manage packages outside Debian repositories

Usage:
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
