#!/usr/bin/env bash
set -euo pipefail

BOLD='\033[1m'
RED='\033[31m'
GREEN='\033[32m'
YELLOW='\033[33m'
BLUE='\033[34m'
GRAY='\033[90m'
RESET='\033[0m'

info()  { printf "${BOLD}${BLUE}[i]${RESET} %s\n" "$*"; }
ok()    { printf "${BOLD}${GREEN}[*]${RESET} %s\n" "$*"; }
warn()  { printf "${BOLD}${YELLOW}[!]${RESET} %s\n" "$*" >&2; }
err()   { printf "${BOLD}${RED}[x]${RESET} %s\n" "$*" >&2; }

REPO_URL="https://github.com/hmwassim/debforge"
BRANCH="main"
DEBFORGE_ROOT="/opt/debforge"
DEBFORGE_BIN="${DEBFORGE_ROOT}/bin"
DEBFORGE_SRC="${DEBFORGE_ROOT}/src"
DEBFORGE_VAR="${DEBFORGE_ROOT}/var"
DEBFORGE_CACHE="${DEBFORGE_VAR}/cache"
DEBFORGE_GOPATH="${DEBFORGE_VAR}/gopath"
DEBFORGE_GOCACHE="${DEBFORGE_GOPATH}/buildcache"
BINARY="/usr/local/bin/debforge"

if [ -x "$DEBFORGE_BIN/debforge" ]; then
	err "debforge appears to be already installed."
	err "Run 'sudo debforge --self-update' to update."
	exit 1
fi

if [ "$(id -u)" -ne 0 ]; then
	err "This script must be run as root."
	exit 1
fi

cleanup() {
  rm -rf "$DEBFORGE_ROOT"
  rm -f "$BINARY"
}
trap 'err "Installation failed"; cleanup' ERR

info "Updating system..."
apt-get update -y
apt-get upgrade -y

info "Installing dependencies..."
apt-get install -y git golang-go

info "Setting up directories..."
mkdir -p "$DEBFORGE_BIN" "$DEBFORGE_SRC"
mkdir -p "$DEBFORGE_VAR" "$DEBFORGE_CACHE" "$DEBFORGE_GOPATH" "$DEBFORGE_GOCACHE"
chmod 700 "$DEBFORGE_VAR" "$DEBFORGE_CACHE" "$DEBFORGE_GOPATH" "$DEBFORGE_GOCACHE"

info "Cloning ${REPO_URL} [${BRANCH}]..."
rm -rf "$DEBFORGE_SRC"
git clone --depth 1 --branch "$BRANCH" -- "$REPO_URL" "$DEBFORGE_SRC"

info "Building debforge..."
export GOPATH="$DEBFORGE_GOPATH"
export GOMODCACHE="$DEBFORGE_GOPATH/mod"
export GOCACHE="$DEBFORGE_GOCACHE"
cd "$DEBFORGE_SRC"
VERSION=$(git describe --tags --always 2>/dev/null || echo "0.1.0-dev")
go build -o "$DEBFORGE_BIN/debforge" -ldflags="-X github.com/hmwassim/debforge/internal/commands.Version=$VERSION" ./cmd/debforge/

info "Cleaning build cache..."
go clean -cache

info "Verifying binary..."
"$DEBFORGE_BIN/debforge" --version >/dev/null 2>&1

info "Running core setup..."
if [ ! -x "$DEBFORGE_BIN/debforge" ]; then
	err "Binary not found at $DEBFORGE_BIN/debforge — build may have failed"
	exit 1
fi
"$DEBFORGE_BIN/debforge" core setup

info "Linking ${DEBFORGE_BIN}/debforge -> ${BINARY}..."
mkdir -p "$(dirname "$BINARY")"
if [ -e "$BINARY" ] && [ ! -L "$BINARY" ]; then
	err "$BINARY exists and is not a symlink -- refusing to overwrite"
	exit 1
fi
ln -sf "$DEBFORGE_BIN/debforge" "$BINARY"

info "Writing state..."
mkdir -p "${DEBFORGE_VAR}/states"
cat > "${DEBFORGE_VAR}/states/debforge.state.json" <<EOF
{
  "installed_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF
chmod 600 "${DEBFORGE_VAR}/states/debforge.state.json"

echo ""
ok "debforge installed at ${BINARY}"
echo "  Run 'sudo debforge --self-update' to update."
