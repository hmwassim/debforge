package files

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/hmwassim/debforge/internal/state"
)

// Place copies a regular file (e.g. a staged build output or an AppImage)
// to dest atomically and returns its manifest record. These "plain" files
// are owned by debforge outright: they are replaced on every install. A
// pre-existing file debforge did not install (prev == nil) is backed up
// first.
func (e *Engine) Place(src, dest string, mode os.FileMode, prev *state.File) (state.File, error) {
	in, err := os.Open(src)
	if err != nil {
		return state.File{}, err
	}
	defer in.Close()
	rec := state.File{Mode: uint32(mode), Plain: true}
	if prev != nil {
		rec.Backup = prev.Backup
	}
	if err := e.backupForeign(dest, prev, &rec); err != nil {
		return state.File{}, err
	}
	if err := os.MkdirAll(filepath.Dir(e.Root+dest), 0o755); err != nil {
		return state.File{}, err
	}
	tmp := fmt.Sprintf("%s.debforge-tmp-%d", e.Root+dest, time.Now().UnixNano())
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return state.File{}, err
	}
	ok := false
	defer func() {
		if !ok {
			out.Close()
			os.Remove(tmp)
		}
	}()
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(out, h), in); err != nil {
		return state.File{}, err
	}
	if err := out.Chmod(mode); err != nil {
		return state.File{}, err
	}
	if err := out.Sync(); err != nil {
		return state.File{}, err
	}
	if err := out.Close(); err != nil {
		return state.File{}, err
	}
	if err := os.Rename(tmp, e.Root+dest); err != nil {
		return state.File{}, err
	}
	ok = true
	rec.SHA256 = hex.EncodeToString(h.Sum(nil))
	return rec, nil
}

// PlaceSymlink creates dest as a symlink to target.
func (e *Engine) PlaceSymlink(target, dest string, prev *state.File) (state.File, error) {
	rec := state.File{Plain: true, SHA256: Hash([]byte("symlink:" + target))}
	if prev != nil {
		rec.Backup = prev.Backup
	}
	if err := e.backupForeign(dest, prev, &rec); err != nil {
		return state.File{}, err
	}
	if err := os.MkdirAll(filepath.Dir(e.Root+dest), 0o755); err != nil {
		return state.File{}, err
	}
	tmp := fmt.Sprintf("%s.debforge-tmp-%d", e.Root+dest, time.Now().UnixNano())
	if err := os.Symlink(target, tmp); err != nil {
		return state.File{}, err
	}
	if err := os.Rename(tmp, e.Root+dest); err != nil {
		os.Remove(tmp)
		return state.File{}, err
	}
	return rec, nil
}

func (e *Engine) backupForeign(dest string, prev *state.File, rec *state.File) error {
	if prev != nil {
		return nil
	}
	if _, err := os.Lstat(e.Root + dest); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	bak, err := e.backup(Spec{Path: dest}, ".debforge-orig")
	if err != nil {
		return fmt.Errorf("back up %s: %w", dest, err)
	}
	rec.Backup = bak
	return nil
}

// plainHash hashes a plain file (or symlink) on disk without reading it
// into memory. exists is false if the path is absent.
func (e *Engine) plainHash(path string) (string, bool, error) {
	fi, err := os.Lstat(e.Root + path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t, err := os.Readlink(e.Root + path)
		if err != nil {
			return "", true, err
		}
		return Hash([]byte("symlink:" + t)), true, nil
	}
	f, err := os.Open(e.Root + path)
	if err != nil {
		return "", true, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", true, err
	}
	return hex.EncodeToString(h.Sum(nil)), true, nil
}

// RemoveRecord removes a managed file described by its manifest record.
// home is the target user's home for files inside it ("" otherwise).
func (e *Engine) RemoveRecord(path string, rec state.File, home string) (Outcome, error) {
	if !rec.Plain {
		spec := Spec{Path: path, UID: rec.UID, GID: rec.GID}
		if home != "" && within(path, home) {
			spec.Home = home
		}
		return e.Remove(spec, rec)
	}
	out := Outcome{Path: path}
	sum, exists, err := e.plainHash(path)
	if err != nil {
		return Outcome{}, err
	}
	switch {
	case !exists:
		out.Action = "missing"
	case sum != rec.SHA256:
		out.State, out.Action = UserModified, "kept"
		return out, nil
	default:
		if err := os.Remove(e.Root + path); err != nil {
			return Outcome{}, err
		}
		out.Action = "deleted"
	}
	if rec.Backup != "" {
		if err := os.Rename(e.Root+rec.Backup, e.Root+path); err == nil {
			out.Action = "restored"
		}
	}
	return out, nil
}

func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && (len(rel) < 3 || rel[:3] != "../")
}
