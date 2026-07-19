// Package textutil provides simple text formatting helpers used across
// the codebase.
package textutil

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// FormatSize formats a byte count as a human-readable string (e.g., "1.5M").
func FormatSize(v int64) string {
	switch {
	case v >= 1000000000:
		return strconv.FormatFloat(float64(v)/1000000000, 'f', 1, 64) + "G"
	case v >= 1000000:
		return strconv.FormatFloat(float64(v)/1000000, 'f', 1, 64) + "M"
	case v >= 1000:
		return strconv.FormatInt(v/1000, 10) + "k"
	default:
		return strconv.FormatInt(v, 10)
	}
}

// UcFirst returns s with its first Unicode character converted to upper case.
func UcFirst(s string) string {
	if s == "" {
		return ""
	}
	r, size := utf8.DecodeRuneInString(s)
	if r == unicode.ToUpper(r) {
		return s
	}
	var buf [utf8.UTFMax]byte
	n := utf8.EncodeRune(buf[:], unicode.ToUpper(r))
	return string(buf[:n]) + s[size:]
}

// ExpandVersion replaces "{version}" in template with version.
// The version string is sanitized to prevent shell injection via
// string interpolation into scripts.
func ExpandVersion(template, version string) string {
	return strings.ReplaceAll(template, "{version}", SanitizeVersion(version))
}

// SanitizeVersion strips shell-unsafe characters from a version string,
// allowing only alphanumeric characters, dots, hyphens, underscores,
// and plus signs. This prevents command injection when version strings
// are interpolated into shell scripts via {version} placeholders.
func SanitizeVersion(v string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' || r == '+' {
			return r
		}
		return -1
	}, v)
}

// Sha256Hex returns the hex-encoded SHA-256 hash of data.
func Sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// SplitLines splits s into lines, stripping a trailing newline if present
// so that an empty trailing element is not produced.
func SplitLines(s string) []string {
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}
