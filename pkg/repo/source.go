package repo

import (
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

	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = installDir
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("getting commit hash: %w", err)
	}
	commit := strings.TrimSpace(string(out))

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

	state.Packages[p.Name] = PkgEntry{Type: p.Type, Version: commit}
	if err := saveState(state); err != nil {
		return fmt.Errorf("%s installed but state not saved: %w", p.Name, err)
	}

	log.Success("%s %s installed", p.Name, commit[:8])
	return nil
}

func (p *RepoPackage) sourceRemove(log *text.Logger, state *PackagesState) error {
	log.Warn("source packages cannot be automatically removed")
	log.Warn("  manually remove files installed by %s and run:", p.Name)
	log.Warn("    sudo debforge remove %s", p.Name)

	delete(state.Packages, p.Name)
	if err := saveState(state); err != nil {
		return fmt.Errorf("state not saved: %w", err)
	}

	log.Info("%s removed from state", p.Name)
	return nil
}

func (p *RepoPackage) sourceUpdate(log *text.Logger, state *PackagesState) error {
	return p.sourceInstall(log, state, true)
}
