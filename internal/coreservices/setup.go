package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hmwassim/debforge/internal/coresetup"
	"github.com/hmwassim/debforge/internal/domain/apt"
	"github.com/hmwassim/debforge/internal/domain/deployer"
	config "github.com/hmwassim/debforge/internal/config"
	"github.com/hmwassim/debforge/internal/ports"
)

type SetupService struct {
	apt       apt.Service
	config    *deployer.Deployer
	logger    ports.UI
	locker    ports.Locker
	fs        ports.FileSystem
	runner    ports.CommandRunner
	cfg       *config.Config
	coreState *CoreStateStore
	fonts     *FontInstaller
}

func NewSetupService(
	apt apt.Service,
	config *deployer.Deployer,
	logger ports.UI,
	locker ports.Locker,
	fs ports.FileSystem,
	runner ports.CommandRunner,
	http ports.HTTPClient,
	cfg *config.Config,
) *SetupService {
	coreStatePath := filepath.Join(cfg.StatesDir, "core.state.json")
	cacheDir := cfg.CacheDir()
	return &SetupService{
		apt:       apt,
		config:    config,
		logger:    logger,
		locker:    locker,
		fs:        fs,
		runner:    runner,
		cfg:       cfg,
		coreState: NewCoreStateStore(fs, runner, logger, coreStatePath),
		fonts:     NewFontInstaller(fs, http, runner, logger, cacheDir, cfg.FontDir, cfg.FontURL),
	}
}

func (s *SetupService) Run(ctx context.Context, force bool) error {
	if err := s.checkRoot(); err != nil {
		return err
	}

	s.logger.Info("Setting up core...")

	if err := config.EnsureDirsExist(s.cfg, s.fs); err != nil {
		return fmt.Errorf("creating directories: %w", err)
	}

	release, err := s.locker.Acquire(ctx, s.cfg.LockFile())
	if err != nil {
		return fmt.Errorf("cannot acquire lock: %w", err)
	}
	defer release()

	st := s.coreState.Load()
	cur := s.coreState.CurrentCommit(ctx, s.cfg.SourceDir())

	desiredPkgs, desiredConfigs := s.collectDesired()
	commitChanged := cur != "" && st.LastSetupCommit != "" && cur != st.LastSetupCommit

	if !force && st.ManagedPackages != nil && !commitChanged && len(setDiff(st.ManagedPackages, desiredPkgs)) == 0 && len(setDiff(st.ManagedConfigs, desiredConfigs)) == 0 {
		if s.verifySetup(ctx, coresetup.GroupDefs) {
			s.logger.Success("Core setup already up to date")
			return nil
		}
	}

	if commitChanged {
		s.logger.Info("New source detected since last setup, reapplying...")
	}

	s.removeStale(ctx, st, desiredPkgs, desiredConfigs)

	var errs []error
	s.prepareApt(ctx, &errs)

	s.installGroups(ctx, &errs)

	if err := s.ensureResolvSymlink(); err != nil {
		errs = append(errs, fmt.Errorf("resolv.conf symlink: %w", err))
	}

	if len(errs) > 0 {
		for _, err := range errs {
			s.logger.Error("%v", err)
		}
		return fmt.Errorf("setup completed with %d error(s)", len(errs))
	}

	st.LastSetupCommit = cur
	st.ManagedPackages = desiredPkgs
	st.ManagedConfigs = desiredConfigs
	if err := s.coreState.Save(st); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	s.logger.Success("Core setup complete")
	return nil
}

func (s *SetupService) checkRoot() error {
	// BYPASS: os.Geteuid is an OS-level root check; no existing port covers it
	if os.Geteuid() != 0 {
		return fmt.Errorf("core setup must be run as root")
	}
	return nil
}

func (s *SetupService) collectDesired() ([]string, []string) {
	var pkgs []string
	var configs []string
	for _, g := range coresetup.GroupDefs {
		pkgs = append(pkgs, g.Packages...)
		for _, cf := range g.Configs {
			configs = append(configs, cf.Dest)
		}
	}
	return pkgs, configs
}

