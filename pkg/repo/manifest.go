package repo

import (
	"fmt"
	"sync"
)

var (
	manifestMu sync.RWMutex
	manifest   = map[string]*RepoPackage{}
)

func Register(p *RepoPackage) error {
	manifestMu.Lock()
	defer manifestMu.Unlock()
	if _, ok := manifest[p.Name]; ok {
		return fmt.Errorf("package %q already registered", p.Name)
	}
	manifest[p.Name] = p
	return nil
}

func Lookup(name string) *RepoPackage {
	manifestMu.RLock()
	defer manifestMu.RUnlock()
	return manifest[name]
}

func List() []string {
	manifestMu.RLock()
	defer manifestMu.RUnlock()
	var names []string
	for n := range manifest {
		names = append(names, n)
	}
	return names
}
