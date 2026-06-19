package services

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type mockHTTPClient struct {
	statusCode int
	body       []byte
	etag       string
	err        error
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	resp := &http.Response{
		StatusCode: m.statusCode,
		Body:       io.NopCloser(bytes.NewReader(m.body)),
		Header:     make(http.Header),
	}
	if m.etag != "" {
		resp.Header.Set("ETag", m.etag)
	}
	return resp, nil
}

func makeTarGz(files map[string]string) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for name, content := range files {
		tw.WriteHeader(&tar.Header{
			Name:     name,
			Size:     int64(len(content)),
			Mode:     0644,
			Typeflag: tar.TypeReg,
		})
		tw.Write([]byte(content))
	}
	tw.Close()
	gw.Close()
	return buf.Bytes()
}

func TestFontInstallerMetaPath(t *testing.T) {
	got := metaPath("/some/cache/fonts.tar.gz")
	want := "/some/cache/fonts.tar.gz.meta"
	if got != want {
		t.Fatalf("metaPath(%q) = %q, want %q", "/some/cache/fonts.tar.gz", got, want)
	}
}

func TestFontInstallerHashFile(t *testing.T) {
	fs := newMemFS()
	content := []byte("font data")
	fs.WriteFile("/cache/fonts.tar.gz", content, 0644)

	f := &FontInstaller{fs: fs}
	sum, err := f.hashFile("/cache/fonts.tar.gz")
	if err != nil {
		t.Fatalf("hashFile: %v", err)
	}
	h := sha256.Sum256(content)
	want := hex.EncodeToString(h[:])
	if sum != want {
		t.Fatalf("hashFile = %q, want %q", sum, want)
	}
}

func TestFontInstallerHashFileMissing(t *testing.T) {
	f := &FontInstaller{fs: newMemFS()}
	_, err := f.hashFile("/nonexistent")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestFontInstallerHeadETag(t *testing.T) {
	http := &mockHTTPClient{statusCode: http.StatusOK, etag: `"abc123"`}
	f := &FontInstaller{http: http}

	etag, err := f.headETag(context.Background(), "https://example.com/fonts.tar.gz")
	if err != nil {
		t.Fatalf("headETag: %v", err)
	}
	if etag != `"abc123"` {
		t.Fatalf("headETag = %q, want %q", etag, `"abc123"`)
	}
}

func TestFontInstallerHeadETagError(t *testing.T) {
	http := &mockHTTPClient{err: fmt.Errorf("network error")}
	f := &FontInstaller{http: http}

	_, err := f.headETag(context.Background(), "https://example.com/fonts.tar.gz")
	if err == nil {
		t.Fatal("expected error for failed HEAD request")
	}
}

func TestFontInstallerSaveFontMeta(t *testing.T) {
	fs := newMemFS()
	content := []byte("some font data")
	fs.WriteFile("/cache/fonts.tar.gz", content, 0644)

	f := &FontInstaller{fs: fs}
	if err := f.saveFontMeta("/cache/fonts.tar.gz", `"etag123"`); err != nil {
		t.Fatalf("saveFontMeta: %v", err)
	}

	metaPath := "/cache/fonts.tar.gz.meta"
	data, err := fs.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("meta file not written: %v", err)
	}
	var meta fontCacheMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("invalid meta JSON: %v", err)
	}
	h := sha256.Sum256(content)
	wantSum := hex.EncodeToString(h[:])
	if meta.SHA256 != wantSum {
		t.Fatalf("meta SHA256 = %q, want %q", meta.SHA256, wantSum)
	}
	if meta.ETag != `"etag123"` {
		t.Fatalf("meta ETag = %q, want %q", meta.ETag, `"etag123"`)
	}
}

func TestFontInstallerSaveFontMetaEmptyETag(t *testing.T) {
	fs := newMemFS()
	fs.WriteFile("/cache/fonts.tar.gz", []byte("data"), 0644)

	f := &FontInstaller{fs: fs}
	if err := f.saveFontMeta("/cache/fonts.tar.gz", ""); err != nil {
		t.Fatalf("saveFontMeta: %v", err)
	}
	data, _ := fs.ReadFile("/cache/fonts.tar.gz.meta")
	var meta fontCacheMeta
	json.Unmarshal(data, &meta)
	if meta.ETag != "" {
		t.Fatalf("expected empty ETag, got %q", meta.ETag)
	}
}

func TestFontInstallerCacheIsFresh(t *testing.T) {
	fs := newMemFS()
	content := []byte("font data")
	fs.WriteFile("/cache/fonts.tar.gz", content, 0644)
	h := sha256.Sum256(content)
	meta := fontCacheMeta{SHA256: hex.EncodeToString(h[:])}
	metaData, _ := json.Marshal(meta)
	fs.WriteFile("/cache/fonts.tar.gz.meta", metaData, 0644)

	f := &FontInstaller{fs: fs}
	fresh, err := f.cacheIsFresh("/cache/fonts.tar.gz")
	if err != nil {
		t.Fatalf("cacheIsFresh: %v", err)
	}
	if !fresh {
		t.Fatal("expected cache to be fresh")
	}
}

