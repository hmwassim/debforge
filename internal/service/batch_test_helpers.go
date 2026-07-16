package service

import (
	"context"
	"strings"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
)

// batchRecorder records Prepare/Finalize/Abort calls and returns controlled results.
type batchRecorder struct {
	prepareArgs map[string]installer.BatchArgs // keyed by package name
	prepareErr  map[string]error
	finalizeErr map[string]error
	finalized   []string
	aborted     []string
}

func newBatchRecorder() *batchRecorder {
	return &batchRecorder{
		prepareArgs: make(map[string]installer.BatchArgs),
		prepareErr:  make(map[string]error),
		finalizeErr: make(map[string]error),
	}
}

func (r *batchRecorder) Install(_ context.Context, _ *pkg.Package, _ ports.Spinner) error {
	return nil
}

func (r *batchRecorder) Remove(_ context.Context, _ *pkg.Package, _ ports.Spinner) error {
	return nil
}

func (r *batchRecorder) Prepare(_ context.Context, p *pkg.Package, _ ports.Spinner) (installer.BatchArgs, error) {
	if err, ok := r.prepareErr[p.Name]; ok {
		return installer.BatchArgs{}, err
	}
	if args, ok := r.prepareArgs[p.Name]; ok {
		return args, nil
	}
	return installer.BatchArgs{AptPkgs: []string{p.Name}}, nil
}

func (r *batchRecorder) Finalize(_ context.Context, p *pkg.Package, _ ports.Spinner) error {
	r.finalized = append(r.finalized, p.Name)
	if err, ok := r.finalizeErr[p.Name]; ok {
		return err
	}
	return nil
}

func (r *batchRecorder) Abort(p *pkg.Package) {
	r.aborted = append(r.aborted, p.Name)
}

// testBatchRunner handles common commands for batch tests.
type testBatchRunner struct {
	*testutil.MockRunner
}

func newTestBatchRunner() *testBatchRunner {
	return &testBatchRunner{
		MockRunner: &testutil.MockRunner{
			RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
				cmd := name + " " + strings.Join(args, " ")
				switch {
				case strings.Contains(cmd, "dpkg-query"):
					return []byte("not-installed\n"), nil, nil
				case strings.Contains(cmd, "apt-cache policy"):
					return []byte("Candidate: 1.0\n"), nil, nil
				case strings.Contains(cmd, "git ls-remote"):
					return []byte("abc123 HEAD\n"), nil, nil
				default:
					return nil, nil, nil
				}
			},
		},
	}
}

// noopAptExec is a no-op apt executor for tests.
var noopAptExec aptpty.AptExecFunc = func(_ context.Context, _ ports.CommandRunner, _ []string, _ ports.Spinner) error {
	return nil
}
