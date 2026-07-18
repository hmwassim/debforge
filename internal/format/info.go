package format

import (
	"bufio"
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/textutil"
)

type scriptEntry struct {
	name    string
	content string
}

func FormatInfoOutput(reg *pkg.Registry, st StateView, pkgName string, verbose bool) string {
	p, ok := reg.Lookup(pkgName)
	if !ok {
		return ""
	}

	green, grey, blue, bold, reset := "\033[32m", "\033[90m", "\033[34m", "\033[1m", "\033[0m"
	installed := st.IsInstalled(pkgName)

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	if installed {
		fmt.Fprintf(w, "%s[*]%s %s%s", green, reset, bold, pkgName)
	} else {
		fmt.Fprintf(w, "%s[-]%s %s%s%s", grey, reset, grey, pkgName, reset)
	}
	if p.Description != "" {
		if installed {
			fmt.Fprintf(w, "%s — %s%s", grey, p.Description, reset)
		} else {
			fmt.Fprintf(w, "%s — %s%s%s", grey, p.Description, reset, grey)
		}
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "    %-18s %s\n", "type:", p.Type)
	fmt.Fprintf(w, "    %-18s %s\n", "category:", p.Category)
	if installed {
		ver := st.Version(pkgName)
		if ver == "" {
			ver = "unknown"
		}
		fmt.Fprintf(w, "    %-18s %s%s (v%s)%s\n", "status:", green, "installed", ver, reset)
	} else {
		fmt.Fprintf(w, "    %-18s %s%s%s\n", "status:", grey, "not installed", reset)
	}
	if len(p.Depends) > 0 {
		fmt.Fprintf(w, "    %-18s %s\n", "depends:", strings.Join(p.Depends, ", "))
	}

	switch p.Type {
	case pkg.TypeApt:
		apt := p.Apt
		hasAptInfo := apt != nil && (len(apt.Extrepo) > 0 || len(apt.Backports) > 0 || apt.BackportSuite != "" || len(apt.Conflicts) > 0 || len(apt.Variants) > 0)
		if hasAptInfo || len(p.Packages) > 0 {
			fmt.Fprintf(w, "\n%s[%s]%s apt\n", bold+blue, "i", reset)
			if apt != nil {
				if len(apt.Extrepo) > 0 {
					fmt.Fprintf(w, "    %-18s %s\n", "extrepo:", strings.Join(apt.Extrepo, ", "))
				}
				if len(apt.Backports) > 0 {
					fmt.Fprintf(w, "    %-18s %s\n", "backports:", strings.Join(apt.Backports, ", "))
				}
				if apt.BackportSuite != "" {
					fmt.Fprintf(w, "    %-18s %s\n", "backport_suite:", apt.BackportSuite)
				}
				if len(apt.Conflicts) > 0 {
					fmt.Fprintf(w, "    %-18s %s\n", "conflicts:", strings.Join(apt.Conflicts, ", "))
				}
				if len(apt.Variants) > 0 {
					names := make([]string, 0, len(apt.Variants))
					for n := range apt.Variants {
						names = append(names, n)
					}
					sort.Strings(names)
					fmt.Fprintf(w, "    %-18s %s\n", "variants:", strings.Join(names, ", "))
					if verbose && apt.Variant != "" {
						fmt.Fprintf(w, "    %-18s %s\n", "selected variant:", apt.Variant)
					}
				}
			}
			if len(p.Packages) > 0 {
				fmt.Fprintf(w, "    %-18s %s\n", "packages:", strings.Join(p.Packages, ", "))
			}
		}

	case pkg.TypeDeb:
		if p.Deb != nil {
			fmt.Fprintf(w, "\n%s[%s]%s deb\n", bold+blue, "i", reset)
			if p.Deb.Package != "" {
				fmt.Fprintf(w, "    %-18s %s\n", "package:", p.Deb.Package)
			}
			for i, u := range p.URLs {
				label := "url:"
				if i > 0 {
					label = ""
				}
				fmt.Fprintf(w, "    %-18s %s\n", label, u)
			}
			if verbose && len(p.SHA256s) > 0 {
				for i, s := range p.SHA256s {
					label := "sha256:"
					if i > 0 {
						label = ""
					}
					fmt.Fprintf(w, "    %-18s %s\n", label, s)
				}
			}
			if p.Repo != "" {
				fmt.Fprintf(w, "    %-18s %s\n", "repo:", p.Repo)
			}
			if verbose && p.VersionCmd != "" {
				fmt.Fprintf(w, "    %-18s %s\n", "version_cmd:", p.VersionCmd)
			}
			if verbose && p.TagPrefix != "" {
				fmt.Fprintf(w, "    %-18s %s\n", "tag_prefix:", p.TagPrefix)
			}
		}

	case pkg.TypeSource:
		if p.Source != nil {
			fmt.Fprintf(w, "\n%s[%s]%s source\n", bold+blue, "i", reset)
			src := p.Source
			if p.Repo != "" {
				fmt.Fprintf(w, "    %-18s %s\n", "repo:", p.Repo)
			}
			for i, u := range p.URLs {
				label := "url:"
				if i > 0 {
					label = ""
				}
				fmt.Fprintf(w, "    %-18s %s\n", label, u)
			}
			if verbose && len(p.SHA256s) > 0 {
				for i, s := range p.SHA256s {
					label := "sha256:"
					if i > 0 {
						label = ""
					}
					fmt.Fprintf(w, "    %-18s %s\n", label, s)
				}
			}
			if verbose && p.VersionCmd != "" {
				fmt.Fprintf(w, "    %-18s %s\n", "version_cmd:", p.VersionCmd)
			}
			if verbose && p.TagPrefix != "" {
				fmt.Fprintf(w, "    %-18s %s\n", "tag_prefix:", p.TagPrefix)
			}
			if verbose && src.SourceSubdir != "" {
				fmt.Fprintf(w, "    %-18s %s\n", "source_subdir:", src.SourceSubdir)
			}
			if verbose && src.SkipClone {
				fmt.Fprintf(w, "    %-18s true\n", "skip_clone:")
			}
			if len(p.Packages) > 0 {
				fmt.Fprintf(w, "    %-18s %s\n", "packages:", strings.Join(p.Packages, ", "))
			}
		}

	case pkg.TypeConfig:
		if len(p.Configs) > 0 || len(p.UserConfigs) > 0 {
			fmt.Fprintf(w, "\n%s[%s]%s config\n", bold+blue, "i", reset)
			if len(p.Configs) > 0 {
				paths := sortedMapKeys(p.Configs)
				if verbose {
					for _, path := range paths {
						fmt.Fprintf(w, "    %-18s %s\n", "config:", path)
						for _, line := range textutil.SplitLines(p.Configs[path]) {
							fmt.Fprintf(w, "      %s\n", line)
						}
					}
				} else {
					fmt.Fprintf(w, "    %-18s %s\n", "configs:", strings.Join(paths, ", "))
				}
			}
			if len(p.UserConfigs) > 0 {
				paths := sortedMapKeys(p.UserConfigs)
				if verbose {
					for _, path := range paths {
						fmt.Fprintf(w, "    %-18s %s\n", "user_config:", path)
						for _, line := range textutil.SplitLines(p.UserConfigs[path]) {
							fmt.Fprintf(w, "      %s\n", line)
						}
					}
				} else {
					fmt.Fprintf(w, "    %-18s %s\n", "user_configs:", strings.Join(paths, ", "))
				}
			}
		}
	}

	if len(p.Remove) > 0 || len(p.RemoveConfigs) > 0 {
		fmt.Fprintf(w, "\n%s[%s]%s remove\n", bold+blue, "i", reset)
		if len(p.Remove) > 0 {
			fmt.Fprintf(w, "    %-18s %s\n", "packages:", strings.Join(p.Remove, ", "))
		}
		if len(p.RemoveConfigs) > 0 {
			paths := sortedMapKeys(p.RemoveConfigs)
			if verbose {
				for _, path := range paths {
					fmt.Fprintf(w, "    %-18s %s\n", "config:", path)
					for _, line := range textutil.SplitLines(p.RemoveConfigs[path]) {
						fmt.Fprintf(w, "      %s\n", line)
					}
				}
			} else {
				fmt.Fprintf(w, "    %-18s %s\n", "configs:", strings.Join(paths, ", "))
			}
		}
	}

	var scripts []scriptEntry
	if p.PreInstall != "" {
		scripts = append(scripts, scriptEntry{"pre_install", p.PreInstall})
	}
	if p.PostInstall != "" {
		scripts = append(scripts, scriptEntry{"post_install", p.PostInstall})
	}
	if p.PostRemove != "" {
		scripts = append(scripts, scriptEntry{"post_remove", p.PostRemove})
	}
	if p.Source != nil {
		if p.Source.BuildScript != "" {
			scripts = append(scripts, scriptEntry{"build", p.Source.BuildScript})
		}
		if p.Source.InstallScript != "" {
			scripts = append(scripts, scriptEntry{"install", p.Source.InstallScript})
		}
		if p.Source.RemoveScript != "" {
			scripts = append(scripts, scriptEntry{"remove", p.Source.RemoveScript})
		}
		if p.Source.PostinstallScript != "" {
			scripts = append(scripts, scriptEntry{"postinstall", p.Source.PostinstallScript})
		}
	}
	if len(scripts) > 0 {
		fmt.Fprintf(w, "\n%s[%s]%s scripts\n", bold+blue, "i", reset)
		for _, s := range scripts {
			lines := strings.Count(s.content, "\n")
			if len(s.content) > 0 && !strings.HasSuffix(s.content, "\n") {
				lines++
			}
			if verbose {
				fmt.Fprintf(w, "    %s\n", s.name)
				for _, line := range textutil.SplitLines(s.content) {
					fmt.Fprintf(w, "      %s\n", line)
				}
			} else {
				if lines == 0 {
					fmt.Fprintf(w, "    %s\t(empty)\n", s.name)
				} else {
					fmt.Fprintf(w, "    %s\t(%d line%s)\n", s.name, lines, pluralS(lines))
				}
			}
		}
	}

	if p.Type != pkg.TypeConfig && (len(p.Configs) > 0 || len(p.UserConfigs) > 0) {
		fmt.Fprintf(w, "\n%s[%s]%s configs\n", bold+blue, "i", reset)
		if len(p.Configs) > 0 {
			paths := sortedMapKeys(p.Configs)
			if verbose {
				for _, path := range paths {
					fmt.Fprintf(w, "    %s\n", path)
					for _, line := range textutil.SplitLines(p.Configs[path]) {
						fmt.Fprintf(w, "      %s\n", line)
					}
				}
			} else {
				for _, path := range paths {
					fmt.Fprintf(w, "    %s\n", path)
				}
			}
		}
		if len(p.UserConfigs) > 0 {
			paths := sortedMapKeys(p.UserConfigs)
			if verbose {
				for _, path := range paths {
					fmt.Fprintf(w, "    %s\n", path)
					for _, line := range textutil.SplitLines(p.UserConfigs[path]) {
						fmt.Fprintf(w, "      %s\n", line)
					}
				}
			} else {
				for _, path := range paths {
					fmt.Fprintf(w, "    %s\n", path)
				}
			}
		}
	}

	w.Flush()
	return buf.String()
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
