package registry

import (
	"sync"
	"testing"
)

func TestRegistry_registerAndLookup(t *testing.T) {
	r := New[string, int]()
	r.Register("a", 1)

	v, ok := r.Lookup("a")
	if !ok || v != 1 {
		t.Errorf("got (%d, %v), want (1, true)", v, ok)
	}
}

func TestRegistry_lookupMissing(t *testing.T) {
	r := New[string, int]()
	if _, ok := r.Lookup("missing"); ok {
		t.Error("expected ok=false for a key that was never registered")
	}
}

func TestRegistry_registerOverwrites(t *testing.T) {
	r := New[string, int]()
	r.Register("a", 1)
	r.Register("a", 2)

	v, ok := r.Lookup("a")
	if !ok || v != 2 {
		t.Errorf("expected re-registering to overwrite, got (%d, %v)", v, ok)
	}
}

func TestRegistry_rangeVisitsAll(t *testing.T) {
	r := New[string, int]()
	r.Register("a", 1)
	r.Register("b", 2)
	r.Register("c", 3)

	var visited []string
	r.Range(func(k string, v int) bool {
		visited = append(visited, k)
		return true
	})

	if len(visited) != 3 {
		t.Errorf("Range visited %d entries, want 3: %v", len(visited), visited)
	}
}

func TestRegistry_rangeStopsEarly(t *testing.T) {
	r := New[string, int]()
	r.Register("a", 1)
	r.Register("b", 2)

	var visited []string
	r.Range(func(k string, v int) bool {
		visited = append(visited, k)
		return false
	})

	if len(visited) != 1 {
		t.Errorf("Range visited %d entries after early stop, want 1", len(visited))
	}
}

func TestRegistry_rangeEmpty(t *testing.T) {
	r := New[string, int]()
	count := 0
	r.Range(func(k string, v int) bool {
		count++
		return true
	})
	if count != 0 {
		t.Errorf("Range on empty registry visited %d entries, want 0", count)
	}
}

func TestRegistry_concurrentAccess(t *testing.T) {
	r := New[int, int]()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			r.Register(n, n*n)
			r.Lookup(n)
		}(i)
	}
	wg.Wait()

	v, ok := r.Lookup(42)
	if !ok || v != 42*42 {
		t.Errorf("got (%d, %v), want (%d, true)", v, ok, 42*42)
	}
}
