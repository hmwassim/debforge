package utils

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/hmwassim/debforge/internal/ports"
)

func RunScript(ctx context.Context, fs ports.FileSystem, runner ports.CommandRunner, script string) error {
	if script == "" {
		return nil
	}
	tmpDir, err := fs.MkdirTemp("", "debforge-script-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer fs.RemoveAll(tmpDir)

	scriptPath := filepath.Join(tmpDir, "run.sh")
	content := "#!/bin/sh\n" + script + "\n"
	if err := fs.WriteFile(scriptPath, []byte(content), 0755); err != nil {
		return fmt.Errorf("writing script: %w", err)
	}

	_, _, err = runner.Run(ctx, scriptPath)
	return err
}

func DownloadFile(ctx context.Context, fs ports.FileSystem, httpClient ports.HTTPClient, path, url string) error {
	return RetryHTTP(ctx, func() error {
		dir := filepath.Dir(path)
		base := filepath.Base(path)

		if err := fs.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating parent dir: %w", err)
		}
		tmpDir, err := fs.MkdirTemp(dir, base)
		if err != nil {
			return err
		}
		defer fs.RemoveAll(tmpDir)
		tmpPath := filepath.Join(tmpDir, "data")

		// BYPASS: ports.HTTPClient only abstracts Do(), not request creation
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return err
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("downloading %s: %w", url, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("downloading %s: unexpected HTTP status: %s", url, resp.Status)
		}

		data, err := ReadAllWithLimit(resp.Body, 500*1024*1024)
		if err != nil {
			return fmt.Errorf("reading response: %w", err)
		}

		if err := fs.WriteFile(tmpPath, data, 0644); err != nil {
			return err
		}

		if err := fs.Rename(tmpPath, path); err != nil {
			return err
		}
		return nil
	})
}
