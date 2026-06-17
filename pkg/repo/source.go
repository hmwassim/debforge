package repo

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hmwassim/debforge/pkg/executil"
	"github.com/hmwassim/debforge/pkg/text"
)

func (p *RepoPackage) sourceInstall(log *text.Logger, state *PackagesState, force bool) error {
	_, installed := state.Packages[p.Name]

	for _, check := range p.Checks {
		if _, err := exec.LookPath(check); err != nil {
			log.Info("Installing %s...", check)
			if err := executil.Run(exec.Command("apt-get", "install", "-y", check)); err != nil {
				return fmt.Errorf("%s not found in PATH and apt-get install failed", check)
			}
		}
	}

	var version string

	if p.SkipClone {
		if p.PostInstall != "" {
			if installed && !force {
				log.Info("Updating %s...", p.Name)
			}
			s := text.StartSpinner(os.Stderr, "Installing "+p.Name+"...")
			if err := executil.Run(exec.Command("sh", "-c", p.PostInstall)); err != nil {
				s.Fail()
				return fmt.Errorf("post-install: %w", err)
			}
			s.Done()
		}
		if p.VersionCmd != "" {
			var out bytes.Buffer
			cmd := exec.Command("sh", "-c", p.VersionCmd)
			cmd.Stdout = &out
			if err := cmd.Run(); err == nil {
				version = strings.TrimSpace(out.String())
			}
		}
	} else {
		if _, err := exec.LookPath("git"); err != nil {
			return fmt.Errorf("git is required to install source packages")
		}

		tmpDir, err := os.MkdirTemp("", "debforge-"+p.Name+"-*")
		if err != nil {
			return fmt.Errorf("creating temp dir: %w", err)
		}
		defer os.RemoveAll(tmpDir)

		s := text.StartSpinner(os.Stderr, "Checking latest version...")
		clone := exec.Command("git", "clone", "--depth=1", p.Repo, tmpDir)
		if out, err := clone.CombinedOutput(); err != nil {
			s.Fail()
			return fmt.Errorf("cloning %s: %s", p.Repo, strings.TrimSpace(string(out)))
		}
		s.Done()

		installDir := tmpDir
		if p.SourceSubdir != "" {
			installDir = filepath.Join(tmpDir, p.SourceSubdir)
		}

		if p.VersionCmd != "" {
			var out bytes.Buffer
			cmd := exec.Command("sh", "-c", p.VersionCmd)
			cmd.Dir = installDir
			cmd.Stdout = &out
			if err := cmd.Run(); err == nil {
				version = strings.TrimSpace(out.String())
			}
		} else {
			cmd := exec.Command("git", "rev-parse", "HEAD")
			cmd.Dir = installDir
			out, err := cmd.Output()
			if err != nil {
				return fmt.Errorf("getting commit hash: %w", err)
			}
			version = strings.TrimSpace(string(out))
		}

		if installed && !force {
			entry := state.Packages[p.Name]
			if entry.Version == version {
				log.Info("%s %s is already the latest version", p.Name, version[:8])
				return nil
			}
			log.Info("Updating %s %s → %s", p.Name, entry.Version[:8], version[:8])
		}

		s = text.StartSpinner(os.Stderr, "Running install script...")
		install := exec.Command("./install.sh")
		install.Dir = installDir
		if out, err := install.CombinedOutput(); err != nil {
			s.Fail()
			return fmt.Errorf("install.sh: %s", strings.TrimSpace(string(out)))
		}
		s.Done()

		if p.PostInstall != "" {
			if err := executil.Run(exec.Command("sh", "-c", p.PostInstall)); err != nil {
				log.Warn("post-install: %s", err)
			}
		}
	}

	state.Packages[p.Name] = PkgEntry{Type: p.Type, Version: version}
	if err := saveState(state); err != nil {
		return fmt.Errorf("%s installed but state not saved: %w", p.Name, err)
	}

	if version != "" {
		if len(version) >= 8 {
			log.Success("%s %s installed", p.Name, version[:8])
		} else {
			log.Success("%s %s installed", p.Name, version)
		}
	} else {
		log.Success("%s installed", p.Name)
	}
	return nil
}

func (p *RepoPackage) sourceRemove(log *text.Logger, state *PackagesState) error {
	if p.PostRemove != "" {
		if err := executil.Run(exec.Command("sh", "-c", p.PostRemove)); err != nil {
			log.Warn("post-remove: %s", err)
		}
	} else {
		log.Warn("source packages cannot be automatically removed")
		log.Warn("  manually remove files installed by %s and run:", p.Name)
		log.Warn("    sudo debforge remove %s", p.Name)
	}

	delete(state.Packages, p.Name)
	if err := saveState(state); err != nil {
		return fmt.Errorf("state not saved: %w", err)
	}

	log.Info("%s removed", p.Name)
	return nil
}

func (p *RepoPackage) sourceUpdate(log *text.Logger, state *PackagesState) error {
	return p.sourceInstall(log, state, true)
}
