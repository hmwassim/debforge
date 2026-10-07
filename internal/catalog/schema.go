// Package catalog loads and validates package definitions. Definitions are
// decoded strictly (unknown keys are errors) and fully validated at load
// time, so a broken definition never reaches the installer.
package catalog

import (
	"os"
	"strings"
)

// Kind is the derived package kind.
type Kind string

const (
	KindApt      Kind = "apt"
	KindDeb      Kind = "deb"
	KindArchive  Kind = "archive"
	KindGit      Kind = "git"
	KindAppImage Kind = "appimage"
	KindConfig   Kind = "config" // no source: files/hooks only
)

// Package is one definition.
type Package struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Category    string `yaml:"category"`
	// Depends lists other debforge packages.
	Depends []string `yaml:"depends"`
	// Requires lists apt packages that must be present; same meaning for
	// every kind.
	Requires []string  `yaml:"requires"`
	Hardware *Hardware `yaml:"hardware"`
	Source   Source    `yaml:"source"`
	Version  *Version  `yaml:"version"`
	Files    []File    `yaml:"files"`
	Hooks    Hooks     `yaml:"hooks"`
	// Debconf lines fed to debconf-set-selections before installing.
	Debconf []string `yaml:"debconf"`
	// Reload lists triggers to run after files change.
	Reload []string `yaml:"reload"`
	// LegacyCleanup lists paths from older layouts to delete on install.
	LegacyCleanup []string `yaml:"legacy_cleanup"`
	// Notes are shown to the user after the package is installed or removed.
	Notes Notes `yaml:"notes"`
	// Trust "upstream-tls" allows unpinned downloads of auto-updating
	// packages (HTTPS only, no checksum).
	Trust string `yaml:"trust"`
	// Matrix expands one file into several packages; each entry's keys are
	// substituted for {{key}} throughout the document.
	Matrix []map[string]string `yaml:"matrix"`

	// Origin is where the definition came from (for messages).
	Origin string `yaml:"-"`
	// Hash identifies the exact definition content including file sources.
	Hash string `yaml:"-"`
}

// Hardware is an install precondition.
type Hardware struct {
	PCIVendor string `yaml:"pci_vendor"`
}

// Source is where the payload comes from; at most one field is set.
type Source struct {
	Apt      *AptSource      `yaml:"apt"`
	Deb      *DebSource      `yaml:"deb"`
	Archive  *ArchiveSource  `yaml:"archive"`
	Git      *GitSource      `yaml:"git"`
	AppImage *AppImageSource `yaml:"appimage"`
}

// AptSource installs from apt repositories.
type AptSource struct {
	Packages []string `yaml:"packages"`
	Extrepo  []string `yaml:"extrepo"`
	// Backports are installed with -t trixie-backports.
	Backports []string            `yaml:"backports"`
	Variants  map[string][]string `yaml:"variants"`
	// Conflicts are removed in the same apt transaction.
	Conflicts []string `yaml:"conflicts"`
	// Repo adds a third-party apt repository (deb822 + Signed-By key).
	Repo *AptRepo `yaml:"repo"`
}

// AptRepo is a third-party apt repository not covered by extrepo.
type AptRepo struct {
	URI           string   `yaml:"uri"`
	Suite         string   `yaml:"suite"`
	Components    []string `yaml:"components"`
	Architectures []string `yaml:"architectures"`
	// Key is the https URL of the repository signing key.
	Key string `yaml:"key"`
}

// RepoPaths returns where a package's repository key and sources file live.
func RepoPaths(name, keyURL string) (key, sources string) {
	ext := ".asc"
	if strings.HasSuffix(keyURL, ".gpg") {
		ext = ".gpg"
	}
	return "/etc/apt/keyrings/debforge-" + name + ext, "/etc/apt/sources.list.d/debforge-" + name + ".sources"
}

