package core

import (
	"os"
	"testing"
)

func TestVerifySetup_EmptyPlan(t *testing.T) {
	if !verifySetup(plan{}) {
		t.Error("verifySetup({}) = false, want true")
	}
}

func TestVerifySetup_ConfigContentMatch(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.conf"
	content := "test config"
	mode := os.FileMode(0644)
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}

	p := plan{
		configs: []configCheck{
			{dest: path, content: content, mode: mode},
		},
	}
	if !verifySetup(p) {
		t.Error("verifySetup with matching file = false, want true")
	}
}

func TestVerifySetup_ConfigModeMismatch(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.conf"
	content := "test config"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	p := plan{
		configs: []configCheck{
			{dest: path, content: content, mode: 0600},
		},
	}
	if verifySetup(p) {
		t.Error("verifySetup with wrong mode = true, want false")
	}
}

func TestVerifySetup_ConfigContentMismatch(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.conf"
	mode := os.FileMode(0644)
	if err := os.WriteFile(path, []byte("actual content"), mode); err != nil {
		t.Fatal(err)
	}

	p := plan{
		configs: []configCheck{
			{dest: path, content: "expected content", mode: mode},
		},
	}
	if verifySetup(p) {
		t.Error("verifySetup with wrong content = true, want false")
	}
}

func TestVerifySetup_ConfigFileMissing(t *testing.T) {
	p := plan{
		configs: []configCheck{
			{dest: "/nonexistent/path.conf", content: "x", mode: 0644},
		},
	}
	if verifySetup(p) {
		t.Error("verifySetup with missing config = true, want false")
	}
}
