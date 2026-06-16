package repo

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hmwassim/debforge/pkg/executil"
	"github.com/hmwassim/debforge/pkg/packages"
	"github.com/hmwassim/debforge/pkg/text"
	"github.com/hmwassim/debforge/pkg/writeutil"
)

type RepoPackage struct {
	Name        string            `yaml:"name"`
	Type        string            `yaml:"type"`
	Packages    []string          `yaml:"packages"`
	Conflicts   []string          `yaml:"conflicts,omitempty"`
	Extrepo     string            `yaml:"extrepo,omitempty"`
	KeyURL      string            `yaml:"key_url,omitempty"`
	KeyPath     string            `yaml:"key_path,omitempty"`
	KeyDearmor  bool              `yaml:"key_dearmor,omitempty"`
	Sources     string            `yaml:"sources,omitempty"`
	SourcePath  string            `yaml:"source_path,omitempty"`
	Primary     string            `yaml:"primary,omitempty"`
	Backports   []string          `yaml:"backports,omitempty"`
	Variants    map[string]string `yaml:"variants,omitempty"`
	Configs     map[string]string `yaml:"configs,omitempty"`
	UserConfigs map[string]string `yaml:"user_configs,omitempty"`
	PostInstall string            `yaml:"post_install,omitempty"`
	PostRemove  string            `yaml:"post_remove,omitempty"`
}

func (p *RepoPackage) Install(log *text.Logger, force bool) error {
	state, err := LoadState()
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	var selectedVariant string
	var switching bool

	if len(p.Variants) > 0 {
		entry, exists := state.Packages[p.Name]
		selectedVariant = promptVariant(log, p.Variants)
		if selectedVariant == "" {
			return nil
		}
		if exists && entry.Variant == selectedVariant {
			if !force {
				log.Info("%s (%s) already installed", p.Name, selectedVariant)
				return nil
			}
			log.Info("Reinstalling %s (%s)", p.Name, selectedVariant)
		}
		if exists && entry.Variant != "" {
			switching = true
			log.Info("Switching from %s to %s variant", entry.Variant, selectedVariant)
		}
	} else {
		if _, ok := state.Packages[p.Name]; ok {
			if !force {
				log.Info("%s already installed", p.Name)
				return nil
			}
			log.Info("Reinstalling %s", p.Name)
		}
	}

	if len(p.Conflicts) > 0 {
		log.Info("Installing %s will remove %s", p.Name, strings.Join(p.Conflicts, ", "))
	} else {
		log.Info("Installing %s...", p.Name)
	}
	if !log.Prompt("Continue?") {
		log.Info("Cancelled")
		return nil
	}

	if p.Extrepo != "" {
		if _, err := exec.LookPath("extrepo"); err != nil {
			log.Info("extrepo not found, installing...")
			if err := executil.Run(exec.Command("apt-get", "install", "-y", "extrepo")); err != nil {
				return fmt.Errorf("installing extrepo: %w", err)
			}
		}
		if err := ensureExtrepoConfig(); err != nil {
			return fmt.Errorf("extrepo config: %w", err)
		}
		if err := executil.Run(exec.Command("extrepo", "enable", p.Extrepo)); err != nil {
			return fmt.Errorf("extrepo enable %s: %w", p.Extrepo, err)
		}
	} else if p.KeyURL != "" {
		keyringsDir := filepath.Dir(p.KeyPath)
		if err := os.MkdirAll(keyringsDir, 0755); err != nil {
			return fmt.Errorf("creating keyrings dir: %w", err)
		}

		if needDownload(p.KeyPath) {
			if p.KeyDearmor {
				tmpPath := p.KeyPath + ".part"
				if err := packages.DownloadFile(tmpPath, p.KeyURL, "Adding repository key..."); err != nil {
					return fmt.Errorf("downloading key: %w", err)
				}
				if err := executil.Run(exec.Command("gpg", "--dearmor", "--output", p.KeyPath, tmpPath)); err != nil {
					os.Remove(tmpPath)
					return fmt.Errorf("dearmoring key: %w", err)
				}
				os.Remove(tmpPath)
			} else {
				if err := packages.DownloadFile(p.KeyPath, p.KeyURL, "Adding repository key..."); err != nil {
					return fmt.Errorf("downloading key: %w", err)
				}
			}
			if err := os.Chmod(p.KeyPath, 0644); err != nil {
				return fmt.Errorf("setting key permissions: %w", err)
			}
		}

		if existing, err := os.ReadFile(p.SourcePath); err != nil || string(existing) != p.Sources {
			if err := writeutil.AtomicFile(p.SourcePath, []byte(p.Sources), 0644); err != nil {
				return fmt.Errorf("writing sources: %w", err)
			}
		}
	}

	hasApt := len(p.Packages) > 0 || len(p.Conflicts) > 0 || len(p.Backports) > 0 || switching

	if hasApt {
		s := text.StartSpinner(os.Stderr, "Installing "+p.Name+"...")

		if err := executil.Run(exec.Command("apt-get", "update")); err != nil {
			s.Fail()
			return fmt.Errorf("apt-get update: %w", err)
		}

		if switching {
			oldPkg := p.Variants[state.Packages[p.Name].Variant]
			oldArgs := append([]string{"purge", "-y"}, oldPkg)
			if err := executil.Run(exec.Command("apt-get", oldArgs...)); err != nil {
				s.Fail()
				return fmt.Errorf("removing previous %s variant: %w", p.Name, err)
			}
		}

		if len(p.Conflicts) > 0 {
			args := append([]string{"autopurge", "-y"}, p.Conflicts...)
			if err := executil.Run(exec.Command("apt-get", args...)); err != nil {
				s.Fail()
				return fmt.Errorf("removing conflicting packages: %w", err)
			}
		}

		installPkgs := p.Packages
		if selectedVariant != "" {
			installPkgs = append([]string{p.Variants[selectedVariant]}, installPkgs...)
		}
		args := append([]string{"install", "-y"}, installPkgs...)
		if err := executil.Run(exec.Command("apt-get", args...)); err != nil {
			s.Fail()
			return fmt.Errorf("installing %s: %w", p.Name, err)
		}

		if len(p.Backports) > 0 {
			bpArgs := append([]string{"install", "-y", "-t", "trixie-backports"}, p.Backports...)
			if err := executil.Run(exec.Command("apt-get", bpArgs...)); err != nil {
				s.Fail()
				return fmt.Errorf("installing backports for %s: %w", p.Name, err)
			}
		}

		s.Done()
	}

	for path, content := range p.Configs {
		if err := packages.DeployConfig(path, content, 0644); err != nil {
			return fmt.Errorf("deploying %s: %w", path, err)
		}
	}

	if err := deployUserConfigs(p.UserConfigs); err != nil {
		return err
	}

	if p.PostInstall != "" {
		if err := executil.Run(userCmd("sh", "-c", p.PostInstall)); err != nil {
			log.Warn("post-install: %s", err)
		}
	}

	state.Packages[p.Name] = PkgEntry{Type: "apt", Variant: selectedVariant}
	if err := saveState(state); err != nil {
		return fmt.Errorf("%s installed but state not saved: %w", p.Name, err)
	}

	log.Success("%s installed", p.Name)
	return nil
}

