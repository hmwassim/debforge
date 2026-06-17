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
	if entry, ok := state.Packages[p.Name]; ok {
		if !force {
			if len(entry.Version) >= 8 {
				log.Info("%s %s already installed", p.Name, entry.Version[:8])
			} else if entry.Version != "" {
				log.Info("%s %s already installed", p.Name, entry.Version)
			} else {
				log.Info("%s already installed", p.Name)
			}
			return nil
		}
		log.Info("Reinstalling %s", p.Name)
	}

	for _, check := range p.Checks {
		if _, err := exec.LookPath(check); err != nil {
			return fmt.Errorf("%s requires %s (install with: sudo debforge install nvidia)", p.Name, check)
		}
	}

	var version string

	if p.SkipClone {
		if p.PostInstall != "" {
			if err := executil.Run(exec.Command("sh", "-c", p.PostInstall)); err != nil {
				return fmt.Errorf("post-install: %w", err)
			}
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

		s := text.StartSpinner(os.Stderr, "Cloning "+p.Name+"...")
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
