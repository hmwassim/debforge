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
BINARY="/usr/local/bin/debforge"

if [ -f "$BINARY" ] || [ -d "$DEBFORGE_ROOT" ]; then
	err "debforge appears to be already installed."
	err "Run 'sudo debforge self-update' to update."
	exit 1
fi

if [ "$(id -u)" -ne 0 ]; then
	err "This script must be run as root."
	exit 1
fi

info "Installing dependencies..."
apt-get update -y
apt-get install -y git golang-go

info "Setting up directories..."
mkdir -p "$DEBFORGE_BIN" "$DEBFORGE_SRC" "$DEBFORGE_VAR" "$DEBFORGE_CACHE" "$DEBFORGE_GOPATH"

info "Cloning ${REPO_URL} [${BRANCH}]..."
git clone --depth 1 --branch "$BRANCH" -- "$REPO_URL" "$DEBFORGE_SRC"

info "Building debforge..."
export GOPATH="$DEBFORGE_GOPATH"
export GOMODCACHE="$DEBFORGE_GOPATH/mod"
export GOCACHE="$DEBFORGE_CACHE"
go build -o "$DEBFORGE_BIN/debforge" "${DEBFORGE_SRC}/cmd/debforge/"

info "Verifying binary..."
"$DEBFORGE_BIN/debforge" --version

info "Linking ${DEBFORGE_BIN}/debforge -> ${BINARY}..."
ln -sf "$DEBFORGE_BIN/debforge" "$BINARY"

info "Writing state..."
cat > "${DEBFORGE_VAR}/state.json" <<EOF
{
  "installed_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "updated_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF

echo ""
ok "debforge installed at ${BINARY}"
echo "  Run 'sudo debforge self-update' to update."
