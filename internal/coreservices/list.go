package services

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/hmwassim/debforge/internal/coresetup"
	"github.com/hmwassim/debforge/internal/domain/apt"
	"github.com/hmwassim/debforge/internal/domain/package"
	"github.com/hmwassim/debforge/internal/services/state"
	"github.com/hmwassim/debforge/internal/ports"
)

type ListService struct {
	registry *pkg.Registry
	stateSvc *state.Service
	logger   ports.UI
	aptSvc   apt.Service
}

func NewListService(registry *pkg.Registry, stateSvc *state.Service, logger ports.UI, aptSvc apt.Service) *ListService {
	return &ListService{registry: registry, stateSvc: stateSvc, logger: logger, aptSvc: aptSvc}
}

func (s *ListService) Run(ctx context.Context) error {
	st, err := s.stateSvc.Load()
	if err != nil {
		return err
	}
	names := s.registry.List()
	sort.Slice(names, func(i, j int) bool { return names[i].Name < names[j].Name })
	for _, p := range names {
		if s.stateSvc.IsInstalled(st, p.Name) {
			s.logger.Success("  %s", p.Name)
		} else {
			s.logger.Muted("  %s", p.Name)
		}
	}
	return nil
}

func (s *ListService) Search(ctx context.Context, query string) error {
	st, err := s.stateSvc.Load()
	if err != nil {
		return err
	}
	names := s.registry.List()
	sort.Slice(names, func(i, j int) bool { return names[i].Name < names[j].Name })
	q := strings.ToLower(query)
	for _, p := range names {
		if !strings.Contains(strings.ToLower(p.Name), q) {
			continue
		}
		if s.stateSvc.IsInstalled(st, p.Name) {
			s.logger.Success("  %s", p.Name)
		} else {
			s.logger.Muted("  %s", p.Name)
		}
	}
	return nil
}

func (s *ListService) RunCore(ctx context.Context, groups *coresetup.Groups) error {
	pkgToGroup := map[string]string{}
	for _, g := range groups.List() {
		for _, pkg := range g.Packages {
			pkgToGroup[pkg] = g.Name
		}
	}

	var allPkgs []string
	for _, g := range groups.List() {
		allPkgs = append(allPkgs, g.Packages...)
	}

	installed, err := s.checkInstalled(ctx, allPkgs)
	if err != nil {
		s.logger.Warn("could not query package status: %v", err)
		return nil
	}

	missing := map[string][]string{}
	for pkgName, gname := range pkgToGroup {
		if !installed[pkgName] {
			missing[gname] = append(missing[gname], pkgName)
		}
	}

	for _, g := range groups.List() {
		if m := missing[g.Name]; len(m) == 0 {
			s.logger.Success("  %s — installed", g.Name)
		} else {
			s.logger.Warn("  %s — missing: %s", g.Name, strings.Join(m, ", "))
		}
	}
	return nil
}

func (s *ListService) checkInstalled(ctx context.Context, pkgs []string) (map[string]bool, error) {
	if len(pkgs) == 0 {
		return map[string]bool{}, nil
	}
	result := make(map[string]bool, len(pkgs))
	for _, pkg := range pkgs {
		result[pkg] = false
	}
	if s.aptSvc == nil {
		return result, fmt.Errorf("apt service not available")
	}
	for _, pkg := range pkgs {
		ok, err := s.aptSvc.CheckInstalled(ctx, pkg)
		if err == nil && ok {
			result[pkg] = true
		}
	}
	return result, nil
}
