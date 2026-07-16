package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
)

// ---- aptBatch tests --------------------------------------------------------

func TestAptBatch_addAndReset(t *testing.T) {
	var b aptBatch

	b.addApt([]string{"pkg-a", "pkg-b"}, batchEntry{pkg: &pkg.Package{Name: "a"}})
	b.addDeb([]string{"/tmp/test.deb"}, batchEntry{pkg: &pkg.Package{Name: "b"}})

	if !b.hasWork() {
		t.Fatal("expected hasWork=true after adding entries")
	}
	if len(b.aptPkgs) != 2 {
		t.Errorf("expected 2 apt pkgs, got %d", len(b.aptPkgs))
	}
	if len(b.debPaths) != 1 {
		t.Errorf("expected 1 deb path, got %d", len(b.debPaths))
	}
	if len(b.entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(b.entries))
	}

	b.reset()
	if b.hasWork() {
		t.Error("expected hasWork=false after reset")
	}
}

func TestAptBatch_hasWork_empty(t *testing.T) {
	var b aptBatch
	if b.hasWork() {
		t.Error("expected hasWork=false for empty batch")
	}
}

func TestAptBatch_hasWork_aptOnly(t *testing.T) {
	var b aptBatch
	b.addApt([]string{"pkg-a"}, batchEntry{})
	if !b.hasWork() {
		t.Error("expected hasWork=true with apt pkgs")
	}
}

func TestAptBatch_hasWork_debOnly(t *testing.T) {
	var b aptBatch
	b.addDeb([]string{"/tmp/test.deb"}, batchEntry{})
	if !b.hasWork() {
		t.Error("expected hasWork=true with deb paths")
	}
}

// ---- flushAptBatch abort / finalize-error tests ----------------------------

func TestFlushAptBatch_aptGetFailureCallsAbort(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "pkg-a",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})
	reg.Register(&pkg.Package{
		Name: "pkg-b",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})

	recA := newBatchRecorder()
	recB := newBatchRecorder()
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, recA)
	instReg.Register(pkg.TypeDeb, recB)

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &InstallService{
		baseService: baseService{
			reg:     reg,
			instReg: instReg,
			state:   stateSvc,
			runner:  newTestBatchRunner(),
			fs:      testutil.NewMockFileSystem(),
		},
		resolver: NewResolver(reg),
		execApt: func(_ context.Context, _ ports.CommandRunner, _ []string, _ ports.Spinner) error {
			return errors.New("apt-get failed")
		},
	}

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	b := &aptBatch{}
	b.addApt([]string{"pkg-a"}, batchEntry{pkg: &pkg.Package{Name: "pkg-a", Type: pkg.TypeApt}, bi: recA})
	b.addApt([]string{"pkg-b"}, batchEntry{pkg: &pkg.Package{Name: "pkg-b", Type: pkg.TypeApt}, bi: recA})

	_, err := svc.flushAptBatch(ctx, b, st, spinner, "install", "installed")
	if err == nil {
		t.Fatal("expected error from flushAptBatch when apt-get fails")
	}

	// Abort should have been called for both entries (recA implements BatchAborter)
	if len(recA.aborted) != 2 {
		t.Errorf("expected 2 Abort calls, got %d: %v", len(recA.aborted), recA.aborted)
	}
	// Finalize should NOT have been called
	if len(recA.finalized) != 0 {
		t.Errorf("expected 0 finalizations, got %d: %v", len(recA.finalized), recA.finalized)
	}
}

func TestFlushAptBatch_partialFinalizeErrorContinues(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "pkg-a",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})
	reg.Register(&pkg.Package{
		Name: "pkg-b",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})
	reg.Register(&pkg.Package{
		Name: "pkg-c",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})

	rec := newBatchRecorder()
	rec.finalizeErr["pkg-b"] = errors.New("postinstall failed")
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, rec)

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &InstallService{
		baseService: baseService{
			reg:     reg,
			instReg: instReg,
			state:   stateSvc,
			runner:  newTestBatchRunner(),
			fs:      testutil.NewMockFileSystem(),
		},
		resolver: NewResolver(reg),
		execApt:  noopAptExec,
	}

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	b := &aptBatch{}
	b.addApt([]string{"pkg-a"}, batchEntry{pkg: &pkg.Package{Name: "pkg-a", Type: pkg.TypeApt, ForceInstall: true}, bi: rec, exists: true})
	b.addApt([]string{"pkg-b"}, batchEntry{pkg: &pkg.Package{Name: "pkg-b", Type: pkg.TypeApt, ForceInstall: true}, bi: rec, exists: true})
	b.addApt([]string{"pkg-c"}, batchEntry{pkg: &pkg.Package{Name: "pkg-c", Type: pkg.TypeApt, ForceInstall: true}, bi: rec, exists: true})

	_, err := svc.flushAptBatch(ctx, b, st, spinner, "install", "installed")

	// All three should have been finalized despite pkg-b's error
	if len(rec.finalized) != 3 {
		t.Errorf("expected 3 finalizations (including past the error), got %d: %v", len(rec.finalized), rec.finalized)
	}
	// The returned error should mention pkg-b
	if err == nil || !strings.Contains(err.Error(), "pkg-b") {
		t.Errorf("expected error mentioning pkg-b, got %v", err)
	}
}

