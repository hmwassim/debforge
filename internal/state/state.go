// Package state persists what debforge has installed: one manifest per
// package listing every apt package, .deb, extrepo and file it put on the
// system, so removal can undo exactly that and nothing else.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Schema is the current on-disk schema version.
const Schema = 2

// State is the whole state file.
type State struct {
	Schema   int                 `json:"schema"`
	Packages map[string]*Package `json:"packages"`
	Setup    Setup               `json:"setup"`
}

// Package is the install manifest of one debforge package.
type Package struct {
	Kind        string    `json:"kind"`
	Version     string    `json:"version,omitempty"`
	Variant     string    `json:"variant,omitempty"`
	Explicit    bool      `json:"explicit"`
	InstalledAt time.Time `json:"installed_at"`
	// DefHash is the hash of the definition that was applied; a change
	// means files/hooks may need re-applying even if Version is the same.
	DefHash string `json:"def_hash,omitempty"`
	// Incomplete is set when the apt part of an install succeeded but a
	// later step (build, files, hooks) did not; the next update redoes it.
	Incomplete bool `json:"incomplete,omitempty"`

	// AptPackages are apt packages this package asked for (payload or
	// prerequisites) that were NOT already present/manual beforehand.
	AptPackages []string `json:"apt_packages,omitempty"`
	// Debs are dpkg package names installed from downloaded .deb files.
	Debs []string `json:"debs,omitempty"`
	// Extrepos enabled for this package.
	Extrepos []string `json:"extrepos,omitempty"`
	// Files written by this package, keyed by absolute path.
	Files map[string]File `json:"files,omitempty"`
}

// File records the baseline of a file debforge wrote.
type File struct {
	// SHA256 of the content debforge last wrote or adopted (the 3-way
	// merge baseline).
	SHA256 string `json:"sha256"`
	Mode   uint32 `json:"mode"`
	UID    int    `json:"uid"`
	GID    int    `json:"gid"`
	// Backup is the path of a pre-existing foreign file debforge moved
	// aside before writing; it is restored on removal.
	Backup string `json:"backup,omitempty"`
	// Plain marks files debforge owns outright (binaries, desktop entries)
	// as opposed to user-editable configuration.
	Plain bool `json:"plain,omitempty"`
}

// Setup holds setup-step bookkeeping.
type Setup struct {
	Steps map[string]Step `json:"steps,omitempty"`
	Files map[string]File `json:"files,omitempty"`
}

// Step records the last successful apply of a setup step.
type Step struct {
	AppliedAt time.Time `json:"applied_at"`
}

// New returns an empty state.
func New() *State {
	return &State{
		Schema:   Schema,
		Packages: map[string]*Package{},
		Setup:    Setup{Steps: map[string]Step{}, Files: map[string]File{}},
	}
}

// Names returns installed package names, sorted.
func (s *State) Names() []string {
	out := make([]string, 0, len(s.Packages))
	for n := range s.Packages {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// ClaimedApt reports whether any package other than except lists apt
// package name in its manifest.
func (s *State) ClaimedApt(name, except string) bool {
	for n, p := range s.Packages {
		if n == except {
			continue
		}
		for _, a := range p.AptPackages {
			if a == name {
				return true
			}
		}
	}
	return false
}

// ClaimedExtrepo reports whether any package other than except uses repo.
func (s *State) ClaimedExtrepo(repo, except string) bool {
	for n, p := range s.Packages {
		if n == except {
			continue
		}
		for _, r := range p.Extrepos {
			if r == repo {
				return true
			}
		}
	}
	return false
}

// Store reads and writes the state file.
type Store struct {
	Path string
	Now  func() time.Time
}

// Load reads the state. A missing file yields an empty state. A corrupt
// file is moved aside and an empty state is returned together with a
// non-nil warning; err is only set for I/O failures such as EACCES.
func (s Store) Load() (st *State, warning error, err error) {
	b, err := os.ReadFile(s.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return New(), nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read state: %w", err)
	}
	st = New()
	if jerr := json.Unmarshal(b, st); jerr != nil || st.Schema != Schema {
		reason := "unsupported schema"
		if jerr != nil {
			reason = jerr.Error()
		}
		now := time.Now
		if s.Now != nil {
			now = s.Now
		}
		aside := fmt.Sprintf("%s.corrupt-%s", s.Path, now().Format("20060102-150405"))
		if rerr := os.Rename(s.Path, aside); rerr != nil {
			return nil, nil, fmt.Errorf("state file is unreadable (%s) and could not be moved aside: %w", reason, rerr)
		}
		return New(), fmt.Errorf("state file was unreadable (%s); moved to %s and starting empty", reason, aside), nil
	}
	if st.Packages == nil {
		st.Packages = map[string]*Package{}
	}
	if st.Setup.Steps == nil {
		st.Setup.Steps = map[string]Step{}
	}
	if st.Setup.Files == nil {
		st.Setup.Files = map[string]File{}
	}
	return st, nil, nil
}

// Save writes the state atomically and durably.
func (s Store) Save(st *State) error {
	st.Schema = Schema
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(s.Path, append(b, '\n'), 0o644)
}

// WriteFileAtomic writes data to a temp file in the same directory, fsyncs
// it, renames it over path and fsyncs the directory.
func WriteFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		d.Close()
	}
	return nil
}
