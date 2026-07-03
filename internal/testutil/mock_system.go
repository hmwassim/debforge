package testutil

import (
	"github.com/hmwassim/debforge/internal/ports"
)

// MockSystem is a ports.System test double with configurable behavior.
// Each method has a corresponding Func field (e.g. IsPrivilegedFunc) that,
// when set, is called instead of the default. Convenience value fields
// (Privileged, Env) are used when the corresponding Func is nil.
type MockSystem struct {
	IsPrivilegedFunc func() bool
	GetenvFunc       func(string) string
	UserHomeDirFunc  func() (string, error)
	LookupUserFunc   func(string) (*ports.UserInfo, error)

	// Privileged is returned by IsPrivileged when IsPrivilegedFunc is nil.
	Privileged bool
	// Env is checked by Getenv when GetenvFunc is nil.
	Env map[string]string
}

func (m *MockSystem) IsPrivileged() bool {
	if m.IsPrivilegedFunc != nil {
		return m.IsPrivilegedFunc()
	}
	return m.Privileged
}

func (m *MockSystem) Getenv(key string) string {
	if m.GetenvFunc != nil {
		return m.GetenvFunc(key)
	}
	if m.Env != nil {
		return m.Env[key]
	}
	return ""
}

func (m *MockSystem) UserHomeDir() (string, error) {
	if m.UserHomeDirFunc != nil {
		return m.UserHomeDirFunc()
	}
	return "/home/test", nil
}

func (m *MockSystem) LookupUser(name string) (*ports.UserInfo, error) {
	if m.LookupUserFunc != nil {
		return m.LookupUserFunc(name)
	}
	return &ports.UserInfo{HomeDir: "/home/test", Uid: 1000, Gid: 1000}, nil
}

var _ ports.System = (*MockSystem)(nil)
