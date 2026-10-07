package apt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/hmwassim/debforge/internal/state"
	"github.com/hmwassim/debforge/internal/system"
)

// PinFile is where debforge keeps its backports source pins.
const PinFile = "/etc/apt/preferences.d/debforge-backports.pref"

// SyncBackportPins pins every source package that has a binary installed
// from backports to backports, for every architecture.
//
// Installing some binaries of a source with -t <backports> leaves its other
// binaries on stable. A later plain install then picks the stable version of
// a sibling (e.g. libegl-mesa0:i386 25.x next to libgbm1:i386 26.x from
// backports) and fails on exact-version dependencies. With the pin, apt
// keeps whole sources together. Pins by name only match the native
// architecture, so each source is also listed with every foreign one.
//
// The file is rewritten only when its content changes; with no backports
// packages installed it is removed.
func (a *Apt) SyncBackportPins(ctx context.Context) error {
	if a.PinFile == "" {
		return nil
	}
	res, err := a.R.Run(ctx, system.Cmd{Name: "dpkg-query", Args: []string{
		"-W", "-f", "${db:Status-Abbrev}\t${Version}\t${source:Package}\n",
	}})
	if err != nil {
		return fmt.Errorf("list installed packages: %w", err)
	}
	set := map[string]bool{}
	for _, line := range strings.Split(string(res.Stdout), "\n") {
		f := strings.Split(line, "\t")
		if len(f) == 3 && strings.HasPrefix(f[0], "ii") && strings.Contains(f[1], "~bpo") && f[2] != "" {
			set[f[2]] = true
		}
	}
	if len(set) == 0 {
		if err := os.Remove(a.PinFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	res, err = a.R.Run(ctx, system.Cmd{Name: "dpkg", Args: []string{"--print-foreign-architectures"}})
	if err != nil {
		return fmt.Errorf("dpkg architectures: %w", err)
	}
	archs := strings.Fields(string(res.Stdout))
	sources := make([]string, 0, len(set))
	for s := range set {
		sources = append(sources, s)
	}
	sort.Strings(sources)
	var names []string
	for _, s := range sources {
		names = append(names, "src:"+s)
		for _, arch := range archs {
			names = append(names, "src:"+s+":"+arch)
		}
	}
	content := []byte("# Managed by debforge; regenerated before every install.\n" +
		"# Source packages with binaries installed from " + BackportsSuite + " stay on\n" +
		"# backports for every architecture, so apt never mixes stable and backports\n" +
		"# builds of one source (exact-version dependencies, i386 libraries).\n" +
		"Package: " + strings.Join(names, " ") + "\n" +
		"Pin: release n=" + BackportsSuite + "\n" +
		"Pin-Priority: 500\n")
	if cur, err := os.ReadFile(a.PinFile); err == nil && bytes.Equal(cur, content) {
		return nil
	}
	return state.WriteFileAtomic(a.PinFile, content, 0o644)
}
