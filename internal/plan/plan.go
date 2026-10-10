// Package plan computes what an operation will do before anything is
// changed. The executor only ever carries out a Plan the user has seen.
package plan

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/hmwassim/debforge/internal/catalog"
	"github.com/hmwassim/debforge/internal/files"
	"github.com/hmwassim/debforge/internal/state"
	"github.com/hmwassim/debforge/internal/system"
)

// Op is what happens to a package.
type Op string

const (
	OpInstall   Op = "install"
	OpUpgrade   Op = "upgrade"
	OpReinstall Op = "reinstall"
	OpRemove    Op = "remove"
)

// FileChange is the predicted effect on one managed file.
type FileChange struct {
	Path  string
	State files.State
}

// Item is one package in the plan.
type Item struct {
	Name       string
	Pkg        *catalog.Package // nil for removal of a package no longer in the catalog
	Op         Op
	Explicit   bool
	Reason     string
	Version    string
	OldVersion string
	Variant    string
	// Apt lists for installs.
	Apt       []string
	Backports []string
	// AptRecord is what the manifest will claim: apt packages this
	// package brings that were not already on the system (or that other
	// debforge packages also claim).
	AptRecord []string
	Files     []FileChange
}

// Plan is the full set of changes.
type Plan struct {
	Items []*Item
	// Install-side apt work.
	AptInstall   []string
	AptBackports []string
	AptConflicts []string
	Extrepos     []string
	// Repos lists packages whose own apt repository will be configured.
	Repos   []string
	Debconf []string
	// Remove-side apt work.
	AptRemove       []string
	ExtreposDisable []string
	Warnings        []string
	// Skipped lists requested packages that need nothing.
	Skipped []string
}

// Empty reports whether the plan changes nothing.
func (p *Plan) Empty() bool { return len(p.Items) == 0 }

// Versions resolves the version to install for non-apt packages.
type Versions interface {
	Resolve(ctx context.Context, p *catalog.Package) (string, error)
}

// Env is everything the planner reads.
type Env struct {
	Cat            *catalog.Catalog
	State          *state.State
	Snap           *system.Snapshot
	Versions       Versions
	Files          *files.Engine
	User           *system.User // nil when no target user is known
	HasHardware    func(*catalog.Hardware) bool
	ExtrepoEnabled func(string) bool
	// Progress, if set, is called as upstream versions are resolved:
	// first with done=0, then once per package.
	Progress func(name string, done, total int)
}

// resolveWorkers bounds concurrent upstream version lookups.
const resolveWorkers = 8

type resolved struct {
	version string
	err     error
}

// resolveVersions looks up every upstream version pkgs need concurrently,
// so planning many packages costs about one network round trip, not one
// per package.
func (e *Env) resolveVersions(ctx context.Context, pkgs []*catalog.Package) map[string]resolved {
	var todo []*catalog.Package
	for _, p := range pkgs {
		if p.Kind() != catalog.KindApt && p.Kind() != catalog.KindConfig && p.Version != nil {
			todo = append(todo, p)
		}
	}
	out := make(map[string]resolved, len(todo))
	if len(todo) == 0 {
		return out
	}
	if e.Progress != nil {
		e.Progress("", 0, len(todo))
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, resolveWorkers)
	for _, p := range todo {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			v, err := e.Versions.Resolve(ctx, p)
			<-sem
			mu.Lock()
			defer mu.Unlock()
			out[p.Name] = resolved{v, err}
			if e.Progress != nil {
				e.Progress(p.Name, len(out), len(todo))
			}
		}()
	}
	wg.Wait()
	return out
}

// Options modify planning.
type Options struct {
	Force    bool
	Variants map[string]string
}

