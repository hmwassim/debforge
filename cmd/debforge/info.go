package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hmwassim/debforge/internal/catalog"
	"github.com/hmwassim/debforge/internal/state"
)

func infoText(a *App, p *catalog.Package, sp *state.Package, verbose bool) string {
	var b strings.Builder
	row := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&b, "%-13s %s\n", k+":", v)
		}
	}
	fmt.Fprintf(&b, "%s\n", a.UI.Bold(p.Name))
	row("Description", p.Description)
	row("Category", "@"+p.Category)
	row("Kind", string(p.Kind()))
	switch {
	case sp == nil:
		row("Status", "not installed")
	case sp.Incomplete:
		row("Status", a.UI.Yellow("installed, incomplete (run 'debforge update "+p.Name+"')"))
	default:
		s := "installed"
		if sp.Version != "" {
			s += " " + sp.Version
		}
		if sp.Variant != "" {
			s += " [" + sp.Variant + "]"
		}
		if !sp.Explicit {
			s += " (as a dependency)"
		}
		row("Status", a.UI.Green(s))
	}
	row("Depends", strings.Join(p.Depends, ", "))
	row("Requires", strings.Join(p.Requires, ", "))
	if p.Hardware != nil {
		row("Hardware", "PCI vendor "+p.Hardware.PCIVendor)
	}
	switch p.Kind() {
	case catalog.KindApt:
		a := p.Source.Apt
		row("Packages", strings.Join(a.Packages, ", "))
		row("Backports", strings.Join(a.Backports, ", "))
		row("Extrepo", strings.Join(a.Extrepo, ", "))
		if a.Repo != nil {
			row("Repository", a.Repo.URI+" "+a.Repo.Suite)
		}
		row("Conflicts", strings.Join(a.Conflicts, ", "))
		var vs []string
		for v, list := range a.Variants {
			vs = append(vs, v+" ("+strings.Join(list, ", ")+")")
		}
		sort.Strings(vs)
		row("Variants", strings.Join(vs, "; "))
	case catalog.KindDeb:
		for _, d := range p.Source.Deb.URLs {
			row("URL", d.URL)
		}
	case catalog.KindArchive:
		row("URL", p.Source.Archive.URL)
	case catalog.KindAppImage:
		row("URL", p.Source.AppImage.URL)
		row("Command", "/usr/local/bin/"+p.Source.AppImage.Bin)
	case catalog.KindGit:
		row("Repository", p.Source.Git.Repo)
	}
	if v := p.Version; v != nil {
		switch v.From {
		case "git-tags":
			row("Versions", "tags "+v.Tag+" of "+v.Repo)
		case "pin":
			row("Versions", "pinned "+v.Pin)
		case "cmd":
			row("Versions", "from command")
		}
	}
	if p.Trust == "upstream-tls" {
		row("Integrity", "upstream HTTPS only (auto-updating, no checksum)")
	}
	for i, f := range p.Files {
		k := ""
		if i == 0 {
			k = "Files"
		}
		fmt.Fprintf(&b, "%-13s %s\n", k+map[bool]string{true: ":", false: " "}[k != ""], f.Dest)
		if verbose {
			for _, l := range strings.Split(strings.TrimRight(string(f.Data()), "\n"), "\n") {
				fmt.Fprintf(&b, "%13s   %s\n", "", a.UI.Dim(l))
			}
		}
	}
	row("Reload", strings.Join(p.Reload, ", "))
	row("On install", strings.TrimSpace(p.Notes.Install))
	row("On removal", strings.TrimSpace(p.Notes.Remove))
	if verbose {
		for name, h := range map[string]string{"build": p.Hooks.Build, "install": p.Hooks.Install,
			"post_install": p.Hooks.PostInstall, "pre_remove": p.Hooks.PreRemove, "post_remove": p.Hooks.PostRemove} {
			if h == "" {
				continue
			}
			fmt.Fprintf(&b, "Hook %s:\n", name)
			for _, l := range strings.Split(strings.TrimRight(h, "\n"), "\n") {
				fmt.Fprintf(&b, "    %s\n", a.UI.Dim(l))
			}
		}
	}
	row("Defined in", p.Origin)
	return strings.TrimRight(b.String(), "\n")
}
