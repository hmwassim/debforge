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

if [ "$(id -u)" -ne 0 ]; then
    error "debforge must be installed as root"
    exit 1
fi

if ! command -v curl &>/dev/null; then
    error "curl is required for installation"
    exit 1
fi

info "Creating directories..."
mkdir -p "$DEBFORGE_BIN"

REMOTE="${GITHUB_SERVER_URL:-https://github.com}/${DEBFORGE_REPO}"

download_url() {
    if [ -n "${DEBFORGE_VERSION:-}" ]; then
        echo "${REMOTE}/releases/download/${DEBFORGE_VERSION}/debforge"
    else
        echo "${REMOTE}/releases/latest/download/debforge"
    fi
}

info "Downloading debforge..."
if ! curl -sSfL "$(download_url)" -o "${DEBFORGE_BIN}/debforge"; then
    error "Download failed. Falling back to manual build..."
    if ! command -v git &>/dev/null || ! command -v go &>/dev/null; then
        info "Installing build dependencies..."
        apt-get install -y git golang-go
    fi
    TMPDIR=$(mktemp -d)
    git clone "https://github.com/${DEBFORGE_REPO}" "$TMPDIR"
    (cd "$TMPDIR" && go build -o debforge .)
    mv "$TMPDIR/debforge" "${DEBFORGE_BIN}/debforge"
    rm -rf "$TMPDIR"
fi

chmod +x "${DEBFORGE_BIN}/debforge"

if [ ! -f /etc/systemd/system/debforge.timer ]; then
    info "Creating systemd timer for periodic setup..."
    cat > /etc/systemd/system/debforge.service <<'EOF'
[Unit]
Description=debforge

[Service]
Type=oneshot
ExecStart=/opt/debforge/bin/debforge core setup
EOF
    cat > /etc/systemd/system/debforge.timer <<'EOF'
[Unit]
Description=Run debforge setup daily

[Timer]
OnCalendar=daily
Persistent=true

[Install]
WantedBy=timers.target
EOF
    systemctl daemon-reload
    systemctl enable --now debforge.timer
    success "Systemd timer created and enabled"
fi

"${DEBFORGE_BIN}/debforge" core setup

success "debforge is installed and up to date"
echo "Run 'sudo /opt/debforge/bin/debforge --help' to see available commands."
