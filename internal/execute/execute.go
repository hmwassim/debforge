// Package execute carries out a plan.Plan. It never decides what to do;
// it only does what the plan says, in phases ordered so that nothing on
// the system changes until every download has succeeded, and it records
// state after each package so an interruption never loses track of what
// was installed.
package execute

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hmwassim/debforge/internal/apt"
	"github.com/hmwassim/debforge/internal/catalog"
	"github.com/hmwassim/debforge/internal/fetch"
	"github.com/hmwassim/debforge/internal/files"
	"github.com/hmwassim/debforge/internal/plan"
	"github.com/hmwassim/debforge/internal/state"
	"github.com/hmwassim/debforge/internal/system"
	"github.com/hmwassim/debforge/internal/ui"
)

// Exec holds everything the executor touches.
type Exec struct {
	R       system.Runner
	Apt     *apt.Apt
	Extrepo *apt.Extrepo
	Files   *files.Engine
	HTTP    *http.Client
	UI      *ui.UI
	Log     *ui.Log
	Store   state.Store
	State   *state.State
	Snap    *system.Snapshot
	User    *system.User
	WorkDir string
	Now     func() time.Time
	// Force replaces user-modified files (with backup).
	Force bool
}

// Summary collects things the user should know after the run.
type Summary struct {
	Notes    []string
	Warnings []string
}

func (x *Exec) now() time.Time {
	if x.Now != nil {
		return x.Now()
	}
	return time.Now()
}

func (x *Exec) save() error {
	if err := x.Store.Save(x.State); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	return nil
}

// prepared holds per-item artifacts from the fetch phase.
type prepared struct {
	dir     string
	debs    []string // local .deb paths
	debPkgs []string // their dpkg package names
	src     string   // extracted/cloned source tree
	appimg  string   // downloaded AppImage
	icon    string   // downloaded icon
	// repoFiles are the key and sources file of the package's own apt
	// repository, written before the apt transaction.
	repoFiles map[string]state.File
}

// Install executes an install/update plan.
func (x *Exec) Install(ctx context.Context, pl *plan.Plan) (*Summary, error) {
	sum := &Summary{Warnings: append([]string(nil), pl.Warnings...)}
	if pl.Empty() {
		return sum, nil
	}

	if err := x.Apt.Preseed(ctx, pl.Debconf); err != nil {
		return sum, fmt.Errorf("debconf preseed: %w", err)
	}
	prep := map[string]*prepared{}
	if err := x.enableRepos(ctx, pl, prep, sum); err != nil {
		return sum, err
	}

	// Fetch everything before changing packages.
	for _, it := range pl.Items {
		if err := ctx.Err(); err != nil {
			return sum, err
		}
		p, err := x.fetch(ctx, it)
		if err != nil {
			x.cleanup(prep)
			return sum, fmt.Errorf("%s: %w", it.Name, err)
		}
		if old := prep[it.Name]; old != nil {
			p.repoFiles = old.repoFiles
		}
		prep[it.Name] = p
	}
	defer x.cleanup(prep)

	// One apt transaction for everything, then backports.
	tx := apt.Transaction{Install: append([]string(nil), pl.AptInstall...), Remove: pl.AptConflicts}
	for _, it := range pl.Items {
		tx.Install = append(tx.Install, prep[it.Name].debs...)
	}
	if err := x.aptStep(ctx, "Installing packages", func(prog apt.ProgressFunc) error { return x.Apt.Install(ctx, tx, prog) }); err != nil {
		return sum, err
	}
	if len(pl.AptBackports) > 0 {
		btx := apt.Transaction{Install: pl.AptBackports, Target: apt.BackportsSuite}
		if err := x.aptStep(ctx, "Installing from backports", func(prog apt.ProgressFunc) error { return x.Apt.Install(ctx, btx, prog) }); err != nil {
			return sum, err
		}
	}

	// Record what apt installed before anything else can fail.
	for _, it := range pl.Items {
		sp := x.State.Packages[it.Name]
		if sp == nil {
			sp = &state.Package{InstalledAt: x.now(), Files: map[string]state.File{}}
			x.State.Packages[it.Name] = sp
		}
		sp.Kind = string(it.Pkg.Kind())
		sp.Variant = it.Variant
		sp.Explicit = it.Explicit
		sp.AptPackages = it.AptRecord
		sp.Debs = append([]string(nil), prep[it.Name].debPkgs...)
		if it.Pkg.Kind() == catalog.KindApt {
			sp.Extrepos = it.Pkg.Source.Apt.Extrepo
		}
		sp.Incomplete = true
	}
	if err := x.save(); err != nil {
		return sum, err
	}

	reloads := map[string]bool{}
	for _, it := range pl.Items {
		if err := ctx.Err(); err != nil {
			return sum, fmt.Errorf("stopped before %s: %w", it.Name, err)
		}
		prog := x.UI.Start(fmt.Sprintf("Configuring %s", it.Name))
		if err := x.finish(ctx, it, prep[it.Name], sum, prog); err != nil {
			prog.Fail(fmt.Sprintf("%s failed", it.Name))
			x.runReloads(ctx, reloads, sum)
			return sum, fmt.Errorf("%s: %w (it is marked incomplete; 'debforge update %s' retries)", it.Name, err, it.Name)
		}
		for _, r := range it.Pkg.Reload {
			reloads[r] = true
		}
		if it.Pkg.Kind() == catalog.KindAppImage && it.Pkg.Source.AppImage.Desktop != nil {
			reloads["desktop"] = true
		}
		label := it.Name
		if it.Version != "" {
			label += " " + it.Version
		}
		prog.Done(map[plan.Op]string{plan.OpInstall: "Installed ", plan.OpUpgrade: "Upgraded ", plan.OpReinstall: "Reinstalled "}[it.Op] + label)
	}
	x.runReloads(ctx, reloads, sum)
	return sum, nil
}

