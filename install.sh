#!/usr/bin/env bash
set -euo pipefail

info() {
    printf "\033[1;34m[*]\033[0m %s\n" "$1"
}

error() {
    printf "\033[1;31m[!]\033[0m %s\n" "$1" >&2
}

success() {
    printf "\033[1;32m[+]\033[0m %s\n" "$1"
}

repo="hmwassim/debforge"
DEBFORGE_REPO="${DEBFORGE_REPO:-$repo}"
DEBFORGE_BIN="${DEBFORGE_BIN:-/opt/debforge/bin}"
DEBFORGE_SRC="${DEBFORGE_SRC:-/opt/debforge/src}"

if [ "$(id -u)" -ne 0 ]; then
    error "debforge must be installed as root"
    exit 1
fi

apt-get update -qq
apt-get upgrade -y -qq

if ! command -v git &>/dev/null || ! command -v go &>/dev/null; then
    info "Installing build dependencies..."
    apt-get install -y git golang-go
fi

info "Creating directories..."
mkdir -p "$DEBFORGE_BIN" "$DEBFORGE_SRC"

REMOTE="https://github.com/${DEBFORGE_REPO}"

info "Cloning ${REMOTE}..."
rm -rf "$DEBFORGE_SRC"
git clone --depth 1 "$REMOTE" "$DEBFORGE_SRC"

info "Building debforge..."
(cd "$DEBFORGE_SRC" && go build -o "$DEBFORGE_BIN/debforge" ./cmd/debforge/)

chmod +x "${DEBFORGE_BIN}/debforge"

"${DEBFORGE_BIN}/debforge" core setup

success "debforge is installed and up to date"
echo "Run 'sudo /opt/debforge/bin/debforge --help' to see available commands."
