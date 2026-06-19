package installers

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hmwassim/debforge/internal/domain/apt"
	"github.com/hmwassim/debforge/internal/domain/package"
	"github.com/hmwassim/debforge/internal/utils"
	"github.com/hmwassim/debforge/internal/ports"
)

type RepoManager struct {
	svc    apt.Service
	runner ports.CommandRunner
	fs     ports.FileSystem
	http   ports.HTTPClient
	logger ports.UI
}

func NewRepoManager(svc apt.Service, runner ports.CommandRunner, fs ports.FileSystem, http ports.HTTPClient, logger ports.UI) *RepoManager {
	return &RepoManager{svc: svc, runner: runner, fs: fs, http: http, logger: logger}
}

func (m *RepoManager) EnsureRepo(ctx context.Context, p *pkg.Package) error {
	if p.Extrepo != "" {
		if err := m.ensureExtrepo(ctx); err != nil {
			return err
		}
		return m.extrepoEnable(ctx, p.Extrepo)
	}
	if p.KeyURL != "" {
		return m.setupKeyAndSources(ctx, p)
	}
	return nil
}

func (m *RepoManager) CleanupRepo(ctx context.Context, p *pkg.Package) {
	if p.Extrepo != "" {
		if _, _, err := m.runner.Run(ctx, "extrepo", "disable", p.Extrepo); err != nil {
			m.logger.Warn("extrepo disable: %v", err)
		}
	} else if p.SourcePath != "" {
		if err := m.fs.RemoveAll(p.SourcePath); err != nil {
			m.logger.Warn("removing source path %s: %v", p.SourcePath, err)
		}
		if p.KeyPath != "" {
			if err := m.fs.RemoveAll(p.KeyPath); err != nil {
				m.logger.Warn("removing key path %s: %v", p.KeyPath, err)
			}
		}
	}
}

func (m *RepoManager) ensureExtrepo(ctx context.Context) error {
	_, _, err := m.runner.Run(ctx, "which", "extrepo")
	if err != nil {
		return m.svc.Install(ctx, []string{"extrepo"})
	}
	return m.ensureExtrepoConfig()
}

func (m *RepoManager) ensureExtrepoConfig() error {
	const configPath = "/etc/extrepo/config.yaml"
	data, err := m.fs.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("reading extrepo config: %w", err)
	}
	if strings.Contains(string(data), "\n- non-free") {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	changed := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		content := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
		if strings.HasPrefix(content, "- ") {
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			lines[i] = indent + content
			changed = true
		}
	}
	if changed {
		if err := m.fs.AtomicWriteFile(configPath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
			return fmt.Errorf("writing extrepo config: %w", err)
		}
	}
	return nil
}

func (m *RepoManager) extrepoEnable(ctx context.Context, name string) error {
	_, stderr, err := m.runner.Run(ctx, "extrepo", "enable", name)
	if err != nil {
		msg := strings.TrimSpace(string(stderr))
		if msg != "" {
			return fmt.Errorf("extrepo enable %s: %s: %w", name, msg, err)
		}
		return fmt.Errorf("extrepo enable %s: %w", name, err)
	}
	return nil
}

func (m *RepoManager) needDownload(path string) bool {
	fi, err := m.fs.Stat(path)
	if err != nil || fi.Size() == 0 {
		return true
	}
	return false
}

func (m *RepoManager) downloadFile(ctx context.Context, path, url string) error {
	return utils.DownloadFile(ctx, m.fs, m.http, path, url)
}

func (m *RepoManager) setupKeyAndSources(ctx context.Context, p *pkg.Package) error {
	if err := m.fs.MkdirAll(filepath.Dir(p.KeyPath), 0755); err != nil {
		return fmt.Errorf("creating keyrings dir: %w", err)
	}

	if m.needDownload(p.KeyPath) {
		if p.KeyDearmor {
			tmpPath := p.KeyPath + ".part"
			if err := m.downloadFile(ctx, tmpPath, p.KeyURL); err != nil {
				return fmt.Errorf("downloading key: %w", err)
			}
			_, stderr, err := m.runner.Run(ctx, "gpg", "--dearmor", "--output", p.KeyPath, tmpPath)
			if err := m.fs.RemoveAll(tmpPath); err != nil {
				m.logger.Warn("removing temp key %s: %v", tmpPath, err)
			}
			if err != nil {
				return fmt.Errorf("dearmoring key: %s: %w", strings.TrimSpace(string(stderr)), err)
			}
		} else {
			if err := m.downloadFile(ctx, p.KeyPath, p.KeyURL); err != nil {
				return fmt.Errorf("downloading key: %w", err)
			}
		}
		if err := m.fs.Chmod(p.KeyPath, 0644); err != nil {
			m.logger.Warn("chmod key %s: %v", p.KeyPath, err)
		}
	}

	existing, err := m.fs.ReadFile(p.SourcePath)
	if err != nil || string(existing) != p.Sources {
		if err := m.fs.AtomicWriteFile(p.SourcePath, []byte(p.Sources), 0644); err != nil {
			return fmt.Errorf("writing sources: %w", err)
		}
	}
	return nil
}
