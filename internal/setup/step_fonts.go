package setup

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/domain/installer"
)

type fontsConfig struct {
	Path    string
	Content string
}

var fontsPackages = []string{
	"fonts-liberation", "fonts-liberation2", "fonts-croscore",
	"fonts-cantarell", "fonts-inter", "fonts-inter-variable",
	"fonts-noto", "fonts-noto-core", "fonts-noto-hinted", "fonts-noto-ui-core",
	"fonts-noto-unhinted", "fonts-noto-cjk", "fonts-noto-cjk-extra",
	"fonts-noto-color-emoji", "fonts-noto-extra", "fonts-noto-mono", "fonts-noto-ui-extra",
	"ttf-mscorefonts-installer",
}

var fontsConfigFiles = []fontsConfig{
	{
		Path: "/etc/fonts/local.conf",
		Content: `<?xml version="1.0"?>
<!DOCTYPE fontconfig SYSTEM "urn:fontconfig:fonts.dtd">
<fontconfig>

  <selectfont>
    <rejectfont>
      <glob>*NotoNastaliq*</glob>
    </rejectfont>
  </selectfont>

  <alias>
    <family>sans-serif</family>
    <prefer>
      <family>Arimo</family>
      <family>Noto Sans Arabic</family>
    </prefer>
  </alias>

  <alias>
    <family>serif</family>
    <prefer>
      <family>Tinos</family>
      <family>Noto Sans Arabic</family>
    </prefer>
  </alias>

  <alias>
    <family>Sans</family>
    <prefer>
      <family>Arimo</family>
      <family>Noto Sans Arabic</family>
    </prefer>
  </alias>

  <alias>
    <family>monospace</family>
    <prefer>
      <family>Cousine</family>
      <family>Noto Sans Arabic</family>
    </prefer>
  </alias>

  <match>
    <test name="family"><string>Arial</string></test>
    <edit name="family" mode="assign" binding="strong">
      <string>Arimo</string>
    </edit>
  </match>
  <match>
    <test name="family"><string>Helvetica</string></test>
    <edit name="family" mode="assign" binding="strong">
      <string>Arimo</string>
    </edit>
  </match>
  <match>
    <test name="family"><string>Verdana</string></test>
    <edit name="family" mode="assign" binding="strong">
      <string>Arimo</string>
    </edit>
  </match>
  <match>
    <test name="family"><string>Tahoma</string></test>
    <edit name="family" mode="assign" binding="strong">
      <string>Arimo</string>
    </edit>
  </match>
  <match>
    <test name="family"><string>Comic Sans MS</string></test>
    <edit name="family" mode="assign" binding="strong">
      <string>Arimo</string>
    </edit>
  </match>
  <match>
    <test name="family"><string>Times New Roman</string></test>
    <edit name="family" mode="assign" binding="strong">
      <string>Tinos</string>
    </edit>
  </match>
  <match>
    <test name="family"><string>Times</string></test>
    <edit name="family" mode="assign" binding="strong">
      <string>Tinos</string>
    </edit>
  </match>
  <match>
    <test name="family"><string>Courier New</string></test>
    <edit name="family" mode="assign" binding="strong">
      <string>Cousine</string>
    </edit>
  </match>

  <match target="pattern">
    <test name="lang" compare="contains"><string>ar</string></test>
    <edit name="family" mode="prepend" binding="strong">
      <string>Noto Sans Arabic</string>
    </edit>
  </match>

  <match target="pattern">
    <test name="lang" compare="contains"><string>ar</string></test>
    <test name="spacing" compare="eq"><int>100</int></test>
    <edit name="family" mode="prepend" binding="strong">
      <string>Noto Sans Arabic UI</string>
    </edit>
  </match>

  <match target="pattern">
    <test name="family"><string>emoji</string></test>
    <edit name="family" mode="prepend" binding="strong">
      <string>Noto Color Emoji</string>
    </edit>
  </match>

  <match target="pattern">
    <test name="lang" compare="contains"><string>zh</string></test>
    <edit name="family" mode="append" binding="weak">
      <string>Noto Sans CJK SC</string>
    </edit>
  </match>
  <match target="pattern">
    <test name="lang" compare="contains"><string>ja</string></test>
    <edit name="family" mode="append" binding="weak">
      <string>Noto Sans CJK JP</string>
    </edit>
  </match>
  <match target="pattern">
    <test name="lang" compare="contains"><string>ko</string></test>
    <edit name="family" mode="append" binding="weak">
      <string>Noto Sans CJK KR</string>
    </edit>
  </match>

</fontconfig>
`,
	},
}

