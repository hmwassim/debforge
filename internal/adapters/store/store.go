// Package store provides a generic JSON file store backed by ports.FileSystem.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"

	"github.com/hmwassim/debforge/internal/ports"
)

// ErrNotFound is returned by Store.Load when the backing file does not exist.
var ErrNotFound = errors.New("store: not found")

// Store is a generic JSON-backed store for a single value of type T.
type Store[T any] struct {
	fs   ports.FileSystem
	path string
}

// NewStore returns a Store that reads and writes a JSON file at path using fs.
func NewStore[T any](fs ports.FileSystem, path string) *Store[T] {
	return &Store[T]{fs: fs, path: path}
}

// Load reads and deserializes the stored value from the JSON file.
// Returns ErrNotFound when the file does not exist.
func (s *Store[T]) Load() (*T, error) {
	ok, err := s.fs.Exists(s.path)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotFound
	}
	data, err := s.fs.ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// Save serializes v as indented JSON and writes it atomically to the
// store's path (write to temp file, rename over target).
func (s *Store[T]) Save(v *T) error {
	if err := s.fs.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	// Atomic write: write to a temp file on the same filesystem, then
	// rename over the target. Rename is atomic on POSIX, so a crash
	// mid-write leaves the original state intact rather than truncating it.
	// Random suffix prevents predictable temp file paths.
	var randBuf [8]byte
	if _, err := rand.Read(randBuf[:]); err != nil {
		return err
	}
	tmpPath := s.path + ".tmp." + hex.EncodeToString(randBuf[:])
	if err := s.fs.WriteFile(tmpPath, data, 0600); err != nil {
		return err
	}
	return s.fs.Rename(tmpPath, s.path)
}
