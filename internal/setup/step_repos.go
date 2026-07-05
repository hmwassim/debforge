package setup

import (
	"context"
)

type ReposStep struct {
	Sources []ConfigFile
}

func (s *ReposStep) Name() string {
	return "Configured Debian repositories"
}

func (s *ReposStep) Check(ctx context.Context, cx *Context) CheckResult {
	return checkConfigFiles(cx, s.Sources)
}

func (s *ReposStep) Apply(ctx context.Context, cx *Context, result CheckResult) error {
	return processConfigFiles(cx, s.Sources, result)
}
