package aptpty

import "testing"

func BenchmarkParseProgress(b *testing.B) {
	line := " 42% [1 hello 52.0 kB/123 kB 42%]"
	var sink1, sink2 int64
	var sink3 string
	var sink4 bool
	for b.Loop() {
		sink1, sink2, sink3, sink4 = parseProgress(line)
	}
	_, _, _, _ = sink1, sink2, sink3, sink4
}

func BenchmarkParseSize(b *testing.B) {
	for _, tc := range []struct{ s, unit string }{
		{"52.0", "kB"},
		{"1.5", "MB"},
		{"2.0", "GB"},
	} {
		b.Run(tc.s+tc.unit, func(b *testing.B) {
			var sink int64
			for b.Loop() {
				sink = parseSize(tc.s, tc.unit)
			}
			_ = sink
		})
	}
}

func BenchmarkStripANSI(b *testing.B) {
	line := "\033[1;32mReading package lists...\033[0m Done"
	var sink string
	for b.Loop() {
		sink = stripANSI(line)
	}
	_ = sink
}

func BenchmarkStripANSI_noEscape(b *testing.B) {
	line := "Setting up hello (2.36-9) ..."
	var sink string
	for b.Loop() {
		sink = stripANSI(line)
	}
	_ = sink
}
