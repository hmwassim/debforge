package ports

import "os"

type FileReader interface {
	ReadFile(name string) ([]byte, error)
	ReadDir(name string) ([]os.DirEntry, error)
	Stat(name string) (os.FileInfo, error)
}

type FileWriter interface {
	WriteFile(name string, data []byte, perm os.FileMode) error
	AtomicWriteFile(name string, data []byte, perm os.FileMode) error
}

type FileManager interface {
	MkdirAll(path string, perm os.FileMode) error
	MkdirTemp(dir, pattern string) (string, error)
	RemoveAll(path string) error
	Chmod(name string, mode os.FileMode) error
	Rename(oldPath, newPath string) error
}

type SymlinkManager interface {
	Lstat(name string) (os.FileInfo, error)
	Readlink(name string) (string, error)
	Symlink(target, link string) error
}

type FileSystem interface {
	FileReader
	FileWriter
	FileManager
	SymlinkManager
}
