package ui

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

type fakeTTY struct {
	io.Reader
	out *bytes.Buffer
}

func (f fakeTTY) Write(p []byte) (int, error) { return f.out.Write(p) }
func (f fakeTTY) Close() error                { return nil }

func newUI(answer string, yes bool) (*UI, *bytes.Buffer, *bytes.Buffer) {
	var out, prompt bytes.Buffer
	// One shared, byte-at-a-time reader behaves like a canonical-mode tty
	// across repeated opens.
	r := iotest.OneByteReader(strings.NewReader(answer))
	return New(Options{
		Out: &out, Stdout: &out, Yes: yes,
		OpenTTY: func() (io.ReadWriteCloser, error) {
			return fakeTTY{Reader: r, out: &prompt}, nil
		},
	}), &out, &prompt
}

func TestConfirm(t *testing.T) {
	cases := []struct {
		ans  string
		def  bool
		want bool
	}{{"y\n", false, true}, {"\n", false, false}, {"\n", true, true}, {"no\n", true, false}}
	for _, c := range cases {
		u, _, _ := newUI(c.ans, false)
		got, err := u.Confirm("ok?", c.def)
		if err != nil || got != c.want {
			t.Errorf("%q def=%v: %v %v", c.ans, c.def, got, err)
		}
	}
	u, _, prompt := newUI("", true)
	if ok, _ := u.Confirm("ok?", false); !ok || prompt.Len() != 0 {
		t.Fatal("-y should confirm without prompting")
	}
}

func TestNoTTY(t *testing.T) {
	u := New(Options{Out: io.Discard, OpenTTY: func() (io.ReadWriteCloser, error) { return nil, errors.New("x") }})
	if _, err := u.Confirm("q", true); !errors.Is(err, ErrNoTTY) {
		t.Fatalf("err = %v", err)
	}
}

func TestChoose(t *testing.T) {
	u, _, _ := newUI("5\n2\n", false)
	i, err := u.Choose("pick", []string{"a", "b"})
	if err != nil || i != 1 {
		t.Fatalf("got %d %v", i, err)
	}
}

func TestProgressNonTTY(t *testing.T) {
	u, out, _ := newUI("", false)
	p := u.Start("Installing")
	p.Update("foo", 50)
	u.Info("hello")
	p.Done("Installed")
	p.Fail("ignored after done")
	got := out.String()
	if strings.Contains(got, "\r") || strings.Contains(got, "\033") {
		t.Fatalf("non-tty output contains control chars: %q", got)
	}
	for _, w := range []string{"-> Installing", ":: hello", "✓ Installed"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in %q", w, got)
		}
	}
	if strings.Contains(got, "ignored") {
		t.Error("Fail after Done printed")
	}
}

func TestLogFailsOpen(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	os.WriteFile(blocker, nil, 0o644)
	l := OpenLog(filepath.Join(blocker, "logs")) // parent is a file
	l.Printf("x")
	l.Printf("y")
	if !l.dead {
		t.Fatal("expected log disabled")
	}
	l2 := OpenLog(dir)
	l2.Now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	l2.Printf("hello %d", 1)
	l2.Close()
	b, _ := os.ReadFile(filepath.Join(dir, "debforge-2026-01-01.log"))
	if !strings.Contains(string(b), "hello 1") {
		t.Fatalf("log = %q", b)
	}
}
