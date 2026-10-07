#!/bin/sh
# Install the latest debforge release.
#
#   curl -fsSL https://raw.githubusercontent.com/hmwassim/debforge/main/install.sh | sudo sh
#
# This only installs the debforge binary and shell completions. It does not
# change any system configuration: run 'sudo debforge setup' afterwards to
# see (and confirm) what setup would do.
set -eu

REPO="hmwassim/debforge"
BASE="https://github.com/$REPO/releases/latest/download"
BIN=/usr/local/bin/debforge

die() { echo "error: $*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run as root (sudo)"
# shellcheck source=/dev/null
. /etc/os-release
[ "${VERSION_CODENAME:-}" = "trixie" ] || die "debforge supports Debian 13 (trixie) only; this is ${PRETTY_NAME:-unknown}"
[ "$(dpkg --print-architecture)" = "amd64" ] || die "debforge supports amd64 only"

if command -v curl >/dev/null 2>&1; then
    fetch() { curl -fsSL --proto '=https' --tlsv1.2 -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
    fetch() { wget -q --https-only -O "$2" "$1"; }
else
    die "curl or wget is required"
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

echo "Downloading debforge..."
fetch "$BASE/debforge-linux-amd64" "$tmp/debforge-linux-amd64"
fetch "$BASE/SHA256SUMS" "$tmp/SHA256SUMS"
(cd "$tmp" && grep ' \*\{0,1\}debforge-linux-amd64$' SHA256SUMS | sha256sum -c --quiet -) \
    || die "checksum verification failed"
chmod 755 "$tmp/debforge-linux-amd64"
"$tmp/debforge-linux-amd64" --version >/dev/null || die "the downloaded binary does not run"

install -Dm755 "$tmp/debforge-linux-amd64" "$BIN.new"
mv -f "$BIN.new" "$BIN"

mkdir -p /usr/local/share/bash-completion/completions /usr/local/share/zsh/site-functions /usr/local/share/fish/vendor_completions.d
"$BIN" completion bash > /usr/local/share/bash-completion/completions/debforge
"$BIN" completion zsh > /usr/local/share/zsh/site-functions/_debforge
"$BIN" completion fish > /usr/local/share/fish/vendor_completions.d/debforge.fish

echo
echo "Installed $("$BIN" --version)."
if [ -d /opt/debforge ]; then
    echo
    echo "Note: an older debforge installation exists in /opt/debforge. The new version"
    echo "does not use it and does not import its state; remove it when convenient:"
    echo "  sudo rm -rf /opt/debforge"
fi
echo
echo "Next steps:"
echo "  sudo debforge setup      # review and apply the system setup"
echo "  debforge list            # browse packages"
echo "  sudo debforge install <package>"
