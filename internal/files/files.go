// Package files writes, tracks and removes the files debforge manages,
// using a 3-way merge between the recorded baseline (what debforge last
// wrote), the file on disk, and the new content.
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
	"strings"
	"syscall"
	"time"

	"github.com/hmwassim/debforge/internal/state"
)

// State is the merge classification of one file.
type State int

const (
	Missing      State = iota // not on disk
	Adoptable                 // no baseline, disk already equals new content
	Foreign                   // no baseline, disk differs: someone else's file
	Current                   // disk equals new content
	Outdated                  // disk equals baseline, new content differs
	UserModified              // disk differs from baseline, new equals baseline
	Conflict                  // disk, baseline and new all differ
)

func (s State) String() string {
	return [...]string{"missing", "adoptable", "foreign", "current", "outdated", "user-modified", "conflict"}[s]
}

// Spec is a file to manage.
type Spec struct {
	Path string // absolute, as seen on the real system
	Data []byte
	Mode os.FileMode
	UID  int
	GID  int
	// Home, when set, marks a user file: Path must be inside Home, missing
	// directories are created owned by UID:GID, and no symlink below Home
	// is ever followed.
	Home  string
	Plain bool
}

// Outcome describes what Apply or Remove did.
type Outcome struct {
	Path   string
	State  State
	Action string // written, adopted, unchanged, kept, sidecar, replaced, deleted, restored
	Note   string // extra path (sidecar/backup) for the summary
	Record state.File
}

// Engine performs file operations. Root is prepended to every path; it is
// "" in production and a temp dir in tests.
type Engine struct {
	Root string
	Now  func() time.Time
}

