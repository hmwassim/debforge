package catalog

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Catalog is the validated set of package definitions.
type Catalog struct {
	pkgs map[string]*Package
}

// Layer is one source of definitions: YAML files in PkgDir and their file
// sources under FilesDir/<name>/.
type Layer struct {
	Name     string // "embedded" or the overlay path, for messages
	FS       fs.FS
	PkgDir   string
	FilesDir string
}

// Load reads every layer in order; later layers override earlier ones by
// package name (with a warning). Any invalid definition fails the load
// with all problems listed.
func Load(layers ...Layer) (*Catalog, []string, error) {
	c := &Catalog{pkgs: map[string]*Package{}}
	var warnings []string
	var errs []error
	for _, l := range layers {
		entries, err := fs.ReadDir(l.FS, l.PkgDir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", l.Name, err)
		}
		seen := map[string]string{}
		for _, e := range entries {
			if e.IsDir() || !(strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml")) {
				continue
			}
			file := path.Join(l.PkgDir, e.Name())
			origin := l.Name + ":" + file
			raw, err := fs.ReadFile(l.FS, file)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", origin, err))
				continue
			}
			pkgs, err := parse(raw, origin)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			for _, p := range pkgs {
				if prev, dup := seen[p.Name]; dup {
					errs = append(errs, fmt.Errorf("%s: package %q already defined in %s", origin, p.Name, prev))
					continue
				}
				seen[p.Name] = origin
				if err := resolveFiles(p, l); err != nil {
					errs = append(errs, fmt.Errorf("%s: %w", origin, err))
					continue
				}
				if old, ok := c.pkgs[p.Name]; ok {
					warnings = append(warnings, fmt.Sprintf("%s overrides %s", origin, old.Origin))
				}
				c.pkgs[p.Name] = p
			}
		}
	}
	errs = append(errs, c.validate()...)
	if len(errs) > 0 {
		return nil, warnings, errors.Join(errs...)
	}
	return c, warnings, nil
}

var tmplVar = regexp.MustCompile(`\{\{\s*([a-z_][a-z0-9_]*)\s*\}\}`)

func parse(raw []byte, origin string) ([]*Package, error) {
	var probe struct {
		Matrix []map[string]string `yaml:"matrix"`
	}
	if err := yaml.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("%s: %w", origin, err)
	}
	docs := [][]byte{raw}
	if len(probe.Matrix) > 0 {
		docs = docs[:0]
		for i, vars := range probe.Matrix {
			var missing []string
			out := tmplVar.ReplaceAllFunc(raw, func(m []byte) []byte {
				k := string(tmplVar.FindSubmatch(m)[1])
				v, ok := vars[k]
				if !ok {
					missing = append(missing, k)
					return m
				}
				return []byte(v)
			})
			if len(missing) > 0 {
				return nil, fmt.Errorf("%s: matrix entry %d lacks %v", origin, i, missing)
			}
			docs = append(docs, out)
		}
	} else if loc := tmplVar.FindIndex(raw); loc != nil {
		return nil, fmt.Errorf("%s: %s used without a matrix", origin, raw[loc[0]:loc[1]])
	}

	var out []*Package
	for _, doc := range docs {
		dec := yaml.NewDecoder(bytes.NewReader(doc))
		dec.KnownFields(true)
		p := &Package{}
		if err := dec.Decode(p); err != nil {
			return nil, fmt.Errorf("%s: %w", origin, err)
		}
		p.Matrix = nil
		p.Origin = origin
		sum := sha256.Sum256(doc)
		p.Hash = hex.EncodeToString(sum[:])
		out = append(out, p)
	}
	return out, nil
}

func resolveFiles(p *Package, l Layer) error {
	if err := ResolveFiles(l.FS, path.Join(l.FilesDir, p.Name), p.Files); err != nil {
		return fmt.Errorf("%s: %w", p.Name, err)
	}
	h := sha256.New()
	h.Write([]byte(p.Hash))
	for _, f := range p.Files {
		h.Write([]byte(f.Dest))
		h.Write(f.data)
		fmt.Fprintf(h, "%o", f.mode)
	}
	p.Hash = hex.EncodeToString(h.Sum(nil))
	return nil
}

// ResolveFiles loads the content of each file (src relative to dir in
// fsys, or inline content) and parses its mode.
func ResolveFiles(fsys fs.FS, dir string, fl []File) error {
	for i := range fl {
		f := &fl[i]
		switch {
		case f.Src != "" && f.Content != "":
			return fmt.Errorf("file %s: set src or content, not both", f.Dest)
		case f.Src != "":
			if !fs.ValidPath(f.Src) || strings.Contains(f.Src, "..") {
				return fmt.Errorf("file %s: bad src %q", f.Dest, f.Src)
			}
			b, err := fs.ReadFile(fsys, path.Join(dir, f.Src))
			if err != nil {
				return fmt.Errorf("file %s: %w", f.Dest, err)
			}
			f.data = b
		default:
			f.data = []byte(f.Content)
		}
		mode := uint64(0o644)
		if f.Mode != "" {
			if _, err := fmt.Sscanf(f.Mode, "%o", &mode); err != nil || mode > 0o7777 {
				return fmt.Errorf("file %s: bad mode %q", f.Dest, f.Mode)
			}
		}
		f.mode = fs.FileMode(mode)
	}
	return nil
}

// Get returns a package by name.
func (c *Catalog) Get(name string) (*Package, bool) {
	p, ok := c.pkgs[name]
	return p, ok
}

// All returns every package sorted by name.
func (c *Catalog) All() []*Package {
	out := make([]*Package, 0, len(c.pkgs))
	for _, p := range c.pkgs {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Select expands names, globs ("steam*") and categories ("@gaming") into a
// sorted, de-duplicated list. Unknown names and empty matches are errors.
func (c *Catalog) Select(args []string) ([]string, error) {
	set := map[string]bool{}
	for _, a := range args {
		var matched []string
		switch {
		case strings.HasPrefix(a, "@"):
			cat := a[1:]
			for _, p := range c.pkgs {
				if p.Category == cat {
					matched = append(matched, p.Name)
				}
			}
			if len(matched) == 0 {
				return nil, fmt.Errorf("no packages in category %q", cat)
			}
		case strings.ContainsAny(a, "*?["):
			for n := range c.pkgs {
				if ok, err := path.Match(a, n); err != nil {
					return nil, fmt.Errorf("bad pattern %q: %w", a, err)
				} else if ok {
					matched = append(matched, n)
				}
			}
			if len(matched) == 0 {
				return nil, fmt.Errorf("pattern %q matches no package", a)
			}
		default:
			if _, ok := c.pkgs[a]; !ok {
				return nil, fmt.Errorf("unknown package %q", a)
			}
			matched = []string{a}
		}
		for _, m := range matched {
			set[m] = true
		}
	}
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out, nil
}
