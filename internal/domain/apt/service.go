package apt

import (
	"context"
	"fmt"
	"strings"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/ports"
)

type Service interface {
	Install(ctx context.Context, packages []string) error
	InstallBackports(ctx context.Context, packages []string, suite string) error
	Remove(ctx context.Context, packages []string) error
	CheckInstalled(ctx context.Context, pkg string) (bool, error)
	Update(ctx context.Context) error
	Upgrade(ctx context.Context) error
}

type service struct {
	runner ports.CommandRunner
	logger ports.UI
}

func NewService(runner ports.CommandRunner, logger ports.UI) Service {
	return &service{runner: runner, logger: logger}
}

func (s *service) Install(ctx context.Context, packages []string) error {
	return aptpty.RunInstall(ctx, packages)
}

func (s *service) InstallBackports(ctx context.Context, packages []string, suite string) error {
	return aptpty.RunInstallBackports(ctx, packages, suite)
}

func (s *service) Remove(ctx context.Context, packages []string) error {
	return aptpty.RunRemove(ctx, packages)
}

func (s *service) CheckInstalled(ctx context.Context, pkg string) (bool, error) {
	stdout, stderr, err := s.runner.Run(ctx, "dpkg", "--get-selections", pkg)
	if err != nil {
		return false, fmt.Errorf("dpkg --get-selections %s: %s: %w", pkg, string(stderr), err)
	}
	result := ParseDpkgSelections(string(stdout), []string{pkg})
	return result[pkg], nil
}

func ParseDpkgSelections(out string, requested []string) map[string]bool {
	installed := make(map[string]bool, len(requested))
	for _, pkg := range requested {
		installed[pkg] = false
	}
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 || parts[1] != "install" {
			continue
		}
		name := parts[0]
		if _, ok := installed[name]; !ok {
			if before, _, found := strings.Cut(name, ":"); found {
				name = before
			}
			if _, ok := installed[name]; !ok {
				continue
			}
		}
		installed[name] = true
	}
	return installed
}

func (s *service) Update(ctx context.Context) error {
	_, _, err := s.runner.Run(ctx, "apt-get", "update")
	return err
}

func (s *service) Upgrade(ctx context.Context) error {
	return aptpty.RunUpgrade(ctx)
}
