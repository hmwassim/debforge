package execute

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hmwassim/debforge/internal/system"
)

func archiveExt(raw string) string {
	p := raw
	if u, err := url.Parse(raw); err == nil {
		p = u.Path
	}
	for _, ext := range []string{".tar.gz", ".tar.xz", ".tar.zst", ".tar.bz2", ".tgz", ".zip", ".tar"} {
		if strings.HasSuffix(strings.ToLower(p), ext) {
			return ext
		}
	}
	return ".tar"
}

// extract unpacks archive into dest, dropping strip leading path components.
func extract(ctx context.Context, r system.Runner, archive, dest string, strip int) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	if strings.HasSuffix(archive, ".zip") {
		return unzip(archive, dest, strip)
	}
	_, err := r.Run(ctx, system.Cmd{Name: "tar", Args: []string{
		"-xf", archive, "-C", dest, "--no-same-owner", "--strip-components=" + strconv.Itoa(strip),
	}})
	if err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	return nil
}

func unzip(archive, dest string, strip int) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		name := path.Clean(strings.ReplaceAll(f.Name, "\\", "/"))
		parts := strings.Split(strings.TrimPrefix(name, "/"), "/")
		if len(parts) <= strip {
			continue
		}
		rel := path.Join(parts[strip:]...)
		if rel == "." || strings.HasPrefix(rel, "../") || rel == ".." {
			return fmt.Errorf("zip entry %q escapes the destination", f.Name)
		}
		target := filepath.Join(dest, filepath.FromSlash(rel))
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if !f.Mode().IsRegular() {
			continue // skip symlinks and devices from zips
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := writeZipEntry(f, target); err != nil {
			return err
		}
	}
	return nil
}

func writeZipEntry(f *zip.File, target string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	mode := f.Mode().Perm()&0o755 | 0o644
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
