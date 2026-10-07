package apt

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hmwassim/debforge/internal/system"
	"github.com/hmwassim/debforge/internal/system/systemtest"
)

func statusWriter(lines string, code int, stdout string) systemtest.Handler {
	return func(c system.Cmd) (system.Result, error) {
		if len(c.ExtraFiles) == 1 {
			c.ExtraFiles[0].WriteString(lines)
		}
		if code != 0 {
			return system.Result{Stdout: []byte(stdout), ExitCode: code}, &system.ExitError{Cmd: c.Name, Code: code}
		}
		return system.Result{Stdout: []byte(stdout)}, nil
	}
}

func TestInstallArgsAndProgress(t *testing.T) {
	r := &systemtest.Runner{}
	r.On("apt-get", statusWriter("dlstatus:1:50:Retrieving file 1\npmstatus:firefox:80:Installing firefox\n", 0, ""))
	a := &Apt{R: r}
	var mu sync.Mutex
	var phases []string
	err := a.Install(context.Background(), Transaction{Install: []string{"firefox"}, Remove: []string{"firefox-esr"}},
		func(phase string, pct float64, d string) {
			mu.Lock()
			phases = append(phases, phase+":"+d)
			mu.Unlock()
		})
	if err != nil {
		t.Fatal(err)
	}
	c := r.Calls[0]
	line := c.String()
	for _, want := range []string{"Dpkg::Options::=--force-confold", "DPkg::Lock::Timeout=300", "APT::Status-Fd=3", "install firefox firefox-esr-"} {
		if !strings.Contains(line, want) {
			t.Errorf("missing %q in %s", want, line)
		}
	}
	if !c.OwnProcessGroup || !strings.Contains(strings.Join(c.Env, " "), "DEBIAN_FRONTEND=noninteractive") {
		t.Error("apt must be non-interactive in its own process group")
	}
	if strings.Join(phases, "|") != "download:Retrieving file 1|install:Installing firefox" {
		t.Errorf("phases = %v", phases)
	}
}

func TestBackportsTarget(t *testing.T) {
	r := &systemtest.Runner{}
	r.On("apt-get", statusWriter("", 0, ""))
	(&Apt{R: r}).Install(context.Background(), Transaction{Install: []string{"mesa-vulkan-drivers"}, Target: BackportsSuite}, nil)
	if !strings.Contains(r.Commands()[0], "-t trixie-backports install mesa-vulkan-drivers") {
		t.Fatal(r.Commands()[0])
	}
}

