package writeutil

import (
	"os"
	"os/exec"
	"path/filepath"
)

// AtomicFile writes data to path atomically: temp file → sync → rename.
func AtomicFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path))
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
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// SetImmutable adds or removes the immutable attribute via chattr.
func SetImmutable(path string, lock bool) error {
	op := "+i"
	if !lock {
		op = "-i"
	}
	return exec.Command("chattr", op, path).Run()
}