func TestFontInstallerCacheIsStale(t *testing.T) {
	fs := newMemFS()
	content := []byte("font data")
	fs.WriteFile("/cache/fonts.tar.gz", content, 0644)
	meta := fontCacheMeta{SHA256: "wronghash"}
	metaData, _ := json.Marshal(meta)
	fs.WriteFile("/cache/fonts.tar.gz.meta", metaData, 0644)

	f := &FontInstaller{fs: fs}
	fresh, err := f.cacheIsFresh("/cache/fonts.tar.gz")
	if err != nil {
		t.Fatalf("cacheIsFresh: %v", err)
	}
	if fresh {
		t.Fatal("expected cache to be stale")
	}
}

func TestFontInstallerCacheIsFreshNoMeta(t *testing.T) {
	f := &FontInstaller{fs: newMemFS()}
	_, err := f.cacheIsFresh("/nonexistent")
	if err == nil {
		t.Fatal("expected error for missing meta")
	}
}

func TestFontInstallerRemoveCache(t *testing.T) {
	fs := newMemFS()
	fs.WriteFile("/cache/fonts.tar.gz", []byte("data"), 0644)
	fs.WriteFile("/cache/fonts.tar.gz.meta", []byte("{}"), 0644)

	f := &FontInstaller{fs: fs, logger: &mockUI{}}
	f.removeCache("/cache/fonts.tar.gz")

	if _, err := fs.ReadFile("/cache/fonts.tar.gz"); err == nil {
		t.Fatal("expected cache file to be removed")
	}
	if _, err := fs.ReadFile("/cache/fonts.tar.gz.meta"); err == nil {
		t.Fatal("expected meta file to be removed")
	}
}

func TestFontInstallerDownloadFile(t *testing.T) {
	fs := newMemFS()
	http := &mockHTTPClient{statusCode: http.StatusOK, body: []byte("downloaded data")}
	dest := "/tmp/fonts.tar.gz"

	f := &FontInstaller{fs: fs, http: http}
	if err := f.downloadFile(context.Background(), "https://example.com/fonts.tar.gz", dest); err != nil {
		t.Fatalf("downloadFile: %v", err)
	}

	data, err := fs.ReadFile(dest)
	if err != nil {
		t.Fatalf("dest not written: %v", err)
	}
	if string(data) != "downloaded data" {
		t.Fatalf("got %q, want %q", string(data), "downloaded data")
	}
}

func TestFontInstallerDownloadFileHTTPError(t *testing.T) {
	fs := newMemFS()
	http := &mockHTTPClient{statusCode: http.StatusNotFound, body: []byte("not found")}
	f := &FontInstaller{fs: fs, http: http}
	err := f.downloadFile(context.Background(), "https://example.com/fonts.tar.gz", "/tmp/fonts.tar.gz")
	if err == nil {
		t.Fatal("expected error for HTTP 404")
	}
}

func TestFontInstallerExtractTarGz(t *testing.T) {
	fs := newMemFS()
	tarData := makeTarGz(map[string]string{
		"font1.ttf":     "font1 content",
		"sub/font2.ttf": "font2 content",
	})
	fs.WriteFile("/cache/fonts.tar.gz", tarData, 0644)

	f := &FontInstaller{fs: fs}
	if err := f.extractTarGz("/cache/fonts.tar.gz", "/fonts"); err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}

	data, err := fs.ReadFile("/fonts/font1.ttf")
	if err != nil {
		t.Fatalf("font1 not extracted: %v", err)
	}
	if string(data) != "font1 content" {
		t.Fatalf("font1 content = %q, want %q", string(data), "font1 content")
	}

	data, err = fs.ReadFile("/fonts/sub/font2.ttf")
	if err != nil {
		t.Fatalf("font2 not extracted: %v", err)
	}
	if string(data) != "font2 content" {
		t.Fatalf("font2 content = %q, want %q", string(data), "font2 content")
	}
}

func TestFontInstallerExtractTarGzInvalid(t *testing.T) {
	fs := newMemFS()
	fs.WriteFile("/cache/bad.tar.gz", []byte("not a tar"), 0644)

	f := &FontInstaller{fs: fs}
	err := f.extractTarGz("/cache/bad.tar.gz", "/fonts")
	if err == nil {
		t.Fatal("expected error for invalid tar.gz")
	}
}

