package download

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/hmwassim/debforge/internal/adapters/fs"
	"github.com/hmwassim/debforge/internal/testutil"
)

func withTLSClient(ts *httptest.Server, fn func()) {
	old := httpClient
	httpClient = ts.Client()
	defer func() { httpClient = old }()
	fn()
}

func TestExpandURL(t *testing.T) {
	got := ExpandURL("https://example.com/pkg-{version}.tar.gz", "1.2.3")
	want := "https://example.com/pkg-1.2.3.tar.gz"
	if got != want {
		t.Errorf("ExpandURL = %q, want %q", got, want)
	}

	got = ExpandURL("https://example.com/pkg-{version}.tar.gz", "")
	want = "https://example.com/pkg-.tar.gz"
	if got != want {
		t.Errorf("ExpandURL with empty version = %q, want %q", got, want)
	}
}

func TestFilenameFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://example.com/file.deb", "file.deb"},
		{"https://example.com/path/to/archive.tar.gz", "archive.tar.gz"},
		{"https://example.com/file.deb?query=param", "file.deb"},
		{"https://example.com/file.deb#fragment", "file.deb"},
		{"/local/path/file.deb", "file.deb"},
		{"", "."},
	}
	for _, tc := range tests {
		got := FilenameFromURL(tc.url)
		if got != tc.want {
			t.Errorf("FilenameFromURL(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestDownload(t *testing.T) {
	body := []byte("hello debforge")
	hash := sha256.Sum256(body)
	hashHex := hex.EncodeToString(hash[:])

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer ts.Close()

	dir := t.TempDir()

	dest := filepath.Join(dir, "out.deb")
	fSys := fs.NewFileSystem()

	withTLSClient(ts, func() {
		if err := Download(context.Background(), fSys, ts.URL, dest, nil, hashHex); err != nil {
			t.Fatalf("Download with valid SHA256: %v", err)
		}
	})

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(body) {
		t.Errorf("Download content = %q, want %q", string(data), string(body))
	}
}

func TestDownload_sha256Mismatch(t *testing.T) {
	body := []byte("hello debforge")

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer ts.Close()

	dir := t.TempDir()

	dest := filepath.Join(dir, "out.deb")
	fSys := fs.NewFileSystem()

	badHash := "0000000000000000000000000000000000000000000000000000000000000000"
	withTLSClient(ts, func() {
		if err := Download(context.Background(), fSys, ts.URL, dest, nil, badHash); err == nil {
			t.Fatal("expected SHA256 mismatch error")
		}
	})

	if _, err := os.Stat(dest); err == nil {
		t.Error("expected dest file to be removed after SHA256 mismatch")
	}
}

func TestDownload_emptyFile(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(nil)
	}))
	defer ts.Close()

	dir := t.TempDir()

	dest := filepath.Join(dir, "empty.deb")
	fSys := fs.NewFileSystem()

	withTLSClient(ts, func() {
		if err := Download(context.Background(), fSys, ts.URL, dest, nil, ""); err == nil {
			t.Fatal("expected empty file error")
		}
	})
}

func TestDownload_statusNotOK(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	dir := t.TempDir()

	dest := filepath.Join(dir, "out.deb")
	fSys := fs.NewFileSystem()

	withTLSClient(ts, func() {
		if err := Download(context.Background(), fSys, ts.URL, dest, nil, ""); err == nil {
			t.Fatal("expected non-200 error")
		}
	})
}

func TestProgressReader_reports(t *testing.T) {
	data := []byte("hello world download test")
	pr := &progressReader{
		reader:   bytes.NewReader(data),
		total:    int64(len(data)),
		filename: "test.bin",
		spinner:  &testutil.MockSpinner{},
	}

	buf := make([]byte, 5)
	n, err := pr.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if n != 5 {
		t.Errorf("Read = %d, want 5", n)
	}
	if pr.done != 5 {
		t.Errorf("done = %d, want 5", pr.done)
	}
}

func TestProgressReader_fullRead(t *testing.T) {
	data := []byte("small")
	pr := &progressReader{
		reader:   bytes.NewReader(data),
		total:    int64(len(data)),
		filename: "small.bin",
		spinner:  &testutil.MockSpinner{},
	}

	buf := make([]byte, 64)
	n, err := pr.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if n != len(data) {
		t.Errorf("Read = %d, want %d", n, len(data))
	}
	if pr.done != int64(len(data)) {
		t.Errorf("done = %d, want %d", pr.done, len(data))
	}
}

func TestDownload_rejectsHTTP(t *testing.T) {
	body := []byte("data")
	hash := sha256.Sum256(body)
	hashHex := hex.EncodeToString(hash[:])

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer ts.Close()

	dir := t.TempDir()

	dest := filepath.Join(dir, "out.deb")
	fSys := fs.NewFileSystem()

	if err := Download(context.Background(), fSys, ts.URL, dest, nil, hashHex); err == nil {
		t.Fatal("expected error for non-HTTPS URL")
	}
}