// ---- processOne batch tests ------------------------------------------------

func TestProcessOne_batchAptPackages(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name:    "pkg-a",
		Type:    pkg.TypeApt,
		Depends: []string{"pkg-b"},
		Apt:     &pkg.AptConfig{},
	})
	reg.Register(&pkg.Package{
		Name: "pkg-b",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})

	rec := newBatchRecorder()
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, rec)

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &InstallService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: stateSvc,
			runner: newTestBatchRunner(),
			fs:     testutil.NewMockFileSystem(),
		},
		resolver: NewResolver(reg),
		execApt:  noopAptExec,
	}

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	_, err := svc.processOne(ctx, "pkg-a", false, true, st, spinner, "install", "installed", nil)
	if err != nil {
		t.Fatalf("processOne: %v", err)
	}

	// Both packages should have been finalized (batch collects both)
	if len(rec.finalized) != 2 {
		t.Errorf("expected 2 finalizations, got %d: %v", len(rec.finalized), rec.finalized)
	}
}

func TestProcessOne_batchBrokenBySource(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name:    "apt-pkg",
		Type:    pkg.TypeApt,
		Depends: []string{"src-pkg", "apt-pkg2"},
		Apt:     &pkg.AptConfig{},
	})
	reg.Register(&pkg.Package{
		Name: "src-pkg",
		Type: pkg.TypeSource,
		Source: &pkg.SourceConfig{
			BuildScript: "echo build",
		},
	})
	reg.Register(&pkg.Package{
		Name: "apt-pkg2",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})

	runner := newTestBatchRunner()
	rec := newBatchRecorder()
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, rec)
	instReg.Register(pkg.TypeSource, newBatchRecorder())

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &InstallService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: stateSvc,
			runner: runner,
			fs:     testutil.NewMockFileSystem(),
		},
		resolver: NewResolver(reg),
		execApt:  noopAptExec,
	}

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	_, err := svc.processOne(ctx, "apt-pkg", false, true, st, spinner, "install", "installed", nil)
	if err != nil {
		t.Fatalf("processOne: %v", err)
	}

	// Both apt packages should be finalized (source breaks batch into two)
	if len(rec.finalized) != 2 {
		t.Errorf("expected 2 apt finalizations, got %d: %v", len(rec.finalized), rec.finalized)
	}
}

func TestProcessOne_batchSkippedPackage(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "pkg-a",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})

	rec := newBatchRecorder()
	rec.prepareArgs["pkg-a"] = installer.BatchArgs{Skipped: true}
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, rec)

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &InstallService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: stateSvc,
			runner: newTestBatchRunner(),
			fs:     testutil.NewMockFileSystem(),
		},
		resolver: NewResolver(reg),
		execApt:  noopAptExec,
	}

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	didWork, err := svc.processOne(ctx, "pkg-a", false, true, st, spinner, "install", "installed", nil)
	if err != nil {
		t.Fatalf("processOne: %v", err)
	}
	if didWork {
		t.Error("expected didWork=false when package is skipped")
	}
	if len(rec.finalized) != 0 {
		t.Errorf("expected 0 finalizations, got %d", len(rec.finalized))
	}
}

// ---- processAll extrepo-skip tests ------------------------------------------

func TestProcessAll_skipsExtrepoWhenAlreadyInstalled(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "pkg-a",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{Extrepo: []string{"my-repo"}},
	})

	var calls []string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			cmd := name + " " + strings.Join(args, " ")
			if strings.Contains(cmd, "dpkg-query") {
				return []byte("installed\n"), nil, nil
			}
			return nil, nil, nil
		},
	}

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &InstallService{
		baseService: baseService{
			reg:    reg,
			state:  stateSvc,
			runner: runner,
			fs:     testutil.NewMockFileSystem(),
		},
		resolver: NewResolver(reg),
		execApt:  noopAptExec,
	}

	st := &State{Packages: map[string]PkgEntry{
		"pkg-a": {Type: "apt", Version: "1.0"},
	}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	err := svc.processAll(ctx, []string{"pkg-a"}, false, false, st, spinner, "install", "installed")
	if err != nil {
		t.Fatalf("processAll: %v", err)
	}

	for _, c := range calls {
		if strings.Contains(c, "extrepo enable") || strings.Contains(c, "apt-get update") {
			t.Errorf("unexpected extrepo/apt-get-update call when package already installed: %s", c)
		}
	}
}

