package deb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/hmwassim/debforge/internal/domain/deployer"
	"github.com/hmwassim/debforge/internal/domain/package"
	"github.com/hmwassim/debforge/internal/installers"
	"github.com/hmwassim/debforge/internal/utils"
	"github.com/hmwassim/debforge/internal/ports"
)

type releaseInfo struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type Installer struct {
	runner   ports.CommandRunner
	http     ports.HTTPClient
	logger   ports.UI
	deployer *deployer.Deployer
	fs       ports.FileSystem
	tmpDir   string
}

func NewInstaller(runner ports.CommandRunner, http ports.HTTPClient, logger ports.UI, deployer *deployer.Deployer, fs ports.FileSystem) *Installer {
	return NewInstallerWithTempDir(runner, http, logger, deployer, fs, "")
}

func NewInstallerWithTempDir(runner ports.CommandRunner, http ports.HTTPClient, logger ports.UI, deployer *deployer.Deployer, fs ports.FileSystem, tmpDir string) *Installer {
	return &Installer{runner: runner, http: http, logger: logger, deployer: deployer, fs: fs, tmpDir: tmpDir}
}

func (i *Installer) Install(ctx context.Context, p *pkg.Package) error {
	if p.Type != pkg.TypeDeb {
		return fmt.Errorf("deb installer called for type %s", p.Type)
	}
	if p.URL == "" {
		return fmt.Errorf("url is required for deb packages")
	}
	if p.Package == "" {
		return fmt.Errorf("package is required for deb packages")
	}

	assetMatch := p.AssetMatch
	if assetMatch == "" {
		assetMatch = `\.deb$`
	}
	assetArch := p.AssetArch
	if assetArch == "" {
		assetArch = "amd64"
	}

	var debURL string
	var latestVersion string

	if isDebURL(p.URL) {
		debURL = p.URL
	} else {
		s := i.logger.Spinner(ctx, "Checking latest version...")
		info, err := i.fetchReleaseInfo(ctx, p.URL)
		if err != nil {
			s.Fail()
			return fmt.Errorf("fetching release info: %w", err)
		}
		s.Done()

		latestVersion = info.TagName
		if p.VersionPrefix != "" && strings.HasPrefix(latestVersion, p.VersionPrefix) {
			latestVersion = strings.TrimPrefix(latestVersion, p.VersionPrefix)
		}

		if !p.ForceInstall {
			installed := i.installedDebVersion(ctx, p.Package)
			if stripDebRevision(installed) == latestVersion {
				i.logger.Info("%s %s is already the latest version", p.Name, installed)
				return nil
			}
			i.logger.Info("Installed: %s  Latest: %s", strDefault(installed, "none"), latestVersion)
		}

		re, err := compileRegexWithTimeout(assetMatch)
		if err != nil {
			return fmt.Errorf("invalid asset_match regex: %w", err)
		}
		var found bool
		for _, asset := range info.Assets {
			if re.MatchString(asset.Name) && strings.Contains(asset.Name, assetArch) {
				debURL = asset.BrowserDownloadURL
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("no %s asset matching %q found in release", assetArch, assetMatch)
		}
	}

	tmpDir, err := os.MkdirTemp(i.tmpDir, "debforge-"+p.Name+"-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	safeName := sanitizeFilename(p.Name) + ".deb"
	debPath := filepath.Join(tmpDir, safeName)
	if err := i.downloadFile(ctx, debPath, debURL); err != nil {
		return fmt.Errorf("downloading %s: %w", p.Name, err)
	}

	if p.SHA256 != "" {
		if err := verifySHA256(debPath, p.SHA256); err != nil {
			return fmt.Errorf("checksum verification failed for %s: %w", p.Name, err)
		}
	}

	if latestVersion == "" {
		v, err := i.debFileVersion(ctx, debPath)
		if err != nil {
			return fmt.Errorf("reading version from .deb: %w", err)
		}
		latestVersion = v

		if !p.ForceInstall {
			installed := i.installedDebVersion(ctx, p.Package)
			if installed == latestVersion {
				i.logger.Info("%s %s is already the latest version", p.Name, installed)
				return nil
			}
		}
	}

	if err := i.installDebFile(ctx, debPath); err != nil {
		return fmt.Errorf("installing %s: %w", p.Name, err)
	}

	if p.PostInstall != "" {
		if err := i.runScript(ctx, p.PostInstall); err != nil {
			i.logger.Warn("post-install: %s", err)
		}
	}

	if err := i.deployer.DeployPackageConfigs(ctx, p.Configs, p.UserConfigs); err != nil {
		return err
	}

	p.Version = latestVersion
	i.logger.Success("%s %s installed", p.Name, latestVersion)
	return nil
}

func (i *Installer) Remove(ctx context.Context, p *pkg.Package) error {
	if p.Type != pkg.TypeDeb {
		return fmt.Errorf("deb installer called for type %s", p.Type)
	}
	if p.Package == "" {
		return fmt.Errorf("package is required for deb packages")
	}

	if _, _, err := i.runner.Run(ctx, "dpkg", "--purge", p.Package); err != nil {
		return fmt.Errorf("purging %s: %w", p.Package, err)
	}

	if p.PostRemove != "" {
		if err := i.runScript(ctx, p.PostRemove); err != nil {
			i.logger.Warn("post-remove: %s", err)
		}
	}

	i.deployer.RemoveConfigs(ctx, p.Configs)
	user, err := deployer.InvokingUser()
	if err != nil {
		i.logger.Warn("cannot determine invoking user: %v", err)
	} else {
		i.deployer.RemoveUserConfigs(ctx, p.UserConfigs, user)
	}

	i.logger.Info("%s removed", p.Name)
	return nil
}

func (i *Installer) Update(ctx context.Context, p *pkg.Package) error {
	p.ForceInstall = true
	return i.Install(ctx, p)
}

func isDebURL(url string) bool {
	return strings.HasSuffix(url, ".deb")
}

func (i *Installer) fetchReleaseInfo(ctx context.Context, url string) (*releaseInfo, error) {
	var info *releaseInfo
	err := utils.RetryHTTP(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json")

		resp, err := i.http.Do(req)
		if err != nil {
			return fmt.Errorf("fetching %s: %w", url, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			body, err := utils.ReadAllWithLimit(resp.Body, releaseJSONLimit)
			if err != nil {
				return fmt.Errorf("reading response: %w", err)
			}
			if err := json.Unmarshal(body, &info); err != nil {
				return fmt.Errorf("parsing JSON from %s: %w", url, err)
			}
			if info.TagName != "" {
				return nil
			}
		}

		if strings.HasSuffix(url, "/latest") {
			listURL := strings.TrimSuffix(url, "/latest")
			info, err = i.fetchLatestFromList(ctx, listURL)
			return err
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("fetching %s: HTTP %s", url, resp.Status)
		}

		return fmt.Errorf("no tag_name in release info from %s", url)
	})
	return info, err
}

func (i *Installer) fetchLatestFromList(ctx context.Context, url string) (*releaseInfo, error) {
	var info *releaseInfo
	err := utils.RetryHTTP(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json")

		resp, err := i.http.Do(req)
		if err != nil {
			return fmt.Errorf("fetching %s: %w", url, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("fetching %s: HTTP %s", url, resp.Status)
		}

		body, err := utils.ReadAllWithLimit(resp.Body, releaseJSONLimit)
		if err != nil {
			return fmt.Errorf("reading response: %w", err)
		}

		var list []struct {
			TagName string `json:"tag_name"`
			Draft   bool   `json:"draft"`
		}
		if err := json.Unmarshal(body, &list); err != nil {
			return fmt.Errorf("parsing release list from %s: %w", url, err)
		}

		for _, item := range list {
			if !item.Draft {
				tagURL := url + "/tags/" + item.TagName
				relInfo, err := i.fetchReleaseInfo(ctx, tagURL)
				if err != nil {
					return err
				}
				info = relInfo
				return nil
			}
		}
		return fmt.Errorf("no published release found in %s", url)
	})
	return info, err
}

func (i *Installer) installedDebVersion(ctx context.Context, pkgName string) string {
	stdout, _, err := i.runner.Run(ctx, "dpkg-query", "-W", "-f=${Version}", pkgName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(stdout))
}

func (i *Installer) installDebFile(ctx context.Context, path string) error {
	_, _, err := i.runner.Run(ctx, "apt-get", "install", "-y", path)
	return err
}

func (i *Installer) debFileVersion(ctx context.Context, path string) (string, error) {
	stdout, _, err := i.runner.Run(ctx, "dpkg-deb", "-f", path, "Version")
	if err != nil {
		return "", fmt.Errorf("dpkg-deb -f: %w", err)
	}
	return strings.TrimSpace(string(stdout)), nil
}

func (i *Installer) downloadFile(ctx context.Context, path, url string) error {
	return utils.RetryHTTP(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return err
		}
		resp, err := i.http.Do(req)
		if err != nil {
			return fmt.Errorf("downloading %s: %w", url, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("downloading %s: unexpected HTTP status: %s", url, resp.Status)
		}

		f, err := os.Create(path)
		if err != nil {
			return fmt.Errorf("creating file: %w", err)
		}
		written, err := io.CopyN(f, io.LimitReader(resp.Body, debDownloadLimit+1), debDownloadLimit+1)
		f.Close()
		if err != nil && err != io.EOF {
			return fmt.Errorf("downloading %s: %w", url, err)
		}
		if written > debDownloadLimit {
			return fmt.Errorf("download %s: response exceeds %d bytes", url, debDownloadLimit)
		}
		return nil
	})
}

func stripDebRevision(version string) string {
	if idx := strings.LastIndex(version, "-"); idx != -1 {
		return version[:idx]
	}
	return version
}

func strDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func (i *Installer) runScript(ctx context.Context, script string) error {
	return utils.RunScript(ctx, i.fs, i.runner, script)
}

func verifySHA256(path, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, expected) {
		return fmt.Errorf("SHA-256 mismatch: expected %s, got %s", expected, got)
	}
	return nil
}

var _ installers.Installer = (*Installer)(nil)

func sanitizeFilename(name string) string {
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, "/", "")
	name = strings.ReplaceAll(name, "\\", "")
	name = strings.ReplaceAll(name, "..", "")
	name = strings.Map(func(r rune) rune {
		if r == 0 || r < 32 {
			return -1
		}
		return r
	}, name)
	if name == "" {
		name = "package"
	}
	return name
}

var (
	releaseJSONLimit int64 = 1 * 1024 * 1024
	debDownloadLimit int64 = 500 * 1024 * 1024
)

func compileRegexWithTimeout(pattern string) (*regexp.Regexp, error) {
	type result struct {
		re  *regexp.Regexp
		err error
	}
	ch := make(chan result, 1)
	go func() {
		re, err := regexp.Compile(pattern)
		ch <- result{re: re, err: err}
	}()
	select {
	case r := <-ch:
		return r.re, r.err
	case <-time.After(500 * time.Millisecond):
		return nil, fmt.Errorf("regex compilation timeout")
	}
}
