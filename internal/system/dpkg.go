package system

import (
	"context"
	"fmt"
	"strings"
)

// PkgStatus is one dpkg database entry.
type PkgStatus struct {
	Name    string
	Arch    string
	Version string
	// Installed means dpkg status "installed" (ii). Packages that were
	// removed but not purged (rc) are not installed.
	Installed bool
}

// Snapshot is a point-in-time view of the dpkg database. Refresh it after
// every apt transaction; it is never updated implicitly.
type Snapshot struct {
	NativeArch string
	pkgs       map[string]PkgStatus // keyed by "name:arch"
}

// TakeSnapshot queries dpkg once for every known package.
func TakeSnapshot(ctx context.Context, r Runner) (*Snapshot, error) {
	res, err := r.Run(ctx, Cmd{Name: "dpkg", Args: []string{"--print-architecture"}})
	if err != nil {
		return nil, fmt.Errorf("dpkg architecture: %w", err)
	}
	native := strings.TrimSpace(string(res.Stdout))
	res, err = r.Run(ctx, Cmd{Name: "dpkg-query", Args: []string{
		"-W", "-f", "${Package}\t${Architecture}\t${db:Status-Abbrev}\t${Version}\n",
	}})
	if err != nil {
		return nil, fmt.Errorf("dpkg-query: %w", err)
	}
	return ParseSnapshot(native, string(res.Stdout)), nil
}

// ParseSnapshot parses dpkg-query output in the format used by TakeSnapshot.
func ParseSnapshot(nativeArch, out string) *Snapshot {
	s := &Snapshot{NativeArch: nativeArch, pkgs: map[string]PkgStatus{}}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 4 || f[0] == "" {
			continue
		}
		abbrev := f[2]
		st := PkgStatus{
			Name:      f[0],
			Arch:      f[1],
			Version:   f[3],
			Installed: len(abbrev) >= 2 && abbrev[1] == 'i',
		}
		s.pkgs[f[0]+":"+f[1]] = st
	}
	return s
}

// Lookup returns the status of name, which may carry an ":arch" qualifier.
// An unqualified name matches the native architecture or "all".
func (s *Snapshot) Lookup(name string) (PkgStatus, bool) {
	if n, arch, ok := strings.Cut(name, ":"); ok {
		st, found := s.pkgs[n+":"+arch]
		return st, found
	}
	if st, ok := s.pkgs[name+":"+s.NativeArch]; ok {
		return st, true
	}
	st, ok := s.pkgs[name+":all"]
	return st, ok
}

// Installed reports whether name is in dpkg state "installed".
func (s *Snapshot) Installed(name string) bool {
	st, ok := s.Lookup(name)
	return ok && st.Installed
}

// Version returns the installed version of name, or "".
func (s *Snapshot) Version(name string) string {
	st, ok := s.Lookup(name)
	if !ok || !st.Installed {
		return ""
	}
	return st.Version
}

// AllInstalled reports whether every name is installed.
func (s *Snapshot) AllInstalled(names []string) bool {
	for _, n := range names {
		if !s.Installed(n) {
			return false
		}
	}
	return true
}

// ManualPackages returns the set of packages apt considers manually installed.
func ManualPackages(ctx context.Context, r Runner) (map[string]bool, error) {
	res, err := r.Run(ctx, Cmd{Name: "apt-mark", Args: []string{"showmanual"}})
	if err != nil {
		return nil, fmt.Errorf("apt-mark showmanual: %w", err)
	}
	out := map[string]bool{}
	for _, l := range strings.Fields(string(res.Stdout)) {
		out[l] = true
	}
	return out, nil
}