// Sources renders the deb822 sources file for r.
func (r *AptRepo) Sources(keyPath string) string {
	var b strings.Builder
	b.WriteString("# Managed by debforge\nTypes: deb\n")
	b.WriteString("URIs: " + r.URI + "\n")
	b.WriteString("Suites: " + r.Suite + "\n")
	if len(r.Components) > 0 {
		b.WriteString("Components: " + strings.Join(r.Components, " ") + "\n")
	}
	if len(r.Architectures) > 0 {
		b.WriteString("Architectures: " + strings.Join(r.Architectures, " ") + "\n")
	}
	b.WriteString("Signed-By: " + keyPath + "\n")
	return b.String()
}

// Download is a URL with an optional checksum.
type Download struct {
	URL    string `yaml:"url"`
	SHA256 string `yaml:"sha256"`
}

// DebSource installs downloaded .deb files through apt.
type DebSource struct {
	URLs []Download `yaml:"urls"`
}

// ArchiveSource builds from a tarball/zip.
type ArchiveSource struct {
	Download `yaml:",inline"`
	// Strip leading path components when extracting (like tar --strip-components).
	Strip int `yaml:"strip"`
}

// GitSource builds from a git checkout.
type GitSource struct {
	Repo string `yaml:"repo"`
	// Ref is a tag/branch, may contain {version}. Empty = default branch.
	Ref string `yaml:"ref"`
}

// AppImageSource installs a single AppImage.
type AppImageSource struct {
	Download `yaml:",inline"`
	// Bin is the command name under /usr/local/bin.
	Bin     string   `yaml:"bin"`
	Desktop *Desktop `yaml:"desktop"`
}

// Desktop describes a generated .desktop entry.
type Desktop struct {
	Name       string `yaml:"name"`
	Comment    string `yaml:"comment"`
	Categories string `yaml:"categories"`
	// Icon is an icon URL (https) or a themed icon name.
	Icon     string `yaml:"icon"`
	Terminal bool   `yaml:"terminal"`
}

// Version says how to discover the version to install.
type Version struct {
	// From is git-tags, cmd or pin.
	From string `yaml:"from"`
	Repo string `yaml:"repo"`
	// Tag is the tag pattern, e.g. "v{version}" or "release-{version}".
	Tag        string `yaml:"tag"`
	Prerelease bool   `yaml:"prerelease"`
	Cmd        string `yaml:"cmd"`
	Pin        string `yaml:"pin"`
}

// File is a file debforge writes and tracks.
type File struct {
	// Dest is absolute, or starts with ~/ for a file in the target user's home.
	Dest    string `yaml:"dest"`
	Src     string `yaml:"src"`
	Content string `yaml:"content"`
	Mode    string `yaml:"mode"`

	data []byte
	mode os.FileMode
}

// Data returns the resolved file content.
func (f File) Data() []byte { return f.data }

// FileMode returns the parsed mode.
func (f File) FileMode() os.FileMode { return f.mode }

// User reports whether the file lives in the target user's home.
func (f File) User() bool { return strings.HasPrefix(f.Dest, "~/") }

// Notes are short messages for the user, e.g. "log out and back in".
type Notes struct {
	Install string `yaml:"install"`
	Remove  string `yaml:"remove"`
}

// Hooks are shell snippets run with sh -eu.
type Hooks struct {
	Build       string `yaml:"build"`
	Install     string `yaml:"install"`
	PostInstall string `yaml:"post_install"`
	PreRemove   string `yaml:"pre_remove"`
	PostRemove  string `yaml:"post_remove"`
}

// Kind returns the package kind derived from Source.
func (p *Package) Kind() Kind {
	switch {
	case p.Source.Apt != nil:
		return KindApt
	case p.Source.Deb != nil:
		return KindDeb
	case p.Source.Archive != nil:
		return KindArchive
	case p.Source.Git != nil:
		return KindGit
	case p.Source.AppImage != nil:
		return KindAppImage
	default:
		return KindConfig
	}
}

// Categories is the closed set of allowed categories.
var Categories = []string{
	"browsers", "communication", "desktop", "development", "fonts",
	"gaming", "media", "system", "utils",
}

// Reloads is the closed set of reload triggers.
var Reloads = []string{"udev", "sysctl", "systemd", "tmpfiles", "fontconfig", "desktop", "modules"}