func TestProcessAll_runsExtrepoWhenNotInstalled(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "pkg-a",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{Extrepo: []string{"my-repo"}},
	})

	var calls []string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			cmd := name + " " + strings.Join(args, " ")
			if strings.Contains(cmd, "dpkg-query") {
				return []byte("not-installed\n"), nil, nil
			}
			if strings.Contains(cmd, "apt-cache policy") {
				return []byte("Candidate: 1.0\n"), nil, nil
			}
			return nil, nil, nil
		},
	}

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	rec := newBatchRecorder()
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, rec)

	svc := &InstallService{
		baseService: baseService{
			reg:     reg,
			instReg: instReg,
			state:   stateSvc,
			runner:  runner,
			fs:      testutil.NewMockFileSystem(),
		},
		resolver: NewResolver(reg),
		execApt:  noopAptExec,
	}

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	err := svc.processAll(ctx, []string{"pkg-a"}, false, false, st, spinner, "install", "installed")
	if err != nil {
		t.Fatalf("processAll: %v", err)
	}

	enableCount := 0
	for _, c := range calls {
		if strings.Contains(c, "extrepo enable") {
			enableCount++
		}
	}
	if enableCount != 1 {
		t.Errorf("expected extrepo enable to run for a not-installed package, got %d calls", enableCount)
	}
}

// ---- enableAllExtrepos tests -----------------------------------------------

func TestEnableAllExtrepos_collectsAndEnables(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "pkg-a",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{Extrepo: []string{"repo1"}},
	})
	reg.Register(&pkg.Package{
		Name: "pkg-b",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{Extrepo: []string{"repo2"}},
	})

	var calls []string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return nil, nil, nil
		},
	}

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &InstallService{
		baseService: baseService{
			reg: reg, state: stateSvc,
			runner: runner,
		},
		resolver: NewResolver(reg),
	}

	ctx := context.Background()
	spinner := &mockSpinner{}

	if err := svc.enableAllExtrepos(ctx, []string{"pkg-a", "pkg-b"}, spinner); err != nil {
		t.Fatalf("enableAllExtrepos: %v", err)
	}

	enabledCount := 0
	updateCount := 0
	for _, c := range calls {
		if strings.Contains(c, "extrepo enable") {
			enabledCount++
		}
		if strings.Contains(c, "apt-get update") {
			updateCount++
		}
	}
	if enabledCount != 2 {
		t.Errorf("expected 2 extrepo enable calls, got %d", enabledCount)
	}
	if updateCount != 1 {
		t.Errorf("expected 1 apt-get update, got %d", updateCount)
	}
}

func TestEnableAllExtrepos_deduplicates(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "pkg-a",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{Extrepo: []string{"shared-repo"}},
	})
	reg.Register(&pkg.Package{
		Name: "pkg-b",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{Extrepo: []string{"shared-repo"}},
	})

	var calls []string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return nil, nil, nil
		},
	}

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &InstallService{
		baseService: baseService{
			reg: reg, state: stateSvc,
			runner: runner,
		},
		resolver: NewResolver(reg),
	}

	ctx := context.Background()
	spinner := &mockSpinner{}

	if err := svc.enableAllExtrepos(ctx, []string{"pkg-a", "pkg-b"}, spinner); err != nil {
		t.Fatalf("enableAllExtrepos: %v", err)
	}

	enableCount := 0
	for _, c := range calls {
		if strings.Contains(c, "extrepo enable") {
			enableCount++
		}
	}
	if enableCount != 1 {
		t.Errorf("expected 1 extrepo enable (deduplicated), got %d", enableCount)
	}
}

func TestEnableAllExtrepos_noRepos(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "pkg-a",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})

	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			t.Fatal("should not call any commands when no extrepos")
			return nil, nil, nil
		},
	}

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &InstallService{
		baseService: baseService{
			reg: reg, state: stateSvc,
			runner: runner,
		},
		resolver: NewResolver(reg),
	}

	ctx := context.Background()
	spinner := &mockSpinner{}

	if err := svc.enableAllExtrepos(ctx, []string{"pkg-a"}, spinner); err != nil {
		t.Fatalf("enableAllExtrepos: %v", err)
	}
}
