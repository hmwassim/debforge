package utils

import (
	"bytes"
	"strings"
	"testing"
)

func TestReadAllWithLimitUnder(t *testing.T) {
	data, err := ReadAllWithLimit(bytes.NewReader([]byte("hello")), 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("got %q, want %q", string(data), "hello")
	}
}

func TestReadAllWithLimitExact(t *testing.T) {
	input := make([]byte, 100)
	data, err := ReadAllWithLimit(bytes.NewReader(input), 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != 100 {
		t.Fatalf("got len %d, want 100", len(data))
	}
}

func TestReadAllWithLimitExceeded(t *testing.T) {
	input := make([]byte, 101)
	_, err := ReadAllWithLimit(bytes.NewReader(input), 100)
	if err == nil {
		t.Fatal("expected error for oversized input")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected limit error, got: %v", err)
	}
}

func TestReadAllWithLimitEmpty(t *testing.T) {
	data, err := ReadAllWithLimit(bytes.NewReader(nil), 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("got len %d, want 0", len(data))
	}
}
