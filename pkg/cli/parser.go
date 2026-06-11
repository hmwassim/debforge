package cli

import (
	"fmt"
	"os"
)

const Version = "0.1.0-dev"

type Operation string

const (
	OpSelfUpdate Operation = "self-update"
	OpSelfRemove Operation = "self-remove"
	OpVersion    Operation = "version"
	OpHelp       Operation = "help"
)

type ParseResult struct {
	Op Operation
}

func Parse() (*ParseResult, error) {
	args := os.Args[1:]
	if len(args) == 0 {
		return &ParseResult{Op: OpHelp}, nil
	}

	op := args[0]
	switch op {
	case "--self-update":
		return &ParseResult{Op: OpSelfUpdate}, nil
	case "--self-remove":
		return &ParseResult{Op: OpSelfRemove}, nil
	case "--version", "-V", "-v":
		return &ParseResult{Op: OpVersion}, nil
	case "--help", "-h":
		return &ParseResult{Op: OpHelp}, nil
	default:
		return nil, fmt.Errorf("unknown flag: %s", op)
	}
}

func PrintUsage() {
	name := "debforge"
	fmt.Fprintf(os.Stderr, `%s – Manage packages outside Debian repositories

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
