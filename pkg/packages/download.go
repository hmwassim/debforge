package packages

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/hmwassim/debforge/pkg/cli"
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
	Timeout: 10 * time.Minute,
}

func DownloadFile(path, url, desc string) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	f, err := os.CreateTemp(dir, base)
	if err != nil {
		return err
	}
	tmp := f.Name()

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

	if resp.ContentLength > 0 {
		p := text.NewProgress(os.Stderr, resp.ContentLength, desc)
		_, err = io.Copy(io.MultiWriter(f, p), resp.Body)
		if err != nil {
			p.Fail()
			return err
		}
		p.Done()
	} else {
		sp := text.StartSpinner(os.Stderr, desc)
		_, err = io.Copy(f, resp.Body)
		if err != nil {
			sp.Fail()
			return err
		}
		sp.Done()
	}

	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	f = nil

	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	abort = false
	return nil
}