type FontsStep struct{}

func (s *FontsStep) Name() string {
	return "Installed fonts"
}

func (s *FontsStep) Check(ctx context.Context, cx *Context) CheckResult {
	ok, err := allInstalled(ctx, cx.Runner, fontsPackages)
	if err != nil {
		return CheckResult{Status: StatusError, Summary: fmt.Sprintf("dpkg query failed: %s", err)}
	}
	if !ok {
		return CheckResult{Status: StatusMissing, Summary: "fonts not installed"}
	}

	for _, cfg := range fontsConfigFiles {
		action := installer.DecideConfigAction(cx.Fsys, cfg.Path, cfg.Content, cx.ConfigHashes[cfg.Path], false)
		exists, _ := cx.Fsys.Exists(cfg.Path)
		switch {
		case action == installer.ConfigWrite && !exists:
			return CheckResult{Status: StatusMissing, Summary: fmt.Sprintf("%s does not exist", cfg.Path)}
		case action == installer.ConfigWrite && exists:
			continue
		case action == installer.ConfigSkip:
			return CheckResult{Status: StatusDrifted, Summary: fmt.Sprintf("%s modified by user", cfg.Path)}
		case action == installer.ConfigConflict:
			return CheckResult{Status: StatusConflict, Summary: fmt.Sprintf("%s: local changes conflict with new defaults", cfg.Path)}
		}
	}

	return CheckResult{Status: StatusSatisfied}
}

func (s *FontsStep) Apply(ctx context.Context, cx *Context, result CheckResult) error {
	initialDesc := "Configuring fontconfig"
	if result.Status == StatusMissing {
		initialDesc = "Installing fonts"
	}
	spinner := cx.UI.Spinner(ctx, initialDesc)
	defer spinner.Stop()

	if result.Status == StatusMissing {
		if err := aptpty.RunInstall(ctx, cx.Runner, fontsPackages, spinner); err != nil {
			return err
		}
		spinner.SetDesc("Configuring fontconfig")
	}

	for _, cfg := range fontsConfigFiles {
		force := cx.Force
		if result.Status == StatusDrifted {
			force = false
		}

		action := installer.DecideConfigAction(cx.Fsys, cfg.Path, cfg.Content, cx.ConfigHashes[cfg.Path], force)

		switch action {
		case installer.ConfigWrite:
			dir := filepath.Dir(cfg.Path)
			if err := cx.Fsys.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("create dir %s: %w", dir, err)
			}
			if err := cx.Fsys.WriteFile(cfg.Path, []byte(cfg.Content), 0644); err != nil {
				return fmt.Errorf("write %s: %w", cfg.Path, err)
			}
			cx.ConfigHashes[cfg.Path] = installer.Sha256Hex([]byte(cfg.Content))

		case installer.ConfigSkip:
			diskData, err := cx.Fsys.ReadFile(cfg.Path)
			if err == nil && diskData != nil {
				cx.ConfigHashes[cfg.Path] = installer.Sha256Hex(diskData)
			}

		case installer.ConfigConflict:
			sidecar := cfg.Path + ".debforge-new"
			if err := cx.Fsys.WriteFile(sidecar, []byte(cfg.Content), 0644); err != nil {
				return fmt.Errorf("write sidecar %s: %w", sidecar, err)
			}
			cx.UI.Warn("%s has local changes; new version saved as %s", cfg.Path, sidecar)
		}
	}

	cx.Runner.Run(ctx, "fc-cache", "-f")
	return nil
}