func (p *RepoPackage) Remove(log *text.Logger) error {
	state, err := LoadState()
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}
	if _, ok := state.Packages[p.Name]; !ok {
		log.Warn("%s is not installed", p.Name)
		return nil
	}

	log.Info("Removing %s...", p.Name)
	if !log.Prompt("Continue?") {
		log.Info("Cancelled")
		return nil
	}

	if p.PostRemove != "" {
		if err := executil.Run(userCmd("sh", "-c", p.PostRemove)); err != nil {
			log.Warn("post-remove: %s", err)
		}
	}

	entry := state.Packages[p.Name]
	hasApt := len(p.Packages) > 0 || p.Primary != ""

	if hasApt {
		s := text.StartSpinner(os.Stderr, "Removing "+p.Name+"...")

		if p.Primary != "" {
			primary := p.Primary
			if entry.Variant != "" {
				if vpkg, ok := p.Variants[entry.Variant]; ok {
					primary = vpkg
				}
			}
			rArgs := []string{"remove", "-y", primary}
			if err := executil.Run(exec.Command("apt-get", rArgs...)); err != nil {
				s.Fail()
				return fmt.Errorf("removing %s: %w", primary, err)
			}
			if err := executil.Run(exec.Command("apt-get", "autoremove", "-y")); err != nil {
				s.Fail()
				return fmt.Errorf("autoremove: %w", err)
			}
		} else {
			if err := executil.Run(exec.Command("apt-get", "autoremove", "-y")); err != nil {
				s.Fail()
				return fmt.Errorf("autoremove: %w", err)
			}
		}

		if p.Extrepo != "" {
			if err := executil.Run(exec.Command("extrepo", "disable", p.Extrepo)); err != nil {
				log.Warn("extrepo disable %s: %s", p.Extrepo, err)
			}
		} else if p.SourcePath != "" {
			if err := os.Remove(p.SourcePath); err != nil && !os.IsNotExist(err) {
				log.Warn("Could not remove sources list: %s", err)
			}
			if err := os.Remove(p.KeyPath); err != nil && !os.IsNotExist(err) {
				log.Warn("Could not remove key file: %s", err)
			}
		}

		for path := range p.Configs {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				log.Warn("Could not remove %s: %s", path, err)
			}
		}
		removeUserConfigs(log, p.UserConfigs)

		if err := executil.Run(exec.Command("apt-get", "update")); err != nil {
			s.Fail()
			return fmt.Errorf("apt-get update: %w", err)
		}

		s.Done()
	} else {
		for path := range p.Configs {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				log.Warn("Could not remove %s: %s", path, err)
			}
		}
		removeUserConfigs(log, p.UserConfigs)
	}

	delete(state.Packages, p.Name)
	if err := saveState(state); err != nil {
		return fmt.Errorf("%s removed but state not saved: %w", p.Name, err)
	}

	log.Info("%s removed", p.Name)
	return nil
}

func needDownload(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.Size() == 0 {
		return true
	}
	return false
}

func promptVariant(log *text.Logger, variants map[string]string) string {
	keys := make([]string, 0, len(variants))
	for k := range variants {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	log.Info("Select variant:")
	for i, k := range keys {
		log.Info("  %d) %s — %s", i+1, k, variants[k])
	}

	input := log.PromptLine(fmt.Sprintf("Enter number [1-%d] or 0 to cancel:", len(keys)))
	var n int
	if _, err := fmt.Sscanf(input, "%d", &n); err != nil || n < 0 || n > len(keys) {
		log.Warn("Invalid selection")
		return ""
	}
	if n == 0 {
		log.Info("Cancelled")
		return ""
	}
	return keys[n-1]
}

func ensureExtrepoConfig() error {
	const path = "/etc/extrepo/config.yaml"
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	if bytes.Contains(data, []byte("\n- non-free")) {
		return nil
	}

	lines := strings.Split(string(data), "\n")
	changed := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		content := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
		if strings.HasPrefix(content, "- ") {
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			lines[i] = indent + content
			changed = true
		}
	}

	if changed {
		return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
	}
	return nil
}
