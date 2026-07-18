package format

import "github.com/hmwassim/debforge/internal/service"

// StateView provides read-only access to installation state. The format
// package uses this interface instead of *service.State so that
// presentation logic can be tested with a lightweight fake.
type StateView interface {
	IsInstalled(name string) bool
	Version(name string) string
	CountInstalled(names []string) int
}

// stateView is a read-only adapter over *service.State.
type stateView struct {
	st *service.State
}

// NewStateView returns a StateView backed by st.
func NewStateView(st *service.State) StateView {
	return &stateView{st: st}
}

func (v *stateView) IsInstalled(name string) bool {
	_, ok := v.st.Packages[name]
	return ok
}

func (v *stateView) Version(name string) string {
	e, ok := v.st.Packages[name]
	if !ok {
		return ""
	}
	return e.Version
}

func (v *stateView) CountInstalled(names []string) int {
	n := 0
	for _, name := range names {
		if _, ok := v.st.Packages[name]; ok {
			n++
		}
	}
	return n
}
