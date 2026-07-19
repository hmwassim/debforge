package installer

import (
	"testing"

	"github.com/hmwassim/debforge/internal/testutil"
)

func BenchmarkValidateConfigPath(b *testing.B) {
	for _, path := range []string{
		"/etc/my-app.conf",
		"/usr/share/my-app/config.yaml",
		"/opt/my-app/settings.toml",
		"/var/lib/my-app/data.json",
	} {
		b.Run(path, func(b *testing.B) {
			var sink error
			for b.Loop() {
				sink = ValidateConfigPath(path)
			}
			_ = sink
		})
	}
}

func BenchmarkValidateConfigPath_traversal(b *testing.B) {
	var sink error
	for b.Loop() {
		sink = ValidateConfigPath("/etc/../../etc/shadow")
	}
	_ = sink
}

func BenchmarkValidateRemovablePath(b *testing.B) {
	for _, path := range []string{
		"/opt/my-app",
		"/var/lib/my-app/data",
	} {
		b.Run(path, func(b *testing.B) {
			var sink error
			for b.Loop() {
				sink = ValidateRemovablePath(path)
			}
			_ = sink
		})
	}
}

func BenchmarkValidateRemovablePath_dangerous(b *testing.B) {
	var sink error
	for b.Loop() {
		sink = ValidateRemovablePath("/etc")
	}
	_ = sink
}

func BenchmarkDecideConfigAction(b *testing.B) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/my-app.conf"] = []byte("original content")
	for b.Loop() {
		_ = DecideConfigAction(fs, "/etc/my-app.conf", "new content", "hash123", false)
	}
}

func BenchmarkCheckPathTraversal(b *testing.B) {
	var sink error
	for b.Loop() {
		sink = checkPathTraversal("/etc/my-app.conf")
	}
	_ = sink
}
