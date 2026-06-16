package core

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hmwassim/debforge/pkg/executil"
	"github.com/hmwassim/debforge/pkg/lock"
	"github.com/hmwassim/debforge/pkg/packages"
	"github.com/hmwassim/debforge/pkg/settings"
	"github.com/hmwassim/debforge/pkg/state"
	"github.com/hmwassim/debforge/pkg/text"
)

type coreState struct {
	LastSetupCommit string   `json:"last_setup_commit,omitempty"`
	ManagedPackages []string `json:"managed_packages,omitempty"`
	ManagedConfigs  []string `json:"managed_configs,omitempty"`
}

type configCheck struct {
	dest    string
	content string
}

type plan struct {
	pkgs    []string
	configs []configCheck
	svcs    []string
}

func buildPlan() plan {
	var p plan
	for _, g := range groups {
		p.pkgs = append(p.pkgs, g.packages...)
		for _, cf := range g.configs {
			p.configs = append(p.configs, configCheck{cf.dest, cf.content})
		}
		p.svcs = append(p.svcs, g.services...)
	}
	return p
}

func Setup(log *text.Logger, force bool) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("core setup must be run as root")
	}

	if force {
		log.Info("Reapplying core...")
	} else {
		log.Info("Setting up core...")
	}

	if err := settings.Default.EnsureDirsExist(); err != nil {
		return fmt.Errorf("creating directories: %w", err)
	}

	release, err := lock.Acquire(settings.Default.LockFile())
	if err != nil {
		return fmt.Errorf("cannot acquire lock: %w", err)
	}
	defer release()

	st := &coreState{}
	store := state.New("core")
	if err := store.Load(st); err != nil {
		log.Warn("State file corrupt, resetting: %s", err)
		st = &coreState{}
	}

	cur := currentCommit(log)
	commitChanged := cur != "" && st.LastSetupCommit != "" && cur != st.LastSetupCommit

	p := buildPlan()
	desiredPkgs := p.pkgs
	desiredConfigs := make([]string, len(p.configs))
	for i, cf := range p.configs {
		desiredConfigs[i] = cf.dest
	}
	stalePkgs := setDiff(st.ManagedPackages, desiredPkgs)
	staleConfigs := setDiff(st.ManagedConfigs, desiredConfigs)

	if !force && !commitChanged && st.ManagedPackages != nil && len(stalePkgs) == 0 && len(staleConfigs) == 0 {
		s := text.StartSpinner(os.Stderr, "Verifying core...")
		if verifySetup(p) {
			s.Done()
			log.Success("Core setup already up to date")
			return nil
		}
		s.Fail()
	}

	if commitChanged {
		log.Info("New source detected since last setup, reapplying...")
	}

	if len(stalePkgs) > 0 {
		log.Info("Removing %d stale package(s)...", len(stalePkgs))
		if err := packages.AptRemove(stalePkgs); err != nil {
			return fmt.Errorf("removing stale packages: %w", err)
		}
	}
	for _, path := range staleConfigs {
		log.Info("Removing stale config: %s", path)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Warn("Could not remove %s: %s", path, err)
		}
	}

	var errs []error

	if err := ensureSourcesList(force); err != nil {
		errs = append(errs, fmt.Errorf("sources.list: %w", err))
	}
	if err := enablei386(force); err != nil {
		errs = append(errs, fmt.Errorf("i386: %w", err))
	}

	if len(errs) == 0 {
		if err := executil.RunWithSpinner(exec.Command("apt-get", "update"), "Updating package lists..."); err != nil {
			errs = append(errs, fmt.Errorf("apt-get update: %w", err))
		}
	}

	for _, g := range groups {
		s := text.StartSpinner(os.Stderr, "Setting up "+g.name+"...")

		if err := packages.AptInstall(g.packages, g.backport, ""); err != nil {
			s.Fail()
			errs = append(errs, fmt.Errorf("installing %s: %w", g.name, err))
			continue
		}

		var failed bool
		for _, cf := range g.configs {
			if err := packages.DeployConfig(cf.dest, cf.content, cf.mode); err != nil {
				errs = append(errs, fmt.Errorf("deploying %s: %w", cf.dest, err))
				failed = true
			}
		}

		for _, svc := range g.services {
			if err := packages.EnableService(svc); err != nil {
				log.Warn("Could not enable %s (non-fatal): %s", svc, err)
			}
		}

		if g.postInstall != nil {
			if err := g.postInstall(log, s, force); err != nil {
				errs = append(errs, fmt.Errorf("post-install %s: %w", g.name, err))
				failed = true
			}
		}

		if failed {
			s.Fail()
		} else {
			s.Done()
		}
	}

	if err := ensureResolvSymlink(log); err != nil {
		errs = append(errs, fmt.Errorf("resolv.conf symlink: %w", err))
	}

	if len(errs) > 0 {
		for _, err := range errs {
			log.Error("%s", err)
		}
		return fmt.Errorf("setup completed with %d error(s)", len(errs))
	}

	st.LastSetupCommit = cur
	st.ManagedPackages = desiredPkgs
	st.ManagedConfigs = desiredConfigs
	if err := store.Save(st); err != nil {
		log.Warn("Could not save state: %s", err)
	}

	log.Success("Core setup complete")
	return nil
}

