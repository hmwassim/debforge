package setup

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hmwassim/debforge/internal/apt"
	"github.com/hmwassim/debforge/internal/files"
	"github.com/hmwassim/debforge/internal/state"
	"github.com/hmwassim/debforge/internal/system"
)

// Facts describe the machine; they decide which steps apply.
type Facts struct {
	CPUVendor      string // intel, amd or ""
	NetworkManager bool
	Desktop        string // kde, gnome or ""
}

// DetectFacts inspects the machine. root prefixes /proc ("" in production).
func DetectFacts(root string, snap *system.Snapshot) Facts {
	f := Facts{NetworkManager: snap.Installed("network-manager")}
	if b, err := os.ReadFile(root + "/proc/cpuinfo"); err == nil {
		switch {
		case strings.Contains(string(b), "GenuineIntel"):
			f.CPUVendor = "intel"
		case strings.Contains(string(b), "AuthenticAMD"):
			f.CPUVendor = "amd"
		}
	}
	// The desktop is detected from installed packages, not from the
	// environment: XDG_CURRENT_DESKTOP is stripped by sudo.
	switch {
	case snap.Installed("plasma-workspace"):
		f.Desktop = "kde"
	case snap.Installed("gnome-shell"):
		f.Desktop = "gnome"
	}
	return f
}

// Applies reports whether s is relevant on this machine.
func (s *Step) Applies(f Facts) bool {
	w := s.When
	if w == nil {
		return true
	}
	if w.CPUVendor != "" && w.CPUVendor != f.CPUVendor {
		return false
	}
	if w.NetworkManager != nil && *w.NetworkManager != f.NetworkManager {
		return false
	}
	if w.Desktop != "" && w.Desktop != f.Desktop {
		return false
	}
	return true
}

// Status is the result of checking a step.
type Status int

const (
	OK Status = iota
	NotApplicable
	Needed
	Modified // only user-modified files differ; not changed without --force
	Failed
)

// Result is a step check result.
type Result struct {
	Step    *Step
	Status  Status
	Reasons []string
}

// Engine checks and applies steps.
type Engine struct {
	R     system.Runner
	Apt   *apt.Apt
	Files *files.Engine
	State *state.State
	Snap  *system.Snapshot
	User  *system.User
	Facts Facts
	// Root prefixes paths read directly (tests); "" in production.
	Root string
	// Sleep is used between verify retries (tests replace it).
	Sleep func(time.Duration)
}

func (e *Engine) sh(ctx context.Context, script string) error {
	env := []string{}
	if e.User != nil {
		env = append(env, "TARGET_USER="+e.User.Name, "TARGET_HOME="+e.User.Home)
	}
	_, err := e.R.Run(ctx, system.Cmd{Name: "sh", Args: []string{"-eu", "-c", script}, Env: env, Timeout: 5 * time.Minute})
	return err
}

func (e *Engine) spec(f fileLike) (files.Spec, error) {
	s := files.Spec{Path: f.dest(), Data: f.data(), Mode: f.mode()}
	if strings.HasPrefix(f.dest(), "~/") {
		if e.User == nil {
			return s, system.ErrNoTargetUser
		}
		s.Path = e.User.ExpandHome(f.dest())
		s.Home, s.UID, s.GID = e.User.Home, e.User.UID, e.User.GID
	}
	return s, nil
}

func (e *Engine) record(path string) *state.File {
	if r, ok := e.State.Setup.Files[path]; ok {
		return &r
	}
	return nil
}

