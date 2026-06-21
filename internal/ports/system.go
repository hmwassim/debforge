package ports

import "context"

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)
}

type FileInfo interface {
	Name() string
	Size() int64
	IsDir() bool
}

type FileSystem interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm int) error
	RemoveAll(path string) error
	MkdirAll(path string, perm int) error
	Stat(path string) (FileInfo, error)
	Glob(pattern string) ([]string, error)
}

type Locker interface {
	Acquire(ctx context.Context, path string) (release func(), err error)
}
