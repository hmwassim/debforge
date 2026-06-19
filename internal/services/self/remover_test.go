package self

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/hmwassim/debforge/internal/ports"
)

type removerSpySpinner struct {
	calls []string
}

func (s *removerSpySpinner) Done()   { s.calls = append(s.calls, "Done") }
func (s *removerSpySpinner) Fail()   { s.calls = append(s.calls, "Fail") }
func (s *removerSpySpinner) Pause()  { s.calls = append(s.calls, "Pause") }
func (s *removerSpySpinner) Resume() { s.calls = append(s.calls, "Resume") }

type removerSpyUI struct {
	promptResult bool
	spy          *removerSpySpinner
}

func (m *removerSpyUI) Info(format string, args ...any)                        {}
func (m *removerSpyUI) Success(format string, args ...any)                     {}
func (m *removerSpyUI) Warn(format string, args ...any)                        {}
func (m *removerSpyUI) Error(format string, args ...any)                       {}
func (m *removerSpyUI) Muted(format string, args ...any)                       {}
func (m *removerSpyUI) Debug(format string, args ...any)                       {}
func (m *removerSpyUI) Prompt(format string, args ...any) bool                 { return m.promptResult }
func (m *removerSpyUI) PromptInput(format string, args ...any) string          { return "" }
func (m *removerSpyUI) Spinner(ctx context.Context, desc string) ports.Spinner { return m.spy }
func (m *removerSpyUI) Progress(total int64, desc string) ports.Progress       { return nil }

type removerMockLocker struct{}

func (m *removerMockLocker) Acquire(ctx context.Context, path string) (func(), error) {
	return func() {}, nil
}

func TestRemoverRemovePausesSpinnerDuringPrompt(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("skipping: requires root")
	}

	spy := &removerSpySpinner{}
	ui := &removerSpyUI{promptResult: false, spy: spy}

	r := &Remover{logger: ui, locker: &removerMockLocker{}, cfg: testConfig()}
	if err := r.Remove(context.Background(), ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// cancelled at first prompt: Pause(), Done()
	want := []string{"Pause", "Done"}
	if !reflect.DeepEqual(spy.calls, want) {
		t.Errorf("spinner calls = %v, want %v", spy.calls, want)
	}
}

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
