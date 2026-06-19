package ports

import "context"

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)
	RunWithSpinner(ctx context.Context, name string, args ...string) error
	RunWithEnv(ctx context.Context, dir string, env []string, name string, args ...string) (stdout, stderr []byte, err error)
}