func (s *SetupService) removeStale(ctx context.Context, st *coreState, desiredPkgs, desiredConfigs []string) {
	stalePkgs := setDiff(st.ManagedPackages, desiredPkgs)
	if len(stalePkgs) > 0 {
		s.logger.Info("Removing %d stale package(s)...", len(stalePkgs))
		if err := s.apt.Remove(ctx, stalePkgs); err != nil {
			s.logger.Error("removing stale packages: %v", err)
		}
	}
	for _, path := range setDiff(st.ManagedConfigs, desiredConfigs) {
		s.logger.Info("Removing stale config: %s", path)
		if err := s.fs.RemoveAll(path); err != nil {
			s.logger.Warn("Could not remove %s: %v", path, err)
		}
	}
}

func (s *SetupService) prepareApt(ctx context.Context, errs *[]error) {
	if err := s.ensureSourcesList(ctx); err != nil {
		*errs = append(*errs, fmt.Errorf("sources.list: %w", err))
	}
	if err := s.enablei386(ctx); err != nil {
		*errs = append(*errs, fmt.Errorf("i386: %w", err))
	}
	if len(*errs) == 0 {
		spinner := s.logger.Spinner(ctx, "apt update")
		if err := s.apt.Update(ctx); err != nil {
			spinner.Fail()
			*errs = append(*errs, fmt.Errorf("apt-get update: %w", err))
		} else {
			spinner.Done()
		}
	}
}

func (s *SetupService) installGroups(ctx context.Context, errs *[]error) {
	for _, g := range coresetup.GroupDefs {
		spinner := s.logger.Spinner(ctx, g.Name)
		var failed bool

		if len(g.Packages) > 0 {
			var pkgErr error
			if g.Backport {
				pkgErr = s.apt.InstallBackports(ctx, g.Packages, "")
			} else {
				pkgErr = s.apt.Install(ctx, g.Packages)
			}
			if pkgErr != nil {
				*errs = append(*errs, fmt.Errorf("installing %s: %w", g.Name, pkgErr))
				failed = true
			}
		}

		if !failed {
			for _, cf := range g.Configs {
				if err := s.config.Deploy(ctx, cf.Content, cf.Dest, cf.Mode); err != nil {
					*errs = append(*errs, fmt.Errorf("deploying %s: %w", cf.Dest, err))
					failed = true
				}
			}
		}

		if !failed {
			for _, svc := range g.Services {
				_, _, err := s.runner.Run(ctx, "systemctl", "enable", "--now", svc)
				if err != nil {
					s.logger.Warn("Could not enable %s (non-fatal): %v", svc, err)
				}
			}
		}

		if !failed {
			switch g.PostInstall {
			case "fonts":
				if err := s.fonts.Install(ctx, spinner); err != nil {
					*errs = append(*errs, err)
					failed = true
				}
			case "flathub":
				if err := s.installFlathub(ctx); err != nil {
					*errs = append(*errs, err)
					failed = true
				}
			case "resolved":
				if err := s.setupResolved(ctx); err != nil {
					*errs = append(*errs, err)
					failed = true
				}
			}
		}

		if failed {
			spinner.Fail()
		} else {
			spinner.Done()
		}
	}
}

func (s *SetupService) verifySetup(ctx context.Context, defs []coresetup.GroupDef) bool {
	for _, g := range defs {
		for _, pkg := range g.Packages {
			ok, err := s.apt.CheckInstalled(ctx, pkg)
			if err != nil || !ok {
				return false
			}
		}
	}

	for _, g := range defs {
		for _, cf := range g.Configs {
			data, err := s.fs.ReadFile(cf.Dest)
			if err != nil || string(data) != cf.Content {
				return false
			}
			fi, err := s.fs.Stat(cf.Dest)
			if err != nil || fi.Mode().Perm() != cf.Mode {
				return false
			}
		}
	}

	for _, g := range defs {
		for _, svc := range g.Services {
			_, _, err := s.runner.Run(ctx, "systemctl", "is-active", "--quiet", svc)
			if err != nil {
				return false
			}
			_, _, err = s.runner.Run(ctx, "systemctl", "is-enabled", "--quiet", svc)
			if err != nil {
				return false
			}
		}
	}

	return true
}

