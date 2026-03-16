#!/usr/bin/env bash
# install.sh - Main installer for DebForge
# Idempotent, phased installation with rollback support
#
# Usage: ./install.sh [OPTIONS]
# Note: Script handles sudo internally - do NOT run with sudo prefix

set -euo pipefail

# Script directory and project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONFIGS_DIR="$REPO_DIR/configs"
BIN_DIR="$REPO_DIR/bin"
SCRIPTS_BASE_DIR="$SCRIPT_DIR"

# State directory in user's home (no sudo needed for state)
STATE_DIR="$HOME/.local/share/debforge"
MANIFEST_FILE="$STATE_DIR/manifest.json"
BACKUP_DIR="$STATE_DIR/backups"
LOG_DIR="$STATE_DIR/logs"

# Source libraries
# shellcheck source=/dev/null
source "$SCRIPT_DIR/lib/logger.sh"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/lib/manifest.sh"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/lib/backup.sh"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/lib/verify.sh"

# State
DRY_RUN=false
FORCE=false
SKIP_VERIFY=false
SKIP_SCRIPTS=false
SKIP_CONFIGS=false
KERNEL_CHOICE=""
NVIDIA_VARIANT=""
PHASE="init"
INSTALLED_FILES=()
FAILED_FILES=()

# Parse arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --dry-run|-n)
                DRY_RUN=true
                log_info "Dry-run mode enabled"
                shift
                ;;
            --force|-f)
                FORCE=true
                log_info "Force mode enabled"
                shift
                ;;
            --skip-verify)
                SKIP_VERIFY=true
                log_info "Skipping verification"
                shift
                ;;
            --skip-scripts)
                SKIP_SCRIPTS=true
                log_info "Skipping setup scripts (configs only)"
                shift
                ;;
            --scripts-only)
                SKIP_CONFIGS=true
                log_info "Scripts only mode (skip configs)"
                shift
                ;;
            --kernel)
                KERNEL_CHOICE="$2"
                log_info "Kernel choice: $KERNEL_CHOICE"
                shift 2
                ;;
            --nvidia)
                NVIDIA_VARIANT="$2"
                log_info "NVIDIA variant: $NVIDIA_VARIANT"
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
DebForge Installer - System Optimization Configuration

Usage: $(basename "$0") [OPTIONS]

Options:
  --dry-run, -n         Preview changes without applying
  --force, -f           Force re-installation of existing configs
  --skip-scripts        Skip setup scripts (01-core to 05-misc)
  --scripts-only        Run setup scripts only, skip configs
  --skip-verify         Skip post-install verification
  --kernel TYPE         Kernel choice: backports|liquorix (non-interactive)
  --nvidia VARIANT      NVIDIA driver: nvidia-open|cuda-drivers (non-interactive)
  --quiet, -q           Suppress output (errors only)
  --verbose, -v         Enable debug output
  --help, -h            Show this help message

Installation Flow:
  1. Run setup scripts (01-core → 05-misc) - package installs, system setup
  2. Deploy configuration files (sysctl, udev, systemd, etc.)
  3. Apply and verify changes

Examples:
  $(basename "$0")                              # Full installation (interactive)
  $(basename "$0") --skip-scripts               # Configs only (skip setup scripts)
  $(basename "$0") --scripts-only               # Scripts only (skip configs)
  $(basename "$0") --kernel backports           # Non-interactive, backports kernel
  $(basename "$0") --kernel liquorix --nvidia nvidia-open  # Fully automated
  $(basename "$0") --dry-run                    # Preview all changes

Note: This script handles sudo internally. Do NOT run with sudo prefix.
EOF
}

# Check if running as root (we don't want that)
check_not_root() {
    if [[ $EUID -eq 0 ]]; then
        log_error "This script should NOT be run as root"
        log_info "The script will use sudo internally where needed"
        log_info "Run as: ./$(basename "$0")"
        exit 1
    fi
}

