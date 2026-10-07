package plan

import (
	"fmt"
	"strings"

	"github.com/hmwassim/debforge/internal/files"
)

// Styler colours plan output; ui.UI satisfies it.
type Styler interface {
	Bold(string) string
	Dim(string) string
	Green(string) string
	Yellow(string) string
	Red(string) string
	Blue(string) string
}

type plain struct{}

func (plain) Bold(s string) string   { return s }
func (plain) Dim(s string) string    { return s }
func (plain) Green(s string) string  { return s }
func (plain) Yellow(s string) string { return s }
func (plain) Red(s string) string    { return s }
func (plain) Blue(s string) string   { return s }

// Plain is a Styler without colour.
var Plain Styler = plain{}

// Render formats the plan for the confirmation prompt.
func (p *Plan) Render(s Styler, home string) string {
	var b strings.Builder
	groups := []struct {
		op    Op
		title string
		color func(string) string
	}{
		{OpInstall, "Install", s.Green},
		{OpUpgrade, "Upgrade", s.Blue},
		{OpReinstall, "Reinstall", s.Blue},
		{OpRemove, "Remove", s.Red},
	}
	for _, g := range groups {
		var lines []string
		for _, it := range p.Items {
			if it.Op != g.op {
				continue
			}
			l := "  " + g.color(it.Name)
			if it.Variant != "" {
				l += s.Dim(" [" + it.Variant + "]")
			}
			switch {
			case it.Op == OpUpgrade && it.OldVersion != "" && it.Version != "":
				l += " " + it.OldVersion + " → " + it.Version
			case it.Version != "":
				l += " " + it.Version
			}
			if it.Reason != "" {
				l += s.Dim(" (" + it.Reason + ")")
			}
			lines = append(lines, l)
		}
		if len(lines) > 0 {
			fmt.Fprintf(&b, "%s:\n%s\n", s.Bold(g.title), strings.Join(lines, "\n"))
		}
	}

	var sys []string
	if len(p.Extrepos) > 0 {
		sys = append(sys, "enable repositories: "+strings.Join(p.Extrepos, ", "))
	}
	if len(p.Repos) > 0 {
		sys = append(sys, "configure apt repositories for: "+strings.Join(p.Repos, ", "))
	}
	if len(p.AptInstall) > 0 {
		sys = append(sys, "apt install: "+strings.Join(p.AptInstall, " "))
	}
	if len(p.AptBackports) > 0 {
		sys = append(sys, "apt install from backports: "+strings.Join(p.AptBackports, " "))
	}
	if len(p.AptConflicts) > 0 {
		sys = append(sys, s.Yellow("apt remove (conflicts): "+strings.Join(p.AptConflicts, " ")))
	}
	if len(p.AptRemove) > 0 {
		sys = append(sys, s.Yellow("apt remove: "+strings.Join(p.AptRemove, " ")))
	}
	if len(p.ExtreposDisable) > 0 {
		sys = append(sys, "disable repositories: "+strings.Join(p.ExtreposDisable, ", "))
	}
	if len(sys) > 0 {
		fmt.Fprintf(&b, "%s:\n  %s\n", s.Bold("System"), strings.Join(sys, "\n  "))
	}

	var fl []string
	for _, it := range p.Items {
		for _, f := range it.Files {
			path := f.Path
			if home != "" && strings.HasPrefix(path, home+"/") {
				path = "~" + path[len(home):]
			}
			if it.Op == OpRemove {
				fl = append(fl, "  "+path+s.Dim(" (remove if unmodified)"))
				continue
			}
			note := map[files.State]string{
				files.Missing:      "new",
				files.Adoptable:    "already matches",
				files.Foreign:      s.Yellow("existing file will be backed up to .debforge-orig"),
				files.Current:      "",
				files.Outdated:     "update",
				files.UserModified: s.Dim("modified by you, kept"),
				files.Conflict:     s.Yellow("modified by you; new version saved as .debforge-new"),
			}[f.State]
			if note == "" {
				continue
			}
			fl = append(fl, "  "+path+" ("+note+")")
		}
	}
	if len(fl) > 0 {
		fmt.Fprintf(&b, "%s:\n%s\n", s.Bold("Files"), strings.Join(fl, "\n"))
	}
	return strings.TrimRight(b.String(), "\n")
}
