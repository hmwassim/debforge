// Package fs provides a concrete implementation of ports.FileSystem
// by delegating to the os and filepath packages.
package fs

import (
	"io"
	"os"
	"path/filepath"

	"github.com/hmwassim/debforge/internal/ports"
)

var _ ports.FileSystem = (*FileSystem)(nil)

// FileSystem implements ports.FileSystem by delegating to os and filepath.
type FileSystem struct{}

// NewFileSystem returns a new FileSystem.
func NewFileSystem() *FileSystem {
	return &FileSystem{}
}

// ReadFile reads the file at path and returns its contents.
func (f *FileSystem) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// WriteFile writes data to path with the given permission bits.
func (f *FileSystem) WriteFile(path string, data []byte, perm int) error {
	return os.WriteFile(path, data, os.FileMode(perm))
}

// RemoveAll removes path and any children it contains.
func (f *FileSystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

// MkdirAll creates a directory named path, along with any necessary parents.
func (f *FileSystem) MkdirAll(path string, perm int) error {
	return os.MkdirAll(path, os.FileMode(perm))
}

// MkdirTemp creates a temporary directory with the given pattern
// in the default temporary directory.
func (f *FileSystem) MkdirTemp(pattern string) (string, error) {
	return os.MkdirTemp("", pattern)
}

// Create creates or truncates the file at path and returns a WriteCloser.
func (f *FileSystem) Create(path string) (io.WriteCloser, error) {
	return os.Create(path)
}

// Stat returns a FileInfo describing the file at path.
func (f *FileSystem) Stat(path string) (ports.FileInfo, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	return &osFileInfo{fi: fi}, nil
}

// Glob returns the names of all files matching pattern.
func (f *FileSystem) Glob(pattern string) ([]string, error) {
	return filepath.Glob(pattern)
}

// Rename renames a file or directory from oldPath to newPath.
func (f *FileSystem) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

// Symlink creates target as a symbolic link to link.
func (f *FileSystem) Symlink(target, link string) error {
	return os.Symlink(target, link)
}

// Walk walks the file tree rooted at root, calling fn for each file or directory.
func (f *FileSystem) Walk(root string, fn func(path string, info ports.FileInfo, err error) error) error {
	return filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return fn(path, nil, err)
		}
		return fn(path, &osFileInfo{fi: fi}, err)
	})
}

// Exists reports whether a file or directory exists at path.
func (f *FileSystem) Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// Readlink returns the destination of the symbolic link at path.
func (f *FileSystem) Readlink(path string) (string, error) {
	return os.Readlink(path)
}

type osFileInfo struct {
	fi os.FileInfo
}

func (f *osFileInfo) Name() string { return f.fi.Name() }
func (f *osFileInfo) Size() int64  { return f.fi.Size() }
func (f *osFileInfo) IsDir() bool  { return f.fi.IsDir() }
