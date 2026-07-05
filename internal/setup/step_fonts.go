package setup

import (
	"context"

	"github.com/hmwassim/debforge/internal/aptpty"
)

var fontsPackages = []string{
	"fonts-liberation", "fonts-liberation2", "fonts-croscore",
	"fonts-cantarell", "fonts-inter", "fonts-inter-variable",
	"fonts-noto", "fonts-noto-core", "fonts-noto-hinted", "fonts-noto-ui-core",
	"fonts-noto-unhinted", "fonts-noto-cjk", "fonts-noto-cjk-extra",
	"fonts-noto-color-emoji", "fonts-noto-extra", "fonts-noto-mono", "fonts-noto-ui-extra",
	"ttf-mscorefonts-installer",
}

var fontsConfigFiles = []ConfigFile{
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
	if r := checkStepPackages(ctx, cx, fontsPackages, "fonts not installed"); r.Status != StatusSatisfied {
		return r
	}
	return checkConfigFiles(cx, fontsConfigFiles)
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

	if err := processConfigFiles(cx, fontsConfigFiles, result); err != nil {
		return err
	}

	_, _, _ = cx.Runner.Run(ctx, "fc-cache", "-f")
	return nil
}
