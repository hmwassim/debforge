package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	catalogdata "github.com/hmwassim/debforge/catalog"
	"github.com/hmwassim/debforge/internal/apt"
	"github.com/hmwassim/debforge/internal/catalog"
	"github.com/hmwassim/debforge/internal/execute"
	"github.com/hmwassim/debforge/internal/fetch"
	"github.com/hmwassim/debforge/internal/files"
	"github.com/hmwassim/debforge/internal/lock"
	"github.com/hmwassim/debforge/internal/plan"
	"github.com/hmwassim/debforge/internal/state"
	"github.com/hmwassim/debforge/internal/system"
	"github.com/hmwassim/debforge/internal/ui"
)

// Paths are debforge's own locations.
type Paths struct {
	State   string
	Lock    string
	Logs    string
	Work    string
	Overlay string
	Root    string // prefix for managed files ("" in production)
	Sources string // /etc/apt/sources.list.d
}

// DefaultPaths are the production locations.
var DefaultPaths = Paths{
	State:   "/var/lib/debforge/state.json",
	Lock:    "/var/lib/debforge/lock",
	Logs:    "/var/log/debforge",
	Work:    "/var/cache/debforge/work",
	Overlay: "/etc/debforge/packages.d",
	Sources: "/etc/apt/sources.list.d",
}

// App holds the long-lived collaborators of one debforge run.
type App struct {
	Ctx      context.Context
	UI       *ui.UI
	Log      *ui.Log
	R        system.Runner
	Paths    Paths
	Catalog  *catalog.Catalog
	Getenv   func(string) string
	Euid     int
	Hardware func(*catalog.Hardware) bool
	Versions plan.Versions
	Files    *files.Engine
	// EmbeddedFS is the built-in catalog (catalogdata.FS in production).
	EmbeddedFS fs.FS
	Version    string
}

func (a *App) loadCatalog() error {
	if a.Catalog != nil {
		return nil
	}
	efs := a.EmbeddedFS
	if efs == nil {
		efs = catalogdata.FS
	}
	layers := []catalog.Layer{{Name: "embedded", FS: efs, PkgDir: "packages", FilesDir: "files"}}
	if a.Paths.Overlay != "" {
		if fi, err := os.Stat(a.Paths.Overlay); err == nil && fi.IsDir() {
			layers = append(layers, catalog.Layer{Name: a.Paths.Overlay, FS: os.DirFS(a.Paths.Overlay), PkgDir: ".", FilesDir: "files"})
		}
	}
	c, warns, err := catalog.Load(layers...)
	for _, w := range warns {
		a.UI.Warn("%s", w)
	}
	if err != nil {
		return fmt.Errorf("package definitions are invalid:\n%w", err)
	}
	a.Catalog = c
	return nil
}

func (a *App) store() state.Store { return state.Store{Path: a.Paths.State} }

// apt returns the apt wrapper; it keeps the backports source pins in sync
// before installs (see apt.SyncBackportPins).
func (a *App) apt() *apt.Apt { return &apt.Apt{R: a.R, PinFile: a.Paths.Root + apt.PinFile} }

// loadStateRO loads state for read-only commands; unreadable state is a
// warning, never fatal.
func (a *App) loadStateRO() *state.State {
	st, warn, err := a.store().Load()
	if warn != nil {
		a.UI.Warn("%v", warn)
	}
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			a.UI.Warn("cannot read %s; showing packages as not installed", a.Paths.State)
		} else {
			a.UI.Warn("%v", err)
		}
		return state.New()
	}
	return st
}

// session is the context of one mutating operation: root, lock, state.
type session struct {
	a     *App
	lock  *lock.Lock
	st    *state.State
	snap  *system.Snapshot
	user  *system.User
	env   *plan.Env
	exec  *execute.Exec
	force bool
}

