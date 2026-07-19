package definition

import (
	"testing"

	"github.com/hmwassim/debforge/internal/testutil"
)

var benchAptYAML = []byte(`
name: bench-pkg
type: apt
description: Benchmark package for testing
depends:
  - dep-a
  - dep-b
install:
  packages:
    - hello
    - world
  extrepo:
    - nonfree
  backports:
    - trixie-backports
  variants:
    desktop:
      - extra-pkg
remove:
  packages:
    - old-pkg
`)

var benchDebYAML = []byte(`
name: bench-deb
type: deb
description: Benchmark deb package
package: bench-deb-pkg
install:
  url: https://example.com/bench-deb-1.0.deb
  sha256: abc123def456
`)

var benchSourceYAML = []byte(`
name: bench-source
type: source
description: Benchmark source package
depends:
  - build-essential
  - cmake
install:
  repo: https://github.com/user/repo
  tag_prefix: v
  build_script: |
    mkdir build && cd build
    cmake .. && make -j$(nproc)
  install_script: make install
  postinstall: echo "installed"
  remove: make uninstall
`)

func BenchmarkParse_apt(b *testing.B) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/repo/packages/apt/bench.yaml"] = benchAptYAML
	var sink interface{}
	for b.Loop() {
		sink, _ = Parse("/repo/packages/apt/bench.yaml", fs)
	}
	_ = sink
}

func BenchmarkParse_deb(b *testing.B) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/repo/packages/deb/bench.yaml"] = benchDebYAML
	var sink interface{}
	for b.Loop() {
		sink, _ = Parse("/repo/packages/deb/bench.yaml", fs)
	}
	_ = sink
}

func BenchmarkParse_source(b *testing.B) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/repo/packages/source/bench.yaml"] = benchSourceYAML
	var sink interface{}
	for b.Loop() {
		sink, _ = Parse("/repo/packages/source/bench.yaml", fs)
	}
	_ = sink
}