func (x *Exec) aptStep(ctx context.Context, title string, run func(apt.ProgressFunc) error) error {
	prog := x.UI.Start(title)
	err := run(func(phase string, pct float64, detail string) {
		if phase == "download" {
			prog.Update("downloading", pct)
		} else {
			prog.Update(detail, pct)
		}
	})
	if err != nil {
		prog.Fail(title + " failed")
		return err
	}
	prog.Done(title)
	return nil
}

func (x *Exec) enableRepos(ctx context.Context, pl *plan.Plan, prep map[string]*prepared, sum *Summary) error {
	changed, err := x.writeRepos(ctx, pl, prep)
	if err != nil {
		return err
	}
	if len(pl.Extrepos) == 0 {
		if !changed {
			return nil
		}
		prog := x.UI.Start("Refreshing package lists")
		warns, err := x.Apt.Update(ctx)
		sum.Warnings = append(sum.Warnings, warns...)
		if err != nil {
			prog.Fail("Refreshing package lists failed")
			return err
		}
		prog.Done("Refreshed package lists")
		return nil
	}
	if !x.Snap.Installed("extrepo") {
		if err := x.aptStep(ctx, "Installing extrepo", func(prog apt.ProgressFunc) error {
			return x.Apt.Install(ctx, apt.Transaction{Install: []string{"extrepo"}}, prog)
		}); err != nil {
			return err
		}
	}
	prog := x.UI.Start("Enabling repositories")
	for _, r := range pl.Extrepos {
		if err := x.Extrepo.Enable(ctx, r); err != nil {
			prog.Fail("Enabling repositories failed")
			return fmt.Errorf("extrepo enable %s: %w", r, err)
		}
	}
	prog.Update("refreshing package lists", -1)
	warns, err := x.Apt.Update(ctx)
	sum.Warnings = append(sum.Warnings, warns...)
	if err != nil {
		prog.Fail("Refreshing package lists failed")
		return err
	}
	prog.Done("Enabled " + strings.Join(pl.Extrepos, ", "))
	return nil
}