func (a *App) begin(force bool) (*session, error) {
	if a.Euid != 0 {
		return nil, errors.New("this command changes the system; run it with sudo")
	}
	if err := a.loadCatalog(); err != nil {
		return nil, err
	}
	l, err := lock.Acquire(a.Ctx, a.Paths.Lock, func() {
		a.UI.Info("waiting for another debforge process (pid %s) to finish...", lock.Holder(a.Paths.Lock))
	})
	if err != nil {
		return nil, fmt.Errorf("acquire lock: %w", err)
	}
	s := &session{a: a, lock: l, force: force}
	st, warn, err := a.store().Load()
	if warn != nil {
		a.UI.Warn("%v", warn)
	}
	if err != nil {
		l.Release()
		return nil, err
	}
	s.st = st
	if err := s.refresh(); err != nil {
		l.Release()
		return nil, err
	}
	if u, err := system.TargetUser(a.Getenv, a.Euid, system.DefaultLookup); err == nil {
		s.user = &u
	} else {
		a.Log.Printf("no target user: %v", err)
	}
	httpc := fetch.NewClient()
	versions := a.Versions
	if versions == nil {
		versions = &fetch.Resolver{R: a.R, HTTP: httpc}
	}
	ext := &apt.Extrepo{R: a.R, SourcesDir: a.Paths.Sources}
	s.env = &plan.Env{
		Cat: a.Catalog, State: st, Snap: s.snap, Versions: versions, Files: a.Files, User: s.user,
		HasHardware: a.Hardware, ExtrepoEnabled: ext.Enabled,
	}
	s.exec = &execute.Exec{
		R: a.R, Apt: a.apt(), Extrepo: ext, Files: a.Files, HTTP: httpc, UI: a.UI, Log: a.Log,
		Store: a.store(), State: st, Snap: s.snap, User: s.user, WorkDir: a.Paths.Work, Force: force,
	}
	return s, nil
}

func (s *session) refresh() error {
	snap, err := system.TakeSnapshot(s.a.Ctx, s.a.R)
	if err != nil {
		return err
	}
	s.snap = snap
	if s.env != nil {
		s.env.Snap = snap
		s.exec.Snap = snap
	}
	return nil
}

func (s *session) end() {
	s.lock.Release()
}

func (s *session) home() string {
	if s.user != nil {
		return s.user.Home
	}
	return ""
}

// watchVersions shows one progress line while the planner checks upstream
// versions, which needs the network. Call the returned function when
// planning ends.
func (s *session) watchVersions() func(ok bool) {
	var prog *ui.Progress
	total := 0
	s.env.Progress = func(name string, done, n int) {
		if prog == nil {
			total = n
			prog = s.a.UI.Start(fmt.Sprintf("Checking %d package(s) for new versions", n))
			return
		}
		prog.Update(fmt.Sprintf("%d/%d %s", done, n, name), float64(done)*100/float64(n))
	}
	return func(ok bool) {
		s.env.Progress = nil
		switch {
		case prog == nil:
		case ok:
			prog.Done(fmt.Sprintf("Checked %d package(s) for new versions", total))
		default:
			prog.Fail("Checking for new versions failed")
		}
	}
}

// confirm prints the plan and asks to proceed. It returns false when there
// is nothing to do, on --dry-run, or when the user declines.
func (s *session) confirm(pl *plan.Plan, inv *invocation) (bool, error) {
	for _, w := range pl.Warnings {
		s.a.UI.Warn("%s", w)
	}
	switch len(pl.Skipped) {
	case 0:
	case 1:
		s.a.UI.Success("%s is already installed and up to date", pl.Skipped[0])
	default:
		s.a.UI.Success("%d packages are up to date", len(pl.Skipped))
		// Name them only when the user named them: with --all the list
		// grows with everything installed and would bury the output.
		if len(inv.args) > 0 {
			s.a.UI.Print("    " + s.a.UI.Dim(strings.Join(pl.Skipped, " ")))
		}
	}
	if pl.Empty() {
		if len(pl.Skipped) == 0 {
			s.a.UI.Info("nothing to do")
		}
		return false, nil
	}
	s.a.UI.Print(pl.Render(s.a.UI, s.home()))
	if inv.has("dry-run") {
		return false, nil
	}
	return s.a.UI.Confirm("Proceed?", true)
}

func (a *App) report(sum *execute.Summary) {
	if sum == nil {
		return
	}
	for _, w := range sum.Warnings {
		a.UI.Warn("%s", w)
	}
	for _, n := range sum.Notes {
		a.UI.Info("%s", n)
	}
}
