package system_test

import (
	"context"
	"errors"
	"os/user"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/system"
	"github.com/hmwassim/debforge/internal/system/systemtest"
)

const dpkgOut = "libc6\tamd64\tii \t2.41-12\n" +
	"libc6\ti386\tii \t2.41-12\n" +
	"pipewire\tamd64\tii \t1.4.2-1\n" +
	"linux-image-6.12.94+deb13-amd64\tamd64\trc \t6.12.94-1\n" +
	"fonts-noto\tall\tii \t20201225-2\n" +
	"half\tamd64\tiU \t1.0\n"

func TestSnapshot(t *testing.T) {
	s := system.ParseSnapshot("amd64", dpkgOut)
	cases := []struct {
		name string
		want bool
	}{
		{"libc6", true},
		{"libc6:i386", true},
		{"libc6:amd64", true},
		{"pipewire", true},
		{"pipewire:i386", false},
		{"linux-image-6.12.94+deb13-amd64", false}, // rc = removed
		{"fonts-noto", true},                       // arch all
		{"half", false},                            // unpacked, not configured
		{"missing", false},
	}
	for _, c := range cases {
		if got := s.Installed(c.name); got != c.want {
			t.Errorf("Installed(%q) = %v, want %v", c.name, got, c.want)
		}
	}
	if v := s.Version("pipewire"); v != "1.4.2-1" {
		t.Errorf("Version = %q", v)
	}
	if v := s.Version("linux-image-6.12.94+deb13-amd64"); v != "" {
		t.Errorf("rc package version = %q, want empty", v)
	}
}

func TestTakeSnapshot(t *testing.T) {
	r := &systemtest.Runner{}
	r.OK("dpkg --print-architecture", "amd64\n").OK("dpkg-query -W", dpkgOut)
	s, err := system.TakeSnapshot(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Installed("libc6:i386") {
		t.Fatal("expected libc6:i386 installed")
	}
}

func TestBaseEnv(t *testing.T) {
	env := system.BaseEnv([]string{"PATH=/bin", "GITHUB_TOKEN=x", "LANG=fr_FR.UTF-8", "SSH_AUTH_SOCK=/s", "AWS_SECRET=y"})
	joined := strings.Join(env, " ")
	for _, want := range []string{"PATH=/bin", "SSH_AUTH_SOCK=/s", "LC_ALL=C", "GIT_TERMINAL_PROMPT=0"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %s in %v", want, env)
		}
	}
	for _, bad := range []string{"GITHUB_TOKEN", "AWS_SECRET", "fr_FR"} {
		if strings.Contains(joined, bad) {
			t.Errorf("leaked %s in %v", bad, env)
		}
	}
}

func TestExecRunner(t *testing.T) {
	r := system.ExecRunner{}
	res, err := r.Run(context.Background(), system.Cmd{Name: "sh", Args: []string{"-c", "echo out; echo err >&2; exit 3"}})
	var ee *system.ExitError
	if !errors.As(err, &ee) || ee.Code != 3 || !strings.Contains(ee.Error(), "err") {
		t.Fatalf("err = %v", err)
	}
	if string(res.Stdout) != "out\n" {
		t.Fatalf("stdout = %q", res.Stdout)
	}
	res, err = r.Run(context.Background(), system.Cmd{Name: "pwd"})
	if err != nil || strings.TrimSpace(string(res.Stdout)) != "/" {
		t.Fatalf("default dir: %q %v", res.Stdout, err)
	}
}

func TestFakeRunnerRejectsUnknown(t *testing.T) {
	r := &systemtest.Runner{}
	if _, err := r.Run(context.Background(), system.Cmd{Name: "apt-get", Args: []string{"install"}}); err == nil {
		t.Fatal("expected unexpected-command error")
	}
}

func TestTargetUser(t *testing.T) {
	lk := system.Lookup{
		ByName: func(n string) (*user.User, error) {
			if n == "alice" {
				return &user.User{Username: "alice", Uid: "1000", Gid: "1000", HomeDir: "/home/alice"}, nil
			}
			return nil, errors.New("unknown")
		},
		ByID: func(id string) (*user.User, error) {
			if id == "1000" {
				return &user.User{Username: "alice", Uid: "1000", Gid: "1000", HomeDir: "/home/alice"}, nil
			}
			return nil, errors.New("unknown")
		},
	}
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

	u, err := system.TargetUser(env(map[string]string{"SUDO_USER": "alice"}), 0, lk)
	if err != nil || u.Home != "/home/alice" || u.UID != 1000 {
		t.Fatalf("sudo: %+v %v", u, err)
	}
	if _, err := system.TargetUser(env(map[string]string{"PKEXEC_UID": "1000"}), 0, lk); err != nil {
		t.Fatalf("pkexec: %v", err)
	}
	if _, err := system.TargetUser(env(nil), 0, lk); !errors.Is(err, system.ErrNoTargetUser) {
		t.Fatalf("plain root: %v", err)
	}
	if u, err := system.TargetUser(env(nil), 1000, lk); err != nil || u.Name != "alice" {
		t.Fatalf("non-root: %+v %v", u, err)
	}
	if got := u.ExpandHome("~/.config/x"); got != "/home/alice/.config/x" {
		t.Fatalf("ExpandHome = %q", got)
	}
}
