package aptpty

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
)

var progressRe = regexp.MustCompile(
	`([0-9]+)% \[([0-9]+) ([^ ]+) ([0-9.,]+) ([A-Za-z]+)/([0-9.,]+) ([A-Za-z]+) [0-9]+%\]`,
)

func parseSize(s, unit string) int64 {
	v, _ := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	switch strings.ToLower(unit) {
	case "kb", "kib":
		return int64(v * 1000)
	case "mb", "mib":
		return int64(v * 1000000)
	case "gb", "gib":
		return int64(v * 1000000000)
	default:
		return int64(v)
	}
}

func parseProgress(line string) (cur, total int64, pkg string, ok bool) {
	m := progressRe.FindStringSubmatch(line)
	if m == nil {
		return 0, 0, "", false
	}
	pkg = m[3]
	cur = parseSize(m[4], m[5])
	total = parseSize(m[6], m[7])
	return cur, total, pkg, true
}

func stripANSI(s string) string {
	var b bytes.Buffer
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && !(s[i] >= 0x40 && s[i] <= 0x7E) {
				i++
			}
			if i < len(s) {
				i++
			}
			i--
		} else {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}
