package repo

import (
	"fmt"
	"path/filepath"
	"strings"

	pkg "github.com/hmwassim/debforge/internal/domain/package"
	"github.com/hmwassim/debforge/internal/ports"
	"gopkg.in/yaml.v3"
)

type Loader struct {
	fs     ports.FileReader
	logger ports.UI
}

func NewLoader(fs ports.FileReader, logger ports.UI) *Loader {
	return &Loader{fs: fs, logger: logger}
}

func (l *Loader) LoadFromDir(dir string, reg *pkg.Registry) error {
	entries, err := l.fs.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", dir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := l.fs.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", entry.Name(), err)
		}
		var p pkg.Package
		if err := yaml.Unmarshal(data, &p); err != nil {
			return fmt.Errorf("parsing %s: %w", entry.Name(), err)
		}
		if p.Name == "" {
			continue
		}
		switch p.Type {
		case pkg.TypeApt, pkg.TypeDeb, pkg.TypeSource, pkg.TypeConfig, pkg.TypeCore:
		default:
			continue
		}
		if p.Type == pkg.TypeConfig {
			p.ConfigDir = filepath.Join(filepath.Dir(dir), "configs", p.Name)
		}
		reg.Register(&p)
	}
	return nil
}
