// Package setup provisions the system from a declarative profile. Every
// step's Check verifies everything its Apply does, so "doctor" is simply
// a check-only run and re-running setup only touches what is missing.
package setup

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/hmwassim/debforge/internal/catalog"
)

// Profile is an ordered list of steps.
type Profile struct {
	Steps []*Step `yaml:"steps"`
}

// Step is one provisioning step.
type Step struct {
	ID    string `yaml:"id"`
	Title string `yaml:"title"`
	// Builtin is one of sources, i386, upgrade, bashrc.
	Builtin   string         `yaml:"builtin"`
	When      *When          `yaml:"when"`
	Packages  []string       `yaml:"packages"`
	Backports bool           `yaml:"backports"`
	Debconf   []string       `yaml:"debconf"`
	Files     []catalog.File `yaml:"files"`
	Services  []Service      `yaml:"services"`
	Commands  []Command      `yaml:"commands"`
	// Verify must succeed (retried for 30s) after Apply.
	Verify string   `yaml:"verify"`
	Reload []string `yaml:"reload"`
	Notes  Notes    `yaml:"notes"`
}

// Notes are shown to the user after Apply.
type Notes struct {
	// Changed is shown when the step changed any of its files.
	Changed string `yaml:"changed"`
}

// When restricts a step to matching machines.
type When struct {
	CPUVendor      string `yaml:"cpu_vendor"`      // intel or amd
	NetworkManager *bool  `yaml:"network_manager"` // NetworkManager installed
	Desktop        string `yaml:"desktop"`         // kde or gnome
}

// Service is a systemd unit the step manages.
type Service struct {
	Name   string `yaml:"name"`
	Enable bool   `yaml:"enable"`
	Start  bool   `yaml:"start"`
	// RestartOnChange restarts (or ReloadOnChange reloads) the unit when
	// the step changed any file.
	RestartOnChange bool `yaml:"restart_on_change"`
	ReloadOnChange  bool `yaml:"reload_on_change"`
}

// Command is an arbitrary check/apply pair; Apply runs only if Check fails.
type Command struct {
	Check string `yaml:"check"`
	Apply string `yaml:"apply"`
}

var (
	idRe     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	builtins = []string{"sources", "i386", "upgrade", "bashrc"}
	unitRe   = regexp.MustCompile(`^[A-Za-z0-9@._-]+\.(service|socket|timer)$`)
)

// Load reads profiles/<name>.yaml and its files from files/setup/.
func Load(fsys fs.FS, name string) (*Profile, error) {
	raw, err := fs.ReadFile(fsys, path.Join("profiles", name+".yaml"))
	if err != nil {
		return nil, fmt.Errorf("setup profile %q: %w", name, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	p := &Profile{}
	if err := dec.Decode(p); err != nil {
		return nil, fmt.Errorf("setup profile %q: %w", name, err)
	}
	var errs []error
	seen := map[string]bool{}
	for _, s := range p.Steps {
		bad := func(f string, a ...any) { errs = append(errs, fmt.Errorf("step %s: %s", s.ID, fmt.Sprintf(f, a...))) }
		if !idRe.MatchString(s.ID) || seen[s.ID] {
			bad("missing, invalid or duplicate id")
		}
		seen[s.ID] = true
		if s.Title == "" {
			bad("title is required")
		}
		if s.Builtin != "" && !slices.Contains(builtins, s.Builtin) {
			bad("unknown builtin %q", s.Builtin)
		}
		if s.When != nil && s.When.CPUVendor != "" && s.When.CPUVendor != "intel" && s.When.CPUVendor != "amd" {
			bad("when.cpu_vendor must be intel or amd")
		}
		if s.When != nil && s.When.Desktop != "" && s.When.Desktop != "kde" && s.When.Desktop != "gnome" {
			bad("when.desktop must be kde or gnome")
		}
		for _, f := range s.Files {
			if err := catalog.CheckDest(f.Dest); err != nil {
				bad("file %s: %v", f.Dest, err)
			}
		}
		for _, sv := range s.Services {
			if !unitRe.MatchString(sv.Name) {
				bad("invalid unit name %q", sv.Name)
			}
		}
		for _, c := range s.Commands {
			if c.Check == "" || c.Apply == "" {
				bad("commands need both check and apply")
			}
		}
		for _, r := range s.Reload {
			if !slices.Contains(catalog.Reloads, r) {
				bad("unknown reload %q", r)
			}
		}
		if err := catalog.ResolveFiles(fsys, "files/setup", s.Files); err != nil {
			bad("%v", err)
		}
	}
	if len(p.Steps) == 0 {
		errs = append(errs, errors.New("profile has no steps"))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("setup profile %q is invalid:\n%w", name, err)
	}
	return p, nil
}
