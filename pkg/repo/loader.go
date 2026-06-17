package repo

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func LoadFromDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading %s: %w", dir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return fmt.Errorf("reading %s: %w", entry.Name(), err)
		}
		var pkg RepoPackage
		if err := yaml.Unmarshal(data, &pkg); err != nil {
			return fmt.Errorf("parsing %s: %w", entry.Name(), err)
		}
		if pkg.Name == "" || (pkg.Type != "apt" && pkg.Type != "config" && pkg.Type != "deb" && pkg.Type != "source") {
			continue
		}
		pkg.ConfigDir = filepath.Join(dir, "..", "configs", pkg.Name)
		if err := Register(&pkg); err != nil {
			return fmt.Errorf("registering %s: %w", entry.Name(), err)
		}
	}
	return nil
}
