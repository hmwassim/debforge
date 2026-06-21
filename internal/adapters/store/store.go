package store

import (
	"encoding/json"
	"path/filepath"

	"github.com/hmwassim/debforge/internal/ports"
)

type Store[T any] struct {
	fs   ports.FileSystem
	path string
}

func NewStore[T any](fs ports.FileSystem, path string) *Store[T] {
	return &Store[T]{fs: fs, path: path}
}

func (s *Store[T]) Load() (*T, error) {
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

func (s *Store[T]) Save(v *T) error {
	if err := s.fs.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return s.fs.WriteFile(s.path, data, 0644)
}
