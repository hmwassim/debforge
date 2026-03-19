#!/usr/bin/env bash
# install.sh - DebForge system setup and configuration installer
# Cleaner, simpler installer with better structure and error handling
#
# Usage: ./install.sh [OPTIONS]
# Note: Script handles sudo internally - do NOT run with sudo prefix

set -euo pipefail

# ─────────────────────────────────────────────────────────────────────────────
# Configuration
# ─────────────────────────────────────────────────────────────────────────────

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONFIGS_DIR="$REPO_DIR/configs"
BIN_DIR="$REPO_DIR/bin"

STATE_DIR="$HOME/.local/share/debforge"
MANIFEST_FILE="$STATE_DIR/manifest.json"
BACKUP_DIR="$STATE_DIR/backups"
LOG_DIR="$STATE_DIR/logs"

# ─────────────────────────────────────────────────────────────────────────────
# Source libraries
# ─────────────────────────────────────────────────────────────────────────────

source "$SCRIPT_DIR/lib/logger.sh"
source "$SCRIPT_DIR/lib/manifest.sh"
source "$SCRIPT_DIR/lib/backup.sh"
source "$SCRIPT_DIR/lib/verify.sh"

# ─────────────────────────────────────────────────────────────────────────────
# Global state
# ─────────────────────────────────────────────────────────────────────────────

DRY_RUN=false
FORCE=false
SKIP_VERIFY=false
SKIP_SCRIPTS=false
SKIP_CONFIGS=false
KERNEL_CHOICE=""
NVIDIA_VARIANT=""
INSTALLED_FILES=()
FAILED_FILES=()

# ─────────────────────────────────────────────────────────────────────────────
# Argument parsing
# ─────────────────────────────────────────────────────────────────────────────

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --dry-run|-n)
                DRY_RUN=true
                shift
                ;;
            --force|-f)
                FORCE=true
                shift
                ;;
            --skip-scripts)
                SKIP_SCRIPTS=true
                shift
                ;;
            --skip-configs)
                SKIP_CONFIGS=true
                shift
                ;;
            --skip-verify)
                SKIP_VERIFY=true
                shift
                ;;
            --kernel)
                KERNEL_CHOICE="$2"
                shift 2
                ;;
            --nvidia)
                NVIDIA_VARIANT="$2"
                shift 2
                ;;
            --quiet|-q)
                LOG_QUIET=true
                shift
                ;;
            --verbose|-v)
                LOG_VERBOSE=true
                shift
                ;;
            --help|-h)
                show_help
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done
}

show_help() {
    cat << EOF
DebForge Setup - System Optimization Configuration

Usage: $(basename "$0") [OPTIONS]

Options:
  --dry-run, -n       Preview changes without applying
  --force, -f         Force re-installation if already installed
  --skip-scripts      Skip setup scripts (configs only)
  --skip-configs      Skip configs deployment (scripts only)
  --skip-verify       Skip verification after installation
  --kernel TYPE       Kernel choice: backports|liquorix (non-interactive)
  --nvidia VARIANT    NVIDIA driver: nvidia-open|cuda-drivers (non-interactive)
  --quiet, -q         Suppress output
  --verbose, -v       Enable debug output
  --help, -h          Show this help message

Installation Flow:
  1. Run setup scripts (01-core → 05-misc) - package installs, package setup
  2. Deploy system configurations (sysctl, udev, systemd, etc.)
  3. Apply changes (reload rules, restart services)
  4. Verify installation

Examples:
  $(basename "$0")                              # Full installation (interactive)
  $(basename "$0") --skip-scripts               # Configs only
  $(basename "$0") --kernel backports           # Non-interactive with backports
  $(basename "$0") --dry-run                    # Preview all changes

Note: This script handles sudo internally. Do NOT run with sudo prefix.
EOF
}

# ─────────────────────────────────────────────────────────────────────────────
# Utility functions
# ─────────────────────────────────────────────────────────────────────────────

check_not_root() {
    if [[ $EUID -eq 0 ]]; then
        log_fatal "Do not run this script as root - it handles sudo internally"
    fi
}

check_existing_install() {
    if ! manifest_exists; then
        return 0
    fi

    if [[ "$FORCE" == "true" ]] || [[ "$DRY_RUN" == "true" ]]; then
        return 0
    fi

    log_warn "DebForge appears to be already installed"
    log_info "Use --force to re-apply configurations"
    log_info "Use --dry-run to preview changes"
    return 1
}