// writeRepos installs signing keys and deb822 sources for packages with
// their own apt repository. It reports whether anything changed.
func (x *Exec) writeRepos(ctx context.Context, pl *plan.Plan, prep map[string]*prepared) (bool, error) {
	changed := false
	for _, it := range pl.Items {
		if it.Pkg.Kind() != catalog.KindApt || it.Pkg.Source.Apt.Repo == nil {
			continue
		}
		r := it.Pkg.Source.Apt.Repo
		keyPath, srcPath := catalog.RepoPaths(it.Name, r.Key)
		dir := filepath.Join(x.WorkDir, it.Name+"-repo")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return false, err
		}
		defer os.RemoveAll(dir)
		key := filepath.Join(dir, "key")
		if err := fetch.Download(ctx, x.HTTP, r.Key, key, "", nil); err != nil {
			return false, fmt.Errorf("%s: repository key: %w", it.Name, err)
		}
		src := filepath.Join(dir, "sources")
		if err := os.WriteFile(src, []byte(r.Sources(keyPath)), 0o644); err != nil {
			return false, err
		}
		var old map[string]state.File
		if sp := x.State.Packages[it.Name]; sp != nil {
			old = sp.Files
		}
		recs := map[string]state.File{}
		for dest, from := range map[string]string{keyPath: key, srcPath: src} {
			var prev *state.File
			if r, ok := old[dest]; ok {
				prev = &r
			}
			rec, err := x.Files.Place(from, dest, 0o644, prev)
			if err != nil {
				return false, fmt.Errorf("%s: %w", it.Name, err)
			}
			if prev == nil || prev.SHA256 != rec.SHA256 {
				changed = true
			}
			recs[dest] = rec
		}
		prep[it.Name] = &prepared{repoFiles: recs}
	}
	return changed, nil
}

// fetch downloads or clones the payload of non-apt items.
func (x *Exec) fetch(ctx context.Context, it *plan.Item) (*prepared, error) {
	p := &prepared{}
	k := it.Pkg.Kind()
	if k == catalog.KindApt || k == catalog.KindConfig {
		return p, nil
	}
	p.dir = filepath.Join(x.WorkDir, it.Name)
	os.RemoveAll(p.dir)
	if err := os.MkdirAll(p.dir, 0o755); err != nil {
		return nil, err
	}
	dl := func(url, sha, dest string) error {
		url = fetch.Expand(url, it.Version)
		prog := x.UI.Start("Downloading " + path.Base(url))
		err := fetch.Download(ctx, x.HTTP, url, dest, sha, func(done, total int64) {
			pct := -1.0
			if total > 0 {
				pct = float64(done) * 100 / float64(total)
			}
			prog.Update(fmt.Sprintf("%.1f MB", float64(done)/1e6), pct)
		})
		if err != nil {
			prog.Fail("Download failed: " + path.Base(url))
			return err
		}
		prog.Done("Downloaded " + path.Base(url))
		return nil
	}

	switch k {
	case catalog.KindDeb:
		for i, d := range it.Pkg.Source.Deb.URLs {
			dest := filepath.Join(p.dir, fmt.Sprintf("%d.deb", i))
			if err := dl(d.URL, d.SHA256, dest); err != nil {
				return nil, err
			}
			res, err := x.R.Run(ctx, system.Cmd{Name: "dpkg-deb", Args: []string{"-f", dest, "Package"}})
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", d.URL, err)
			}
			p.debs = append(p.debs, dest)
			p.debPkgs = append(p.debPkgs, strings.TrimSpace(string(res.Stdout)))
		}
	case catalog.KindAppImage:
		a := it.Pkg.Source.AppImage
		p.appimg = filepath.Join(p.dir, "app.AppImage")
		if err := dl(a.URL, a.SHA256, p.appimg); err != nil {
			return nil, err
		}
		if a.Desktop != nil && strings.HasPrefix(a.Desktop.Icon, "https://") {
			p.icon = filepath.Join(p.dir, "icon"+path.Ext(strings.SplitN(a.Desktop.Icon, "?", 2)[0]))
			if err := dl(a.Desktop.Icon, "", p.icon); err != nil {
				return nil, err
			}
		}
	case catalog.KindArchive:
		a := it.Pkg.Source.Archive
		arch := filepath.Join(p.dir, "source"+archiveExt(a.URL))
		if err := dl(a.URL, a.SHA256, arch); err != nil {
			return nil, err
		}
		p.src = filepath.Join(p.dir, "src")
		if err := extract(ctx, x.R, arch, p.src, a.Strip); err != nil {
			return nil, err
		}
	case catalog.KindGit:
		g := it.Pkg.Source.Git
		p.src = filepath.Join(p.dir, "src")
		args := []string{"clone", "--depth", "1"}
		if g.Ref != "" {
			args = append(args, "--branch", fetch.Expand(g.Ref, it.Version))
		}
		args = append(args, "--", g.Repo, p.src)
		prog := x.UI.Start("Cloning " + g.Repo)
		if _, err := x.R.Run(ctx, system.Cmd{Name: "git", Args: args, Timeout: 15 * time.Minute}); err != nil {
			prog.Fail("Clone failed")
			return nil, err
		}
		prog.Done("Cloned " + g.Repo)
	}
	return p, nil
}

