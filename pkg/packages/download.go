package packages

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/hmwassim/debforge/pkg/cli"
	"github.com/hmwassim/debforge/pkg/executil"
	"github.com/hmwassim/debforge/pkg/text"
)

var httpClient = &http.Client{
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
	},
	Timeout: 0,
}

func DownloadFile(path, url, desc string) error {
	tmp := path + ".tmp"

	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	abort := true
	defer func() {
		if f != nil {
			f.Close()
		}
		if abort {
			os.Remove(tmp)
		}
	}()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "debforge/"+cli.Version)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading %s: unexpected HTTP status: %s", url, resp.Status)
	}

	total := resp.ContentLength
	if total <= 0 {
		_, err = io.Copy(f, resp.Body)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "[i] %s...\n", desc)
	} else {
		start := time.Now()
		pb := &progressWriter{total: total, start: start, desc: desc}
		if _, err := io.Copy(f, io.TeeReader(resp.Body, pb)); err != nil {
			return err
		}
		pb.done()
	}

	if err := f.Close(); err != nil {
		return err
	}
	f = nil
	abort = false

	return os.Rename(tmp, path)
}

func ExtractTarGz(src, dest string) error {
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}
	extract := exec.Command("tar", "-xzf", src, "-C", dest)
	extract.Stdout = io.Discard
	return executil.Run(extract)
}

var spinnerFrames = []string{"|", "/", "-", "\\"}

type progressWriter struct {
	total     int64
	current   int64
	start     time.Time
	lastPrint time.Time
	desc      string
	frameIdx  int
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

func (w *progressWriter) print() {
	if !text.IsTerminal(os.Stderr) {
		return
	}
	pct := float64(w.current) / float64(w.total) * 100
	elapsed := time.Since(w.start)
	rate := float64(w.current) / elapsed.Seconds()
	if w.current >= w.total {
		fmt.Fprintf(os.Stderr, "\r[i] %s...\033[K\n", w.desc)
	} else {
		frame := spinnerFrames[w.frameIdx%len(spinnerFrames)]
		w.frameIdx++
		fmt.Fprintf(os.Stderr, "\r[%s] %s... %3.0f%% \u2022 %s\033[K", frame, w.desc, pct, formatRate(rate))
	}
}

func formatRate(bytesPerSec float64) string {
	switch {
	case bytesPerSec >= 1024*1024:
		return fmt.Sprintf("%.1fMB/s", bytesPerSec/(1024*1024))
	case bytesPerSec >= 1024:
		return fmt.Sprintf("%.0fKB/s", bytesPerSec/1024)
	default:
		return fmt.Sprintf("%.0fB/s", bytesPerSec)
	}
}
