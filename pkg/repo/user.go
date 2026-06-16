package repo

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"

	"github.com/hmwassim/debforge/pkg/text"
	"github.com/hmwassim/debforge/pkg/writeutil"
)

func userHomeDir() (string, error) {
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		u, err := user.Lookup(sudoUser)
		if err == nil {
			return u.HomeDir, nil
		}
	}
	return os.UserHomeDir()
}

func deployUserConfigs(configs map[string]string) error {
	if len(configs) == 0 {
		return nil
	}
	home, err := userHomeDir()
	if err != nil {
		return fmt.Errorf("resolving home directory: %w", err)
	}
	for path, content := range configs {
		fullPath := filepath.Join(home, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return fmt.Errorf("creating directory for %s: %w", path, err)
		}
		if err := writeutil.AtomicFile(fullPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
	}
	chownUserConfigs(configs)
	return nil
}

func chownUserConfigs(configs map[string]string) {
	uid, _ := strconv.Atoi(os.Getenv("SUDO_UID"))
	gid, _ := strconv.Atoi(os.Getenv("SUDO_GID"))
	if uid == 0 {
		return
	}
	home, err := userHomeDir()
	if err != nil {
		return
	}
	for path := range configs {
		os.Chown(filepath.Join(home, path), uid, gid)
	}
}

func removeUserConfigs(log *text.Logger, configs map[string]string) {
	if len(configs) == 0 {
		return
	}
	home, err := userHomeDir()
	if err != nil {
		return
	}
	for path := range configs {
		fullPath := filepath.Join(home, path)
		if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
			log.Warn("Could not remove %s: %s", path, err)
		}
	}
}

func userCmd(name string, arg ...string) *exec.Cmd {
	cmd := exec.Command(name, arg...)
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		uid := os.Getenv("SUDO_UID")
		if uid != "" {
			cmd = exec.Command("sudo", "-u", sudoUser, "-H",
				"--preserve-env=XDG_RUNTIME_DIR,DBUS_SESSION_BUS_ADDRESS",
				"sh", "-c", arg[1])
			cmd.Env = os.Environ()
			cmd.Env = append(cmd.Env,
				"XDG_RUNTIME_DIR=/run/user/"+uid,
				"DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/"+uid+"/bus")
		} else {
			cmd = exec.Command("sudo", "-u", sudoUser, "-H", "sh", "-c", arg[1])
			cmd.Env = os.Environ()
		}
	}
	return cmd
}