// Check inspects one step without changing anything.
func (e *Engine) Check(ctx context.Context, s *Step) Result {
	r := Result{Step: s, Status: OK}
	if !s.Applies(e.Facts) {
		r.Status = NotApplicable
		return r
	}
	need := func(f string, a ...any) {
		r.Reasons = append(r.Reasons, fmt.Sprintf(f, a...))
		if r.Status == OK || r.Status == Modified {
			r.Status = Needed
		}
	}
	fail := func(err error) Result {
		r.Status = Failed
		r.Reasons = append(r.Reasons, err.Error())
		return r
	}

	if s.Builtin != "" {
		if err := e.checkBuiltin(ctx, s, need); err != nil {
			return fail(err)
		}
	}
	var missing []string
	for _, p := range s.Packages {
		if !e.Snap.Installed(p) {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		need("not installed: %s", strings.Join(missing, " "))
	} else if s.Backports && len(s.Packages) > 0 {
		n, err := e.Apt.PendingInstall(ctx, s.Packages, apt.BackportsSuite)
		if err != nil {
			return fail(err)
		}
		if n > 0 {
			need("%d package(s) can be upgraded from backports", n)
		}
	}
	for _, f := range e.allFiles(s) {
		spec, err := e.spec(f)
		if err != nil {
			return fail(err)
		}
		st, err := e.Files.Classify(spec, e.record(spec.Path))
		if err != nil {
			return fail(err)
		}
		switch st {
		case files.Missing:
			need("%s will be created", spec.Path)
		case files.Outdated:
			need("%s will be updated", spec.Path)
		case files.Foreign:
			need("%s differs and will be replaced (the current file is kept as .debforge-orig)", spec.Path)
		case files.Adoptable:
			need("%s already matches; it will be tracked", spec.Path)
		case files.UserModified:
			r.Reasons = append(r.Reasons, spec.Path+": modified by you (kept)")
			if r.Status == OK {
				r.Status = Modified
			}
		case files.Conflict:
			r.Reasons = append(r.Reasons, spec.Path+": modified by you; new defaults differ (see debforge diff after setup)")
			if r.Status == OK {
				r.Status = Modified
			}
		}
	}
	for _, sv := range s.Services {
		if sv.Enable && !e.unitIs(ctx, "is-enabled", sv.Name, "enabled") {
			need("%s is not enabled", sv.Name)
		}
		if sv.Start && !e.unitIs(ctx, "is-active", sv.Name, "active") {
			need("%s is not running", sv.Name)
		}
	}
	for _, c := range s.Commands {
		if e.sh(ctx, c.Check) != nil {
			need("check failed: %s", firstLine(c.Check))
		}
	}
	return r
}

func (e *Engine) unitIs(ctx context.Context, verb, unit, want string) bool {
	res, _ := e.R.Run(ctx, system.Cmd{Name: "systemctl", Args: []string{verb, unit}})
	return strings.TrimSpace(string(res.Stdout)) == want
}

// Apply brings a step into the desired state. force replaces
// user-modified files (with backup).
func (e *Engine) Apply(ctx context.Context, s *Step, force bool, prog apt.ProgressFunc) (notes []string, err error) {
	if s.Builtin != "" {
		n, err := e.applyBuiltin(ctx, s, force, prog)
		notes = append(notes, n...)
		if err != nil {
			return notes, err
		}
	}
	install := !e.Snap.AllInstalled(s.Packages)
	if !install && s.Backports && len(s.Packages) > 0 {
		n, err := e.Apt.PendingInstall(ctx, s.Packages, apt.BackportsSuite)
		if err != nil {
			return notes, err
		}
		install = n > 0
	}
	if install {
		if err := e.Apt.Preseed(ctx, s.Debconf); err != nil {
			return notes, err
		}
		tx := apt.Transaction{Install: s.Packages}
		if s.Backports {
			tx.Target = apt.BackportsSuite
		}
		if err := e.Apt.Install(ctx, tx, prog); err != nil {
			return notes, err
		}
		if snap, err := system.TakeSnapshot(ctx, e.R); err == nil {
			*e.Snap = *snap
		}
	}
	changed := false
	for _, f := range e.allFiles(s) {
		spec, err := e.spec(f)
		if err != nil {
			return notes, err
		}
		out, err := e.Files.Apply(spec, e.record(spec.Path), force)
		if err != nil {
			return notes, err
		}
		e.State.Setup.Files[spec.Path] = out.Record
		switch out.Action {
		case "written", "replaced":
			changed = true
			if out.Note != "" {
				notes = append(notes, fmt.Sprintf("%s existed; previous version saved as %s", spec.Path, out.Note))
			}
		case "sidecar":
			notes = append(notes, fmt.Sprintf("%s was modified by you; new defaults are in %s", spec.Path, out.Note))
		}
	}
	if changed && s.Builtin == "sources" {
		warns, err := e.Apt.Update(ctx)
		notes = append(notes, warns...)
		if err != nil {
			return notes, err
		}
	}
	if changed && len(s.Reload) > 0 {
		want := map[string]bool{}
		for _, r := range s.Reload {
			want[r] = true
		}
		for _, err := range system.Reload(ctx, e.R, want) {
			notes = append(notes, err.Error())
		}
	}
	for _, c := range s.Commands {
		if e.sh(ctx, c.Check) == nil {
			continue
		}
		if err := e.sh(ctx, c.Apply); err != nil {
			return notes, fmt.Errorf("%s: %w", firstLine(c.Apply), err)
		}
	}
	for _, sv := range s.Services {
		if err := e.service(ctx, sv, changed); err != nil {
			return notes, err
		}
	}
	if s.Verify != "" {
		if err := e.verify(ctx, s.Verify); err != nil {
			return notes, err
		}
	}
	return notes, nil
}

func (e *Engine) service(ctx context.Context, sv Service, changed bool) error {
	run := func(args ...string) error {
		_, err := e.R.Run(ctx, system.Cmd{Name: "systemctl", Args: args, Timeout: 2 * time.Minute})
		return err
	}
	if changed {
		if err := run("daemon-reload"); err != nil {
			return err
		}
	}
	if sv.Enable && !e.unitIs(ctx, "is-enabled", sv.Name, "enabled") {
		if err := run("enable", sv.Name); err != nil {
			return fmt.Errorf("enable %s: %w", sv.Name, err)
		}
	}
	active := e.unitIs(ctx, "is-active", sv.Name, "active")
	switch {
	case sv.Start && !active:
		if err := run("start", sv.Name); err != nil {
			return fmt.Errorf("start %s: %w", sv.Name, err)
		}
	case changed && sv.RestartOnChange && active:
		if err := run("restart", sv.Name); err != nil {
			return fmt.Errorf("restart %s: %w", sv.Name, err)
		}
	case changed && sv.ReloadOnChange && active:
		if err := run("reload", sv.Name); err != nil {
			return fmt.Errorf("reload %s: %w", sv.Name, err)
		}
	}
	return nil
}

func (e *Engine) verify(ctx context.Context, script string) error {
	sleep := e.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	var err error
	for i := 0; i < 15; i++ {
		if err = e.sh(ctx, script); err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		sleep(2 * time.Second)
	}
	return fmt.Errorf("verification failed after 30s (%s): %w", firstLine(script), err)
}

func firstLine(s string) string {
	l, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return l
}

// fileLike lets builtins contribute generated files alongside catalog.File.
type fileLike interface {
	dest() string
	data() []byte
	mode() os.FileMode
}

type genFile struct {
	path    string
	content []byte
}

func (g genFile) dest() string      { return g.path }
func (g genFile) data() []byte      { return g.content }
func (g genFile) mode() os.FileMode { return 0o644 }

func (e *Engine) allFiles(s *Step) []fileLike {
	var out []fileLike
	for _, f := range s.Files {
		out = append(out, catalogFile{f.Dest, f.Data(), f.FileMode()})
	}
	return append(out, e.builtinFiles(s)...)
}

type catalogFile struct {
	d string
	b []byte
	m os.FileMode
}

func (c catalogFile) dest() string      { return c.d }
func (c catalogFile) data() []byte      { return c.b }
func (c catalogFile) mode() os.FileMode { return c.m }
