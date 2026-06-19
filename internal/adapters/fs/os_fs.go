package fs

import (
	"os"
	"path/filepath"

	"github.com/hmwassim/debforge/internal/ports"
)

type OSFileSystem struct{}

func NewOSFileSystem() *OSFileSystem {
	return &OSFileSystem{}
}

func (f *OSFileSystem) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func (f *OSFileSystem) ReadDir(name string) ([]os.DirEntry, error) {
	return os.ReadDir(name)
}

func (f *OSFileSystem) WriteFile(name string, data []byte, perm os.FileMode) error {
	return os.WriteFile(name, data, perm)
}

func (f *OSFileSystem) AtomicWriteFile(name string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(name)
	tmp, err := os.CreateTemp(dir, filepath.Base(name))
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), name); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func (f *OSFileSystem) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func (f *OSFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (f *OSFileSystem) MkdirTemp(dir, pattern string) (string, error) {
	return os.MkdirTemp(dir, pattern)
}

func (f *OSFileSystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (f *OSFileSystem) Chmod(name string, mode os.FileMode) error {
	return os.Chmod(name, mode)
}

func (f *OSFileSystem) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func (f *OSFileSystem) Lstat(name string) (os.FileInfo, error) {
	return os.Lstat(name)
}

func (f *OSFileSystem) Readlink(name string) (string, error) {
	return os.Readlink(name)
}

func (f *OSFileSystem) Symlink(target, link string) error {
	return os.Symlink(target, link)
}

var (
	_ ports.FileReader     = (*OSFileSystem)(nil)
	_ ports.FileWriter     = (*OSFileSystem)(nil)
	_ ports.FileManager    = (*OSFileSystem)(nil)
	_ ports.SymlinkManager = (*OSFileSystem)(nil)
	_ ports.FileSystem     = (*OSFileSystem)(nil)
)
