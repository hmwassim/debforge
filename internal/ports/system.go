package ports

import (
	"context"
	"io"
)

// RunOptions customizes how a command is executed. The zero value runs the
// command with the current process's environment, current working
// directory, and buffered (captured) stdout/stderr - i.e. the same
// behavior as calling Run directly.
type RunOptions struct {
	// Dir sets the command's working directory. Empty means inherit.
	Dir string
	// Env, if non-empty, is appended to the current process environment
	// (matching the append(os.Environ(), ...) convention used throughout
	// this codebase) rather than replacing it.
	Env []string
	// Stdout, if set, receives streamed stdout instead of it being
	// buffered and returned.
	Stdout io.Writer
	// Stderr, if set, receives streamed stderr instead of it being
	// buffered and returned.
	Stderr io.Writer
}

// CommandRunner executes external commands. Run is a convenience for the
// common case; RunWithOptions covers everything Run cannot (a working
// directory, extra environment variables, or live output streaming) so
// callers never need to fall back to os/exec directly.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)
	RunWithOptions(ctx context.Context, opts RunOptions, name string, args ...string) (stdout, stderr []byte, err error)
}

type FileInfo interface {
	Name() string
	Size() int64
	IsDir() bool
}

type FileSystem interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm int) error
	Create(path string) (io.WriteCloser, error)
	RemoveAll(path string) error
	MkdirAll(path string, perm int) error
	MkdirTemp(pattern string) (string, error)
	Stat(path string) (FileInfo, error)
	Glob(pattern string) ([]string, error)
	Walk(root string, fn func(path string, info FileInfo, err error) error) error
	Rename(oldPath, newPath string) error
	Symlink(target, link string) error
	Readlink(path string) (string, error)
	Exists(path string) (bool, error)
}

type Locker interface {
	Acquire(ctx context.Context, path string) (release func(), err error)
}

type System interface {
	IsPrivileged() bool
}
