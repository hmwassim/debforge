package system

import (
	"errors"
	"fmt"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

// User is the human account debforge acts on behalf of when it writes
// files into a home directory or runs per-user commands.
type User struct {
	Name string
	UID  int
	GID  int
	Home string
}

// ErrNoTargetUser is returned when debforge runs as plain root with no
// sudo/pkexec/doas context, so there is no sensible home to write to.
var ErrNoTargetUser = errors.New("cannot determine the target user (run debforge through sudo from your user account)")

// Lookup abstracts os/user for tests.
type Lookup struct {
	ByName func(string) (*user.User, error)
	ByID   func(string) (*user.User, error)
}

// DefaultLookup uses os/user.
var DefaultLookup = Lookup{ByName: user.Lookup, ByID: user.LookupId}

// TargetUser resolves the invoking human user from the privilege-escalation
// environment. getenv is usually os.Getenv. uid is the effective uid.
func TargetUser(getenv func(string) string, uid int, lk Lookup) (User, error) {
	if uid != 0 {
		u, err := lk.ByID(strconv.Itoa(uid))
		if err != nil {
			return User{}, fmt.Errorf("look up uid %d: %w", uid, err)
		}
		return toUser(u)
	}
	if n := getenv("SUDO_USER"); n != "" && n != "root" {
		return byName(lk, n)
	}
	if n := getenv("DOAS_USER"); n != "" && n != "root" {
		return byName(lk, n)
	}
	if id := getenv("PKEXEC_UID"); id != "" && id != "0" {
		u, err := lk.ByID(id)
		if err != nil {
			return User{}, fmt.Errorf("look up PKEXEC_UID %s: %w", id, err)
		}
		return toUser(u)
	}
	return User{}, ErrNoTargetUser
}

func byName(lk Lookup, n string) (User, error) {
	u, err := lk.ByName(n)
	if err != nil {
		return User{}, fmt.Errorf("look up user %q: %w", n, err)
	}
	return toUser(u)
}

func toUser(u *user.User) (User, error) {
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return User{}, fmt.Errorf("bad uid %q", u.Uid)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return User{}, fmt.Errorf("bad gid %q", u.Gid)
	}
	if u.HomeDir == "" || !filepath.IsAbs(u.HomeDir) {
		return User{}, fmt.Errorf("user %s has no usable home directory", u.Username)
	}
	return User{Name: u.Username, UID: uid, GID: gid, Home: filepath.Clean(u.HomeDir)}, nil
}

// ExpandHome replaces a leading "~/" with the user's home directory.
func (u User) ExpandHome(p string) string {
	if p == "~" {
		return u.Home
	}
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return filepath.Join(u.Home, rest)
	}
	return p
}
