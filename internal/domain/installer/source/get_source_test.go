package source

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
)

func TestGetSource_gitClone(t *testing.T) {
	var calls []string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			calls = append(calls, name)
			return nil, nil, nil
		},
	}
	inst := &Installer{runner: runner, fs: testutil.NewMockFileSystem()}
	p := &pkg.Package{
		Name:   "test-src",
		Repo:   "https://example.com/repo.git",
		Source: &pkg.SourceConfig{},
	}

	_, err := inst.getSource(context.Background(), p, "/tmp/x", &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("getSource: %v", err)
	}
	if len(calls) == 0 || calls[0] != "git" {
		t.Errorf("expected git clone, got %v", calls)
	}
}

func TestGetSource_gitCloneWithVersion(t *testing.T) {
	var recordedArgs []string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			recordedArgs = append(recordedArgs, name)
			recordedArgs = append(recordedArgs, args...)
			return nil, nil, nil
		},
	}
	inst := &Installer{runner: runner, fs: testutil.NewMockFileSystem()}
	p := &pkg.Package{
		Name:    "test-src",
		Repo:    "https://example.com/repo.git",
		Version: "1.0.0",
		Source:  &pkg.SourceConfig{},
	}

	_, err := inst.getSource(context.Background(), p, "/tmp/x", &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("getSource: %v", err)
	}
	if len(recordedArgs) < 7 || recordedArgs[4] != "--branch" || recordedArgs[5] != "v1.0.0" {
		t.Errorf("expected --branch v1.0.0 in git clone args, got %v", recordedArgs)
	}
}

func TestGetSource_gitCloneSkipCloneNoURL(t *testing.T) {
	inst := &Installer{fs: testutil.NewMockFileSystem()}
	p := &pkg.Package{
		Name:   "test-src",
		Repo:   "https://example.com/repo.git",
		Source: &pkg.SourceConfig{SkipClone: true},
	}

	_, err := inst.getSource(context.Background(), p, "/tmp/x", &testutil.MockSpinner{})
	if err == nil {
		t.Fatal("expected error when SkipClone is set but no URL")
	}
}

func TestGetSource_noRepoNoURL(t *testing.T) {
	inst := &Installer{fs: testutil.NewMockFileSystem()}
	p := &pkg.Package{
		Name:   "test-src",
		Type:   pkg.TypeSource,
		Repo:   "",
		URLs:   nil,
		Source: &pkg.SourceConfig{},
	}

	_, err := inst.getSource(context.Background(), p, "/tmp/x", &testutil.MockSpinner{})
	if err == nil {
		t.Fatal("expected error when neither repo nor URL is set")
	}
}

func TestGetSource_downloadTar(t *testing.T) {
	var recorded [][]string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			recorded = append(recorded, append([]string{name}, args...))
			if name == "tar" && len(args) >= 2 && args[0] == "tf" {
				return []byte("usr/bin/hello\nusr/share/man/hello.1\n"), nil, nil
			}
			return nil, nil, nil
		},
	}
	inst := &Installer{
		runner: runner,
		fs:     testutil.NewMockFileSystem(),
		downloadFunc: func(_ context.Context, _ ports.FileSystem, _, _ string, _ ports.Spinner, _ string) error {
			return nil
		},
	}
	p := &pkg.Package{
		Name: "test-src",
		Type: pkg.TypeSource,
		URLs: []string{"https://example.com/test-src-{version}.tar.gz"},
		Source: &pkg.SourceConfig{
			BuildScript: "echo built",
		},
	}

	_, err := inst.getSource(context.Background(), p, "/tmp/x", &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("getSource: %v", err)
	}

	var extractArgs []string
	for _, cmd := range recorded {
		if cmd[0] == "tar" && len(cmd) > 1 && cmd[1] == "-xf" {
			extractArgs = cmd
			break
		}
	}
	if extractArgs == nil {
		t.Fatal("expected tar -xf extract call")
	}
	hasStrip := false
	hasC := false
	for _, a := range extractArgs {
		if strings.Contains(a, "--strip-components") {
			hasStrip = true
		}
		if a == "-C" {
			hasC = true
		}
	}
	if !hasStrip {
		t.Error("expected --strip-components=1 in tar extract args when archive has top-level dir")
	}
	if !hasC {
		t.Error("expected -C in tar extract args")
	}
}

func TestGetSource_downloadTar_noTopDir(t *testing.T) {
	var recorded [][]string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			recorded = append(recorded, append([]string{name}, args...))
			if name == "tar" && len(args) >= 2 && args[0] == "tf" {
				return []byte("hello\nhello.1\n"), nil, nil
			}
			return nil, nil, nil
		},
	}
	inst := &Installer{
		runner: runner,
		fs:     testutil.NewMockFileSystem(),
		downloadFunc: func(_ context.Context, _ ports.FileSystem, _, _ string, _ ports.Spinner, _ string) error {
			return nil
		},
	}
	p := &pkg.Package{
		Name: "test-src",
		Type: pkg.TypeSource,
		URLs: []string{"https://example.com/test-src.tar.gz"},
		Source: &pkg.SourceConfig{
			BuildScript: "echo built",
		},
	}

	_, err := inst.getSource(context.Background(), p, "/tmp/x", &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("getSource: %v", err)
	}

	for _, cmd := range recorded {
		if cmd[0] == "tar" && len(cmd) > 1 && cmd[1] == "-xf" {
			for _, a := range cmd {
				if strings.Contains(a, "--strip-components") {
					t.Error("did not expect --strip-components when archive has no top-level dir")
				}
			}
		}
	}
}

func TestGetSource_downloadZip(t *testing.T) {
	var recorded [][]string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			recorded = append(recorded, append([]string{name}, args...))
			return nil, nil, nil
		},
	}
	inst := &Installer{
		runner: runner,
		fs:     testutil.NewMockFileSystem(),
		downloadFunc: func(_ context.Context, _ ports.FileSystem, _, _ string, _ ports.Spinner, _ string) error {
			return nil
		},
	}
	p := &pkg.Package{
		Name: "test-src",
		Type: pkg.TypeSource,
		URLs: []string{"https://example.com/test-src-{version}.zip"},
		Source: &pkg.SourceConfig{
			BuildScript: "echo built",
		},
	}

	_, err := inst.getSource(context.Background(), p, "/tmp/x", &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("getSource: %v", err)
	}

	foundUnzip := false
	for _, cmd := range recorded {
		if cmd[0] == "unzip" {
			foundUnzip = true
			break
		}
	}
	if !foundUnzip {
		t.Errorf("expected unzip command, got %v", recorded)
	}
}

func TestGetSource_downloadError(t *testing.T) {
	inst := &Installer{
		fs: testutil.NewMockFileSystem(),
		downloadFunc: func(_ context.Context, _ ports.FileSystem, _, _ string, _ ports.Spinner, _ string) error {
			return errors.New("download failed")
		},
	}
	p := &pkg.Package{
		Name: "test-src",
		Type: pkg.TypeSource,
		URLs: []string{"https://example.com/test-src.tar.gz"},
		Source: &pkg.SourceConfig{
			BuildScript: "echo built",
		},
	}

	_, err := inst.getSource(context.Background(), p, "/tmp/x", &testutil.MockSpinner{})
	if err == nil {
		t.Fatal("expected error when download fails")
	}
}
