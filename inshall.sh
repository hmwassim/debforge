#!/usr/bin/env bash
set -euo pipefail

BOLD='\033[1m'; RED='\033[31m'; GREEN='\033[32m'; YELLOW='\033[33m'; BLUE='\033[34m'; RESET='\033[0m'
info()  { printf "${BOLD}${BLUE}[i]${RESET} %s\n" "$*"; }
ok()    { printf "${BOLD}${GREEN}[*]${RESET} %s\n" "$*"; }
warn()  { printf "${BOLD}${YELLOW}[!]${RESET} %s\n" "$*" >&2; }
err()   { printf "${BOLD}${RED}[x]${RESET} %s\n" "$*" >&2; }

REPO_URL="https://github.com/hmwassim/debforge"
BRANCH="main"
ROOT="/opt/debforge"
BIN_DIR="${ROOT}/bin"
SRC_DIR="${ROOT}/src"
VAR_DIR="${ROOT}/var"
GOPATH="${VAR_DIR}/gopath"
GOCACHE="${GOPATH}/buildcache"
STATE_DIR="${VAR_DIR}/states"
LOCK_DIR="${VAR_DIR}"
LINK="/usr/local/bin/debforge"

if [ -x "$BIN_DIR/debforge" ]; then
	err "debforge appears to be already installed at $BIN_DIR/debforge"
	err "Run 'sudo debforge --self-update' to update, or remove first."
	exit 1
fi

if [ "$(id -u)" -ne 0 ]; then
	err "This script must be run as root."
	exit 1
fi

cleanup() { rm -rf "$ROOT"; rm -f "$LINK"; }
trap 'err "Installation failed"; cleanup' ERR

info "Updating system packages..."
apt-get update -y

info "Installing build dependencies..."
apt-get install -y git golang-go

info "Setting up directory tree..."
mkdir -p "$BIN_DIR" "$SRC_DIR" "$VAR_DIR" "$GOPATH" "$GOCACHE" "$STATE_DIR" "${VAR_DIR}/cache"
chmod 755 "$VAR_DIR"
chmod 755 "$STATE_DIR"
chmod 700 "$GOPATH" "${VAR_DIR}/cache"
touch "${VAR_DIR}/.lock"
chmod 600 "${VAR_DIR}/.lock"

info "Cloning ${REPO_URL} [${BRANCH}]..."
rm -rf "$SRC_DIR"
git clone --depth 1 --branch "$BRANCH" -- "$REPO_URL" "$SRC_DIR"

info "Building debforge..."
export GOPATH="$GOPATH"
export GOMODCACHE="${GOPATH}/mod"
export GOCACHE="$GOCACHE"
cd "$SRC_DIR"
VERSION=$(git describe --tags --always 2>/dev/null || echo "0.1.0-dev")
go build -o "$BIN_DIR/debforge" -ldflags="-X main.version=$VERSION" ./cmd/debforge/

info "Cleaning build cache..."
go clean -cache

info "Verifying binary..."
"$BIN_DIR/debforge" --version >/dev/null 2>&1

info "Creating symlink ${LINK} -> ${BIN_DIR}/debforge..."
if [ -e "$LINK" ] && [ ! -L "$LINK" ]; then
	err "$LINK exists and is not a symlink -- refusing to overwrite"
	exit 1
fi
ln -sf "$BIN_DIR/debforge" "$LINK"

echo ""
ok "debforge installed at ${LINK}"
echo "  Run 'sudo debforge --self-update' to update."
