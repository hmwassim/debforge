package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hmwassim/debforge/internal/fetch"
	"github.com/hmwassim/debforge/internal/system"
)

const (
	releaseBase = "https://github.com/hmwassim/debforge/releases/latest/download/"
	binaryName  = "debforge-linux-amd64"
	installPath = "/usr/local/bin/debforge"
)

// completionPaths are where install.sh and update --self put completions.
var completionPaths = map[string]string{
	"bash": "/usr/local/share/bash-completion/completions/debforge",
	"zsh":  "/usr/local/share/zsh/site-functions/_debforge",
	"fish": "/usr/local/share/fish/vendor_completions.d/debforge.fish",
}

func cmdUpdateSelf(a *App, inv *invocation) error {
	if a.Euid != 0 {
		return errors.New("update --self must be run with sudo")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}
	if exe != installPath && !inv.has("force") {
		return fmt.Errorf("debforge runs from %s, not %s; refusing to replace a development build (use --force)", exe, installPath)
	}
	httpc := fetch.NewClient()
	dir, err := os.MkdirTemp(filepath.Dir(installPath), ".debforge-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	prog := a.UI.Start("Checking for a new debforge release")
	verFile := filepath.Join(dir, "VERSION")
	if err := fetch.Download(a.Ctx, httpc, releaseBase+"VERSION", verFile, "", nil); err != nil {
		prog.Fail("Could not check for updates")
		return err
	}
	b, _ := os.ReadFile(verFile)
	latest := strings.TrimSpace(string(b))
	if !fetch.ValidVersion(strings.TrimPrefix(latest, "v")) {
		prog.Fail("Invalid release version")
		return fmt.Errorf("release VERSION file contains %q", latest)
	}
	if latest == a.Version && !inv.has("force") {
		prog.Done("debforge " + a.Version + " is up to date")
		return nil
	}
	prog.Done(fmt.Sprintf("New release: %s (installed: %s)", latest, a.Version))
	if inv.has("dry-run") {
		return nil
	}
	ok, err := a.UI.Confirm("Update debforge to "+latest+"?", true)
	if err != nil || !ok {
		return err
	}

	sums := filepath.Join(dir, "SHA256SUMS")
	if err := fetch.Download(a.Ctx, httpc, releaseBase+"SHA256SUMS", sums, "", nil); err != nil {
		return err
	}
	want, err := checksumFor(sums, binaryName)
	if err != nil {
		return err
	}
	newBin := filepath.Join(dir, "debforge")
	prog = a.UI.Start("Downloading debforge " + latest)
	if err := fetch.Download(a.Ctx, httpc, releaseBase+binaryName, newBin, want, nil); err != nil {
		prog.Fail("Download failed")
		return err
	}
	if err := os.Chmod(newBin, 0o755); err != nil {
		return err
	}
	res, err := a.R.Run(a.Ctx, system.Cmd{Name: newBin, Args: []string{"--version"}})
	if err != nil || !strings.Contains(string(res.Stdout), strings.TrimPrefix(latest, "v")) {
		prog.Fail("The downloaded binary did not run")
		return fmt.Errorf("new binary failed its self-check (%q): %v", strings.TrimSpace(string(res.Stdout)), err)
	}
	if err := os.Rename(newBin, installPath); err != nil {
		prog.Fail("Could not install the new binary")
		return err
	}
	prog.Done("Updated debforge to " + latest)
	writeCompletions(a, installPath)
	return nil
}

func checksumFor(sumsFile, name string) (string, error) {
	f, err := os.Open(sumsFile)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name && len(fields[0]) == 64 {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("no checksum for %s in SHA256SUMS", name)
}

func writeCompletions(a *App, bin string) {
	for shell, path := range completionPaths {
		res, err := a.R.Run(a.Ctx, system.Cmd{Name: bin, Args: []string{"completion", shell}})
		if err == nil {
			err = os.MkdirAll(filepath.Dir(path), 0o755)
		}
		if err == nil {
			err = os.WriteFile(path, res.Stdout, 0o644)
		}
		if err != nil {
			a.UI.Warn("could not update %s completion: %v", shell, err)
		}
	}
}

func cmdRemoveSelf(a *App, inv *invocation) error {
	if a.Euid != 0 {
		return errors.New("remove --self must be run with sudo")
	}
	st := a.loadStateRO()
	if inv.has("all") && len(st.Packages) > 0 {
		s, err := a.begin(false)
		if err != nil {
			return err
		}
		pl, err := s.env.Remove(s.st.Names())
		if err != nil {
			s.end()
			return err
		}
		ok, err := s.confirm(pl, inv)
		if err != nil || !ok {
			s.end()
			return err
		}
		sum, err := s.exec.Remove(a.Ctx, pl)
		a.report(sum)
		s.end()
		if err != nil {
			return err
		}
	} else if len(st.Packages) > 0 {
		a.UI.Info("%d installed package(s) stay installed and become unmanaged (use 'remove --self --all' to remove them too)", len(st.Packages))
	}

	targets := []string{installPath, a.Paths.State, filepath.Dir(a.Paths.State), a.Paths.Logs, filepath.Dir(a.Paths.Work)}
	for _, p := range completionPaths {
		targets = append(targets, p)
	}
	a.UI.Print("This removes:\n  " + strings.Join(targets, "\n  ") + "\n  the debforge block in ~/.bashrc")
	a.UI.Print("System configuration written by 'debforge setup' (apt sources, DNS, zram, fonts) is left in place.")
	if inv.has("dry-run") {
		return nil
	}
	ok, err := a.UI.Confirm("Remove debforge?", false)
	if err != nil || !ok {
		return err
	}
	if u, err := system.TargetUser(a.Getenv, a.Euid, system.DefaultLookup); err == nil {
		if err := removeBashrcBlock(a, &u); err != nil {
			a.UI.Warn("~/.bashrc: %v", err)
		}
	}
	for _, p := range targets {
		if err := os.RemoveAll(a.Paths.Root + p); err != nil {
			a.UI.Warn("%s: %v", p, err)
		}
	}
	a.UI.Success("debforge removed")
	if fi, err := os.Stat(a.Paths.Overlay); err == nil && fi.IsDir() {
		a.UI.Info("your own package definitions in %s were kept", a.Paths.Overlay)
	}
	return nil
}

func removeBashrcBlock(a *App, u *system.User) error {
	path := a.Paths.Root + filepath.Join(u.Home, ".bashrc")
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	s := string(b)
	start := strings.Index(s, "# >>> debforge bashrc.d loader >>>")
	end := strings.Index(s, "# <<< debforge bashrc.d loader <<<")
	if start < 0 || end < start {
		return nil
	}
	end += len("# <<< debforge bashrc.d loader <<<")
	if end < len(s) && s[end] == '\n' {
		end++
	}
	out := strings.TrimRight(s[:start], "\n") + "\n" + s[end:]
	f, err := os.CreateTemp(filepath.Dir(path), ".bashrc.debforge-")
	if err != nil {
		return err
	}
	if _, err := io.WriteString(f, out); err != nil {
		f.Close()
		os.Remove(f.Name())
		return err
	}
	f.Chmod(0o644)
	f.Chown(u.UID, u.GID)
	f.Close()
	return os.Rename(f.Name(), path)
}
