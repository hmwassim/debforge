package repo

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/hmwassim/debforge/pkg/executil"
	"github.com/hmwassim/debforge/pkg/packages"
	"github.com/hmwassim/debforge/pkg/text"
)

var debHTTPClient = &http.Client{
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
	},
	Timeout: 5 * time.Minute,
}

type releaseInfo struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func (p *RepoPackage) debInstall(log *text.Logger, state *PackagesState, force bool) error {
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
		s := text.StartSpinner(os.Stderr, "Checking latest version...")
		info, err := fetchReleaseInfo(p.URL)
		if err != nil {
			s.Fail()
			return fmt.Errorf("fetching release info: %w", err)
		}
		s.Done()

		latestVersion = info.TagName
		if p.VersionPrefix != "" && strings.HasPrefix(latestVersion, p.VersionPrefix) {
			latestVersion = strings.TrimPrefix(latestVersion, p.VersionPrefix)
		}

		if !force {
			installed := installedDebVersion(p.Package)
			if stripDebRevision(installed) == latestVersion {
				log.Info("%s %s is already the latest version", p.Name, installed)
				state.Packages[p.Name] = PkgEntry{Type: p.Type, Version: latestVersion}
				if err := saveState(state); err != nil {
					return fmt.Errorf("%s verified but state not saved: %w", p.Name, err)
				}
				return nil
			}
			log.Info("Installed: %s  Latest: %s", strDefault(installed, "none"), latestVersion)
		}

		re, err := regexp.Compile(assetMatch)
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

	tmpDir, err := os.MkdirTemp("", "debforge-"+p.Name+"-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	debPath := filepath.Join(tmpDir, filepath.Base(debURL))
	if err := packages.DownloadFile(debPath, debURL, "Downloading "+p.Name+"..."); err != nil {
		return fmt.Errorf("downloading %s: %w", p.Name, err)
	}

	if latestVersion == "" {
		v, err := debFileVersion(debPath)
		if err != nil {
			return fmt.Errorf("reading version from .deb: %w", err)
		}
		latestVersion = v

		if !force {
			installed := installedDebVersion(p.Package)
			if stripDebRevision(installed) == stripDebRevision(latestVersion) {
				log.Info("%s %s is already the latest version", p.Name, installed)
				return nil
			}
		}
	}

	s := text.StartSpinner(os.Stderr, "Installing "+p.Name+"...")
	if err := installDebFile(debPath); err != nil {
		s.Fail()
		return fmt.Errorf("installing %s: %w", p.Name, err)
	}
	s.Done()

	if p.PostInstall != "" {
		if err := executil.Run(exec.Command("sh", "-c", p.PostInstall)); err != nil {
			log.Warn("post-install: %s", err)
		}
	}

	state.Packages[p.Name] = PkgEntry{Type: p.Type, Version: latestVersion}
	if err := saveState(state); err != nil {
		return fmt.Errorf("%s installed but state not saved: %w", p.Name, err)
	}

	log.Success("%s %s installed", p.Name, latestVersion)
	return nil
}

func (p *RepoPackage) debUpdate(log *text.Logger, state *PackagesState) error {
	return p.debInstall(log, state, false)
}

func (p *RepoPackage) debRemove(log *text.Logger, state *PackagesState) error {
	if p.Package == "" {
		return fmt.Errorf("package is required for deb packages")
	}

	s := text.StartSpinner(os.Stderr, "Purging "+p.Package+"...")
	if err := executil.Run(exec.Command("dpkg", "--purge", p.Package)); err != nil {
		s.Fail()
		return fmt.Errorf("purging %s: %w", p.Package, err)
	}
	s.Done()

	if p.PostRemove != "" {
		if err := executil.Run(exec.Command("sh", "-c", p.PostRemove)); err != nil {
			log.Warn("post-remove: %s", err)
		}
	}

	for path := range p.Configs {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Warn("Could not remove %s: %s", path, err)
		}
	}
	removeUserConfigs(log, p.UserConfigs)

	delete(state.Packages, p.Name)
	if err := saveState(state); err != nil {
		return fmt.Errorf("%s removed but state not saved: %w", p.Name, err)
	}

	log.Info("%s removed", p.Name)
	return nil
}

func isDebURL(url string) bool {
	return strings.HasSuffix(url, ".deb")
}

func fetchReleaseInfo(url string) (*releaseInfo, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := debHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching %s: HTTP %s", url, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var info releaseInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("parsing JSON from %s: %w", url, err)
	}
	if info.TagName == "" {
		return nil, fmt.Errorf("no tag_name in release info from %s", url)
	}

	return &info, nil
}

func installedDebVersion(pkgName string) string {
	cmd := exec.Command("dpkg-query", "-W", "-f=${Version}", pkgName)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func installDebFile(path string) error {
	return executil.Run(exec.Command("apt-get", "install", "-y", path))
}

func debFileVersion(path string) (string, error) {
	cmd := exec.Command("dpkg-deb", "-f", path, "Version")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("dpkg-deb -f: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func stripDebRevision(version string) string {
	if idx := strings.Index(version, "-"); idx != -1 {
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
