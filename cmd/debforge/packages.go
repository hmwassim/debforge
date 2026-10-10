package main

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/hmwassim/debforge/internal/catalog"
	"github.com/hmwassim/debforge/internal/plan"
	"github.com/hmwassim/debforge/internal/system"
)

func parseVariants(inv *invocation) (map[string]string, error) {
	out := map[string]string{}
	for _, v := range inv.values("variant") {
		pkg, name, ok := strings.Cut(v, "=")
		if !ok || pkg == "" || name == "" {
			return nil, fmt.Errorf("--variant expects package=variant, got %q", v)
		}
		out[pkg] = name
	}
	return out, nil
}

// chooseVariants asks for any missing variant choices up front.
func (s *session) chooseVariants(names []string, opts *plan.Options) error {
	need, err := s.env.NeedsVariant(names, *opts)
	if err != nil {
		return err
	}
	for _, p := range need {
		var vs []string
		for v := range p.Source.Apt.Variants {
			vs = append(vs, v)
		}
		sort.Strings(vs)
		if s.a.UI.Yes() {
			return fmt.Errorf("%s needs a variant; pass --variant %s=<%s>", p.Name, p.Name, strings.Join(vs, "|"))
		}
		var labels []string
		for _, v := range vs {
			labels = append(labels, fmt.Sprintf("%s (%s)", v, strings.Join(p.Source.Apt.Variants[v], ", ")))
		}
		i, err := s.a.UI.Choose(fmt.Sprintf("Choose a variant for %s:", p.Name), labels)
		if err != nil {
			return err
		}
		opts.Variants[p.Name] = vs[i]
	}
	return nil
}

func cmdInstall(a *App, inv *invocation) error {
	if len(inv.args) == 0 {
		return usageErr("install needs at least one package")
	}
	variants, err := parseVariants(inv)
	if err != nil {
		return usageErr(err.Error())
	}
	s, err := a.begin(inv.has("force"))
	if err != nil {
		return err
	}
	defer s.end()
	names, err := a.Catalog.Select(inv.args)
	if err != nil {
		return err
	}
	opts := plan.Options{Force: inv.has("force"), Variants: variants}
	if err := s.chooseVariants(names, &opts); err != nil {
		return err
	}
	pl, err := s.env.Install(a.Ctx, names, opts)
	if err != nil {
		return err
	}
	ok, err := s.confirm(pl, inv)
	if err != nil || !ok {
		return err
	}
	sum, err := s.exec.Install(a.Ctx, pl)
	a.report(sum)
	return err
}

func cmdRemove(a *App, inv *invocation) error {
	if inv.has("self") {
		return cmdRemoveSelf(a, inv)
	}
	if len(inv.args) == 0 {
		return usageErr("remove needs at least one package")
	}
	if inv.has("all") {
		return usageErr("--all only applies to remove --self")
	}
	s, err := a.begin(false)
	if err != nil {
		return err
	}
	defer s.end()
	var names []string
	for _, arg := range inv.args {
		if strings.ContainsAny(arg, "*?[@") {
			sel, err := a.Catalog.Select([]string{arg})
			if err != nil {
				return err
			}
			for _, n := range sel {
				if _, ok := s.st.Packages[n]; ok {
					names = append(names, n)
				}
			}
			continue
		}
		names = append(names, arg)
	}
	if len(names) == 0 {
		return errors.New("none of the selected packages are installed")
	}
	pl, err := s.env.Remove(names)
	if err != nil {
		return err
	}
	ok, err := s.confirm(pl, inv)
	if err != nil || !ok {
		return err
	}
	sum, err := s.exec.Remove(a.Ctx, pl)
	a.report(sum)
	return err
}

func cmdUpdate(a *App, inv *invocation) error {
	if inv.has("self") {
		return cmdUpdateSelf(a, inv)
	}
	if len(inv.args) == 0 && !inv.has("all") {
		return usageErr("update needs package names or --all")
	}
	variants, err := parseVariants(inv)
	if err != nil {
		return usageErr(err.Error())
	}
	s, err := a.begin(inv.has("force"))
	if err != nil {
		return err
	}
	defer s.end()

	if inv.has("all") {
		ap := a.apt()
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
		n, err := ap.PendingUpgrades(a.Ctx)
		if err != nil {
			return err
		}
		if n > 0 {
			ok := inv.has("dry-run")
			if !ok {
				if ok, err = a.UI.Confirm(fmt.Sprintf("Upgrade %d system packages?", n), true); err != nil {
					return err
				}
			}
			if ok && !inv.has("dry-run") {
				up := a.UI.Start("Upgrading system packages")
				if err := ap.FullUpgrade(a.Ctx, func(_ string, pct float64, d string) { up.Update(d, pct) }); err != nil {
					up.Fail("System upgrade failed")
					return err
				}
				up.Done(fmt.Sprintf("Upgraded %d system packages", n))
			} else if inv.has("dry-run") {
				a.UI.Info("%d system packages can be upgraded", n)
			}
			if err := s.refresh(); err != nil {
				return err
			}
		}
	}

	var names []string
	if len(inv.args) > 0 {
		sel, err := a.Catalog.Select(inv.args)
		if err != nil {
			return err
		}
		names = sel
	}
	pl, err := s.env.Update(a.Ctx, names, plan.Options{Force: inv.has("force"), Variants: variants})
	if err != nil {
		return err
	}
	ok, err := s.confirm(pl, inv)
	if err != nil || !ok {
		return err
	}
	sum, err := s.exec.Install(a.Ctx, pl)
	a.report(sum)
	return err
}

