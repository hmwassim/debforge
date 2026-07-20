package service

import (
	"fmt"
	"sync"
	"testing"
)

func TestStateManager_concurrentAccess(t *testing.T) {
	stateSvc, _ := newStateManagerForTest(t)
	st, err := stateSvc.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	const goroutines = 10
	const opsPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(goroutines * 3) // readers + writers + removers

	// Concurrent readers
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				_ = stateSvc.IsInstalled(st, "pkg-a")
				_, _ = stateSvc.Entry(st, "pkg-a")
				_ = stateSvc.ListPackages(st)
			}
		}()
	}

	// Concurrent writers
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				name := fmt.Sprintf("pkg-%d-%d", id, j)
				stateSvc.Add(st, name, PkgEntry{Type: "apt", Version: "1.0"})
			}
		}(i)
	}

	// Concurrent removers
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				name := fmt.Sprintf("pkg-%d-%d", id, j)
				stateSvc.Remove(st, name)
			}
		}(i)
	}

	wg.Wait()

	// Verify state is consistent (no panic = success)
	if st.Packages == nil {
		t.Error("expected non-nil Packages map after concurrent access")
	}
}
