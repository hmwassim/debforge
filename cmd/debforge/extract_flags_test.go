package main

import (
	"reflect"
	"testing"
)

func TestExtractFlags(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
		wantY bool
		wantF bool
		wantA bool
		wantV bool
	}{
		{
			name:  "no flags",
			input: []string{"firefox", "vim"},
			want:  []string{"firefox", "vim"},
		},
		{
			name:  "short yes",
			input: []string{"-y", "firefox"},
			want:  []string{"firefox"},
			wantY: true,
		},
		{
			name:  "long yes",
			input: []string{"--yes", "firefox"},
			want:  []string{"firefox"},
			wantY: true,
		},
		{
			name:  "short force",
			input: []string{"-f", "firefox"},
			want:  []string{"firefox"},
			wantF: true,
		},
		{
			name:  "long force",
			input: []string{"--force", "firefox"},
			want:  []string{"firefox"},
			wantF: true,
		},
		{
			name:  "short all",
			input: []string{"-a", "firefox"},
			want:  []string{"firefox"},
			wantA: true,
		},
		{
			name:  "long all",
			input: []string{"--all", "firefox"},
			want:  []string{"firefox"},
			wantA: true,
		},
		{
			name:  "multiple flags combined",
			input: []string{"-y", "-f", "firefox", "-a"},
			want:  []string{"firefox"},
			wantY: true,
			wantF: true,
			wantA: true,
		},
		{
			name:  "flag after positional arg",
			input: []string{"firefox", "-y"},
			want:  []string{"firefox"},
			wantY: true,
		},
		{
			name:  "flag before positional arg",
			input: []string{"-y", "firefox"},
			want:  []string{"firefox"},
			wantY: true,
		},
		{
			name:  "flags interspersed",
			input: []string{"firefox", "-y", "vscodium"},
			want:  []string{"firefox", "vscodium"},
			wantY: true,
		},
		{
			name:  "combined short flags",
			input: []string{"-yf", "firefox"},
			want:  []string{"firefox"},
			wantY: true,
			wantF: true,
		},
		{
			name:  "combined short flags all three",
			input: []string{"-yfa", "firefox"},
			want:  []string{"firefox"},
			wantY: true,
			wantF: true,
			wantA: true,
		},
		{
			name:  "unknown short flag passes through",
			input: []string{"-x", "firefox"},
			want:  []string{"-x", "firefox"},
		},
		{
			name:  "mixed known and unknown short flags",
			input: []string{"-yx", "firefox"},
			want:  []string{"-x", "firefox"},
			wantY: true,
		},
		{
			name:  "empty input",
			input: []string{},
			want:  []string{},
		},
		{
			name:  "long self",
			input: []string{"--self", "firefox"},
			want:  []string{"firefox"},
		},
		{
			name:  "unknown flag --selfish",
			input: []string{"--selfish", "firefox"},
			want:  []string{"--selfish", "firefox"},
		},
		{
			name:  "short verbose",
			input: []string{"-v", "firefox"},
			want:  []string{"firefox"},
			wantV: true,
		},
		{
			name:  "long verbose",
			input: []string{"--verbose", "firefox"},
			want:  []string{"firefox"},
			wantV: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var yes, force, all, self, verbose bool
			got := extractFlags(tc.input, &yes, &force, &all, &self, &verbose)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("extractFlags(%v) = %v, want %v", tc.input, got, tc.want)
			}
			if yes != tc.wantY {
				t.Errorf("yes = %v, want %v", yes, tc.wantY)
			}
			if force != tc.wantF {
				t.Errorf("force = %v, want %v", force, tc.wantF)
			}
			if all != tc.wantA {
				t.Errorf("all = %v, want %v", all, tc.wantA)
			}
			if verbose != tc.wantV {
				t.Errorf("verbose = %v, want %v", verbose, tc.wantV)
			}
		})
	}
}