# Check if already installed
check_existing_install() {
    if manifest_exists && [[ "$FORCE" != "true" ]] && [[ "$SKIP_CONFIGS" != "true" ]]; then
        log_warn "DebForge appears to be already installed"
        log_info "Use --force to re-apply configurations"
        log_info "Use --dry-run to preview changes"

        if [[ "$DRY_RUN" == "true" ]]; then
            manifest_summary
            return 0
        fi

        log_info "Run with --force to re-apply, or run uninstall.sh first"
        return 1
    fi
}

# Phase 1: Pre-flight checks
phase_preflight() {
    PHASE="preflight"
    log_section "Phase 1: Pre-flight Checks"

    if ! preflight_checks; then
        log_fatal "Pre-flight checks failed"
    fi

    log_success "All pre-flight checks passed"
}

# Phase 2: Initialize manifest (user-level operation)
phase_init_manifest() {
    PHASE="init"
    log_section "Phase 2: Initialize Manifest"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY-RUN] Would create manifest"
        return 0
    fi

    # Create state directories (user-level)
    mkdir -p "$STATE_DIR" "$BACKUP_DIR" "$LOG_DIR"

    manifest_create "1.0"
    log_success "Manifest initialized: $MANIFEST_FILE"
}

# Phase 3: Run setup scripts (01-core → 05-misc)
phase_setup_scripts() {
    PHASE="scripts"

    if [[ "$SKIP_SCRIPTS" == "true" ]]; then
        log_section "Phase 3: Setup Scripts (skipped)"
        log_info "Skipping setup scripts (--skip-scripts or --configs-only)"
        return 0
    fi

    log_section "Phase 3: Running Setup Scripts"
    log_info "Running scripts in order: 01-core → 02-hardware → 03-desktop → 04-gaming → 05-misc"
    log_info "These scripts install packages and configure your system"
    log_info "Note: Some scripts may require interaction or take a long time"
    echo ""

    # Find all script directories in order
    local script_dirs=()
    for dir in "$SCRIPTS_BASE_DIR"/[0-9][0-9]-*/; do
        if [[ -d "$dir" ]]; then
            script_dirs+=("$dir")
        fi
    done

    if [[ ${#script_dirs[@]} -eq 0 ]]; then
        log_warn "No setup script directories found in $SCRIPTS_BASE_DIR"
        return 0
    fi

    local total_dirs=${#script_dirs[@]}
    local dir_index=0

    for script_dir in "${script_dirs[@]}"; do
        dir_index=$((dir_index + 1))
        local dir_name
        dir_name=$(basename "$script_dir")

        log_step "$dir_index" "$total_dirs" "Processing: $dir_name"

        # Find all .sh files in this directory, sorted
        local scripts=()
        while IFS= read -r -d '' script; do
            scripts+=("$script")
        done < <(find "$script_dir" -maxdepth 1 -name "*.sh" -type f -print0 | sort -z)

        if [[ ${#scripts[@]} -eq 0 ]]; then
            log_debug "  No scripts found in $dir_name"
            continue
        fi

        for script in "${scripts[@]}"; do
            local script_name
            script_name=$(basename "$script")

            if [[ "$DRY_RUN" == "true" ]]; then
                log_info "[DRY-RUN] Would run: $script"
                continue
            fi

            log_progress "Running: $script_name"
            echo ""

            # Make sure script is executable (use sudo if needed for system-owned files)
            if [ ! -x "$script" ]; then
                chmod +x "$script" 2>/dev/null || sudo chmod +x "$script" 2>/dev/null || true
            fi

            # Build arguments for specific scripts
            local script_args=()
            
            # Pass kernel choice to 01.03-kernel.sh
            if [[ "$script_name" == "01.03-kernel.sh" ]] && [[ -n "$KERNEL_CHOICE" ]]; then
                script_args+=("--kernel" "$KERNEL_CHOICE")
            fi
            
            # Pass NVIDIA variant to 02.02-nvidia.sh
            if [[ "$script_name" == "02.02-nvidia.sh" ]] && [[ -n "$NVIDIA_VARIANT" ]]; then
                script_args+=("--variant" "$NVIDIA_VARIANT")
            fi

            # Run the script (temporarily disable errexit for this subshell)
            set +e
            if [[ ${#script_args[@]} -gt 0 ]]; then
                log_debug "  Arguments: ${script_args[*]}"
                bash "$script" "${script_args[@]}"
            else
                bash "$script"
            fi
            local exit_code=$?
            set -e

            if [[ $exit_code -eq 0 ]]; then
                log_success "Completed: $script_name"
            else
                log_error "Failed: $script_name (exit code: $exit_code)"
                log_warn "Continuing to next script despite failure..."
            fi
            echo ""
        done
    done

    log_success "Setup scripts phase complete"
}

# Install a config file (requires sudo for system destinations)
install_config() {
    local src="$1"
    local dest="$2"
    local perms="$3"
    local type="${4:-config}"

    log_debug "Installing: $src -> $dest (mode: $perms)"

    if [[ "$DRY_RUN" == "true" ]]; then
        if [[ -f "$dest" ]]; then
            log_info "[DRY-RUN] Would backup and replace: $dest"
        else
            log_info "[DRY-RUN] Would install: $dest"
        fi
        return 0
    fi

    # Create destination directory (sudo if needed)
    local dest_dir
    dest_dir=$(dirname "$dest")
    sudo mkdir -p "$dest_dir"

    # Backup existing file (read with sudo, store in user dir)
    local backup_path=""
    if [[ -f "$dest" ]]; then
        local backup_name
        backup_name=$(basename "$dest" | tr '/' '_').bak
        backup_path="$BACKUP_DIR/$backup_name"
        sudo cp -p "$dest" "$backup_path"
        log_debug "Backup created: $backup_path"
    fi

    # Install file
    if sudo install -m "$perms" "$src" "$dest"; then
        # Calculate checksum
        local checksum
        checksum="sha256:$(sha256sum "$dest" | cut -d' ' -f1)"

        # Add to manifest (user-level)
        manifest_add_file "$dest" "$src" "$perms" "$type" "$checksum" "$backup_path"

        INSTALLED_FILES+=("$dest")
        log_debug "Installed: $dest"
        return 0
    else
        FAILED_FILES+=("$dest")
        log_error "Failed to install: $dest"
        return 1
    fi
}

# Install a systemd service unit (requires sudo)
install_service() {
    local name="$1"
    local content="$2"
    local dest="$3"
    local enabled="${4:-true}"

    log_debug "Installing service: $name -> $dest"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY-RUN] Would install service: $dest"
        return 0
    fi

    # Create destination directory
    local dest_dir
    dest_dir=$(dirname "$dest")
    sudo mkdir -p "$dest_dir"

    # Backup existing file
    local backup_path=""
    if [[ -f "$dest" ]]; then
        local backup_name
        backup_name=$(basename "$dest" | tr '/' '_').bak
        backup_path="$BACKUP_DIR/$backup_name"
        sudo cp -p "$dest" "$backup_path"
    fi

    # Write service file using sudo tee
    if echo "$content" | sudo tee "$dest" > /dev/null && sudo chmod 0644 "$dest"; then
        # Add to manifest
        manifest_add_file "$dest" "inline" "0644" "service" "" "$backup_path"
        manifest_add_service "$name" "$dest" "$enabled"

        INSTALLED_FILES+=("$dest")
        log_debug "Installed service: $dest"
        return 0
    else
        FAILED_FILES+=("$dest")
        log_error "Failed to install service: $dest"
        return 1
    fi
}

# Phase 4: Install configurations
phase_install_configs() {
    PHASE="install"

    if [[ "$SKIP_CONFIGS" == "true" ]]; then
        log_section "Phase 4: Installing Configurations (skipped)"
        log_info "Skipping configs (--scripts-only)"
        return 0
    fi

    log_section "Phase 4: Installing Configurations"
    echo ""

    local total_steps=14
    local current_step=0

    # ── 1. Sysctl ───────────────────────────────────────────────────────────────
    current_step=$((current_step + 1))
    log_step "$current_step" "$total_steps" "Sysctl tuning (VM, kernel, network, filesystem)"
    if [[ -f "$CONFIGS_DIR/sysctl.d/99-debforge.conf" ]]; then
        install_config "$CONFIGS_DIR/sysctl.d/99-debforge.conf" \
            /etc/sysctl.d/99-debforge.conf 0644 "sysctl" || log_warn "Failed to install sysctl config"
    fi

    # ── 2. Udev rules ──────────────────────────────────────────────────────────
    current_step=$((current_step + 1))
    log_step "$current_step" "$total_steps" "Udev rules (I/O schedulers, audio PM, ZRAM, etc.)"
    for rule in "$CONFIGS_DIR"/udev/*.rules; do
        [[ -f "$rule" ]] || continue
        install_config "$rule" "/usr/lib/udev/rules.d/$(basename "$rule")" 0644 "udev" || log_warn "Failed to install: $rule"
    done

    # ── 3. Modprobe configs ────────────────────────────────────────────────────
    current_step=$((current_step + 1))
    log_step "$current_step" "$total_steps" "Modprobe (NVIDIA tuning, watchdog blacklist)"
    for conf in "$CONFIGS_DIR"/modprobe.d/*.conf; do
        [[ -f "$conf" ]] || continue
        install_config "$conf" "/usr/lib/modprobe.d/$(basename "$conf")" 0644 "modprobe" || log_warn "Failed to install: $conf"
    done

    # ── 4. Tmpfiles ────────────────────────────────────────────────────────────
    current_step=$((current_step + 1))
    log_step "$current_step" "$total_steps" "Tmpfiles (THP tuning, coredump cleanup, KSM)"
    for conf in "$CONFIGS_DIR"/tmpfiles.d/*.conf; do
        [[ -f "$conf" ]] || continue
        install_config "$conf" "/usr/lib/tmpfiles.d/$(basename "$conf")" 0644 "tmpfiles" || log_warn "Failed to install: $conf"
    done

    # ── 5. Systemd manager configs ─────────────────────────────────────────────
    current_step=$((current_step + 1))
    log_step "$current_step" "$total_steps" "Systemd timeouts and limits"
    sudo mkdir -p /usr/lib/systemd/system.conf.d /usr/lib/systemd/user.conf.d || true
    for conf in "$CONFIGS_DIR"/systemd/system.conf.d/*.conf; do
        [[ -f "$conf" ]] || continue
        install_config "$conf" "/usr/lib/systemd/system.conf.d/$(basename "$conf")" 0644 "systemd" || log_warn "Failed to install: $conf"
    done
    for conf in "$CONFIGS_DIR"/systemd/user.conf.d/*.conf; do
        [[ -f "$conf" ]] || continue
        install_config "$conf" "/usr/lib/systemd/user.conf.d/$(basename "$conf")" 0644 "systemd" || log_warn "Failed to install: $conf"
    done

    # ── 6. Journald ────────────────────────────────────────────────────────────
    current_step=$((current_step + 1))
    log_step "$current_step" "$total_steps" "Journald configuration"
    sudo mkdir -p /usr/lib/systemd/journald.conf.d || true
    if [[ -f "$CONFIGS_DIR/systemd/journald.conf.d/00-journal-size.conf" ]]; then
        install_config "$CONFIGS_DIR/systemd/journald.conf.d/00-journal-size.conf" \
            /usr/lib/systemd/journald.conf.d/00-journal-size.conf 0644 "systemd" || log_warn "Failed to install journald config"
    fi

    # ── 7. Timesyncd ───────────────────────────────────────────────────────────
    current_step=$((current_step + 1))
    log_step "$current_step" "$total_steps" "Timesyncd (Cloudflare NTP)"
    sudo mkdir -p /usr/lib/systemd/timesyncd.conf.d || true
    if [[ -f "$CONFIGS_DIR/systemd/timesyncd.conf.d/10-timesyncd.conf" ]]; then
        install_config "$CONFIGS_DIR/systemd/timesyncd.conf.d/10-timesyncd.conf" \
            /usr/lib/systemd/timesyncd.conf.d/10-timesyncd.conf 0644 "systemd" || log_warn "Failed to install timesyncd config"
    fi

    # ── 8. Resolved (DNS-over-TLS) ─────────────────────────────────────────────
    current_step=$((current_step + 1))
    log_step "$current_step" "$total_steps" "Resolved (DNS-over-TLS)"
    sudo mkdir -p /etc/systemd/resolved.conf.d || true
    if [[ -f "$CONFIGS_DIR/systemd/resolved.conf.d/99-dot.conf" ]]; then
        install_config "$CONFIGS_DIR/systemd/resolved.conf.d/99-dot.conf" \
            /etc/systemd/resolved.conf.d/99-dot.conf 0644 "systemd" || log_warn "Failed to install resolved config"
    fi

    # ── 9. Security limits ─────────────────────────────────────────────────────
    current_step=$((current_step + 1))
    log_step "$current_step" "$total_steps" "Security limits (audio RT priority)"
    sudo mkdir -p /etc/security/limits.d || true
    if [[ -f "$CONFIGS_DIR/security/20-audio.conf" ]]; then
        install_config "$CONFIGS_DIR/security/20-audio.conf" \
            /etc/security/limits.d/20-audio.conf 0644 "security" || log_warn "Failed to install security limits"
    fi

    # ── 10. ZRAM ───────────────────────────────────────────────────────────────
    current_step=$((current_step + 1))
    log_step "$current_step" "$total_steps" "ZRAM configuration"
    if command -v apt &>/dev/null; then
        if ! dpkg -l systemd-zram-generator &>/dev/null; then
            log_progress "Installing systemd-zram-generator package"
            sudo apt install -y -qq systemd-zram-generator || log_warn "Failed to install zram-generator"
        fi
    fi
    if [[ -f "$CONFIGS_DIR/systemd/zram-generator.conf" ]]; then
        install_config "$CONFIGS_DIR/systemd/zram-generator.conf" \
            /etc/systemd/zram-generator.conf 0644 "zram" || log_warn "Failed to install zram config"
    fi

    # ── 11. EarlyOOM ───────────────────────────────────────────────────────────
    current_step=$((current_step + 1))
    log_step "$current_step" "$total_steps" "EarlyOOM configuration"
    if command -v apt &>/dev/null; then
        if ! dpkg -l earlyoom &>/dev/null; then
            log_progress "Installing earlyoom package"
            sudo apt install -y -qq earlyoom || log_warn "Failed to install earlyoom"
        fi
    fi
    if [[ -f "$CONFIGS_DIR/earlyoom" ]]; then
        install_config "$CONFIGS_DIR/earlyoom" /etc/default/earlyoom 0644 "earlyoom" || log_warn "Failed to install earlyoom config"
    fi

    # ── 12. Binaries ───────────────────────────────────────────────────────────
    current_step=$((current_step + 1))
    log_step "$current_step" "$total_steps" "Installing binaries"
    for bin in pci-latency ksmctl game-performance; do
        if [[ -f "$BIN_DIR/$bin" ]]; then
            install_config "$BIN_DIR/$bin" "/usr/local/bin/$bin" 0755 "binary" || log_warn "Failed to install: $bin"
        fi
    done

    # ── 13. Systemd Services (inline) ──────────────────────────────────────────
    current_step=$((current_step + 1))
    log_step "$current_step" "$total_steps" "Installing systemd services"

    # PCI Latency Service
    install_service "pci-latency" '[Unit]
Description=Adjust PCI latency timers for audio and devices

[Service]
Type=oneshot
ExecStart=/usr/local/bin/pci-latency

[Install]
WantedBy=multi-user.target' \
        "/etc/systemd/system/pci-latency.service" || log_warn "Failed to install pci-latency service"

    # Set Min Free Memory Service
    install_service "set-min-free-mem" '[Unit]
Description=Set vm.min_free_kbytes dynamically
DefaultDependencies=no
After=local-fs.target
Before=sysinit.target

[Service]
Type=oneshot
ExecStart=/bin/sh -c "sysctl -w vm.min_free_kbytes=$(awk '"'"'/MemTotal/ {printf "%0.f", $2 * 0.01}'"'"' /proc/meminfo)"

[Install]
WantedBy=sysinit.target' \
        "/etc/systemd/system/set-min-free-mem.service" || log_warn "Failed to install set-min-free-mem service"

    # ── 14. Environment config ─────────────────────────────────────────────────
    current_step=$((current_step + 1))
    log_step "$current_step" "$total_steps" "Environment configuration"
    if [[ ! -f /etc/environment.d/90-debforge-env.conf ]]; then
        local env_dest="/etc/environment.d/90-debforge-env.conf"
        if [[ "$DRY_RUN" != "true" ]]; then
            sudo mkdir -p /etc/environment.d || true
            if sudo tee "$env_dest" > /dev/null << 'ENV'
# Proton Wayland support
PROTON_ENABLE_WAYLAND=1
# NVIDIA shader disk cache — 12 GB
__GL_SHADER_DISK_CACHE_SIZE=12000000000
# Intel iGPU Mesa shader cache
MESA_SHADER_CACHE_MAX_SIZE=4G
# Force GL renderer for GTK4 apps (GNOME apps on KDE, etc.).
GSK_RENDERER=gl
ENV
            then
                sudo chmod 0644 "$env_dest" || true
                manifest_add_file "$env_dest" "inline" "0644" "environment" "" "" || true
                INSTALLED_FILES+=("$env_dest")
                log_debug "Installed: $env_dest"
            else
                log_warn "Failed to create environment config"
            fi
        else
            log_debug "Environment config already exists, skipping"
        fi
    fi

    log_success "Configuration installation complete"
}

# Install a home config file (no sudo needed - user's own config)
install_home_config() {
    local src="$1"
    local dest="$2"
    local perms="${3:-0644}"

    log_debug "Installing home config: $src -> $dest"

    if [[ "$DRY_RUN" == "true" ]]; then
        if [[ -f "$dest" ]]; then
            log_info "[DRY-RUN] Would backup and replace: $dest"
        else
            log_info "[DRY-RUN] Would install: $dest"
        fi
        return 0
    fi

    # Create destination directory
    local dest_dir
    dest_dir=$(dirname "$dest")
    mkdir -p "$dest_dir"

    # Backup existing file (no sudo needed)
    local backup_path=""
    if [[ -f "$dest" ]]; then
        local backup_name
        backup_name=$(basename "$dest" | tr '/' '_').bak
        backup_path="$BACKUP_DIR/$backup_name"
        cp -p "$dest" "$backup_path"
        log_debug "Backup created: $backup_path"
    fi

    # Install file
    if install -m "$perms" "$src" "$dest"; then
        # Calculate checksum
        local checksum
        checksum="sha256:$(sha256sum "$dest" | cut -d' ' -f1)"

        # Add to manifest as home config
        manifest_add_home_config "$dest" "$src" "$perms" "$checksum" "$backup_path"

        INSTALLED_FILES+=("$dest")
        log_debug "Installed home config: $dest"
        return 0
    else
        FAILED_FILES+=("$dest")
        log_error "Failed to install home config: $dest"
        return 1
    fi
}

# Phase 4b: Install home directory configurations
phase_install_home_configs() {
    PHASE="home_configs"

    if [[ "$SKIP_CONFIGS" == "true" ]]; then
        log_section "Phase 4b: Home Configurations (skipped)"
        return 0
    fi

    log_section "Phase 4b: Installing Home Directory Configurations"
    echo ""

    local total_steps=4
    local current_step=0

    # ── 1. WirePlumber ─────────────────────────────────────────────────────────
    current_step=$((current_step + 1))
    log_step "$current_step" "$total_steps" "WirePlumber audio configs"
    mkdir -p "$HOME/.config/wireplumber/wireplumber.conf.d"
    for conf in "$CONFIGS_DIR/home/wireplumber"/*.conf; do
        [[ -f "$conf" ]] || continue
        local dest="$HOME/.config/wireplumber/wireplumber.conf.d/$(basename "$conf")"
        install_home_config "$conf" "$dest" 0644 || log_warn "Failed to install: $conf"
    done

    # ── 2. PipeWire ────────────────────────────────────────────────────────────
    current_step=$((current_step + 1))
    log_step "$current_step" "$total_steps" "PipeWire configs"
    mkdir -p "$HOME/.config/pipewire/pipewire.conf.d" "$HOME/.config/pipewire/pipewire-pulse.conf.d"
    if [[ -f "$CONFIGS_DIR/home/pipewire/10-clock.conf" ]]; then
        install_home_config "$CONFIGS_DIR/home/pipewire/10-clock.conf" \
            "$HOME/.config/pipewire/pipewire.conf.d/10-clock.conf" 0644 || log_warn "Failed to install clock config"
    fi
    if [[ -f "$CONFIGS_DIR/home/pipewire/10-pulse.conf" ]]; then
        install_home_config "$CONFIGS_DIR/home/pipewire/10-pulse.conf" \
            "$HOME/.config/pipewire/pipewire-pulse.conf.d/10-pulse.conf" 0644 || log_warn "Failed to install pulse config"
    fi

    # ── 3. KWin (KDE compositor) ───────────────────────────────────────────────
    current_step=$((current_step + 1))
    log_step "$current_step" "$total_steps" "KWin compositor settings"
    if [[ -f "$CONFIGS_DIR/home/kwinrc" ]]; then
        # Merge with existing kwinrc if it exists
        local kwin_dest="$HOME/.config/kwinrc"
        if [[ -f "$kwin_dest" ]]; then
            log_debug "Merging with existing kwinrc"
            # Backup first
            local backup_path="$BACKUP_DIR/kwinrc.bak"
            cp -p "$kwin_dest" "$backup_path"
            manifest_add_home_config "$kwin_dest" "backup" "0644" "" "$backup_path"
        fi
        install_home_config "$CONFIGS_DIR/home/kwinrc" "$kwin_dest" 0644 || log_warn "Failed to install kwinrc"
    fi

    # ── 4. Baloo (KDE file indexing) ───────────────────────────────────────────
    current_step=$((current_step + 1))
    log_step "$current_step" "$total_steps" "Baloo file indexing config"
    if [[ -f "$CONFIGS_DIR/home/baloofilerc" ]]; then
        install_home_config "$CONFIGS_DIR/home/baloofilerc" \
            "$HOME/.config/baloofilerc" 0644 || log_warn "Failed to install baloofilerc"
    fi

    log_success "Home configuration installation complete"
}

# Phase 5: Apply changes (requires sudo for system operations)
phase_apply_changes() {
    PHASE="apply"
    
    if [[ "$SKIP_CONFIGS" == "true" ]]; then
        log_section "Phase 5: Applying Changes (skipped)"
        return 0
    fi
    
    log_section "Phase 5: Applying Changes"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY-RUN] Would apply system changes"
        return 0
    fi

    # Reload udev rules
    log_progress "Reloading udev rules"
    sudo udevadm control --reload-rules 2>/dev/null || log_warn "Failed to reload udev rules"

    # Apply sysctl
    log_progress "Applying sysctl settings"
    sudo sysctl --system 2>/dev/null || log_warn "Failed to apply sysctl settings"

    # Reload systemd
    log_progress "Reloading systemd daemon"
    sudo systemctl daemon-reload 2>/dev/null || log_warn "Failed to reload systemd"

    # Enable services
    log_progress "Enabling systemd services"
    sudo systemctl enable pci-latency.service 2>/dev/null || log_warn "Failed to enable pci-latency"
    sudo systemctl enable set-min-free-mem.service 2>/dev/null || log_warn "Failed to enable set-min-free-mem"
    sudo systemctl enable earlyoom 2>/dev/null || log_warn "Failed to enable earlyoom"

    # Restart services
    log_progress "Restarting services"
    sudo systemctl restart systemd-journald 2>/dev/null || true
    sudo systemctl restart systemd-timesyncd 2>/dev/null || true
    sudo systemctl restart systemd-resolved 2>/dev/null || true

    log_success "Changes applied"
}

# Phase 6: Verification
phase_verify() {
    PHASE="verify"

    if [[ "$SKIP_VERIFY" == "true" ]]; then
        log_info "Skipping verification"
        return 0
    fi

    log_section "Phase 6: Verification"

    if ! post_install_verify; then
        log_warn "Some verifications failed - check logs"
    fi
}

# Phase 7: Finalize
phase_finalize() {
    PHASE="finalize"
    log_section "Installation Complete"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY-RUN] Installation preview complete"
        return 0
    fi

    # Update manifest with final state
    for file in "${INSTALLED_FILES[@]}"; do
        manifest_update_status "$file" "applied" 2>/dev/null || true
    done

    for file in "${FAILED_FILES[@]}"; do
        manifest_update_status "$file" "failed" 2>/dev/null || true
    done

    # Create state marker (user-level)
    touch "$STATE_DIR/installed"

    local summary_items=()
    summary_items+=("Files installed: ${#INSTALLED_FILES[@]}")
    summary_items+=("Files failed: ${#FAILED_FILES[@]}")
    summary_items+=("Manifest: $MANIFEST_FILE")
    summary_items+=("Log: $LOG_FILE")

    if [[ ${#FAILED_FILES[@]} -gt 0 ]]; then
        log_warn "Some files failed to install"
        for file in "${FAILED_FILES[@]}"; do
            summary_items+=("  Failed: $file")
        done
    fi

    log_summary "Installation Summary" "${summary_items[@]}"

    echo ""
    echo "A reboot is recommended to apply all changes."
    echo "To uninstall: $REPO_DIR/scripts/uninstall.sh"
    echo "To check status: $REPO_DIR/scripts/status.sh"
}

# Rollback on failure
rollback() {
    log_error "Installation failed during phase: $PHASE"
    log_info "Rolling back changes..."

    for file in "${INSTALLED_FILES[@]}"; do
        local backup_name
        backup_name=$(basename "$file" | tr '/' '_').bak
        local backup_path="$BACKUP_DIR/$backup_name"
        
        if [[ -f "$backup_path" ]]; then
            log_progress "Restoring: $file"
            sudo cp -p "$backup_path" "$file" || true
        else
            log_progress "Removing: $file"
            sudo rm -f "$file" || true
        fi
    done

    log_warn "Rollback complete - system restored to previous state"
}

# Main
main() {
    parse_args "$@"

    # Safety check: don't run as root
    check_not_root

    # Initialize logging (user-level)
    mkdir -p "$LOG_DIR"
    log_init "install"

    log_section "DebForge Installer"
    log_info "Repository: $REPO_DIR"
    log_info "Configs: $CONFIGS_DIR"
    log_info "Binaries: $BIN_DIR"
    log_info "Scripts: $SCRIPTS_BASE_DIR"
    log_info "State: $STATE_DIR"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_warn "DRY-RUN MODE - No changes will be made"
    fi

    # Check existing install (only for configs phase)
    if [[ "$SKIP_CONFIGS" != "true" ]]; then
        if ! check_existing_install; then
            if [[ "$FORCE" != "true" ]] && [[ "$DRY_RUN" != "true" ]]; then
                log_info "Exiting without applying configs."
                log_info "Use --force to re-apply configs."
                exit 0
            fi
        fi
    fi

    # Set trap for rollback on failure (only for configs phase)
    if [[ "$SKIP_CONFIGS" != "true" ]]; then
        trap rollback ERR
    fi

    # Run phases
    phase_preflight
    phase_init_manifest
    phase_setup_scripts
    phase_install_configs
    phase_install_home_configs
    phase_apply_changes
    phase_verify
    phase_finalize

    # Clear trap on success
    if [[ "$SKIP_CONFIGS" != "true" ]]; then
        trap - ERR
    fi

    log_success "Installation completed successfully"
}

main "$@"