func verifySetup(p plan) bool {
	installed, err := packages.CheckInstalled(p.pkgs)
	if err != nil {
		return false
	}
	for _, pkg := range p.pkgs {
		if !installed[pkg] {
			return false
		}
	}

	for _, cf := range p.configs {
		data, err := os.ReadFile(cf.dest)
		if err != nil || string(data) != cf.content {
			return false
		}
	}

	if len(p.svcs) > 0 {
		args := append([]string{"is-active", "--quiet"}, p.svcs...)
		if err := executil.Run(exec.Command("systemctl", args...)); err != nil {
			return false
		}
	}

	return true
}

func currentCommit(log *text.Logger) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = settings.Default.SourceDir()
	out, err := cmd.Output()
	if err != nil {
		log.Warn("Could not check source commit: %s", err)
		return ""
	}
	return strings.TrimSpace(string(out))
}

func setDiff(prev, cur []string) []string {
	if len(prev) == 0 {
		return nil
	}
	curSet := make(map[string]bool, len(cur))
	for _, s := range cur {
		curSet[s] = true
	}
	var diff []string
	for _, s := range prev {
		if !curSet[s] {
			diff = append(diff, s)
		}
	}
	return diff
}

func List(log *text.Logger) {
	p := buildPlan()
	pkgToGroup := map[string]string{}
	for _, g := range groups {
		for _, pkg := range g.packages {
			pkgToGroup[pkg] = g.name
		}
	}

	installed, err := packages.CheckInstalled(p.pkgs)
	if err != nil {
		log.Warn("Could not query package status: %s", err)
		return
	}

	missing := map[string][]string{}
	for pkg, gname := range pkgToGroup {
		if !installed[pkg] {
			missing[gname] = append(missing[gname], pkg)
		}
	}
	for _, g := range groups {
		if m := missing[g.name]; len(m) == 0 {
			log.Success("  %s — installed", g.name)
		} else {
			log.Warn("  %s — missing: %s", g.name, strings.Join(m, ", "))
		}
	}
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
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
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
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

func setImmutable(path string, lock bool) {
	op := "+i"
	verb := "lock"
	if !lock {
		op = "-i"
		verb = "unlock"
	}
	if err := exec.Command("chattr", op, path).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not %s %s: %v\n", verb, path, err)
	}
}

func createSourcesBackupOnce(backupPath, path string) error {
	if _, err := os.Stat(backupPath); err == nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if string(data) == sourcesList {
		return nil
	}
	if err := atomicWriteFile(backupPath, data, 0644); err != nil {
		return err
	}
	setImmutable(backupPath, true)
	return nil
}

func ensureSourcesList(force bool) error {
	const path = "/etc/apt/sources.list"
	const backupPath = "/etc/apt/sources.list.debforge-backup"

	if err := createSourcesBackupOnce(backupPath, path); err != nil {
		return fmt.Errorf("backup: %w", err)
	}

	if !force {
		data, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(data), "trixie") {
			return nil
		}
	}

	return atomicWriteFile(path, []byte(sourcesList), 0644)
}

func enablei386(force bool) error {
	if !force {
		cmd := exec.Command("dpkg", "--print-foreign-architectures")
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		err := executil.Run(cmd)
		if err == nil && strings.Contains(stdout.String(), "i386") {
			return nil
		}
	}
	return executil.Run(exec.Command("dpkg", "--add-architecture", "i386"))
}

func installFlathub(log *text.Logger, s *text.Spinner, force bool) error {
	s.Pause()
	err := executil.Run(exec.Command("flatpak", "remote-add", "--if-not-exists", "flathub", "https://flathub.org/repo/flathub.flatpakrepo"))
	s.Resume()
	return err
}

func ensureResolvSymlink(log *text.Logger) error {
	const target = "/run/systemd/resolve/stub-resolv.conf"
	const link = "/etc/resolv.conf"
	fi, err := os.Lstat(link)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		return os.Symlink(target, link)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		log.Warn("%s is a regular file, not a symlink — skipping (systemd-resolved won't manage DNS)", link)
		return nil
	}
	current, err := os.Readlink(link)
	if err != nil {
		return err
	}
	if current == target {
		return nil
	}
	if err := os.Remove(link); err != nil {
		return err
	}
	if err := os.Symlink(target, link); err != nil {
		return err
	}
	if _, err := os.Stat(target); os.IsNotExist(err) {
		log.Warn("/etc/resolv.conf symlink created but systemd-resolved is not running yet; DNS will work once the service starts")
	}
	return nil
}
