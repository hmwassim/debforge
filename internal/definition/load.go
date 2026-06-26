package definition

import (
	"fmt"
	"strings"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

// LoadAll walks dir for .yaml files, parses each one, and registers the
// resulting packages into reg. It is a no-op when dir does not exist.
func LoadAll(dir string, fsys ports.FileSystem, reg *pkg.Registry) error {
	exists, err := fsys.Exists(dir)
	if err != nil || !exists {
		return nil
	}

	return fsys.Walk(dir, func(path string, info ports.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}
		p, err := Parse(path, fsys)
		if err != nil {
			return fmt.Errorf("load %s: %w", path, err)
		}
		reg.Register(p)
		return nil
	})
}