func (x *Exec) cleanup(prep map[string]*prepared) {
	for _, p := range prep {
		if p != nil && p.dir != "" {
			os.RemoveAll(p.dir)
		}
	}
}

// finish does everything after apt for one item and marks it complete.
func (x *Exec) finish(ctx context.Context, it *plan.Item, p *prepared, sum *Summary, prog *ui.Progress) error {
	sp := x.State.Packages[it.Name]
	old := sp.Files
	if old == nil {
		old = map[string]state.File{}
	}
	newFiles := map[string]state.File{}
	for k, v := range p.repoFiles {
		newFiles[k] = v
	}
	prevOf := func(path string) *state.File {
		if r, ok := old[path]; ok {
			return &r
		}
		return nil
	}

	switch it.Pkg.Kind() {
	case catalog.KindArchive, catalog.KindGit:
		prog.Update("building", -1)
		stage := filepath.Join(p.dir, "stage")
		if err := os.MkdirAll(stage, 0o755); err != nil {
			return err
		}
		if it.Pkg.Hooks.Build != "" {
			if err := x.hook(ctx, it, "build", it.Pkg.Hooks.Build, p.src, nil, 60*time.Minute); err != nil {
				return err
			}
		}
		prog.Update("installing", -1)
		if err := x.hook(ctx, it, "install", it.Pkg.Hooks.Install, p.src, []string{"DESTDIR=" + stage}, 20*time.Minute); err != nil {
			return err
		}
		if err := x.placeStaged(stage, prevOf, newFiles); err != nil {
			return err
		}
	case catalog.KindAppImage:
		a := it.Pkg.Source.AppImage
		bin := "/usr/local/bin/" + a.Bin
		rec, err := x.Files.Place(p.appimg, bin, 0o755, prevOf(bin))
		if err != nil {
			return err
		}
		newFiles[bin] = rec
		if a.Desktop != nil {
			icon := a.Desktop.Icon
			if p.icon != "" {
				dest := "/usr/local/share/debforge/icons/" + it.Name + filepath.Ext(p.icon)
				rec, err := x.Files.Place(p.icon, dest, 0o644, prevOf(dest))
				if err != nil {
					return err
				}
				newFiles[dest], icon = rec, dest
			}
			entry := filepath.Join(p.dir, "entry.desktop")
			if err := os.WriteFile(entry, []byte(desktopEntry(a, bin, icon)), 0o644); err != nil {
				return err
			}
			dest := "/usr/local/share/applications/" + it.Name + ".desktop"
			rec, err := x.Files.Place(entry, dest, 0o644, prevOf(dest))
			if err != nil {
				return err
			}
			newFiles[dest] = rec
		}
	}

	for _, f := range it.Pkg.Files {
		spec := plan.FileSpec(f, x.User)
		out, err := x.Files.Apply(spec, prevOf(spec.Path), x.Force)
		if err != nil {
			return err
		}
		newFiles[spec.Path] = out.Record
		switch out.Action {
		case "sidecar":
			sum.Notes = append(sum.Notes, fmt.Sprintf("%s was modified by you; the new version is in %s (run 'debforge diff')", spec.Path, out.Note))
		case "replaced":
			sum.Notes = append(sum.Notes, fmt.Sprintf("%s existed; the previous file was saved as %s", spec.Path, out.Note))
		case "kept":
			x.Log.Printf("%s: kept user-modified %s", it.Name, spec.Path)
		}
	}

	// Files the previous version had but this one does not.
	for path, rec := range old {
		if _, still := newFiles[path]; still {
			continue
		}
		home := ""
		if x.User != nil {
			home = x.User.Home
		}
		out, err := x.Files.RemoveRecord(path, rec, home)
		if err != nil {
			sum.Warnings = append(sum.Warnings, fmt.Sprintf("could not remove old file %s: %v", path, err))
		} else if out.Action == "kept" {
			sum.Notes = append(sum.Notes, fmt.Sprintf("%s is no longer managed but was modified; left in place", path))
		}
	}
	sp.Files = newFiles
	if err := x.save(); err != nil {
		return err
	}

	for _, l := range it.Pkg.LegacyCleanup {
		if err := x.Files.Delete(l); err != nil {
			sum.Warnings = append(sum.Warnings, fmt.Sprintf("legacy cleanup %s: %v", l, err))
		}
	}
	if h := it.Pkg.Hooks.PostInstall; h != "" {
		prog.Update("post-install", -1)
		if err := x.hook(ctx, it, "post_install", h, "", nil, 10*time.Minute); err != nil {
			return err
		}
	}

	sp.Version = it.Version
	sp.DefHash = it.Pkg.Hash
	sp.Incomplete = false
	if it.Op != plan.OpInstall {
		sp.InstalledAt = x.now()
	}
	return x.save()
}