func (s *SetupService) ensureSourcesList(ctx context.Context) error {
	path := s.cfg.SourcesListPath
	backupPath := path + ".debforge-backup"

	if err := s.createSourcesBackupOnce(ctx, backupPath, path); err != nil {
		return fmt.Errorf("backup: %w", err)
	}

	data, err := s.fs.ReadFile(path)
	if err == nil && strings.Contains(string(data), "trixie") {
		return nil
	}

	return s.fs.AtomicWriteFile(path, []byte(coresetup.SourcesList), 0644)
}

func (s *SetupService) createSourcesBackupOnce(ctx context.Context, backupPath, path string) error {
	if _, err := s.fs.Stat(backupPath); err == nil {
		return nil
	}
	data, err := s.fs.ReadFile(path)
	if err != nil {
		return nil
	}
	if string(data) == coresetup.SourcesList {
		return nil
	}
	if err := s.fs.AtomicWriteFile(backupPath, data, 0644); err != nil {
		return err
	}
	_, _, err = s.runner.Run(ctx, "chattr", "+i", backupPath)
	if err != nil {
		s.logger.Warn("could not lock backup %s: %v", backupPath, err)
	}
	return nil
}

func (s *SetupService) enablei386(ctx context.Context) error {
	stdout, _, err := s.runner.Run(ctx, "dpkg", "--print-foreign-architectures")
	if err == nil && strings.Contains(string(stdout), "i386") {
		return nil
	}
	_, _, err = s.runner.Run(ctx, "dpkg", "--add-architecture", "i386")
	return err
}

func (s *SetupService) ensureResolvSymlink() error {
	target := s.cfg.StubResolvConfPath
	link := s.cfg.ResolvConfPath

	fi, err := s.fs.Lstat(link)
	if err != nil {
		if os.IsNotExist(err) {
			return s.fs.Symlink(target, link)
		}
		return err
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		s.logger.Warn("%s is a regular file, not a symlink — skipping (systemd-resolved won't manage DNS)", link)
		return nil
	}
	current, err := s.fs.Readlink(link)
	if err != nil {
		return err
	}
	if current == target {
		return nil
	}
	if err := s.fs.RemoveAll(link); err != nil {
		return err
	}
	if err := s.fs.Symlink(target, link); err != nil {
		return err
	}
	if _, err := s.fs.Stat(target); os.IsNotExist(err) {
		s.logger.Warn("%s symlink created but systemd-resolved is not running yet", link)
	}
	return nil
}

func (s *SetupService) installFlathub(ctx context.Context) error {
	stdout, _, err := s.runner.Run(ctx, "flatpak", "remotes", "--columns=name")
	if err == nil && containsLine(string(stdout), "flathub") {
		return nil
	}
	_, _, err = s.runner.Run(ctx, "flatpak", "remote-add", "--if-not-exists", "flathub", "https://flathub.org/repo/flathub.flatpakrepo")
	return err
}

func containsLine(haystack, needle string) bool {
	for _, line := range strings.Split(haystack, "\n") {
		if strings.TrimSpace(line) == needle {
			return true
		}
	}
	return false
}

func (s *SetupService) setupResolved(ctx context.Context) error {
	if err := s.ensureResolvSymlink(); err != nil {
		return fmt.Errorf("resolv.conf symlink: %w", err)
	}
	if _, _, err := s.runner.Run(ctx, "nmcli", "general", "reload"); err != nil {
		s.logger.Warn("nmcli reload failed (non-fatal): %v", err)
	}
	if _, _, err := s.runner.Run(ctx, "systemctl", "restart", "systemd-resolved"); err != nil {
		s.logger.Warn("restarting systemd-resolved failed (non-fatal): %v", err)
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for i := 0; i < 15; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			_, _, err := s.runner.Run(ctx, "resolvectl", "query", "debian.org")
			if err == nil {
				return nil
			}
		}
	}

	s.logger.Warn("DNS resolution check failed after configuring systemd-resolved")
	return nil
}
