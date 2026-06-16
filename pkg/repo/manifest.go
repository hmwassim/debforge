package repo

import "fmt"

var manifest = map[string]*RepoPackage{}

func Register(p *RepoPackage) error {
	if _, ok := manifest[p.Name]; ok {
		return fmt.Errorf("package %q already registered", p.Name)
	}
	manifest[p.Name] = p
	return nil
}

func Lookup(name string) *RepoPackage {
	return manifest[name]
}

func List() []string {
	var names []string
	for n := range manifest {
		names = append(names, n)
	}
	return names
}
