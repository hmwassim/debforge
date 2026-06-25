package fs

import (
	"io"
	"os"
	"path/filepath"

	"github.com/hmwassim/debforge/internal/ports"
)

var _ ports.FileSystem = (*FileSystem)(nil)

type FileSystem struct{}

func NewFileSystem() *FileSystem {
	return &FileSystem{}
}

func (f *FileSystem) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (f *FileSystem) WriteFile(path string, data []byte, perm int) error {
	return os.WriteFile(path, data, os.FileMode(perm))
}

func (f *FileSystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (f *FileSystem) MkdirAll(path string, perm int) error {
	return os.MkdirAll(path, os.FileMode(perm))
}

func (f *FileSystem) MkdirTemp(pattern string) (string, error) {
	return os.MkdirTemp("", pattern)
}

func (f *FileSystem) Create(path string) (io.WriteCloser, error) {
	return os.Create(path)
}

func (f *FileSystem) Stat(path string) (ports.FileInfo, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	return &osFileInfo{fi: fi}, nil
}

func (f *FileSystem) Glob(pattern string) ([]string, error) {
	return filepath.Glob(pattern)
}

func (f *FileSystem) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func (f *FileSystem) Symlink(target, link string) error {
	return os.Symlink(target, link)
}

func (f *FileSystem) Walk(root string, fn func(path string, info ports.FileInfo, err error) error) error {
	return filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return fn(path, nil, err)
		}
		return fn(path, &osFileInfo{fi: fi}, err)
	})
}

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

func (f *FileSystem) Readlink(path string) (string, error) {
	return os.Readlink(path)
}

type osFileInfo struct {
	fi os.FileInfo
}

func (f *osFileInfo) Name() string { return f.fi.Name() }
func (f *osFileInfo) Size() int64  { return f.fi.Size() }
func (f *osFileInfo) IsDir() bool  { return f.fi.IsDir() }
