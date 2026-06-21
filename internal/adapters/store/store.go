package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Store[T any] struct {
	mu   sync.Mutex
	path string
}

func NewStore[T any](path string) *Store[T] {
	return &Store[T]{path: path}
}

func (s *Store[T]) Load() (*T, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
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
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}
