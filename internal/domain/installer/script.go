package installer

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/dpkg"
	"github.com/hmwassim/debforge/internal/ports"
)

// RunScript executes a shell script via the runner, setting the spinner
// description to verb + name, and wraps errors with the same pattern.
func RunScript(ctx context.Context, runner ports.CommandRunner, spinner ports.Spinner, name, script, verb string) error {
	spinner.SetDesc(verb + " " + name)
	if _, stderr, err := runner.Run(ctx, "sh", "-c", script); err != nil {
		return fmt.Errorf("%s %s: %w%s", verb, name, err, trimErr(stderr))
	}
	return nil
}

// RunScriptInDir is like RunScript but runs the script in the given directory.
func RunScriptInDir(ctx context.Context, runner ports.CommandRunner, spinner ports.Spinner, name, script, verb, dir string) error {
	spinner.SetDesc(verb + " " + name)
	if _, stderr, err := runner.RunWithOptions(ctx, ports.RunOptions{Dir: dir}, "sh", "-c", script); err != nil {
		return fmt.Errorf("%s %s: %w%s", verb, name, err, trimErr(stderr))
	}
	return nil
}

func trimErr(stderr []byte) string {
	out := strings.TrimSpace(string(stderr))
	if out == "" {
		return ""
	}
	if len(out) > 500 {
		out = out[:500] + "..."
	}
	return ": " + out
}

// WriteConfigs writes all config files defined in p.Configs.
func WriteConfigs(fs ports.FileSystem, spinner ports.Spinner, p *pkg.Package) error {
	if len(p.Configs) == 0 {
		return nil
	}

	spinner.SetDesc("writing configs for " + p.Name)
	for path, content := range p.Configs {
		dir := filepath.Dir(path)
		if err := fs.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create config dir %s: %w", dir, err)
		}
		if err := fs.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("write config %s: %w", path, err)
		}
	}
	return nil
}

// MkdirTemp creates a temporary directory with the debforge-* pattern.
func MkdirTemp(fs ports.FileSystem) (string, error) {
	tmpDir, err := fs.MkdirTemp("debforge-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	return tmpDir, nil
}

// RunPostInstall executes the post-install script if non-empty.
func RunPostInstall(ctx context.Context, runner ports.CommandRunner, spinner ports.Spinner, name, script string) error {
	if script == "" {
		return nil
	}
	return RunScript(ctx, runner, spinner, name, script, "running post-install for")
}

// RunPostRemove executes the post-remove script if non-empty.
func RunPostRemove(ctx context.Context, runner ports.CommandRunner, spinner ports.Spinner, name, script string) error {
	if script == "" {
		return nil
	}
	return RunScript(ctx, runner, spinner, name, script, "running post-remove for")
}

// CheckInstalled verifies that p is installed on the system.
//   - apt: all system package names in p.Packages (or PrimarySystemPackage()
//     when Packages is empty) are dpkg-installed.
//   - deb: PrimarySystemPackage() is dpkg-installed.
//   - config: every file in p.Configs exists on disk.
//   - source: falls back to package metadata (state.json); no universal
//     system check exists, so returns true unconditionally.
//
// The caller is responsible for reconciling this result with state.json.
func CheckInstalled(ctx context.Context, runner ports.CommandRunner, fs ports.FileSystem, p *pkg.Package) bool {
	switch p.Type {
	case pkg.TypeApt:
		names := p.Packages
		if len(names) == 0 {
			names = []string{p.PrimarySystemPackage()}
		}
		for _, name := range names {
			if !dpkg.IsInstalled(ctx, runner, name) {
				return false
			}
		}
		return true
	case pkg.TypeDeb:
		return dpkg.IsInstalled(ctx, runner, p.PrimarySystemPackage())
	case pkg.TypeConfig:
		for path := range p.Configs {
			ok, err := fs.Exists(path)
			if err != nil || !ok {
				return false
			}
		}
		return true
	default: // source
		return true
	}
}
