package packages

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/hmwassim/debforge/pkg/executil"
)

func DownloadFile(path, url string) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}

	total := resp.ContentLength
	if total <= 0 {
		_, err = io.Copy(out, resp.Body)
		return err
	}

	start := time.Now()
	pb := &progressWriter{total: total, start: start}
	if _, err := io.Copy(out, io.TeeReader(resp.Body, pb)); err != nil {
		return err
	}
	pb.done()
	fmt.Fprintln(os.Stderr)
	return nil
}

func ExtractTarGz(src, dest string) error {
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}
	extract := exec.Command("tar", "-xzf", src, "-C", dest)
	extract.Stdout = nil
	return executil.Run(extract)
}

type progressWriter struct {
	total     int64
	current   int64
	start     time.Time
	lastPrint time.Time
}

func (w *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	w.current += int64(n)
	if time.Since(w.lastPrint) < 100*time.Millisecond {
		return n, nil
	}
	w.lastPrint = time.Now()
	w.print()
	return n, nil
}

func (w *progressWriter) done() {
	w.current = w.total
	w.print()
}

func isStderrTerminal() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func (w *progressWriter) print() {
	if !isStderrTerminal() {
		return
	}
	pct := float64(w.current) / float64(w.total) * 100
	barWidth := 40
	filled := int(float64(barWidth) * float64(w.current) / float64(w.total))
	bar := strings.Repeat("=", filled) + strings.Repeat("-", barWidth-filled)
	if filled < barWidth {
		bar = bar[:filled] + ">" + bar[filled+1:]
	}
	elapsed := time.Since(w.start)
	rate := float64(w.current) / elapsed.Seconds()
	var etaStr string
	if rate > 0 {
		remaining := time.Duration(float64(w.total-w.current)/rate) * time.Second
		etaStr = remaining.Truncate(time.Second).String()
	} else {
		etaStr = "?"
	}
	fmt.Fprintf(os.Stderr, "\033[2K\r  [%s] %3.0f%%  ETA %s", bar, pct, etaStr)
}
