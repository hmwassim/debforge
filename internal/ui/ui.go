// Package ui owns all terminal output. Every write goes through one mutex so
// progress lines, messages and prompts never interleave. Colour and
// animation are decided once from the terminal and NO_COLOR.
package ui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrNoTTY is returned when a prompt is needed but there is no terminal.
var ErrNoTTY = errors.New("confirmation needed but no terminal is available (use -y)")

// Options configures a UI.
type Options struct {
	Out    io.Writer // messages and progress (usually stderr)
	Stdout io.Writer // command output such as list/info
	// TTY enables animation and in-place redraws.
	TTY   bool
	Color bool
	// Yes answers every confirmation with yes.
	Yes bool
	// OpenTTY opens the terminal for prompts; defaults to /dev/tty.
	OpenTTY func() (io.ReadWriteCloser, error)
}

// UI is the terminal front end.
type UI struct {
	mu     sync.Mutex
	o      Options
	active *Progress
}

// New creates a UI.
func New(o Options) *UI {
	if o.OpenTTY == nil {
		o.OpenTTY = func() (io.ReadWriteCloser, error) { return os.OpenFile("/dev/tty", os.O_RDWR, 0) }
	}
	return &UI{o: o}
}

// Detect builds Options for the real process.
func Detect(yes bool) Options {
	tty := isTerminal(os.Stderr)
	color := tty && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
	return Options{Out: os.Stderr, Stdout: os.Stdout, TTY: tty && os.Getenv("TERM") != "dumb", Color: color, Yes: yes}
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// Yes reports whether prompts are auto-confirmed.
func (u *UI) Yes() bool { return u.o.Yes }

// Color helpers.
func (u *UI) paint(code, s string) string {
	if !u.o.Color {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}
func (u *UI) Bold(s string) string   { return u.paint("1", s) }
func (u *UI) Dim(s string) string    { return u.paint("2", s) }
func (u *UI) Red(s string) string    { return u.paint("31", s) }
func (u *UI) Green(s string) string  { return u.paint("32", s) }
func (u *UI) Yellow(s string) string { return u.paint("33", s) }
func (u *UI) Blue(s string) string   { return u.paint("34", s) }

// MarkColor is the colour of a bracketed status marker.
type MarkColor string

const (
	MarkPlain   MarkColor = ""
	MarkRed     MarkColor = "31"
	MarkGreen   MarkColor = "32"
	MarkYellow  MarkColor = "33"
	MarkBlue    MarkColor = "34"
	MarkMagenta MarkColor = "95"
)

// Mark renders an ASCII status marker such as "[*]", bold and coloured, so
// output reads the same on any terminal.
func (u *UI) Mark(sym string, c MarkColor) string {
	if c == MarkPlain {
		return "[" + sym + "]"
	}
	return u.paint("1;"+string(c), "["+sym+"]")
}

// line writes one message line, temporarily clearing an active progress line.
func (u *UI) line(w io.Writer, s string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.clearLocked()
	fmt.Fprintln(w, s)
	u.redrawLocked()
}

func (u *UI) Info(format string, a ...any) {
	u.line(u.o.Out, u.Mark("i", MarkBlue)+" "+fmt.Sprintf(format, a...))
}
func (u *UI) Success(format string, a ...any) {
	u.line(u.o.Out, u.Mark("*", MarkGreen)+" "+fmt.Sprintf(format, a...))
}
func (u *UI) Warn(format string, a ...any) {
	u.line(u.o.Out, u.Mark("!", MarkYellow)+" "+fmt.Sprintf(format, a...))
}
func (u *UI) Error(format string, a ...any) {
	u.line(u.o.Out, u.Mark("x", MarkRed)+" "+fmt.Sprintf(format, a...))
}

// Print writes plain command output to stdout.
func (u *UI) Print(s string) {
	u.line(u.o.Stdout, strings.TrimRight(s, "\n"))
}

// Confirm asks a yes/no question. With Yes set it returns true without
// prompting.
func (u *UI) Confirm(q string, def bool) (bool, error) {
	if u.o.Yes {
		return true, nil
	}
	hint := "[y/N]"
	if def {
		hint = "[Y/n]"
	}
	ans, err := u.ask(u.Mark("?", MarkYellow) + " " + q + " " + hint + " ")
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(ans)) {
	case "":
		return def, nil
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

// Choose asks the user to pick one of options and returns its index.
func (u *UI) Choose(q string, options []string) (int, error) {
	var b strings.Builder
	b.WriteString(u.Mark("?", MarkYellow) + " " + q + "\n")
	for i, o := range options {
		fmt.Fprintf(&b, "  %d) %s\n", i+1, o)
	}
	b.WriteString("Choice: ")
	for attempt := 0; attempt < 3; attempt++ {
		ans, err := u.ask(b.String())
		if err != nil {
			return -1, err
		}
		n, err := strconv.Atoi(strings.TrimSpace(ans))
		if err == nil && n >= 1 && n <= len(options) {
			return n - 1, nil
		}
		b.Reset()
		b.WriteString("Choice: ")
	}
	return -1, errors.New("no valid choice given")
}

func (u *UI) ask(prompt string) (string, error) {
	tty, err := u.o.OpenTTY()
	if err != nil {
		return "", ErrNoTTY
	}
	defer tty.Close()
	u.mu.Lock()
	u.clearLocked()
	fmt.Fprint(tty, prompt)
	u.mu.Unlock()
	line, err := readLine(tty)
	u.mu.Lock()
	u.redrawLocked()
	u.mu.Unlock()
	if err != nil && line == "" {
		return "", fmt.Errorf("read answer: %w", err)
	}
	return line, nil
}

// readLine reads up to and including '\n' one byte at a time, so nothing
// beyond the answer is consumed from the terminal.
func readLine(r io.Reader) (string, error) {
	var b []byte
	buf := make([]byte, 1)
	for {
		n, err := r.Read(buf)
		if n == 1 {
			if buf[0] == '\n' {
				return string(b), nil
			}
			b = append(b, buf[0])
			if len(b) > 4096 {
				return string(b), errors.New("answer too long")
			}
		}
		if err != nil {
			return string(b), err
		}
	}
}

// Progress is a single in-place status line.
type Progress struct {
	u      *UI
	title  string
	detail string
	pct    float64
	frame  int
	stop   chan struct{}
	done   bool
}

var frames = []string{"|", "/", "-", "\\"}

// Start begins a progress line. Only one is active at a time; starting a
// new one finishes the previous one silently.
func (u *UI) Start(title string) *Progress { return u.start(title, true) }

// Wait is Start for short waits that are followed by their own output: on a
// terminal it animates, otherwise it prints nothing.
func (u *UI) Wait(title string) *Progress { return u.start(title, false) }

func (u *UI) start(title string, announce bool) *Progress {
	p := &Progress{u: u, title: title, pct: -1, stop: make(chan struct{})}
	u.mu.Lock()
	if u.active != nil {
		u.active.finishLocked("")
	}
	u.active = p
	if u.o.TTY {
		u.redrawLocked()
		go p.tick()
	} else if announce {
		fmt.Fprintln(u.o.Out, u.Mark("i", MarkBlue)+" "+title)
	}
	u.mu.Unlock()
	return p
}

func (p *Progress) tick() {
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-t.C:
			p.u.mu.Lock()
			if !p.done {
				p.frame++
				p.u.redrawLocked()
			}
			p.u.mu.Unlock()
		}
	}
}

// Update changes the detail text and percentage (pct < 0 hides it).
func (p *Progress) Update(detail string, pct float64) {
	p.u.mu.Lock()
	defer p.u.mu.Unlock()
	if p.done {
		return
	}
	p.detail, p.pct = detail, pct
	if p.u.o.TTY {
		p.u.redrawLocked()
	}
}

// Done finishes successfully, printing msg (or the title).
func (p *Progress) Done(msg string) {
	if msg == "" {
		msg = p.title
	}
	p.finish(p.u.Mark("*", MarkGreen) + " " + msg)
}

// Fail finishes unsuccessfully.
func (p *Progress) Fail(msg string) {
	if msg == "" {
		msg = p.title
	}
	p.finish(p.u.Mark("x", MarkRed) + " " + msg)
}

// Clear finishes without printing anything, for progress lines that
// only cover a wait and are followed by their own output.
func (p *Progress) Clear() { p.finish("") }

func (p *Progress) finish(line string) {
	p.u.mu.Lock()
	defer p.u.mu.Unlock()
	p.finishLocked(line)
}

func (p *Progress) finishLocked(line string) {
	if p.done {
		return
	}
	p.done = true
	close(p.stop)
	if p.u.active == p {
		p.u.clearLocked()
		p.u.active = nil
	}
	if line != "" {
		fmt.Fprintln(p.u.o.Out, line)
	}
}

func (u *UI) clearLocked() {
	if u.active != nil && u.o.TTY {
		fmt.Fprint(u.o.Out, "\r\033[K")
	}
}

func (u *UI) redrawLocked() {
	p := u.active
	if p == nil || !u.o.TTY || p.done {
		return
	}
	s := u.Mark(frames[p.frame%len(frames)], MarkMagenta) + " " + p.title
	if p.detail != "" {
		s += "  " + u.Dim(p.detail)
	}
	if p.pct >= 0 {
		s += fmt.Sprintf("  %3.0f%%", p.pct)
	}
	fmt.Fprint(u.o.Out, "\r\033[K"+s)
}
