package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestUsage(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	usage()

	w.Close()
	os.Stdout = old

	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read: %v", err)
	}
	output := buf.String()

	if len(output) == 0 {
		t.Error("usage() output is empty")
	}
	if !strings.Contains(output, "debforge") {
		t.Errorf("usage() output does not contain %q: %q", "debforge", output)
	}
}