func TestFontInstallerExtractTarGzPathTraversal(t *testing.T) {
	fs := newMemFS()
	tarData := makeTarGz(map[string]string{
		"../../../etc/passwd": "evil",
	})
	fs.WriteFile("/cache/fonts.tar.gz", tarData, 0644)

	f := &FontInstaller{fs: fs}
	err := f.extractTarGz("/cache/fonts.tar.gz", "/fonts")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { return len(p), nil }

func TestFontInstallerExtractTarGzEntryTooLarge(t *testing.T) {
	old := tarEntryLimit
	tarEntryLimit = 100
	defer func() { tarEntryLimit = old }()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	hdr := &tar.Header{
		Name:     "large.ttf",
		Size:     150,
		Mode:     0644,
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := io.CopyN(tw, zeroReader{}, 150); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gw.Close()

	fs := newMemFS()
	fs.WriteFile("/cache/big.tar.gz", buf.Bytes(), 0644)

	f := &FontInstaller{fs: fs}
	err := f.extractTarGz("/cache/big.tar.gz", "/fonts")
	if err == nil {
		t.Fatal("expected error for oversized tar entry")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected limit error, got: %v", err)
	}
}

func TestFontInstallerExtractTarGzMissingFile(t *testing.T) {
	f := &FontInstaller{fs: newMemFS()}
	err := f.extractTarGz("/nonexistent.tar.gz", "/fonts")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestFontInstallerInstallFreshDownload(t *testing.T) {
	fs := newMemFS()
	tarData := makeTarGz(map[string]string{"font.ttf": "font"})
	http := &mockHTTPClient{statusCode: http.StatusOK, body: tarData, etag: `"v1"`}
	runner := &mockRunner{}
	logger := &mockUI{}

	f := NewFontInstaller(fs, http, runner, logger, "/cache", "/fonts", "https://example.com/fonts.tar.gz")
	if err := f.Install(context.Background()); err != nil {
		t.Fatalf("Install: %v", err)
	}

	if _, err := fs.ReadFile("/fonts/font.ttf"); err != nil {
		t.Fatal("expected font to be extracted")
	}
	if _, err := fs.ReadFile("/cache/fonts.tar.gz.meta"); err != nil {
		t.Fatal("expected meta file")
	}
}

func TestFontInstallerInstallCachedFresh(t *testing.T) {
	fs := newMemFS()
	tarData := makeTarGz(map[string]string{"font.ttf": "font"})
	fs.WriteFile("/cache/fonts.tar.gz", tarData, 0644)
	h := sha256.Sum256(tarData)
	meta := fontCacheMeta{SHA256: hex.EncodeToString(h[:])}
	metaData, _ := json.Marshal(meta)
	fs.WriteFile("/cache/fonts.tar.gz.meta", metaData, 0644)

	runner := &mockRunner{}
	logger := &mockUI{}

	f := NewFontInstaller(fs, &mockHTTPClient{}, runner, logger, "/cache", "/fonts", "")
	if err := f.Install(context.Background()); err != nil {
		t.Fatalf("Install: %v", err)
	}
}

func TestFontInstallerInstallCachedCorrupt(t *testing.T) {
	fs := newMemFS()
	fs.WriteFile("/cache/fonts.tar.gz", []byte("corrupt data"), 0644)
	h := sha256.Sum256([]byte("corrupt data"))
	meta := fontCacheMeta{SHA256: hex.EncodeToString(h[:])}
	metaData, _ := json.Marshal(meta)
	fs.WriteFile("/cache/fonts.tar.gz.meta", metaData, 0644)

	http := &mockHTTPClient{statusCode: http.StatusOK, body: makeTarGz(map[string]string{"font.ttf": "new font"})}
	runner := &mockRunner{}
	logger := &mockUI{}

	f := NewFontInstaller(fs, http, runner, logger, "/cache", "/fonts", "https://example.com/fonts.tar.gz")
	if err := f.Install(context.Background()); err != nil {
		t.Fatalf("Install: %v", err)
	}
}

func TestFontInstallerInstallDownloadFails(t *testing.T) {
	fs := newMemFS()
	http := &mockHTTPClient{err: fmt.Errorf("connection refused")}
	logger := &mockUI{}

	f := NewFontInstaller(fs, http, &mockRunner{}, logger, "/cache", "/fonts", "https://example.com/fonts.tar.gz")
	err := f.Install(context.Background())
	if err == nil {
		t.Fatal("expected error for download failure")
	}
	if !strings.Contains(err.Error(), "downloading fonts") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFontInstallerInstallDownloadBadStatus(t *testing.T) {
	fs := newMemFS()
	http := &mockHTTPClient{statusCode: http.StatusNotFound, body: []byte("not found")}
	logger := &mockUI{}

	f := NewFontInstaller(fs, http, &mockRunner{}, logger, "/cache", "/fonts", "https://example.com/fonts.tar.gz")
	err := f.Install(context.Background())
	if err == nil {
		t.Fatal("expected error for HTTP error")
	}
}