func (x *Exec) placeStaged(stage string, prevOf func(string) *state.File, out map[string]state.File) error {
	n := 0
	err := filepath.WalkDir(stage, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		dest := "/" + filepath.ToSlash(strings.TrimPrefix(p, stage+string(filepath.Separator)))
		if err := catalog.CheckDest(dest); err != nil {
			return fmt.Errorf("install hook wrote %s: %w", dest, err)
		}
		var rec state.File
		if d.Type()&fs.ModeSymlink != 0 {
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			rec, err = x.Files.PlaceSymlink(target, dest, prevOf(dest))
			if err != nil {
				return err
			}
		} else {
			info, err := d.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("install hook created unsupported file type at %s", dest)
			}
			// Keep setuid/setgid (e.g. tools that change GPU clocks), but
			// never group/world write.
			mode := info.Mode() & (fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid) &^ 0o022
			rec, err = x.Files.Place(p, dest, mode, prevOf(dest))
			if err != nil {
				return err
			}
		}
		out[dest] = rec
		n++
		return nil
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New(`install hook installed nothing; it must install into "$DESTDIR"`)
	}
	return nil
}

// hook runs a package hook with sh -eu.
func (x *Exec) hook(ctx context.Context, it *plan.Item, name, script, dir string, extra []string, timeout time.Duration) error {
	if dir == "" {
		dir = filepath.Join(x.WorkDir, it.Name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	env := append([]string{"VERSION=" + it.Version, "PKGNAME=" + it.Name}, extra...)
	if x.User != nil {
		env = append(env,
			"TARGET_USER="+x.User.Name, "TARGET_HOME="+x.User.Home,
			fmt.Sprintf("TARGET_UID=%d", x.User.UID), fmt.Sprintf("TARGET_GID=%d", x.User.GID))
	}
	x.Log.Printf("%s: running %s hook", it.Name, name)
	w := x.Log.Writer(it.Name + "/" + name + ": ")
	_, err := x.R.Run(ctx, system.Cmd{
		Name: "sh", Args: []string{"-eu", "-c", script}, Dir: dir, Env: env,
		Stdin: strings.NewReader(""), Stdout: w, Stderr: w, Timeout: timeout,
	})
	if err != nil {
		return fmt.Errorf("%s hook failed: %w", name, err)
	}
	return nil
}

var reloadCmds = map[string][][]string{
	"udev":       {{"udevadm", "control", "--reload"}, {"udevadm", "trigger", "--action=change"}},
	"sysctl":     {{"sysctl", "--system"}},
	"systemd":    {{"systemctl", "daemon-reload"}},
	"tmpfiles":   {{"systemd-tmpfiles", "--create"}},
	"fontconfig": {{"fc-cache", "-f"}},
	"desktop":    {{"update-desktop-database", "-q", "/usr/local/share/applications"}},
	"modules":    {{"systemctl", "restart", "systemd-modules-load.service"}},
}

func (x *Exec) runReloads(ctx context.Context, reloads map[string]bool, sum *Summary) {
	var keys []string
	for k := range reloads {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, c := range reloadCmds[k] {
			if _, err := x.R.Run(context.WithoutCancel(ctx), system.Cmd{Name: c[0], Args: c[1:], Timeout: 2 * time.Minute}); err != nil {
				sum.Warnings = append(sum.Warnings, fmt.Sprintf("reload %s: %v (changes apply after reboot)", k, err))
				break
			}
		}
	}
}

// Remove executes a removal plan.
func (x *Exec) Remove(ctx context.Context, pl *plan.Plan) (*Summary, error) {
	sum := &Summary{}
	if pl.Empty() {
		return sum, nil
	}
	for _, it := range pl.Items {
		if it.Pkg != nil && it.Pkg.Hooks.PreRemove != "" {
			if err := x.hook(ctx, it, "pre_remove", it.Pkg.Hooks.PreRemove, "", nil, 10*time.Minute); err != nil {
				return sum, fmt.Errorf("%s: %w (nothing was removed)", it.Name, err)
			}
		}
	}
	if len(pl.AptRemove) > 0 {
		if err := x.aptStep(ctx, "Removing packages", func(prog apt.ProgressFunc) error {
			return x.Apt.Remove(ctx, pl.AptRemove, prog)
		}); err != nil {
			return sum, err
		}
	}
	for _, r := range pl.ExtreposDisable {
		if err := x.Extrepo.Disable(ctx, r); err != nil {
			sum.Warnings = append(sum.Warnings, fmt.Sprintf("extrepo disable %s: %v", r, err))
		}
	}
	reloads := map[string]bool{}
	home := ""
	if x.User != nil {
		home = x.User.Home
	}
	for _, it := range pl.Items {
		sp := x.State.Packages[it.Name]
		var paths []string
		for p := range sp.Files {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		for _, p := range paths {
			out, err := x.Files.RemoveRecord(p, sp.Files[p], home)
			switch {
			case err != nil:
				sum.Warnings = append(sum.Warnings, fmt.Sprintf("%s: %v", p, err))
			case out.Action == "kept":
				sum.Notes = append(sum.Notes, fmt.Sprintf("%s was modified, so it was left in place", p))
			case out.Action == "restored":
				sum.Notes = append(sum.Notes, fmt.Sprintf("%s: restored the file that existed before debforge", p))
			}
		}
		if it.Pkg != nil {
			if it.Pkg.Hooks.PostRemove != "" {
				if err := x.hook(ctx, it, "post_remove", it.Pkg.Hooks.PostRemove, "", nil, 10*time.Minute); err != nil {
					sum.Warnings = append(sum.Warnings, fmt.Sprintf("%s: %v", it.Name, err))
				}
			}
			for _, r := range it.Pkg.Reload {
				reloads[r] = true
			}
		}
		delete(x.State.Packages, it.Name)
		if err := x.save(); err != nil {
			return sum, err
		}
		x.UI.Success("Removed %s", it.Name)
	}
	x.runReloads(ctx, reloads, sum)
	return sum, nil
}

func desktopEntry(a *catalog.AppImageSource, bin, icon string) string {
	d := a.Desktop
	var b strings.Builder
	b.WriteString("[Desktop Entry]\nType=Application\n")
	fmt.Fprintf(&b, "Name=%s\n", d.Name)
	if d.Comment != "" {
		fmt.Fprintf(&b, "Comment=%s\n", d.Comment)
	}
	fmt.Fprintf(&b, "Exec=%s %%U\n", bin)
	if icon != "" {
		fmt.Fprintf(&b, "Icon=%s\n", icon)
	}
	fmt.Fprintf(&b, "Terminal=%t\n", d.Terminal)
	if d.Categories != "" {
		fmt.Fprintf(&b, "Categories=%s\n", d.Categories)
	}
	return b.String()
}