func cmdSync(a *App, inv *invocation) error {
	s, err := a.begin(false)
	if err != nil {
		return err
	}
	defer s.end()
	drift := s.env.Drift()
	if len(drift) == 0 {
		a.UI.Success("state matches the system")
		return nil
	}
	a.UI.Print("These packages are recorded as installed but are missing from the system:\n  " + strings.Join(drift, "\n  "))
	a.UI.Print("Reinstall them with 'debforge update <name>', or forget them here (nothing on the system is changed).")
	if inv.has("dry-run") {
		return nil
	}
	ok, err := a.UI.Confirm("Forget them?", false)
	if err != nil || !ok {
		return err
	}
	for _, n := range drift {
		delete(s.st.Packages, n)
	}
	if err := a.store().Save(s.st); err != nil {
		return err
	}
	a.UI.Success("forgot %d packages", len(drift))
	return nil
}

func cmdList(a *App, inv *invocation) error {
	if err := a.loadCatalog(); err != nil {
		return err
	}
	st := a.loadStateRO()
	var filter string
	if len(inv.args) > 0 {
		if !strings.HasPrefix(inv.args[0], "@") || len(inv.args) > 1 {
			return usageErr("list takes at most one @category")
		}
		filter = inv.args[0][1:]
	}
	if inv.has("names") {
		var names []string
		for _, p := range a.Catalog.All() {
			if (filter == "" || p.Category == filter) && (!inv.has("installed") || st.Packages[p.Name] != nil) {
				names = append(names, p.Name)
			}
		}
		a.UI.Print(strings.Join(names, "\n"))
		return nil
	}
	byCat := map[string][]*catalog.Package{}
	for _, p := range a.Catalog.All() {
		if filter != "" && p.Category != filter {
			continue
		}
		if inv.has("installed") && st.Packages[p.Name] == nil {
			continue
		}
		byCat[p.Category] = append(byCat[p.Category], p)
	}
	if filter != "" && len(byCat) == 0 && !inv.has("installed") {
		return fmt.Errorf("no category %q (have %s)", filter, strings.Join(catalog.Categories, ", "))
	}
	var b strings.Builder
	for _, c := range catalog.Categories {
		ps := byCat[c]
		if len(ps) == 0 {
			continue
		}
		fmt.Fprintf(&b, "%s\n", a.UI.Bold("@"+c))
		for _, p := range ps {
			mark := "  "
			if sp := st.Packages[p.Name]; sp != nil {
				mark = a.UI.Green("* ")
				if sp.Incomplete {
					mark = a.UI.Yellow("! ")
				}
			}
			fmt.Fprintf(&b, "  %s%-26s %s\n", mark, p.Name, a.UI.Dim(p.Description))
		}
	}
	a.UI.Print(b.String())
	return nil
}

func cmdSearch(a *App, inv *invocation) error {
	if len(inv.args) == 0 {
		return usageErr("search needs a term")
	}
	if err := a.loadCatalog(); err != nil {
		return err
	}
	st := a.loadStateRO()
	var b strings.Builder
	n := 0
	for _, p := range a.Catalog.All() {
		hay := strings.ToLower(p.Name + " " + p.Description + " " + p.Category)
		match := true
		for _, t := range inv.args {
			if !strings.Contains(hay, strings.ToLower(t)) {
				match = false
			}
		}
		if !match {
			continue
		}
		n++
		mark := "  "
		if st.Packages[p.Name] != nil {
			mark = a.UI.Green("* ")
		}
		fmt.Fprintf(&b, "%s%-26s %s %s\n", mark, p.Name, a.UI.Dim("@"+p.Category), p.Description)
	}
	if n == 0 {
		return fmt.Errorf("no packages match %q", strings.Join(inv.args, " "))
	}
	a.UI.Print(b.String())
	return nil
}

func cmdInfo(a *App, inv *invocation) error {
	if len(inv.args) == 0 {
		return usageErr("info needs a package")
	}
	if err := a.loadCatalog(); err != nil {
		return err
	}
	st := a.loadStateRO()
	names, err := a.Catalog.Select(inv.args)
	if err != nil {
		return err
	}
	for i, n := range names {
		p, _ := a.Catalog.Get(n)
		if i > 0 {
			a.UI.Print("")
		}
		a.UI.Print(infoText(a, p, st.Packages[n], inv.has("verbose")))
	}
	return nil
}

func cmdDiff(a *App, inv *invocation) error {
	st := a.loadStateRO()
	want := map[string]bool{}
	for _, p := range inv.args {
		want[p] = true
	}
	var paths []string
	for _, sp := range st.Packages {
		for p := range sp.Files {
			paths = append(paths, p)
		}
	}
	for p := range st.Setup.Files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	found := 0
	for _, p := range paths {
		if len(want) > 0 && !want[p] {
			continue
		}
		side := a.Paths.Root + p + ".debforge-new"
		if _, err := os.Stat(side); err != nil {
			continue
		}
		found++
		res, err := a.R.Run(a.Ctx, system.Cmd{Name: "diff", Args: []string{"-u", a.Paths.Root + p, side}})
		var ee *system.ExitError
		if err != nil && !(errors.As(err, &ee) && ee.Code == 1) {
			return err
		}
		a.UI.Print(string(res.Stdout))
	}
	if found == 0 {
		a.UI.Info("no pending .debforge-new files")
	}
	return nil
}

func cmdVersion(a *App, _ *invocation) error {
	a.UI.Print("debforge " + a.Version)
	return nil
}
