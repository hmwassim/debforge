package self

import (
	"reflect"
	"testing"
)

func TestParseSelection(t *testing.T) {
	tests := []struct {
		name  string
		input string
		max   int
		want  []int
	}{
		{name: "empty input", input: "", max: 10, want: nil},
		{name: "zero", input: "0", max: 10, want: nil},
		{name: "single", input: "1", max: 10, want: []int{0}},
		{name: "last", input: "10", max: 10, want: []int{9}},
		{name: "comma separated", input: "1,3,5", max: 10, want: []int{0, 2, 4}},
		{name: "range", input: "1-3", max: 10, want: []int{0, 1, 2}},
		{name: "reversed range", input: "3-1", max: 10, want: []int{0, 1, 2}},
		{name: "mixed", input: "1, 3-5, 7", max: 10, want: []int{0, 2, 3, 4, 6}},
		{name: "duplicates deduped", input: "1, 1-2", max: 10, want: []int{0, 1}},
		{name: "out of bounds filtered", input: "0, 1, 11", max: 10, want: []int{0}},
		{name: "non-numeric part ignored", input: "1, abc, 3", max: 10, want: []int{0, 2}},
		{name: "invalid range ignored", input: "1, abc-5, 3", max: 10, want: []int{0, 2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSelection(tt.input, tt.max)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseSelection(%q, %d) = %v, want %v", tt.input, tt.max, got, tt.want)
			}
		})
	}
}
