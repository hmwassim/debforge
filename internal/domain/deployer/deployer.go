package deployer

import (
	"context"
	"fmt"
	"os"
	osuser "os/user"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hmwassim/debforge/internal/ports"
)

var homeDirCache sync.Map

// InvokingUser returns the non-root user who invoked the program via sudo.
// Returns error if not running under sudo with a valid user.
// BYPASS: uses os.Getenv and os.Geteuid — no existing port covers env/UID queries
func InvokingUser() (string, error) {
	if u := os.Getenv("SUDO_USER"); u != "" {
		return u, nil
	}
	if os.Geteuid() == 0 {
		return "", fmt.Errorf("not running under sudo; SUDO_USER not set")
	}
	return "", fmt.Errorf("must run as root via sudo")
}

type Deployer struct {
	fs     ports.FileSystem
	runner ports.CommandRunner
	logger ports.UI
}

func NewDeployer(fs ports.FileSystem, runner ports.CommandRunner, logger ports.UI) *Deployer {
	return &Deployer{fs: fs, runner: runner, logger: logger}
}

func (d *Deployer) Deploy(ctx context.Context, content, path string, mode os.FileMode) error {
	if err := d.fs.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	existing, err := d.fs.ReadFile(path)
	if err == nil {
		if string(existing) == content {
			fi, statErr := d.fs.Stat(path)
			if statErr == nil && fi.Mode().Perm() == mode {
				return nil
			}
			return d.fs.Chmod(path, mode)
		}
	}
	return d.fs.AtomicWriteFile(path, []byte(content), mode)
}

func UserHomeDir(name string) string {
	if name == "root" {
		return "/root"
	}
	if cached, ok := homeDirCache.Load(name); ok {
		return cached.(string)
	}
	u, err := osuser.Lookup(name)
	var home string
	if err == nil && u.HomeDir != "" {
		home = u.HomeDir
	} else {
		home = filepath.Join("/home", name)
	}
	homeDirCache.Store(name, home)
	return home
}

func (d *Deployer) DeployUserConfig(ctx context.Context, content, path, user string) error {
	home := UserHomeDir(user)
	fullPath := filepath.Clean(filepath.Join(home, path))
	cleanHome := filepath.Clean(home)
	if !strings.HasPrefix(fullPath, cleanHome+string(os.PathSeparator)) && fullPath != cleanHome {
		return fmt.Errorf("path %q escapes user home %q", path, home)
	}
	if err := d.fs.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}
	existing, err := d.fs.ReadFile(fullPath)
	if err == nil && string(existing) == content {
		return nil
	}
	if err := d.fs.AtomicWriteFile(fullPath, []byte(content), 0644); err != nil {
		return err
	}
	if user != "root" {
		if _, _, err := d.runner.Run(ctx, "chown", user+":"+user, fullPath); err != nil {
			d.logger.Warn("chown %s: %v", fullPath, err)
		}
	}
	return nil
}

func (d *Deployer) DeployPackageConfigs(ctx context.Context, configs, userConfigs map[string]string) error {
	for dest, source := range configs {
		if err := d.Deploy(ctx, source, dest, 0644); err != nil {
			return fmt.Errorf("deploying %s: %w", dest, err)
		}
	}
	if len(userConfigs) > 0 {
		user, err := InvokingUser()
		if err != nil {
			return fmt.Errorf("determine invoking user: %w", err)
		}
		for path, source := range userConfigs {
			if err := d.DeployUserConfig(ctx, source, path, user); err != nil {
				return fmt.Errorf("deploying user config %s: %w", path, err)
			}
		}
	}
	return nil
}

func (d *Deployer) RemoveConfigs(ctx context.Context, configs map[string]string) error {
	for path := range configs {
		if err := d.fs.RemoveAll(path); err != nil {
			d.logger.Warn("could not remove %s: %v", path, err)
		}
	}
	return nil
}

func (d *Deployer) RemoveUserConfigs(ctx context.Context, userConfigs map[string]string, user string) error {
	home := UserHomeDir(user)
	cleanHome := filepath.Clean(home)
	for path := range userConfigs {
		fullPath := filepath.Clean(filepath.Join(home, path))
		if !strings.HasPrefix(fullPath, cleanHome+string(os.PathSeparator)) && fullPath != cleanHome {
			d.logger.Warn("path %q escapes user home %q, skipping", path, home)
			continue
		}
		if err := d.fs.RemoveAll(fullPath); err != nil {
			d.logger.Warn("could not remove %s: %v", fullPath, err)
		}
	}
	return nil
}

func (d *Deployer) runScript(ctx context.Context, script string, onErr func(error)) {
	if script == "" {
		return
	}
	tmpDir, err := d.fs.MkdirTemp("", "debforge-script-*")
	if err != nil {
		onErr(fmt.Errorf("creating temp dir: %w", err))
		return
	}
	defer d.fs.RemoveAll(tmpDir)
	scriptPath := filepath.Join(tmpDir, "run.sh")
	content := "#!/bin/sh\n" + script + "\n"
	if err := d.fs.WriteFile(scriptPath, []byte(content), 0755); err != nil {
		onErr(fmt.Errorf("writing script: %w", err))
		return
	}
	_, _, err = d.runner.Run(ctx, scriptPath)
	if err != nil {
		onErr(err)
	}
}

func (d *Deployer) RunPostInstall(ctx context.Context, script string) error {
	var firstErr error
	d.runScript(ctx, script, func(err error) {
		if firstErr == nil {
			firstErr = fmt.Errorf("post-install: %w", err)
		}
	})
	return firstErr
}

func (d *Deployer) RunPostRemove(ctx context.Context, script string) error {
	d.runScript(ctx, script, func(err error) {
		d.logger.Warn("post-remove: %v", err)
	})
	return nil
}