// Hash returns the hex sha256 of b.
func Hash(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// Classify determines the merge state of spec against its baseline record
// (nil when debforge has never written the file).
func (e *Engine) Classify(spec Spec, rec *state.File) (State, error) {
	disk, exists, err := e.read(spec)
	if err != nil {
		return 0, err
	}
	return classify(disk, exists, Hash(spec.Data), rec), nil
}

func classify(disk []byte, exists bool, newHash string, rec *state.File) State {
	if !exists {
		return Missing
	}
	d := Hash(disk)
	if rec == nil || rec.SHA256 == "" {
		if d == newHash {
			return Adoptable
		}
		return Foreign
	}
	switch {
	case d == newHash:
		return Current
	case d == rec.SHA256 && newHash != rec.SHA256:
		return Outdated
	case d == rec.SHA256:
		return Current
	case newHash == rec.SHA256:
		return UserModified
	default:
		return Conflict
	}
}

// Apply installs spec. With force, user-modified and conflicting files are
// backed up and replaced instead of kept.
func (e *Engine) Apply(spec Spec, rec *state.File, force bool) (Outcome, error) {
	st, err := e.Classify(spec, rec)
	if err != nil {
		return Outcome{}, err
	}
	out := Outcome{Path: spec.Path, State: st}
	newRec := state.File{SHA256: Hash(spec.Data), Mode: uint32(spec.Mode), UID: spec.UID, GID: spec.GID, Plain: spec.Plain}
	if rec != nil {
		newRec.Backup = rec.Backup
	}

	switch {
	case st == Missing || st == Outdated:
		out.Action = "written"
		err = e.write(spec, spec.Path, spec.Data)
	case st == Adoptable:
		out.Action = "adopted"
		err = e.fixMeta(spec)
	case st == Current:
		out.Action = "unchanged"
		err = e.fixMeta(spec)
	case st == Foreign || force && (st == UserModified || st == Conflict):
		suffix := ".debforge-orig"
		if st != Foreign {
			suffix = ".debforge-old"
		}
		var bak string
		bak, err = e.backup(spec, suffix)
		if err == nil {
			err = e.write(spec, spec.Path, spec.Data)
		}
		out.Action, out.Note = "replaced", bak
		if st == Foreign {
			newRec.Backup = bak
		}
	case st == UserModified:
		out.Action = "kept"
		newRec.SHA256 = rec.SHA256
	case st == Conflict:
		// The user has now been shown this version: recording it as the
		// baseline means their eventual merge is respected next time.
		side := spec.Path + ".debforge-new"
		out.Action, out.Note = "sidecar", side
		err = e.write(spec, side, spec.Data)
	}
	if err != nil {
		return Outcome{}, fmt.Errorf("%s: %w", spec.Path, err)
	}
	out.Record = newRec
	return out, nil
}

// Remove deletes a managed file if it is unmodified, restoring any backup
// of a foreign file it replaced. Modified files are kept.
func (e *Engine) Remove(spec Spec, rec state.File) (Outcome, error) {
	disk, exists, err := e.read(spec)
	if err != nil {
		return Outcome{}, err
	}
	out := Outcome{Path: spec.Path}
	if !exists {
		out.Action = "missing"
		if rec.Backup != "" {
			if err := e.rename(spec, rec.Backup, spec.Path); err == nil {
				out.Action = "restored"
			}
		}
		return out, nil
	}
	if Hash(disk) != rec.SHA256 {
		out.State, out.Action = UserModified, "kept"
		if rec.Backup != "" {
			out.Note = rec.Backup
		}
		return out, nil
	}
	if rec.Backup != "" {
		if err := e.rename(spec, rec.Backup, spec.Path); err != nil {
			return Outcome{}, fmt.Errorf("restore %s: %w", rec.Backup, err)
		}
		out.Action = "restored"
		return out, nil
	}
	if err := e.unlink(spec, spec.Path); err != nil {
		return Outcome{}, fmt.Errorf("remove %s: %w", spec.Path, err)
	}
	out.Action = "deleted"
	return out, nil
}

// Read returns the current content of spec.Path (no symlinks followed for
// user files).
func (e *Engine) Read(spec Spec) ([]byte, bool, error) { return e.read(spec) }

// Write atomically writes spec.Data to spec.Path with its mode and owner,
// without any merge logic (used for managed blocks inside user files).
func (e *Engine) Write(spec Spec) error { return e.write(spec, spec.Path, spec.Data) }

// EnsureDir creates directory spec.Path (and parents below spec.Home,
// owned by spec.UID:GID for user directories).
func (e *Engine) EnsureDir(spec Spec) error {
	probe := spec
	probe.Path = filepath.Join(spec.Path, ".debforge-probe")
	fd, _, err := e.dir(probe, probe.Path)
	if err != nil {
		return err
	}
	return syscall.Close(fd)
}

// Delete removes a path unconditionally (legacy cleanup). Missing is fine.
func (e *Engine) Delete(path string) error {
	err := os.Remove(e.Root + path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// ---- low-level ----------------------------------------------------------

// dir opens the parent directory of p, creating missing directories. For
// user files the walk starts at Home and refuses symlinks.
func (e *Engine) dir(spec Spec, p string) (int, string, error) {
	parent, base := filepath.Split(p)
	parent = filepath.Clean(parent)
	if spec.Home == "" {
		if err := os.MkdirAll(e.Root+parent, 0o755); err != nil {
			return -1, "", err
		}
		fd, err := syscall.Open(e.Root+parent, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
		return fd, base, err
	}
	home := filepath.Clean(spec.Home)
	rel, err := filepath.Rel(home, parent)
	if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
		return -1, "", fmt.Errorf("%s is outside %s", p, home)
	}
	fd, err := syscall.Open(e.Root+home, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return -1, "", err
	}
	if rel == "." {
		return fd, base, nil
	}
	for _, comp := range strings.Split(rel, "/") {
		if err := syscall.Mkdirat(fd, comp, 0o755); err == nil {
			if err := syscall.Fchownat(fd, comp, spec.UID, spec.GID, atSymlinkNoFollow); err != nil {
				syscall.Close(fd)
				return -1, "", err
			}
		} else if !errors.Is(err, syscall.EEXIST) {
			syscall.Close(fd)
			return -1, "", err
		}
		nfd, err := syscall.Openat(fd, comp, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		syscall.Close(fd)
		if err != nil {
			if errors.Is(err, syscall.ELOOP) || errors.Is(err, syscall.ENOTDIR) {
				return -1, "", fmt.Errorf("refusing to follow symlink or non-directory at %s", comp)
			}
			return -1, "", err
		}
		fd = nfd
	}
	return fd, base, nil
}

// atSymlinkNoFollow is AT_SYMLINK_NOFOLLOW from <fcntl.h>.
const atSymlinkNoFollow = 0x100

func (e *Engine) read(spec Spec) ([]byte, bool, error) {
	if spec.Home == "" {
		b, err := os.ReadFile(e.Root + spec.Path)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		return b, err == nil, err
	}
	parent, base := filepath.Split(spec.Path)
	rel, err := filepath.Rel(filepath.Clean(spec.Home), filepath.Clean(parent))
	if err != nil || strings.HasPrefix(rel, "..") {
		return nil, false, fmt.Errorf("%s is outside %s", spec.Path, spec.Home)
	}
	fd, err := syscall.Open(e.Root+filepath.Clean(spec.Home), syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, false, err
	}
	comps := []string{}
	if rel != "." {
		comps = strings.Split(rel, "/")
	}
	for _, c := range comps {
		nfd, err := syscall.Openat(fd, c, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		syscall.Close(fd)
		if errors.Is(err, syscall.ENOENT) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, fmt.Errorf("refusing to follow symlink or non-directory at %s", c)
		}
		fd = nfd
	}
	defer syscall.Close(fd)
	ffd, err := syscall.Openat(fd, base, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if errors.Is(err, syscall.ENOENT) {
		return nil, false, nil
	}
	if errors.Is(err, syscall.ELOOP) {
		// A symlink where we expect a regular file: treat as foreign
		// content that will be moved aside.
		return []byte("symlink"), true, nil
	}
	if err != nil {
		return nil, false, err
	}
	f := os.NewFile(uintptr(ffd), base)
	defer f.Close()
	b, err := io.ReadAll(f)
	return b, true, err
}

func (e *Engine) write(spec Spec, p string, data []byte) error {
	dfd, base, err := e.dir(spec, p)
	if err != nil {
		return err
	}
	defer syscall.Close(dfd)
	tmp := fmt.Sprintf(".%s.debforge-tmp-%d", base, time.Now().UnixNano())
	fd, err := syscall.Openat(dfd, tmp, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0o600)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), tmp)
	ok := false
	defer func() {
		if !ok {
			f.Close()
			syscall.Unlinkat(dfd, tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Chown(spec.UID, spec.GID); err != nil && !isPermOnTest(err) {
		return err
	}
	if err := f.Chmod(spec.Mode.Perm()); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := syscall.Renameat(dfd, tmp, dfd, base); err != nil {
		return err
	}
	ok = true
	_ = syscall.Fsync(dfd)
	return nil
}

// isPermOnTest tolerates EPERM from chown when not running as root, so the
// engine is testable unprivileged; as root chown never fails this way.
func isPermOnTest(err error) bool {
	return os.Geteuid() != 0 && errors.Is(err, syscall.EPERM)
}

func (e *Engine) fixMeta(spec Spec) error {
	dfd, base, err := e.dir(spec, spec.Path)
	if err != nil {
		return err
	}
	defer syscall.Close(dfd)
	fd, err := syscall.Openat(dfd, base, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)
	if err := syscall.Fchown(fd, spec.UID, spec.GID); err != nil && !isPermOnTest(err) {
		return err
	}
	return syscall.Fchmod(fd, uint32(spec.Mode.Perm()))
}

func (e *Engine) backup(spec Spec, suffix string) (string, error) {
	dfd, base, err := e.dir(spec, spec.Path)
	if err != nil {
		return "", err
	}
	defer syscall.Close(dfd)
	dirPath := e.Root + filepath.Dir(spec.Path)
	name := base + suffix
	for i := 1; ; i++ {
		if _, err := os.Lstat(filepath.Join(dirPath, name)); errors.Is(err, fs.ErrNotExist) {
			break
		}
		name = fmt.Sprintf("%s%s.%d", base, suffix, i)
	}
	if err := syscall.Renameat(dfd, base, dfd, name); err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(spec.Path), name), nil
}

func (e *Engine) rename(spec Spec, from, to string) error {
	if filepath.Dir(from) != filepath.Dir(to) {
		return fmt.Errorf("backup %s is not next to %s", from, to)
	}
	dfd, base, err := e.dir(spec, to)
	if err != nil {
		return err
	}
	defer syscall.Close(dfd)
	return syscall.Renameat(dfd, filepath.Base(from), dfd, base)
}

func (e *Engine) unlink(spec Spec, p string) error {
	dfd, base, err := e.dir(spec, p)
	if err != nil {
		return err
	}
	defer syscall.Close(dfd)
	err = syscall.Unlinkat(dfd, base)
	if errors.Is(err, syscall.ENOENT) {
		return nil
	}
	return err
}
