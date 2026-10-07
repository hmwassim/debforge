package catalog

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var (
	nameRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]*$`)
	aptRe     = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]*(:[a-z0-9]+)?$`)
	sha256Re  = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
	pciRe     = regexp.MustCompile(`^[0-9a-f]{4}$`)
	extrepoRe = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
	binRe     = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
)

// SystemPrefixes are the only absolute locations package files and
// legacy_cleanup entries may touch. dpkg-owned trees (/usr, /lib, /boot,
// /var) are excluded on purpose.
var SystemPrefixes = []string{"/etc/", "/usr/local/", "/opt/"}

func (c *Catalog) validate() []error {
	var errs []error
	for _, p := range c.All() {
		for _, e := range validatePackage(p) {
			errs = append(errs, fmt.Errorf("%s (%s): %s", p.Name, p.Origin, e))
		}
		for _, d := range p.Depends {
			if _, ok := c.pkgs[d]; !ok {
				errs = append(errs, fmt.Errorf("%s (%s): depends on unknown package %q", p.Name, p.Origin, d))
			}
		}
	}
	return errs
}

func validatePackage(p *Package) []string {
	var e []string
	bad := func(f string, a ...any) { e = append(e, fmt.Sprintf(f, a...)) }

	if !nameRe.MatchString(p.Name) {
		bad("invalid name %q", p.Name)
	}
	if strings.TrimSpace(p.Description) == "" {
		bad("description is required")
	}
	if !slices.Contains(Categories, p.Category) {
		bad("category %q is not one of %v", p.Category, Categories)
	}
	for _, r := range p.Requires {
		if !aptRe.MatchString(r) {
			bad("requires: invalid apt package %q", r)
		}
	}
	if p.Hardware != nil && !pciRe.MatchString(p.Hardware.PCIVendor) {
		bad("hardware.pci_vendor must be 4 lowercase hex digits")
	}
	for _, r := range p.Reload {
		if !slices.Contains(Reloads, r) {
			bad("reload %q is not one of %v", r, Reloads)
		}
	}
	for _, d := range p.Debconf {
		if len(strings.Fields(d)) < 4 {
			bad("debconf line %q needs: package question type value", d)
		}
	}
	if p.Trust != "" && p.Trust != "upstream-tls" {
		bad("trust must be empty or upstream-tls")
	}

	n := 0
	for _, set := range []bool{p.Source.Apt != nil, p.Source.Deb != nil, p.Source.Archive != nil, p.Source.Git != nil, p.Source.AppImage != nil} {
		if set {
			n++
		}
	}
	if n > 1 {
		bad("source must have exactly one of apt, deb, archive, git, appimage")
	}

	var downloads []Download
	usesVersion := false
	switch p.Kind() {
	case KindApt:
		a := p.Source.Apt
		if len(a.Packages) == 0 && len(a.Variants) == 0 && len(a.Backports) == 0 {
			bad("apt source needs packages, backports or variants")
		}
		for _, list := range [][]string{a.Packages, a.Backports, a.Conflicts} {
			for _, x := range list {
				if !aptRe.MatchString(x) {
					bad("apt: invalid package %q", x)
				}
			}
		}
		for v, list := range a.Variants {
			if !nameRe.MatchString(v) || len(list) == 0 {
				bad("apt: variant %q must have a simple name and packages", v)
			}
			for _, x := range list {
				if !aptRe.MatchString(x) {
					bad("apt: variant %s: invalid package %q", v, x)
				}
			}
		}
		if r := a.Repo; r != nil {
			if u, err := url.Parse(r.URI); err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
				bad("apt.repo.uri %q is not a valid URL", r.URI)
			}
			if r.Suite == "" || strings.ContainsAny(r.Suite+strings.Join(r.Components, ""), " \n") {
				bad("apt.repo needs a suite (and components without spaces)")
			}
			if err := checkHTTPS(r.Key); err != nil {
				bad("apt.repo.key: %v", err)
			}
		}
		for _, r := range a.Extrepo {
			if !extrepoRe.MatchString(r) {
				bad("apt: invalid extrepo name %q", r)
			}
		}
		if p.Version != nil {
			bad("apt packages take their version from apt; remove version")
		}
	case KindDeb:
		if len(p.Source.Deb.URLs) == 0 {
			bad("deb source needs urls")
		}
		downloads = p.Source.Deb.URLs
	case KindArchive:
		downloads = []Download{p.Source.Archive.Download}
		if p.Source.Archive.Strip < 0 {
			bad("archive.strip must be >= 0")
		}
		if p.Hooks.Install == "" {
			bad("archive source needs an install hook")
		}
	case KindGit:
		g := p.Source.Git
		if err := checkHTTPS(g.Repo); err != nil {
			bad("git.repo: %v", err)
		}
		usesVersion = strings.Contains(g.Ref, "{version}")
		if p.Hooks.Install == "" {
			bad("git source needs an install hook")
		}
	case KindAppImage:
		a := p.Source.AppImage
		downloads = []Download{a.Download}
		if !binRe.MatchString(a.Bin) {
			bad("appimage.bin must be a plain command name")
		}
		if a.Desktop != nil {
			if a.Desktop.Name == "" {
				bad("appimage.desktop.name is required")
			}
			if strings.HasPrefix(a.Desktop.Icon, "http") {
				if err := checkHTTPS(a.Desktop.Icon); err != nil {
					bad("appimage.desktop.icon: %v", err)
				}
			}
		}
	case KindConfig:
		if len(p.Files) == 0 && p.Hooks.PostInstall == "" && len(p.Requires) == 0 {
			bad("package has no source, files, requires or hooks")
		}
	}

	for _, d := range downloads {
		if err := checkHTTPS(d.URL); err != nil {
			bad("url: %v", err)
		}
		if strings.Contains(d.URL, "{version}") {
			usesVersion = true
		}
		if d.SHA256 != "" && !sha256Re.MatchString(d.SHA256) {
			bad("sha256 %q is not 64 hex digits", d.SHA256)
		}
		if d.SHA256 == "" {
			if p.Version == nil || p.Version.From == "pin" {
				bad("download %s needs sha256 (unpinned downloads are only allowed for auto-updating packages with trust: upstream-tls)", d.URL)
			} else if p.Trust != "upstream-tls" {
				bad("download %s has no sha256; add one or set trust: upstream-tls", d.URL)
			}
		} else if strings.Contains(d.URL, "{version}") && p.Version != nil && p.Version.From != "pin" {
			bad("download %s: a sha256 cannot match an auto-updating {version} URL; use version.from: pin", d.URL)
		}
	}

	if usesVersion && p.Version == nil {
		bad("{version} is used but no version source is defined")
	}
	if v := p.Version; v != nil {
		switch v.From {
		case "git-tags":
			if err := checkHTTPS(v.Repo); err != nil {
				bad("version.repo: %v", err)
			}
			if !strings.Contains(v.Tag, "{version}") {
				bad(`version.tag must contain {version}, e.g. "v{version}"`)
			}
		case "cmd":
			if v.Cmd == "" {
				bad("version.cmd is required for from: cmd")
			}
		case "pin":
			if v.Pin == "" {
				bad("version.pin is required for from: pin")
			}
		default:
			bad("version.from must be git-tags, cmd or pin")
		}
	}

	for _, f := range p.Files {
		if err := checkDest(f.Dest); err != nil {
			bad("file %s: %v", f.Dest, err)
		}
	}
	for _, l := range p.LegacyCleanup {
		if err := checkDest(l); err != nil {
			bad("legacy_cleanup %s: %v", l, err)
		}
	}
	return e
}

func checkHTTPS(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%q is not a valid URL", raw)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("%q must use https", raw)
	}
	return nil
}

// checkDest validates a file destination: ~/... for user files, or an
// absolute path under SystemPrefixes, clean and without "..".
func checkDest(d string) error {
	p := d
	if rest, ok := strings.CutPrefix(d, "~/"); ok {
		if rest == "" {
			return fmt.Errorf("must name a file inside the home directory")
		}
		p = "/home/user/" + rest
	} else {
		if !filepath.IsAbs(d) {
			return fmt.Errorf("must be absolute or start with ~/")
		}
		ok := false
		for _, pre := range SystemPrefixes {
			if strings.HasPrefix(d, pre) && len(d) > len(pre) {
				ok = true
			}
		}
		if !ok {
			return fmt.Errorf("must be under one of %v", SystemPrefixes)
		}
	}
	if filepath.Clean(p) != p || strings.HasSuffix(d, "/") {
		return fmt.Errorf("path is not clean")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return fmt.Errorf("path contains a .. segment")
		}
	}
	return nil
}

// CheckDest is exported for setup profiles, which share the same rules.
func CheckDest(d string) error { return checkDest(d) }
