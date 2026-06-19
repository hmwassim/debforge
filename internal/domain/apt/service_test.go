package apt

import (
	"testing"
)

func TestParseDpkgSelections(t *testing.T) {
	tests := []struct {
		name      string
		out       string
		requested []string
		want      map[string]bool
	}{
		{
			name:      "empty",
			out:       "",
			requested: []string{},
			want:      map[string]bool{},
		},
		{
			name:      "single package installed",
			out:       "curl\tinstall\n",
			requested: []string{"curl"},
			want:      map[string]bool{"curl": true},
		},
		{
			name:      "single package not installed",
			out:       "",
			requested: []string{"nonexistent"},
			want:      map[string]bool{"nonexistent": false},
		},
		{
			name:      "multiple packages mixed",
			out:       "curl\tinstall\ngit\tinstall\n",
			requested: []string{"curl", "git", "vim"},
			want:      map[string]bool{"curl": true, "git": true, "vim": false},
		},
		{
			name:      "architecture-qualified package requested and found",
			out:       "libfoo:i386\tinstall\n",
			requested: []string{"libfoo:i386"},
			want:      map[string]bool{"libfoo:i386": true},
		},
		{
			name:      "unqualified request matches arch-qualified dpkg output",
			out:       "libfoo:amd64\tinstall\n",
			requested: []string{"libfoo"},
			want:      map[string]bool{"libfoo": true},
		},
		{
			name:      "i386 qualified request matches only i386",
			out:       "libfoo:amd64\tinstall\n",
			requested: []string{"libfoo:i386"},
			want:      map[string]bool{"libfoo:i386": false},
		},
		{
			name:      "both architectures installed",
			out:       "libfoo:amd64\tinstall\nlibfoo:i386\tinstall\n",
			requested: []string{"libfoo:amd64", "libfoo:i386"},
			want:      map[string]bool{"libfoo:amd64": true, "libfoo:i386": true},
		},
		{
			name:      "mixed qualified and unqualified in same request",
			out:       "libfoo:amd64\tinstall\nlibfoo:i386\tinstall\n",
			requested: []string{"libfoo", "libfoo:i386"},
			want:      map[string]bool{"libfoo": true, "libfoo:i386": true},
		},
		{
			name:      "package with architecture but not in install state",
			out:       "libfoo:amd64\tdeinstall\n",
			requested: []string{"libfoo"},
			want:      map[string]bool{"libfoo": false},
		},
		{
			name:      "multiple packages with same base different archs",
			out:       "curl\tinstall\n",
			requested: []string{"curl:amd64", "curl:i386"},
			want:      map[string]bool{"curl:amd64": false, "curl:i386": false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseDpkgSelections(tt.out, tt.requested)
			if len(got) != len(tt.want) {
				t.Errorf("got %d packages, want %d", len(got), len(tt.want))
			}
			for pkg, wantInstalled := range tt.want {
				if got[pkg] != wantInstalled {
					t.Errorf("package %q: got installed=%v, want %v", pkg, got[pkg], wantInstalled)
				}
			}
		})
	}
}
