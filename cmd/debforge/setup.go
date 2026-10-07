package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	catalogdata "github.com/hmwassim/debforge/catalog"
	"github.com/hmwassim/debforge/internal/apt"
	"github.com/hmwassim/debforge/internal/files"
	"github.com/hmwassim/debforge/internal/setup"
	"github.com/hmwassim/debforge/internal/state"
	"github.com/hmwassim/debforge/internal/system"
)

func (a *App) profile() (*setup.Profile, error) {
	fsys := a.EmbeddedFS
	if fsys == nil {
		fsys = catalogdata.FS
	}
	return setup.Load(fsys, "default")
}

func (a *App) setupEngine(st *state.State, snap *system.Snapshot, user *system.User) *setup.Engine {
	fe := a.Files
	if fe == nil {
		fe = &files.Engine{}
	}
	return &setup.Engine{
		R: a.R, Apt: &apt.Apt{R: a.R}, Files: fe, State: st, Snap: snap, User: user,
		Facts: setup.DetectFacts(a.Paths.Root, snap), Root: a.Paths.Root,
	}
}

func (a *App) describe(r setup.Result) string {
	var mark string
	switch r.Status {
	case setup.OK:
		mark = a.UI.Green("✓")
	case setup.NotApplicable:
		mark = a.UI.Dim("-")
	case setup.Needed:
		mark = a.UI.Blue("•")
	case setup.Modified:
		mark = a.UI.Yellow("~")
	case setup.Failed:
		mark = a.UI.Red("✗")
	}
	s := fmt.Sprintf("%s %s", mark, r.Step.Title)
	if r.Status == setup.NotApplicable {
		s += a.UI.Dim(" (not applicable)")
	}
	for _, reason := range r.Reasons {
		s += "\n    " + a.UI.Dim(reason)
	}
	return s
}

func cmdSetup(a *App, inv *invocation) error {
	force := inv.has("force")
	s, err := a.begin(force)
	if err != nil {
		return err
	}
	defer s.end()
	prof, err := a.profile()
	if err != nil {
		return err
	}

	ap := &apt.Apt{R: a.R}
	prog := a.UI.Start("Refreshing package lists")
	warns, err := ap.Update(a.Ctx)
	for _, w := range warns {
		a.UI.Warn("%s", w)
	}
	if err != nil {
		prog.Fail("Refreshing package lists failed")
		return err
	}
	prog.Done("Refreshed package lists")
	if err := s.refresh(); err != nil {
		return err
	}

	eng := a.setupEngine(s.st, s.snap, s.user)
	var todo []*setup.Step
	var lines []string
	for _, step := range prof.Steps {
		r := eng.Check(a.Ctx, step)
		if r.Status == setup.Failed {
			return fmt.Errorf("checking %q: %s", step.Title, strings.Join(r.Reasons, "; "))
		}
		if r.Status == setup.NotApplicable {
			continue
		}
		if r.Status == setup.Needed || force {
			todo = append(todo, step)
			lines = append(lines, a.describe(r))
		} else if r.Status == setup.Modified {
			lines = append(lines, a.describe(r))
		}
	}
	if len(todo) == 0 {
		for _, l := range lines {
			a.UI.Print(l)
		}
		a.UI.Success("the system is set up")
		return nil
	}
	a.UI.Print(a.UI.Bold("Setup will apply:") + "\n" + strings.Join(lines, "\n"))
	if inv.has("dry-run") {
		return nil
	}
	ok, err := a.UI.Confirm(fmt.Sprintf("Apply %d step(s)?", len(todo)), true)
	if err != nil || !ok {
		return err
	}

	rebootHint := false
	for _, step := range todo {
		if err := a.Ctx.Err(); err != nil {
			return fmt.Errorf("stopped before %q: %w", step.Title, err)
		}
		p := a.UI.Start(step.Title)
		notes, err := eng.Apply(a.Ctx, step, force, func(_ string, pct float64, d string) { p.Update(d, pct) })
		for _, n := range notes {
			a.UI.Info("%s", n)
		}
		// Record progress after every step so a failure later keeps it.
		s.st.Setup.Steps[step.ID] = state.Step{AppliedAt: time.Now()}
		if serr := a.store().Save(s.st); serr != nil {
			err = errors.Join(err, serr)
		}
		if err != nil {
			p.Fail(step.Title + " failed")
			return fmt.Errorf("%s: %w (run 'debforge setup' again to continue)", step.Title, err)
		}
		p.Done(step.Title)
		switch step.ID {
		case "kernel", "firmware", "microcode-intel", "microcode-amd":
			rebootHint = true
		}
	}
	a.UI.Success("setup complete")
	if rebootHint {
		a.UI.Info("reboot to use the new kernel, firmware or microcode")
	}
	return nil
}

func cmdDoctor(a *App, inv *invocation) error {
	prof, err := a.profile()
	if err != nil {
		return err
	}
	snap, err := system.TakeSnapshot(a.Ctx, a.R)
	if err != nil {
		return err
	}
	st := a.loadStateRO()
	var user *system.User
	if u, err := system.TargetUser(a.Getenv, a.Euid, system.DefaultLookup); err == nil {
		user = &u
	}
	eng := a.setupEngine(st, snap, user)
	bad := 0
	for _, step := range prof.Steps {
		r := eng.Check(a.Ctx, step)
		a.UI.Print(a.describe(r))
		if r.Status == setup.Needed || r.Status == setup.Failed {
			bad++
		}
	}
	if bad > 0 {
		return fmt.Errorf("%d setup step(s) need attention; run 'sudo debforge setup'", bad)
	}
	a.UI.Success("all setup steps are in place")
	return nil
}
