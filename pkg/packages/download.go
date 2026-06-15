package packages

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/hmwassim/debforge/pkg/cli"
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

	_, err = io.Copy(f, resp.Body)
	if err != nil {
		return err
	}
	if desc != "" {
		fmt.Fprintf(os.Stderr, "\r[*] %s...\n", desc)
	}

	if err := f.Close(); err != nil {
		return err
	}
	f = nil
	abort = false

	return os.Rename(tmp, path)
}
