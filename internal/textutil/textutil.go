package textutil

import (
	"strconv"
	"unicode"
)

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

func UcFirst(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
