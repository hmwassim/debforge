package setup

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/aptpty"
)

var multimediaPackages = []string{
	"pipewire", "pipewire:i386",
	"pipewire-audio", "pipewire-pulse", "pipewire-alsa", "pipewire-jack",
	"pipewire-bin", "pipewire-doc", "pipewire-tests",
	"pipewire-v4l2", "pipewire-libcamera",
	"pipewire-audio-client-libraries",
	"libpipewire-0.3-0t64", "libpipewire-0.3-0t64:i386",
	"libpipewire-0.3-common", "libpipewire-0.3-dev", "libpipewire-0.3-modules",
	"libpipewire-0.3-modules-x11",
	"libspa-0.2-bluetooth", "libspa-0.2-jack", "libspa-0.2-dev",
	"libspa-0.2-libcamera", "libspa-0.2-modules", "libspa-0.2-modules:i386",
	"wireplumber", "rtkit",
	"alsa-utils", "alsa-tools", "alsa-firmware-loaders",
	"libasound2-plugins", "libasound2-plugins:i386",
	"libasound2t64", "libcanberra-pulse",
	"easyeffects", "lsp-plugins-lv2", "calf-plugins", "x42-plugins", "zam-plugins",
	"gstreamer1.0-pipewire",
	"ffmpeg",
	"gstreamer1.0-libav", "gstreamer1.0-libav:i386",
	"gstreamer1.0-plugins-good", "gstreamer1.0-plugins-good:i386",
	"gstreamer1.0-plugins-bad", "gstreamer1.0-plugins-bad:i386",
	"gstreamer1.0-plugins-ugly", "gstreamer1.0-plugins-ugly:i386",
	"gstreamer1.0-vaapi", "gstreamer1.0-alsa", "gstreamer1.0-tools",
	"mpv", "vlc", "mpg123", "lame", "x264", "x265", "opus-tools", "flvmeta",
	"libgif7", "libgif7:i386", "giflib-tools",
	"libglfw3", "libglfw3:i386",
	"libosmesa6", "libosmesa6:i386",
	"libvulkan-dev", "libvulkan-dev:i386",
	"libgtk-3-0t64", "libgtk-3-0t64:i386",
	"libopenal1", "libopenal1:i386",
	"libturbojpeg0", "libturbojpeg0:i386",
	"libjpeg62-turbo", "libjpeg62-turbo:i386",
	"ocl-icd-libopencl1", "ocl-icd-libopencl1:i386",
	"libxslt1-dev", "libxml2-dev",
	"timidity", "fluidsynth", "dosbox",
}

type MultimediaStep struct{}

func (s *MultimediaStep) Name() string {
	return "Installed multimedia stack"
}

func (s *MultimediaStep) Check(ctx context.Context, cx *Context) CheckResult {
	ok, err := allInstalled(ctx, cx.Runner, multimediaPackages)
	if err != nil {
		return CheckResult{Status: StatusError, Summary: fmt.Sprintf("dpkg query failed: %s", err)}
	}
	if !ok {
		return CheckResult{Status: StatusMissing, Summary: "multimedia packages not installed"}
	}
	return CheckResult{Status: StatusSatisfied}
}

func (s *MultimediaStep) Apply(ctx context.Context, cx *Context, result CheckResult) error {
	spinner := cx.UI.Spinner(ctx, "Installing multimedia stack")
	defer spinner.Stop()
	return aptpty.RunInstall(ctx, cx.Runner, multimediaPackages, spinner)
}
