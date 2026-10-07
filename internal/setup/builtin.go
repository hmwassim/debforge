package setup

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hmwassim/debforge/internal/apt"
	"github.com/hmwassim/debforge/internal/files"
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

const (
	bashrcStart = "# >>> debforge bashrc.d loader >>>"
	bashrcEnd   = "# <<< debforge bashrc.d loader <<<"
	bashrcBlock = bashrcStart + `
if [ -d "$HOME/.config/bashrc.d" ]; then
    for file in "$HOME/.config/bashrc.d"/*.sh; do
        [ -f "$file" ] && . "$file"
    done
fi
` + bashrcEnd + "\n"
)

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
	case "bashrc":
		if e.User == nil {
			return system.ErrNoTargetUser
		}
		home := files.Spec{Home: e.User.Home, UID: e.User.UID, GID: e.User.GID}
		dir := home
		dir.Path = filepath.Join(e.User.Home, ".config/bashrc.d")
		if fi, err := os.Lstat(e.Root + dir.Path); err != nil || !fi.IsDir() {
			need("~/.config/bashrc.d does not exist")
		}
		rc := home
		rc.Path = filepath.Join(e.User.Home, ".bashrc")
		b, _, err := e.Files.Read(rc)
		if err != nil {
			return err
		}
		if !bytes.Contains(b, []byte(bashrcBlock)) {
			need("~/.bashrc does not load ~/.config/bashrc.d")
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
	case "bashrc":
		if e.User == nil {
			return nil, system.ErrNoTargetUser
		}
		base := files.Spec{Home: e.User.Home, UID: e.User.UID, GID: e.User.GID, Mode: 0o644}
		dir := base
		dir.Path = filepath.Join(e.User.Home, ".config/bashrc.d")
		if err := e.Files.EnsureDir(dir); err != nil {
			return nil, fmt.Errorf("create ~/.config/bashrc.d: %w", err)
		}
		rc := base
		rc.Path = filepath.Join(e.User.Home, ".bashrc")
		cur, _, err := e.Files.Read(rc)
		if err != nil {
			return nil, err
		}
		rc.Data = withBashrcBlock(cur)
		if bytes.Equal(rc.Data, cur) {
			return nil, nil
		}
		return nil, e.Files.Write(rc)
	}
	return nil, nil
}

// withBashrcBlock returns content with exactly one up-to-date loader block,
// replacing a well-formed existing block in place and removing stray
// markers otherwise.
func withBashrcBlock(cur []byte) []byte {
	s := string(cur)
	start := strings.Index(s, bashrcStart)
	end := strings.Index(s, bashrcEnd)
	if start >= 0 && end > start && strings.Count(s, bashrcStart) == 1 && strings.Count(s, bashrcEnd) == 1 {
		endLine := end + len(bashrcEnd)
		if endLine < len(s) && s[endLine] == '\n' {
			endLine++
		}
		return []byte(s[:start] + bashrcBlock + s[endLine:])
	}
	var kept []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) == bashrcStart || strings.TrimSpace(l) == bashrcEnd {
			continue
		}
		kept = append(kept, l)
	}
	out := strings.TrimRight(strings.Join(kept, "\n"), "\n")
	if out != "" {
		out += "\n\n"
	}
	return []byte(out + bashrcBlock)
}
