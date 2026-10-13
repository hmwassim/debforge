package setup

import (
	"context"
	"os"
	"sort"
	"strings"

	"github.com/hmwassim/debforge/internal/apt"
	"github.com/hmwassim/debforge/internal/system"
)

// DebianSources is the deb822 source list setup installs.
const DebianSources = `# Managed by debforge setup
Types: deb
URIs: http://deb.debian.org/debian
Suites: trixie trixie-updates trixie-backports
Components: main contrib non-free non-free-firmware
Signed-By: /usr/share/keyrings/debian-archive-keyring.gpg

Types: deb
URIs: http://security.debian.org/debian-security
Suites: trixie-security
Components: main contrib non-free non-free-firmware
Signed-By: /usr/share/keyrings/debian-archive-keyring.gpg
`

const legacySourcesNote = "# The Debian sources are configured in /etc/apt/sources.list.d/debian.sources\n# (managed by debforge setup). The previous content of this file was saved\n# as sources.list.debforge-orig.\n"

// hasActiveDeb reports whether a one-line-style sources file still has
// enabled entries.
func hasActiveDeb(b []byte) bool {
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "deb ") || strings.HasPrefix(l, "deb-src ") || strings.HasPrefix(l, "deb [") {
			return true
		}
	}
	return false
}

// builtinFiles returns files a builtin step manages.
func (e *Engine) builtinFiles(s *Step) []fileLike {
	if s.Builtin != "sources" {
		return nil
	}
	out := []fileLike{genFile{"/etc/apt/sources.list.d/debian.sources", []byte(DebianSources)}}
	legacy := "/etc/apt/sources.list"
	b, err := os.ReadFile(e.Root + legacy)
	if err == nil && (hasActiveDeb(b) || e.record(legacy) != nil) {
		out = append(out, genFile{legacy, []byte(legacySourcesNote)})
	}
	return out
}

func (e *Engine) checkBuiltin(ctx context.Context, s *Step, need func(string, ...any)) error {
	switch s.Builtin {
	case "i386":
		res, err := e.R.Run(ctx, system.Cmd{Name: "dpkg", Args: []string{"--print-foreign-architectures"}})
		if err != nil {
			return err
		}
		if !strings.Contains(" "+strings.Join(strings.Fields(string(res.Stdout)), " ")+" ", " i386 ") {
			need("i386 architecture not enabled")
		}
	case "upgrade":
		n, err := e.Apt.PendingUpgrades(ctx)
		if err != nil {
			return err
		}
		if n > 0 {
			need("%d package(s) can be upgraded", n)
		}
	}
	return nil
}

func (e *Engine) applyBuiltin(ctx context.Context, s *Step, force bool, prog apt.ProgressFunc) ([]string, error) {
	switch s.Builtin {
	case "sources":
		// Files are written by the generic file handling; refresh after.
		return nil, nil
	case "i386":
		if _, err := e.R.Run(ctx, system.Cmd{Name: "dpkg", Args: []string{"--add-architecture", "i386"}}); err != nil {
			return nil, err
		}
		warns, err := e.Apt.Update(ctx)
		return warns, err
	case "upgrade":
		return nil, e.Apt.FullUpgrade(ctx, prog)
	}
	return nil, nil
}

// retired returns the recorded setup files that no applicable step
// manages any more, sorted.
func (e *Engine) retired(p *Profile) ([]string, error) {
	wanted := map[string]bool{}
	for _, s := range p.Steps {
		if s.profile != nil || !s.Applies(e.Facts) {
			continue
		}
		for _, f := range e.allFiles(s) {
			if strings.HasPrefix(f.dest(), "~/") && e.User == nil {
				// Home files can't be located; keep every home record.
				return nil, nil
			}
			spec, err := e.spec(f)
			if err != nil {
				return nil, err
			}
			wanted[spec.Path] = true
		}
	}
	var out []string
	for path := range e.State.Setup.Files {
		if !wanted[path] {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out, nil
}

// retireReloads maps where a removed file lived to the trigger that makes
// the removal take effect.
var retireReloads = map[string]string{
	"/etc/udev/":           "udev",
	"/etc/sysctl.d/":       "sysctl",
	"/etc/systemd/":        "systemd",
	"/etc/modules-load.d/": "modules",
	"/etc/fonts/":          "fontconfig",
	"/etc/tmpfiles.d/":     "tmpfiles",
}

// retire removes retired files the way removing a package does: unmodified
// files are deleted (or the file they replaced is restored), files the user
// edited are kept.
func (e *Engine) retire(ctx context.Context, p *Profile) ([]string, error) {
	stale, err := e.retired(p)
	if err != nil {
		return nil, err
	}
	var notes []string
	reload := map[string]bool{}
	home := ""
	if e.User != nil {
		home = e.User.Home
	}
	for _, path := range stale {
		out, err := e.Files.RemoveRecord(path, e.State.Setup.Files[path], home)
		if err != nil {
			return notes, err
		}
		delete(e.State.Setup.Files, path)
		switch out.Action {
		case "kept":
			notes = append(notes, path+" is no longer managed but was modified by you; left in place")
		case "deleted", "restored":
			for prefix, r := range retireReloads {
				if strings.HasPrefix(path, prefix) {
					reload[r] = true
				}
			}
		}
	}
	for _, err := range system.Reload(ctx, e.R, reload) {
		notes = append(notes, err.Error())
	}
	return notes, nil
}
