package repo

import "fmt"

var manifest = map[string]*RepoPackage{}

func Register(p *RepoPackage) {
	if _, ok := manifest[p.Name]; ok {
		panic(fmt.Sprintf("repo package %q already registered", p.Name))
	}
	manifest[p.Name] = p
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

func init() {
	Register(&RepoPackage{
		Name:       "firefox",
		Packages:   []string{"firefox"},
		Conflicts:  []string{"firefox-esr"},
		KeyURL:     "https://packages.mozilla.org/apt/repo-signing-key.gpg",
		KeyPath:    "/etc/apt/keyrings/packages.mozilla.org.asc",
		SourcePath: "/etc/apt/sources.list.d/mozilla.sources",
		Sources: `Types: deb
URIs: https://packages.mozilla.org/apt
Suites: mozilla
Components: main
Signed-By: /etc/apt/keyrings/packages.mozilla.org.asc
`,
	})
}