// NeedsVariant returns packages in the dependency closure of names that
// have variants but no choice (neither in opts nor recorded in state).
func (e *Env) NeedsVariant(names []string, opts Options) ([]*catalog.Package, error) {
	pkgs, err := Resolve(e.Cat, names)
	if err != nil {
		return nil, err
	}
	var out []*catalog.Package
	for _, p := range pkgs {
		if p.Kind() != catalog.KindApt || len(p.Source.Apt.Variants) == 0 {
			continue
		}
		if opts.Variants[p.Name] != "" {
			continue
		}
		if s, ok := e.State.Packages[p.Name]; ok && s.Variant != "" {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// Install plans installing names (and their dependencies).
func (e *Env) Install(ctx context.Context, names []string, o Options) (*Plan, error) {
	return e.build(ctx, names, o, false)
}

// Update plans updating names, or every installed package when names is
// empty.
func (e *Env) Update(ctx context.Context, names []string, o Options) (*Plan, error) {
	pl := &Plan{}
	if len(names) == 0 {
		for _, n := range e.State.Names() {
			if _, ok := e.Cat.Get(n); !ok {
				pl.Warnings = append(pl.Warnings, fmt.Sprintf("%s is installed but no longer defined; skipping (use 'debforge remove %s')", n, n))
				continue
			}
			names = append(names, n)
		}
	} else {
		for _, n := range names {
			if _, ok := e.State.Packages[n]; !ok {
				return nil, fmt.Errorf("%s is not installed", n)
			}
		}
	}
	p, err := e.build(ctx, names, o, true)
	if err != nil {
		return nil, err
	}
	p.Warnings = append(pl.Warnings, p.Warnings...)
	return p, nil
}

func (e *Env) build(ctx context.Context, names []string, o Options, update bool) (*Plan, error) {
	pkgs, err := Resolve(e.Cat, names)
	if err != nil {
		return nil, err
	}
	requested := map[string]bool{}
	for _, n := range names {
		requested[n] = true
	}
	dependents := map[string][]string{}
	for _, p := range pkgs {
		for _, d := range p.Depends {
			dependents[d] = append(dependents[d], p.Name)
		}
	}

	versions := e.resolveVersions(ctx, pkgs)
	pl := &Plan{}
	for _, p := range pkgs {
		old := e.State.Packages[p.Name]
		it := &Item{Name: p.Name, Pkg: p, Explicit: requested[p.Name] && !update || old != nil && old.Explicit}
		if p.Hardware != nil && e.HasHardware != nil && !e.HasHardware(p.Hardware) {
			return nil, fmt.Errorf("%s requires PCI vendor %s, which was not found on this machine", p.Name, p.Hardware.PCIVendor)
		}
		if p.Kind() == catalog.KindApt && len(p.Source.Apt.Variants) > 0 {
			it.Variant = o.Variants[p.Name]
			if it.Variant == "" && old != nil {
				it.Variant = old.Variant
			}
			if _, ok := p.Source.Apt.Variants[it.Variant]; !ok {
				if it.Variant == "" {
					return nil, fmt.Errorf("%s needs a variant (one of %s); pass --variant %s=<name>", p.Name, variantNames(p), p.Name)
				}
				return nil, fmt.Errorf("%s has no variant %q (have %s)", p.Name, it.Variant, variantNames(p))
			}
		}
		if hasUserFiles(p) && e.User == nil {
			return nil, fmt.Errorf("%s writes files into your home directory, but the target user is unknown (%v)", p.Name, system.ErrNoTargetUser)
		}

		if r, ok := versions[p.Name]; ok {
			v, err := r.version, r.err
			if err != nil {
				if old != nil && !o.Force {
					pl.Warnings = append(pl.Warnings, fmt.Sprintf("%s: cannot check for a new version: %v", p.Name, err))
					continue
				}
				return nil, fmt.Errorf("%s: %w", p.Name, err)
			}
			it.Version = v
		}

		switch {
		case old == nil:
			it.Op = OpInstall
			if !requested[p.Name] {
				it.Reason = "dependency of " + strings.Join(dependents[p.Name], ", ")
			}
		case o.Force && requested[p.Name]:
			it.Op, it.Reason = OpReinstall, "forced"
		case old.Incomplete:
			it.Op, it.Reason = OpReinstall, "previous install did not finish"
		case it.Version != "" && it.Version != old.Version:
			it.Op, it.OldVersion = OpUpgrade, old.Version
		case old.Variant != it.Variant:
			it.Op, it.Reason = OpReinstall, fmt.Sprintf("variant %s -> %s", old.Variant, it.Variant)
		case old.DefHash != p.Hash:
			it.Op, it.Reason = OpUpgrade, "definition changed"
		case e.missingOnSystem(p, it.Variant, old):
			it.Op, it.Reason = OpReinstall, "missing on the system"
		case update && p.Kind() == catalog.KindApt:
			// apt packages are upgraded by the system upgrade; nothing
			// debforge-specific to do.
			if requested[p.Name] {
				pl.Skipped = append(pl.Skipped, p.Name)
			}
			continue
		default:
			if requested[p.Name] {
				pl.Skipped = append(pl.Skipped, p.Name)
			}
			continue
		}
		if it.Op == OpUpgrade && it.OldVersion == "" && old != nil {
			it.OldVersion = old.Version
		}

		e.aptFor(pl, it, old)
		if err := e.filesFor(it, old, o.Force); err != nil {
			return nil, err
		}
		pl.Debconf = append(pl.Debconf, p.Debconf...)
		pl.Items = append(pl.Items, it)
	}
	pl.AptInstall = uniq(pl.AptInstall)
	pl.AptBackports = uniq(pl.AptBackports)
	pl.AptConflicts = uniq(pl.AptConflicts)
	pl.Extrepos = uniq(pl.Extrepos)
	return pl, nil
}

func (e *Env) missingOnSystem(p *catalog.Package, variant string, old *state.Package) bool {
	switch p.Kind() {
	case catalog.KindApt:
		return !e.Snap.AllInstalled(aptPayload(p, variant)) || !e.Snap.AllInstalled(p.Source.Apt.Backports)
	case catalog.KindDeb:
		return len(old.Debs) == 0 || !e.Snap.AllInstalled(old.Debs)
	}
	return !e.Snap.AllInstalled(p.Requires)
}

func aptPayload(p *catalog.Package, variant string) []string {
	if p.Kind() != catalog.KindApt {
		return nil
	}
	out := append([]string(nil), p.Source.Apt.Packages...)
	return append(out, p.Source.Apt.Variants[variant]...)
}

func (e *Env) aptFor(pl *Plan, it *Item, old *state.Package) {
	p := it.Pkg
	it.Apt = append(aptPayload(p, it.Variant), p.Requires...)
	if p.Kind() == catalog.KindAppImage {
		// Type-2 AppImages need the FUSE 2 runtime library.
		it.Apt = append(it.Apt, "libfuse2t64")
	}
	if p.Kind() == catalog.KindApt {
		it.Backports = p.Source.Apt.Backports
		for _, c := range p.Source.Apt.Conflicts {
			if e.Snap.Installed(c) {
				pl.AptConflicts = append(pl.AptConflicts, c)
			}
		}
		if p.Source.Apt.Repo != nil {
			pl.Repos = append(pl.Repos, it.Name)
		}
		for _, r := range p.Source.Apt.Extrepo {
			if e.ExtrepoEnabled == nil || !e.ExtrepoEnabled(r) {
				pl.Extrepos = append(pl.Extrepos, r)
			}
		}
	}
	var prev []string
	if old != nil {
		prev = old.AptPackages
	}
	for _, a := range append(append([]string(nil), it.Apt...), it.Backports...) {
		if !e.Snap.Installed(a) || slices.Contains(prev, a) || e.State.ClaimedApt(a, it.Name) {
			it.AptRecord = append(it.AptRecord, a)
		}
	}
	pl.AptInstall = append(pl.AptInstall, it.Apt...)
	pl.AptBackports = append(pl.AptBackports, it.Backports...)
}

// FileSpec builds the files.Spec for a catalog file.
func FileSpec(f catalog.File, u *system.User) files.Spec {
	s := files.Spec{Path: f.Dest, Data: f.Data(), Mode: f.FileMode()}
	if f.User() {
		s.Path = u.ExpandHome(f.Dest)
		s.Home, s.UID, s.GID = u.Home, u.UID, u.GID
	}
	return s
}

func (e *Env) filesFor(it *Item, old *state.Package, force bool) error {
	for _, f := range it.Pkg.Files {
		spec := FileSpec(f, e.User)
		var rec *state.File
		if old != nil {
			if r, ok := old.Files[spec.Path]; ok {
				rec = &r
			}
		}
		st, err := e.Files.Classify(spec, rec)
		if err != nil {
			return fmt.Errorf("%s: %w", it.Name, err)
		}
		if force && (st == files.UserModified || st == files.Conflict) {
			st = files.Outdated
		}
		it.Files = append(it.Files, FileChange{Path: spec.Path, State: st})
	}
	return nil
}

// Drift returns installed packages whose payload is no longer on the
// system (for example removed with apt directly), sorted.
func (e *Env) Drift() []string {
	var out []string
	for _, n := range e.State.Names() {
		sp := e.State.Packages[n]
		p, ok := e.Cat.Get(n)
		if !ok {
			if !e.Snap.AllInstalled(sp.AptPackages) || !e.Snap.AllInstalled(sp.Debs) {
				out = append(out, n)
			}
			continue
		}
		if e.missingOnSystem(p, sp.Variant, sp) {
			out = append(out, n)
		}
	}
	return out
}

// Remove plans removing names, their installed dependents, and
// dependencies that were only pulled in for them.
func (e *Env) Remove(names []string) (*Plan, error) {
	removing := map[string]string{} // name -> reason
	for _, n := range names {
		if _, ok := e.State.Packages[n]; !ok {
			return nil, fmt.Errorf("%s is not installed by debforge", n)
		}
		removing[n] = ""
	}
	depends := func(n string) []string {
		if p, ok := e.Cat.Get(n); ok {
			return p.Depends
		}
		return nil
	}
	// Dependents of anything being removed must go too.
	for changed := true; changed; {
		changed = false
		for _, n := range e.State.Names() {
			if _, ok := removing[n]; ok {
				continue
			}
			for _, d := range depends(n) {
				if _, ok := removing[d]; ok {
					removing[n] = "depends on " + d
					changed = true
					break
				}
			}
		}
	}
	// Dependencies pulled in automatically and no longer needed.
	for changed := true; changed; {
		changed = false
		for _, n := range e.State.Names() {
			if _, ok := removing[n]; ok || e.State.Packages[n].Explicit {
				continue
			}
			needed := false
			for _, m := range e.State.Names() {
				if _, gone := removing[m]; gone {
					continue
				}
				if slices.Contains(depends(m), n) {
					needed = true
					break
				}
			}
			if !needed {
				removing[n] = "no longer needed"
				changed = true
			}
		}
	}

	// Order: dependents before their dependencies.
	order := e.removalOrder(removing, depends)
	pl := &Plan{}
	remaining := func(name string) bool { _, gone := removing[name]; return !gone }
	for _, n := range order {
		sp := e.State.Packages[n]
		pk, _ := e.Cat.Get(n)
		it := &Item{Name: n, Pkg: pk, Op: OpRemove, Reason: removing[n], OldVersion: sp.Version, Variant: sp.Variant}
		for path := range sp.Files {
			it.Files = append(it.Files, FileChange{Path: path})
		}
		sort.Slice(it.Files, func(i, j int) bool { return it.Files[i].Path < it.Files[j].Path })
		for _, a := range append(append([]string(nil), sp.AptPackages...), sp.Debs...) {
			if !claimedByRemaining(e.State, a, remaining) {
				pl.AptRemove = append(pl.AptRemove, a)
			}
		}
		for _, r := range sp.Extrepos {
			claimed := false
			for m, mp := range e.State.Packages {
				if remaining(m) && slices.Contains(mp.Extrepos, r) {
					claimed = true
				}
			}
			if !claimed {
				pl.ExtreposDisable = append(pl.ExtreposDisable, r)
			}
		}
		pl.Items = append(pl.Items, it)
	}
	pl.AptRemove = uniq(pl.AptRemove)
	pl.ExtreposDisable = uniq(pl.ExtreposDisable)
	return pl, nil
}

func claimedByRemaining(st *state.State, apt string, remaining func(string) bool) bool {
	for n, p := range st.Packages {
		if remaining(n) && (slices.Contains(p.AptPackages, apt) || slices.Contains(p.Debs, apt)) {
			return true
		}
	}
	return false
}

func (e *Env) removalOrder(removing map[string]string, depends func(string) []string) []string {
	var names []string
	for n := range removing {
		names = append(names, n)
	}
	sort.Strings(names)
	done := map[string]bool{}
	var out []string
	var visit func(n string)
	visit = func(n string) {
		if done[n] {
			return
		}
		done[n] = true
		// Visit dependents (packages being removed that depend on n) first.
		for _, m := range names {
			if slices.Contains(depends(m), n) {
				visit(m)
			}
		}
		out = append(out, n)
	}
	for _, n := range names {
		visit(n)
	}
	return out
}

func hasUserFiles(p *catalog.Package) bool {
	for _, f := range p.Files {
		if f.User() {
			return true
		}
	}
	return false
}

func variantNames(p *catalog.Package) string {
	var v []string
	for k := range p.Source.Apt.Variants {
		v = append(v, k)
	}
	sort.Strings(v)
	return strings.Join(v, ", ")
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
