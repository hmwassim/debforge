package format

import (
	"bufio"
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/hmwassim/debforge/internal/domain/pkg"
)

func writePackageLine(w *bufio.Writer, name, desc string, installed bool, pad int) {
	green, grey, reset := "\033[32m", "\033[90m", "\033[0m"
	if installed {
		fmt.Fprintf(w, "%s[*]%s %-*s", green, reset, pad, name)
		if desc != "" {
			fmt.Fprintf(w, "%s%s%s", grey, desc, reset)
		}
		fmt.Fprintln(w)
	} else {
		fmt.Fprintf(w, "%s[-]%s %s%-*s%s", grey, reset, grey, pad, name, reset)
		if desc != "" {
			fmt.Fprintf(w, "%s%s%s", grey, desc, reset)
		}
		fmt.Fprintln(w)
	}
}

func FormatSearchOutput(reg *pkg.Registry, st StateView, patterns []string) string {
	var names []string
	reg.Range(func(name string, p *pkg.Package) bool {
		for _, pat := range patterns {
			if strings.HasPrefix(pat, "@") {
				cat := pat[1:]
				if p.Category != cat {
					return true
				}
			} else {
				patLower := strings.ToLower(pat)
				n := strings.ToLower(name)
				d := strings.ToLower(p.Description)
				if !strings.Contains(n, patLower) && !strings.Contains(d, patLower) {
					return true
				}
			}
		}
		names = append(names, name)
		return true
	})
	sort.Strings(names)

	maxLen := 0
	for _, name := range names {
		if len(name) > maxLen {
			maxLen = len(name)
		}
	}
	pad := maxLen + 2

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	for _, name := range names {
		p, _ := reg.Lookup(name)
		writePackageLine(w, name, p.Description, st.IsInstalled(name), pad)
	}
	w.Flush()
	return buf.String()
}

func FormatListCategories(reg *pkg.Registry, st StateView) string {
	idx := reg.Categories()
	cats := make([]string, 0, len(idx))
	for c := range idx {
		cats = append(cats, c)
	}
	sort.Strings(cats)

	if len(cats) == 0 {
		return ""
	}

	maxLen := 0
	for _, c := range cats {
		if len(c) > maxLen {
			maxLen = len(c)
		}
	}

	green, blue, bold, reset := "\033[32m", "\033[34m", "\033[1m", "\033[0m"
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	for _, c := range cats {
		pkgs := idx[c]
		inst := st.CountInstalled(pkgs)
		marker, color := "i", bold+blue
		if inst == len(pkgs) {
			marker, color = "*", bold+green
		}
		fmt.Fprintf(w, "%s[%s]%s %-*s (%d/%d)\n", color, marker, reset, maxLen, c, inst, len(pkgs))
	}
	w.Flush()
	return buf.String()
}

func FormatListCategory(reg *pkg.Registry, st StateView, category string) string {
	idx := reg.Categories()
	pkgs, ok := idx[category]
	if !ok {
		return ""
	}

	maxLen := 0
	for _, name := range pkgs {
		if len(name) > maxLen {
			maxLen = len(name)
		}
	}
	pad := maxLen + 2

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	fmt.Fprintln(w, category)
	fmt.Fprintln(w)
	for _, name := range pkgs {
		p, _ := reg.Lookup(name)
		writePackageLine(w, name, p.Description, st.IsInstalled(name), pad)
	}
	w.Flush()
	return buf.String()
}

func FormatListPackages(reg *pkg.Registry, st StateView) string {
	idx := reg.Categories()
	cats := make([]string, 0, len(idx))
	for c := range idx {
		cats = append(cats, c)
	}
	sort.Strings(cats)

	if len(cats) == 0 {
		return ""
	}

	maxLen := 0
	reg.Range(func(name string, _ *pkg.Package) bool {
		if len(name) > maxLen {
			maxLen = len(name)
		}
		return true
	})
	pad := maxLen + 2

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	for i, c := range cats {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, c)
		for _, name := range idx[c] {
			p, _ := reg.Lookup(name)
		writePackageLine(w, name, p.Description, st.IsInstalled(name), pad)
		}
	}
	w.Flush()
	return buf.String()
}

func sortedMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