install_file() {
    local src="$1"
    local dest="$2"
    local perms="$3"
    local desc="${4:-config}"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY-RUN] Would install: $dest"
        return 0
    fi

    # Backup existing
    if [[ -f "$dest" ]] && ! sudo cmp -s "$src" "$dest"; then
        local backup_dest
        backup_dest=$(backup_path "$dest")
        sudo mkdir -p "$(dirname "$backup_dest")"
        sudo cp -p "$dest" "$backup_dest"
        log_debug "Backed up: $backup_dest"
    fi

    # Install
    sudo mkdir -p "$(dirname "$dest")"
    if sudo install -m "$perms" "$src" "$dest"; then
        local checksum
        checksum="sha256:$(sha256sum "$dest" | cut -d' ' -f1)"
        manifest_add_file "$dest" "$src" "$perms" "$desc" "$checksum" "" || true
        INSTALLED_FILES+=("$dest")
        return 0
    else
        FAILED_FILES+=("$dest")
        log_error "Failed to install: $dest"
        return 1
    fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 1: Pre-flight
# ─────────────────────────────────────────────────────────────────────────────

phase_preflight() {
    log_section "Phase 1: Pre-flight Checks"

    if ! check_not_root; then
        return 1
    fi

    # Check required commands
    local missing=()
    for cmd in jq sudo dpkg; do
        if ! command -v "$cmd" &>/dev/null; then
            missing+=("$cmd")
        fi
    done

    if [[ ${#missing[@]} -gt 0 ]]; then
        log_fatal "Missing required commands: ${missing[*]}"
    fi

    log_success "Pre-flight checks passed"
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 2: Initialize
# ─────────────────────────────────────────────────────────────────────────────

phase_init() {
    log_section "Phase 2: Initialize"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY-RUN] Would create state directory: $STATE_DIR"
        return 0
    fi

    mkdir -p "$STATE_DIR" "$BACKUP_DIR" "$LOG_DIR"
    if ! manifest_exists; then
        manifest_create "1.0"
    fi

    log_success "State initialized: $STATE_DIR"
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 3: Run setup scripts
# ─────────────────────────────────────────────────────────────────────────────

phase_setup_scripts() {
    if [[ "$SKIP_SCRIPTS" == "true" ]]; then
        log_section "Phase 3: Setup Scripts (skipped)"
        return 0
    fi

    log_section "Phase 3: Running Setup Scripts"
    log_info "Scripts: 01-core → 02-hardware → 03-desktop → 04-gaming → 05-misc"
    echo ""

    local script_dirs=()
    for dir in "$SCRIPT_DIR"/[0-9][0-9]-*/; do
        if [[ -d "$dir" ]]; then
            script_dirs+=("$dir")
        fi
    done

    if [[ ${#script_dirs[@]} -eq 0 ]]; then
        log_warn "No setup script directories found"
        return 0
    fi

    local total=${#script_dirs[@]}
    local index=0

    for dir in "${script_dirs[@]}"; do
        index=$((index + 1))
        local dirname
        dirname=$(basename "$dir")

        log_step "$index" "$total" "$dirname"

        local scripts=()
        while IFS= read -r -d '' script; do
            scripts+=("$script")
        done < <(find "$dir" -maxdepth 1 -name "*.sh" -type f -print0 | sort -z)

        if [[ ${#scripts[@]} -eq 0 ]]; then
            continue
        fi

        for script in "${scripts[@]}"; do
            local script_name
            script_name=$(basename "$script")

            if [[ "$DRY_RUN" == "true" ]]; then
                log_info "[DRY-RUN] Would run: $script_name"
                continue
            fi

            # Make executable
            chmod +x "$script" 2>/dev/null || sudo chmod +x "$script" 2>/dev/null || true

            # Build arguments
            local args=()
            [[ "$script_name" == "01.03-kernel.sh" ]] && [[ -n "$KERNEL_CHOICE" ]] && \
                args+=("--kernel" "$KERNEL_CHOICE")
            [[ "$script_name" == "02.02-nvidia.sh" ]] && [[ -n "$NVIDIA_VARIANT" ]] && \
                args+=("--variant" "$NVIDIA_VARIANT")

            # Run script
            log_progress "Running: $script_name"
            (
                set +e
                if [[ ${#args[@]} -gt 0 ]]; then
                    bash "$script" "${args[@]}"
                else
                    bash "$script"
                fi
                exit $?
            ) && log_success "✓ $script_name" || log_warn "✗ $script_name (exit code: $?)"
        done
    done

    echo ""
    log_success "Setup scripts complete"
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 4: Deploy configurations
# ─────────────────────────────────────────────────────────────────────────────

phase_deploy_configs() {
    if [[ "$SKIP_CONFIGS" == "true" ]]; then
        log_section "Phase 4: Deploy Configurations (skipped)"
        return 0
    fi

    log_section "Phase 4: Deploy Configurations"
    echo ""

    local total=10
    local step=0

    # 1. Sysctl
    step=$((step + 1))
    log_step "$step" "$total" "Sysctl tuning (VM, kernel, network, filesystem)"
    install_file "$CONFIGS_DIR/sysctl.d/99-debforge.conf" \
        /etc/sysctl.d/99-debforge.conf 0644 "sysctl" || true

    # 2. Modprobe
    step=$((step + 1))
    log_step "$step" "$total" "Modprobe (NVIDIA, watchdog blacklist)"
    for conf in "$CONFIGS_DIR"/modprobe.d/*.conf; do
        [[ -f "$conf" ]] || continue
        install_file "$conf" "/usr/lib/modprobe.d/$(basename "$conf")" 0644 "modprobe" || true
    done

    # 3. Tmpfiles
    step=$((step + 1))
    log_step "$step" "$total" "Tmpfiles (THP, coredump, KSM)"
    for conf in "$CONFIGS_DIR"/tmpfiles.d/*.conf; do
        [[ -f "$conf" ]] || continue
        install_file "$conf" "/usr/lib/tmpfiles.d/$(basename "$conf")" 0644 "tmpfiles" || true
    done

    # 4. Udev
    step=$((step + 1))
    log_step "$step" "$total" "Udev rules (I/O scheduling, audio PM, etc.)"
    for rule in "$CONFIGS_DIR"/udev/*.rules; do
        [[ -f "$rule" ]] || continue
        install_file "$rule" "/usr/lib/udev/rules.d/$(basename "$rule")" 0644 "udev" || true
    done

    # 5. Systemd system
    step=$((step + 1))
    log_step "$step" "$total" "Systemd manager configs (timeouts, limits)"
    for conf in "$CONFIGS_DIR"/systemd/{system.conf.d,user.conf.d,journald.conf.d,timesyncd.conf.d}/*.conf; do
        [[ -f "$conf" ]] || continue
        local subdir
        subdir=$(echo "$conf" | sed "s|.*systemd/||" | sed "s|/[^/]*$||")
        install_file "$conf" "/usr/lib/systemd/$subdir/$(basename "$conf")" 0644 "systemd" || true
    done

    # 6. Resolved (DNS-over-TLS)
    step=$((step + 1))
    log_step "$step" "$total" "Resolved (DNS-over-TLS)"
    if [[ -f "$CONFIGS_DIR/systemd/resolved.conf.d/99-dot.conf" ]]; then
        sudo mkdir -p /etc/systemd/resolved.conf.d
        install_file "$CONFIGS_DIR/systemd/resolved.conf.d/99-dot.conf" \
            /etc/systemd/resolved.conf.d/99-dot.conf 0644 "systemd" || true
    fi

    # 7. Security limits (audio RT)
    step=$((step + 1))
    log_step "$step" "$total" "Security limits (audio RT priority)"
    sudo mkdir -p /etc/security/limits.d
    install_file "$CONFIGS_DIR/security/20-audio.conf" \
        /etc/security/limits.d/20-audio.conf 0644 "security" || true

    # 8. Binaries
    step=$((step + 1))
    log_step "$step" "$total" "Install binaries (pci-latency, ksmctl, game-performance)"
    for bin in pci-latency ksmctl game-performance; do
        [[ -f "$BIN_DIR/$bin" ]] || continue
        install_file "$BIN_DIR/$bin" "/usr/local/bin/$bin" 0755 "binary" || true
    done

    # 9. Systemd services
    step=$((step + 1))
    log_step "$step" "$total" "Install systemd services"
    if [[ "$DRY_RUN" != "true" ]]; then
        # pci-latency service
        sudo tee /etc/systemd/system/pci-latency.service > /dev/null << 'EOF'
[Unit]
Description=Adjust PCI latency timers for audio and devices

[Service]
Type=oneshot
ExecStart=/usr/local/bin/pci-latency

[Install]
WantedBy=multi-user.target
EOF
        sudo chmod 0644 /etc/systemd/system/pci-latency.service
        INSTALLED_FILES+=("/etc/systemd/system/pci-latency.service")

        # set-min-free-mem service
        sudo tee /etc/systemd/system/set-min-free-mem.service > /dev/null << 'EOF'
[Unit]
Description=Set vm.min_free_kbytes dynamically
DefaultDependencies=no
After=local-fs.target
Before=sysinit.target

[Service]
Type=oneshot
ExecStart=/bin/sh -c "sysctl -w vm.min_free_kbytes=$(awk '/MemTotal/ {printf "%%.0f", $2 * 0.01}' /proc/meminfo)"

[Install]
WantedBy=sysinit.target
EOF
        sudo chmod 0644 /etc/systemd/system/set-min-free-mem.service
        INSTALLED_FILES+=("/etc/systemd/system/set-min-free-mem.service")
    fi

    # 10. Environment (PROTON_ENABLE_WAYLAND, shader caches, etc.)
    step=$((step + 1))
    log_step "$step" "$total" "Environment configuration"
    if [[ "$DRY_RUN" != "true" ]]; then
        sudo mkdir -p /etc/environment.d
        sudo tee /etc/environment.d/90-debforge-env.conf > /dev/null << 'EOF'
# DebForge environment optimizations
PROTON_ENABLE_WAYLAND=1
EOF
        sudo chmod 0644 /etc/environment.d/90-debforge-env.conf
        INSTALLED_FILES+=("/etc/environment.d/90-debforge-env.conf")
    else
        log_info "[DRY-RUN] Would create: /etc/environment.d/90-debforge-env.conf"
    fi

    echo ""
    log_success "Configuration deployment complete"
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 5: Apply changes
# ─────────────────────────────────────────────────────────────────────────────

phase_apply() {
    if [[ "$SKIP_CONFIGS" == "true" ]]; then
        log_section "Phase 5: Apply Changes (skipped)"
        return 0
    fi

    log_section "Phase 5: Apply Changes"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY-RUN] Would apply system changes"
        return 0
    fi

    # Reload udev
    log_progress "Reloading udev rules"
    sudo udevadm control --reload-rules 2>/dev/null || log_warn "udev reload failed"

    # Apply sysctl
    log_progress "Applying sysctl settings"
    sudo sysctl --system 2>/dev/null || log_warn "sysctl apply failed"

    # Reload systemd
    log_progress "Reloading systemd"
    sudo systemctl daemon-reload || log_warn "systemd reload failed"

    # Enable services
    log_progress "Enabling services"
    sudo systemctl enable pci-latency.service 2>/dev/null || true
    sudo systemctl enable set-min-free-mem.service 2>/dev/null || true

    # Restart services
    log_progress "Restarting services"
    sudo systemctl restart systemd-journald 2>/dev/null || true
    sudo systemctl restart systemd-timesyncd 2>/dev/null || true
    sudo systemctl restart systemd-resolved 2>/dev/null || true

    echo ""
    log_success "Changes applied"
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 6: Verify
# ─────────────────────────────────────────────────────────────────────────────

phase_verify() {
    if [[ "$SKIP_VERIFY" == "true" ]]; then
        log_info "Verification skipped"
        return 0
    fi

    log_section "Phase 6: Verification"

    if ! post_install_verify; then
        log_warn "Some verifications failed"
    fi

    log_success "Verification complete"
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 7: Summary
# ─────────────────────────────────────────────────────────────────────────────

phase_summary() {
    log_section "Installation Summary"

    local summary=()
    summary+=("Installed files: ${#INSTALLED_FILES[@]}")
    [[ ${#FAILED_FILES[@]} -gt 0 ]] && summary+=("Failed files: ${#FAILED_FILES[@]}")
    summary+=("State directory: $STATE_DIR")
    summary+=("Manifest: $MANIFEST_FILE")
    summary+=("Logs: $LOG_DIR")

    if [[ "$DRY_RUN" == "true" ]]; then
        summary+=("Mode: DRY-RUN (no changes made)")
    fi

    log_summary "DebForge Installation" "${summary[@]}"

    if [[ ${#FAILED_FILES[@]} -eq 0 ]] && [[ "$DRY_RUN" != "true" ]]; then
        echo ""
        echo "Setup complete! A reboot is recommended to apply all changes."
        echo ""
        echo "To check status:  $SCRIPT_DIR/status.sh"
        echo "To uninstall:     $SCRIPT_DIR/uninstall.sh"
    fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Main
# ─────────────────────────────────────────────────────────────────────────────

main() {
    parse_args "$@"
    check_not_root

    mkdir -p "$LOG_DIR"
    log_init "install"

    log_section "DebForge Installation"
    log_info "Repository: $REPO_DIR"
    log_info "Configs: $CONFIGS_DIR"
    log_info "State: $STATE_DIR"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_warn "DRY-RUN MODE - No changes will be made"
        echo ""
    fi

    # Check existing install
    if ! check_existing_install; then
        exit 1
    fi

    # Run phases
    phase_preflight || exit 1
    phase_init
    phase_setup_scripts
    phase_deploy_configs
    phase_apply
    phase_verify
    phase_summary

    log_success "Installation completed!"
}

main "$@"
