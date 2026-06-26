// Package registry provides a single, generic, thread-safe key/value
// registry implementation. Before this package existed, debforge had two
// hand-written registries (internal/domain/pkg.Registry and
// internal/domain/installer.Registry) that implemented the exact same
// "map + Register + Lookup" pattern independently. Both now build on top
// of this type instead of duplicating the bookkeeping.
package registry

import "sync"

// Registry is a generic, concurrency-safe map from K to V.
type Registry[K comparable, V any] struct {
	mu    sync.RWMutex
	items map[K]V
}

// New creates an empty Registry.
func New[K comparable, V any]() *Registry[K, V] {
	return &Registry[K, V]{items: make(map[K]V)}
}

// Register stores value under key, overwriting any existing entry.
func (r *Registry[K, V]) Register(key K, value V) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[key] = value
}

// Lookup returns the value stored under key, if any.
func (r *Registry[K, V]) Lookup(key K) (V, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.items[key]
	return v, ok
}