func TestInstallErrorMessage(t *testing.T) {
	r := &systemtest.Runner{}
	r.On("apt-get", statusWriter("pmerror:/var/cache/x.deb:40:dependency problems\n", 100, "E: Sub-process /usr/bin/dpkg returned an error code (1)\n"))
	err := (&Apt{R: r}).Install(context.Background(), Transaction{Install: []string{"x"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "dependency problems") || !strings.Contains(err.Error(), "dpkg returned") {
		t.Fatalf("err = %v", err)
	}
}

func TestInterruptedStillCompletes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &systemtest.Runner{}
	r.On("apt-get", func(c system.Cmd) (system.Result, error) {
		cancel() // user presses Ctrl-C mid-transaction
		return system.Result{}, nil
	})
	err := (&Apt{R: r}).Install(ctx, Transaction{Install: []string{"x"}}, nil)
	if err != nil {
		t.Fatalf("a completed transaction must report success so it gets recorded: %v", err)
	}
	r.Fail("apt-get", 100, "E: boom")
	if err := (&Apt{R: r}).Install(ctx, Transaction{Install: []string{"x"}}, nil); !errors.Is(err, ErrInterrupted) {
		t.Fatalf("failed + interrupted: %v", err)
	}
}

func TestUpdateWarnings(t *testing.T) {
	r := &systemtest.Runner{}
	r.OK("apt-get", "Hit:1 http://deb.debian.org trixie InRelease\nErr:2 https://repo.example stable InRelease\nW: Failed to fetch https://repo.example\n")
	w, err := (&Apt{R: r}).Update(context.Background())
	if err != nil || len(w) != 2 {
		t.Fatalf("w=%v err=%v", w, err)
	}
}

func TestPendingUpgrades(t *testing.T) {
	r := &systemtest.Runner{}
	r.OK("apt-get -s", "Inst a [1] (2 Debian)\nConf a (2 Debian)\nInst b [1] (2)\n")
	n, err := (&Apt{R: r}).PendingUpgrades(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestPreseed(t *testing.T) {
	r := &systemtest.Runner{}
	var got string
	r.On("debconf-set-selections", func(c system.Cmd) (system.Result, error) {
		b := new(strings.Builder)
		buf := make([]byte, 256)
		n, _ := c.Stdin.Read(buf)
		b.Write(buf[:n])
		got = b.String()
		return system.Result{}, nil
	})
	(&Apt{R: r}).Preseed(context.Background(), []string{"ttf-mscorefonts-installer msttcorefonts/accepted-mscorefonts-eula boolean true"})
	if !strings.Contains(got, "accepted-mscorefonts-eula boolean true\n") {
		t.Fatalf("stdin = %q", got)
	}
}

func TestExtrepoEnabled(t *testing.T) {
	dir := t.TempDir()
	e := &Extrepo{SourcesDir: dir}
	if e.Enabled("mozilla") {
		t.Fatal("absent should be disabled")
	}
	os.WriteFile(filepath.Join(dir, "extrepo_mozilla.sources"), []byte("Types: deb\nEnabled: no\n"), 0o644)
	if e.Enabled("mozilla") {
		t.Fatal("Enabled: no")
	}
	os.WriteFile(filepath.Join(dir, "extrepo_mozilla.sources"), []byte("Types: deb\n"), 0o644)
	if !e.Enabled("mozilla") {
		t.Fatal("should be enabled")
	}
}

func TestRealExecRunnerFd3(t *testing.T) {
	// End-to-end check that fd 3 reaches a real child process.
	dir := t.TempDir()
	fake := filepath.Join(dir, "apt-get")
	os.WriteFile(fake, []byte("#!/bin/sh\necho 'pmstatus:x:100:Installed x' >&3\n"), 0o755)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	var got string
	err := (&Apt{R: system.ExecRunner{}}).Install(context.Background(), Transaction{Install: []string{"x"}},
		func(_ string, _ float64, d string) { got = d })
	if err != nil || got != "Installed x" {
		t.Fatalf("got %q err %v", got, err)
	}
}

const pinDpkg = "ii \t26.1.6-1~bpo13+1\tmesa\n" +
	"ii \t0.196-1~bpo13+2\telfutils\n" +
	"ii \t1.4.2-1\tpipewire\n" +
	"rc \t7.2.6-1~bpo13+1\tlinux-signed-amd64\n" +
	"ii \t26.1.6-1~bpo13+1\tmesa\n"

func TestSyncBackportPins(t *testing.T) {
	pin := filepath.Join(t.TempDir(), "preferences.d", "debforge-backports.pref")
	r := (&systemtest.Runner{}).OK("dpkg-query", pinDpkg).OK("dpkg --print-foreign-architectures", "i386\n")
	a := &Apt{R: r, PinFile: pin}
	if err := a.SyncBackportPins(context.Background()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(pin)
	want := "Package: src:elfutils src:elfutils:i386 src:mesa src:mesa:i386\nPin: release n=trixie-backports\nPin-Priority: 500\n"
	if !strings.HasSuffix(string(b), want) {
		t.Fatalf("pin file:\n%s", b)
	}
	// Nothing from backports left: the file goes away.
	r.OK("dpkg-query", "ii \t1.4.2-1\tpipewire\n")
	if err := a.SyncBackportPins(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pin); !os.IsNotExist(err) {
		t.Fatal("pin file should be removed")
	}
}

func TestInstallSyncsPinsAroundBackports(t *testing.T) {
	pin := filepath.Join(t.TempDir(), "debforge-backports.pref")
	r := &systemtest.Runner{}
	r.OK("dpkg --print-foreign-architectures", "i386\n")
	r.OK("dpkg-query", "")
	r.On("apt-get", func(c system.Cmd) (system.Result, error) {
		r.OK("dpkg-query", pinDpkg) // the backports install put mesa on the system
		return system.Result{}, nil
	})
	a := &Apt{R: r, PinFile: pin}
	if err := a.Install(context.Background(), Transaction{Install: []string{"mesa-vulkan-drivers"}, Target: BackportsSuite}, nil); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(pin); err != nil || !strings.Contains(string(b), "src:mesa:i386") {
		t.Fatalf("pins not written after backports install: %q %v", b, err)
	}
	cmds := strings.Join(r.Commands(), "\n")
	if strings.Index(cmds, "dpkg-query") > strings.Index(cmds, "apt-get") {
		t.Fatalf("pins must be synced before the install too:\n%s", cmds)
	}
}
