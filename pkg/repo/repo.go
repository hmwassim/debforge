package repo

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hmwassim/debforge/pkg/executil"
	"github.com/hmwassim/debforge/pkg/packages"
	"github.com/hmwassim/debforge/pkg/text"
)

type RepoPackage struct {
	Name       string   `yaml:"name"`
	Type       string   `yaml:"type"`
	Packages   []string `yaml:"packages"`
	Conflicts  []string `yaml:"conflicts,omitempty"`
	KeyURL     string   `yaml:"key_url"`
	KeyPath    string   `yaml:"key_path"`
	Sources    string   `yaml:"sources"`
	SourcePath string   `yaml:"source_path"`
}

func (p *RepoPackage) Install(log *text.Logger) error {
	state := LoadState()
	if _, ok := state.Packages[p.Name]; ok {
		log.Info("%s already installed", p.Name)
		return nil
	}

	if len(p.Conflicts) > 0 {
		log.Info("Installing %s will delete %s", p.Name, strings.Join(p.Conflicts, ", "))
	} else {
		log.Info("Installing %s...", p.Name)
	}
	if !log.Prompt("Continue?") {
		log.Info("Cancelled")
		return nil
	}

	keyringsDir := filepath.Dir(p.KeyPath)
	if err := os.MkdirAll(keyringsDir, 0755); err != nil {
		return fmt.Errorf("creating keyrings dir: %w", err)
	}

	if needDownload(p.KeyPath) {
		if err := packages.DownloadFile(p.KeyPath, p.KeyURL, "Adding repository key..."); err != nil {
			return fmt.Errorf("downloading key: %w", err)
		}
	}

	if existing, err := os.ReadFile(p.SourcePath); err != nil || string(existing) != p.Sources {
		if err := atomicWrite(p.SourcePath, p.Sources); err != nil {
			return fmt.Errorf("writing sources: %w", err)
		}
	}

	s := text.StartSpinner(os.Stderr, "Installing "+p.Name+"...")

	if err := executil.Run(exec.Command("apt-get", "update")); err != nil {
		s.Fail()
		return fmt.Errorf("apt-get update: %w", err)
	}

	if len(p.Conflicts) > 0 {
		args := append([]string{"autopurge", "-y"}, p.Conflicts...)
		if err := executil.Run(exec.Command("apt-get", args...)); err != nil {
			s.Fail()
			return fmt.Errorf("removing conflicting packages: %w", err)
		}
	}

	args := append([]string{"install", "-y"}, p.Packages...)
	if err := executil.Run(exec.Command("apt-get", args...)); err != nil {
		s.Fail()
		return fmt.Errorf("installing %s: %w", p.Name, err)
	}

	s.Done()

	state.Packages[p.Name] = PkgEntry{Type: "apt"}
	if err := saveState(state); err != nil {
		log.Warn("Could not save state: %s", err)
	}

	log.Success("%s installed", p.Name)
	return nil
}

func (p *RepoPackage) Remove(log *text.Logger) error {
	state := LoadState()
	if _, ok := state.Packages[p.Name]; !ok {
		log.Warn("%s is not installed", p.Name)
		return nil
	}

	log.Info("Removing %s...", p.Name)
	if !log.Prompt("Continue?") {
		log.Info("Cancelled")
		return nil
	}

	s := text.StartSpinner(os.Stderr, "Removing "+p.Name+"...")

	args := append([]string{"purge", "-y"}, p.Packages...)
	if err := executil.Run(exec.Command("apt-get", args...)); err != nil {
		s.Fail()
		return fmt.Errorf("purging %s: %w", p.Name, err)
	}

	os.Remove(p.SourcePath)
	os.Remove(p.KeyPath)

	if err := executil.Run(exec.Command("apt-get", "update")); err != nil {
		s.Fail()
		return fmt.Errorf("apt-get update: %w", err)
	}

	s.Done()

	delete(state.Packages, p.Name)
	if err := saveState(state); err != nil {
		log.Warn("Could not save state: %s", err)
	}

	log.Info("%s removed", p.Name)
	return nil
}

func needDownload(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return true
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Size() == 0 {
		return true
	}
	return false
}

func atomicWrite(path, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path))
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	if _, err := tmp.WriteString(content); err != nil {
		return err
	}
	if err := tmp.Chmod(0644); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	cleanup = false
	return nil
}
