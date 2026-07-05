package main

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/hmwassim/debforge/internal/aptpty"
	aptInst "github.com/hmwassim/debforge/internal/domain/installer/apt"
	"github.com/hmwassim/debforge/internal/definition"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/service"
)

func (h *commandHandler) checkGPUPreconditions(ctx context.Context, u ports.UI, names []string) bool {
	resolver := service.NewResolver(h.reg)
	for _, name := range names {
		p, ok := h.reg.Lookup(name)
		if !ok {
			continue
		}
		deps, err := resolver.Resolve(p)
		if err != nil {
			u.Error("resolve deps: %s", err)
			return false
		}
		for _, dep := range deps {
			if strings.ToLower(dep.Name) == "nvidia" {
				spinner := u.Spinner(ctx, "checking gpu...")
				if err := aptInst.CheckGPU(ctx, h.runner, dep.Name); err != nil {
					spinner.DoneWarn()
					u.Warn("%s", err)
					return false
				}
				spinner.Done()
			}
		}
	}
	return true
}

func (h *commandHandler) checkConflicts(ctx context.Context, u ports.UI, names []string) []string {
	var conflicts []string
	for _, name := range names {
		p, ok := h.reg.Lookup(name)
		if !ok {
			continue
		}
		if p.Apt != nil {
			conflicts = append(conflicts, aptpty.FindInstalledConflicts(ctx, h.runner, p.Apt.Conflicts)...)
		}
	}
	return conflicts
}

func extractFlags(ss []string, yes, force, all, self *bool) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		switch {
		case s == "--yes":
			*yes = true
		case s == "--force":
			*force = true
		case s == "--all":
			*all = true
		case s == "--self":
			*self = true
		case strings.HasPrefix(s, "-") && len(s) > 1 && s[1] != '-':
			for _, c := range s[1:] {
				switch c {
				case 'y':
					*yes = true
				case 'f':
					*force = true
				case 'a':
					*all = true
				default:
					out = append(out, "-"+string(c))
				}
			}
		default:
			out = append(out, s)
		}
	}
	return out
}

func loadYAMLDefinitions(reg *pkg.Registry, names []string, fsys ports.FileSystem) error {
	for i, n := range names {
		if !strings.HasSuffix(n, ".yaml") {
			continue
		}
		p, err := definition.Parse(n, fsys)
		if err != nil {
			return fmt.Errorf("load %s: %w", n, err)
		}
		reg.Register(p)
		names[i] = p.Name
	}
	return nil
}

func loadDefs(reg *pkg.Registry, names []string, fsys ports.FileSystem, u ports.UI) bool {
	if err := loadYAMLDefinitions(reg, names, fsys); err != nil {
		u.Error("%s", err)
		return false
	}
	return true
}

func expandGlobs(reg *pkg.Registry, names []string) []string {
	var out []string
	seen := make(map[string]bool)

	var catIndex map[string][]string

	for _, name := range names {
		if strings.HasPrefix(name, "@") {
			if catIndex == nil {
				catIndex = reg.Categories()
			}
			cat := name[1:]
			pkgs, ok := catIndex[cat]
			if !ok {
				out = append(out, name)
				continue
			}
			for _, key := range pkgs {
				if !seen[key] {
					out = append(out, key)
					seen[key] = true
				}
			}
			continue
		}
		if !containsGlob(name) || globPrefixLen(name) < 3 {
			if !seen[name] {
				out = append(out, name)
				seen[name] = true
			}
			continue
		}
		reg.Range(func(key string, _ *pkg.Package) bool {
			if ok, _ := path.Match(name, key); ok && !seen[key] {
				out = append(out, key)
				seen[key] = true
			}
			return true
		})
	}
	return out
}

func firstUnknownCategory(names []string) string {
	for _, n := range names {
		if strings.HasPrefix(n, "@") {
			return n[1:]
		}
	}
	return ""
}

// resolveNames expands glob patterns and @category references in names
// against the registry, and checks that every @category is known. Returns
// the cleaned name list and whether the caller should proceed. When a
// category is unknown, an error is printed to u and ok is false.
func (h *commandHandler) resolveNames(names []string, u ports.UI) ([]string, bool) {
	names = expandGlobs(h.reg, names)
	if cat := firstUnknownCategory(names); cat != "" {
		u.Error("unknown category: %s", cat)
		return nil, false
	}
	return names, true
}

func containsGlob(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

func globPrefixLen(s string) int {
	for i, r := range s {
		if r == '*' || r == '?' || r == '[' {
			return i
		}
	}
	return len(s)
}
