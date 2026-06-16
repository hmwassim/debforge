package repo

import (
	"os"
	"testing"
)

func TestNeedDownload(t *testing.T) {
	t.Run("missing path", func(t *testing.T) {
		if !needDownload("/nonexistent/path") {
			t.Error("needDownload on missing path = false, want true")
		}
	})

	t.Run("empty file", func(t *testing.T) {
		f := t.TempDir() + "/empty"
		if err := os.WriteFile(f, []byte{}, 0644); err != nil {
			t.Fatal(err)
		}
		if !needDownload(f) {
			t.Error("needDownload on empty file = false, want true")
		}
	})

	t.Run("non-empty file", func(t *testing.T) {
		f := t.TempDir() + "/data"
		if err := os.WriteFile(f, []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
		if needDownload(f) {
			t.Error("needDownload on non-empty file = true, want false")
		}
	})
}
