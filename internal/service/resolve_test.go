package service

import (
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name           string
		packages       []pkg.Package
		resolvePkg     string
		wantErr        bool
		wantErrContains []string
		wantOrder      []string
	}{
		{
			name: "detects cycle",
			packages: []pkg.Package{
				{Name: "a", Type: pkg.TypeApt, Depends: []string{"b"}},
				{Name: "b", Type: pkg.TypeApt, Depends: []string{"a"}},
			},
			resolvePkg:     "a",
			wantErr:        true,
			wantErrContains: []string{"dependency cycle", "a", "b"},
		},
		{
			name: "detects transitive cycle",
			packages: []pkg.Package{
				{Name: "a", Type: pkg.TypeApt, Depends: []string{"b"}},
				{Name: "b", Type: pkg.TypeApt, Depends: []string{"c"}},
				{Name: "c", Type: pkg.TypeApt, Depends: []string{"a"}},
			},
			resolvePkg: "a",
			wantErr:    true,
		},
		{
			name: "detects cycle among non-root deps",
			packages: []pkg.Package{
				{Name: "root", Type: pkg.TypeApt, Depends: []string{"a"}},
				{Name: "a", Type: pkg.TypeApt, Depends: []string{"b"}},
				{Name: "b", Type: pkg.TypeApt, Depends: []string{"a"}},
			},
			resolvePkg:     "root",
			wantErr:        true,
			wantErrContains: []string{"dependency cycle"},
		},
		{
			name: "no cycle for shared dep",
			packages: []pkg.Package{
				{Name: "a", Type: pkg.TypeApt, Depends: []string{"c"}},
				{Name: "b", Type: pkg.TypeApt, Depends: []string{"c"}},
				{Name: "c", Type: pkg.TypeApt},
			},
			resolvePkg: "a",
			wantOrder:  []string{"c", "a"},
		},
		{
			name: "topological order",
			packages: []pkg.Package{
				{Name: "a", Type: pkg.TypeApt, Depends: []string{"b", "c"}},
				{Name: "b", Type: pkg.TypeApt, Depends: []string{"d"}},
				{Name: "c", Type: pkg.TypeApt},
				{Name: "d", Type: pkg.TypeApt},
			},
			resolvePkg: "a",
			wantOrder:  []string{"d", "b", "c", "a"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := pkg.NewRegistry()
			for i := range tc.packages {
				reg.Register(&tc.packages[i])
			}

			r := NewResolver(reg)
			p, _ := reg.Lookup(tc.resolvePkg)
			ordered, err := r.Resolve(p)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				for _, substr := range tc.wantErrContains {
					if !strings.Contains(err.Error(), substr) {
						t.Errorf("expected error containing %q, got: %v", substr, err)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.wantOrder != nil {
				names := make([]string, len(ordered))
				for i, p := range ordered {
					names[i] = p.Name
				}
				if len(names) != len(tc.wantOrder) {
					t.Fatalf("expected %d packages, got %d", len(tc.wantOrder), len(names))
				}
				for i, want := range tc.wantOrder {
					if names[i] != want {
						t.Errorf("ordered[%d] = %q, want %q", i, names[i], want)
					}
				}
			}
		})
	}
}

func indexOf(ss []string, s string) int {
	for i, v := range ss {
		if v == s {
			return i
		}
	}
	return -1
}
