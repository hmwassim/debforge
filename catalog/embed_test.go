package catalogdata

import (
	"testing"

	"github.com/hmwassim/debforge/internal/catalog"
	"github.com/hmwassim/debforge/internal/setup"
)

// TestEmbeddedCatalogLoads validates every shipped definition.
func TestEmbeddedCatalogLoads(t *testing.T) {
	c, warns, err := catalog.Load(catalog.Layer{Name: "embedded", FS: FS, PkgDir: "packages", FilesDir: "files"})
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) > 0 {
		t.Fatalf("warnings: %v", warns)
	}
	if n := len(c.All()); n < 40 {
		t.Fatalf("only %d packages loaded", n)
	}
	for _, name := range []string{"firefox", "fonts-nerd-hack", "config-system", "cursor", "discord"} {
		if _, ok := c.Get(name); !ok {
			t.Errorf("missing %s", name)
		}
	}
}

func TestEmbeddedProfileLoads(t *testing.T) {
	if _, err := setup.Load(FS, "default"); err != nil {
		t.Fatal(err)
	}
}
