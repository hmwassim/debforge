#!/usr/bin/env bash
# uninstall.sh - Clean removal for DebForge
# Restores backed up files and removes installed configurations
#
# Usage: ./uninstall.sh [OPTIONS]
# Note: Script handles sudo internally - do NOT run with sudo prefix

set -euo pipefail

# Script directory and project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# State directory in user's home (matches install.sh)
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

# State
DRY_RUN=false
FORCE=false
KEEP_BACKUPS=false
UNINSTALL_MODE="interactive"  # interactive|debforge-only|system|full

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
            --keep-backups)
                KEEP_BACKUPS=true
                log_info "Will keep backup files"
                shift
                ;;
            --debforge-only)
                UNINSTALL_MODE="debforge-only"
                log_info "Mode: Remove DebForge only (keep all configs)"
                shift
                ;;
            --system)
                UNINSTALL_MODE="system"
                log_info "Mode: Remove DebForge + system configs"
                shift
                ;;
            --full)
                UNINSTALL_MODE="full"
                log_info "Mode: Full cleanup (remove everything)"
                shift
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
DebForge Uninstaller - Clean Configuration Removal

Usage: $(basename "$0") [OPTIONS]

Options:
  --dry-run, -n       Preview changes without applying
  --force, -f         Force removal even without manifest
  --keep-backups      Keep backup files after uninstall
  --debforge-only     Remove DebForge but keep all configs
  --system            Remove DebForge + system configs (default)
  --full              Remove everything (system + home configs)
  --quiet, -q         Suppress output (errors only)
  --verbose, -v       Enable debug output
  --help, -h          Show this help message

Uninstall Modes:
  interactive         Prompt for uninstall mode (default)
  debforge-only       Keep all system and home configs
  system              Remove system configs, keep home configs
  full                Remove everything (system + home configs)

Examples:
  $(basename "$0")                      # Interactive mode (prompts you)
  $(basename "$0") --debforge-only      # Keep all configs
  $(basename "$0") --system             # Remove system configs (default)
  $(basename "$0") --full               # Remove everything
  $(basename "$0") --dry-run            # Preview changes

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

# Check if manifest exists
check_manifest() {
    if ! manifest_exists; then
        log_warn "No manifest found at $MANIFEST_FILE"
        
        # Check if /opt/debforge exists
        if [[ -d "/opt/debforge" ]]; then
            log_info "DebForge installation detected but manifest is missing"
            log_info "This can happen if the installation was incomplete or corrupted"
            echo ""
            echo "Options:"
            echo "  1. Use --force to remove /opt/debforge and /usr/local/bin/debforge"
            echo "  2. Re-run install.sh to recreate manifest, then uninstall"
            echo "  3. Manually remove: sudo rm -rf /opt/debforge /usr/local/bin/debforge"
            echo ""
            log_info "Note: Without manifest, system config restoration may be incomplete"
        else
            log_error "No manifest found at $MANIFEST_FILE"
            log_info "Cannot uninstall without manifest - it tracks what was installed"
            echo ""
            echo "Options:"
            echo "  1. Use --force to attempt removal without manifest"
            echo "  2. Re-run install.sh to recreate manifest, then uninstall"
            echo "  3. Manually remove configurations"
        fi
        exit 1
    fi
}

# Interactive mode selection
prompt_uninstall_mode() {
    if [[ "$UNINSTALL_MODE" != "interactive" ]]; then
        return 0
    fi

    if [[ "$DRY_RUN" == "true" ]] || [[ "$FORCE" == "true" ]]; then
        UNINSTALL_MODE="system"
        return 0
    fi

    echo ""
    echo "How do you want to uninstall DebForge?"
    echo ""
    echo "  1) Remove DebForge only"
    echo "     - Remove /usr/local/bin/debforge and /opt/debforge"
    echo "     - Keep all system configs (recommended if you want to keep tweaks)"
    echo ""
    echo "  2) Remove DebForge + undo system configs"
    echo "     - Everything in option 1"
    echo "     - Restore /etc configs from backup"
    echo "     - Remove systemd services, udev rules, etc."
    echo "     - Keep home directory configs (~/.config)"
    echo ""
    echo "  3) Full cleanup (remove everything)"
    echo "     - Everything in option 2"
    echo "     - Remove ~/.config/debforge, ~/.local/share/debforge"
    echo "     - Remove all user-level configs"
    echo ""
    
    while true; do
        read -rp "Select option (1-3): " choice
        case "$choice" in
            1)
                UNINSTALL_MODE="debforge-only"
                break
                ;;
            2)
                UNINSTALL_MODE="system"
                break
                ;;
            3)
                UNINSTALL_MODE="full"
                break
                ;;
            *)
                echo "Please enter 1, 2, or 3"
                ;;
        esac
    done
    
    echo ""
    case "$UNINSTALL_MODE" in
        debforge-only)
            log_info "Selected: Remove DebForge only (keep all configs)"
            ;;
        system)
            log_info "Selected: Remove DebForge + system configs"
            ;;
        full)
            log_info "Selected: Full cleanup (remove everything)"
            ;;
    esac
}

# Remove home config files (no sudo needed)
remove_home_configs() {
    log_section "Removing Home Directory Configs"

    if ! manifest_exists; then
        log_warn "No manifest - skipping home config removal"
        return 0
    fi

    local restored=0
    local removed=0

    while IFS= read -r config_json; do
        [[ -z "$config_json" ]] && continue

        local dest backup_path
        dest=$(echo "$config_json" | jq -r '.dest')
        backup_path=$(echo "$config_json" | jq -r '.backup // empty')

        [[ -z "$dest" ]] && continue

        if [[ "$DRY_RUN" == "true" ]]; then
            if [[ -n "$backup_path" ]] && [[ -f "$backup_path" ]]; then
                log_info "[DRY-RUN] Would restore: $dest (from backup)"
            else
                log_info "[DRY-RUN] Would remove: $dest (no backup)"
            fi
            continue
        fi

        # Restore or remove
        if [[ -n "$backup_path" ]] && [[ -f "$backup_path" ]]; then
            log_progress "Restoring: $dest"
            if restore_home_config "$dest" "$backup_path"; then
                ((restored++))
            else
                log_error "Failed to restore: $dest"
            fi
        elif [[ -f "$dest" ]]; then
            log_progress "Removing: $dest"
            if remove_home_config "$dest"; then
                ((removed++))
            else
                log_error "Failed to remove: $dest"
            fi
        fi
    done < <(manifest_get_home_configs)

    echo ""
    log_info "Home configs restored: $restored"
    log_info "Home configs removed: $removed"
}

# Remove home state directory
# For full mode: removes entire STATE_DIR (including logs)
# For other modes: only removes legacy files outside STATE_DIR
remove_home_state() {
    log_section "Removing Home State Directory"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY-RUN] Would remove: $STATE_DIR"
        return 0
    fi

    if [[ "$UNINSTALL_MODE" == "full" ]]; then
        # Full mode: remove entire state directory
        if [[ -d "$STATE_DIR" ]]; then
            rm -rf "$STATE_DIR"
            log_progress "Removed: $STATE_DIR"
        fi
        # Clear LOG_FILE to prevent further writes
        LOG_FILE=""
    else
        # Other modes: only remove legacy files outside STATE_DIR
        for legacy_file in "$HOME/.local/.debforge.state" "$HOME/.local/.debforge.manifest"; do
            if [[ -f "$legacy_file" ]]; then
                rm -f "$legacy_file"
                log_progress "Removed legacy: $legacy_file"
            fi
        done
    fi
}

# Final cleanup - ensure logging stops cleanly
# This should be the LAST function called before print_summary
cleanup_final() {
    # For non-full modes, just remove the state marker
    if [[ "$UNINSTALL_MODE" == "system" ]] || [[ "$UNINSTALL_MODE" == "debforge-only" ]]; then
        if [[ -f "$STATE_DIR/installed" ]]; then
            rm -f "$STATE_DIR/installed"
        fi
    fi
    # For full mode, LOG_FILE was already cleared in remove_home_state
}

# Stop and disable services (requires sudo)
stop_services() {
    log_section "Stopping Services"

    local services=(
        "pci-latency.service"
        "set-min-free-mem.service"
        "earlyoom"
    )

    for service in "${services[@]}"; do
        if [[ "$DRY_RUN" == "true" ]]; then
            log_info "[DRY-RUN] Would stop and disable: $service"
            continue
        fi

        if sudo systemctl is-active "$service" &>/dev/null; then
            log_progress "Stopping: $service"
            sudo systemctl stop "$service" 2>/dev/null || log_warn "Failed to stop: $service"
        fi

        if sudo systemctl is-enabled "$service" &>/dev/null; then
            log_progress "Disabling: $service"
            sudo systemctl disable "$service" 2>/dev/null || log_warn "Failed to disable: $service"
        fi
    done
}

# Restore files from backup (requires sudo for system destinations)
restore_files() {
    log_section "Restoring Files"

    if ! manifest_exists; then
        log_warn "No manifest - skipping file restoration"
        return 0
    fi

    local restored=0
    local removed=0
    local skipped=0

    # Read files from manifest
    while IFS= read -r file_json; do
        [[ -z "$file_json" ]] && continue

        local dest backup_path file_type
        dest=$(echo "$file_json" | jq -r '.dest')
        backup_path=$(echo "$file_json" | jq -r '.backup // empty')
        file_type=$(echo "$file_json" | jq -r '.type // "unknown"')

        [[ -z "$dest" ]] && continue

        if [[ "$DRY_RUN" == "true" ]]; then
            if [[ -n "$backup_path" ]] && [[ -f "$backup_path" ]]; then
                log_info "[DRY-RUN] Would restore: $dest (from backup)"
            else
                log_info "[DRY-RUN] Would remove: $dest (no backup)"
            fi
            continue
        fi

        # Restore or remove
        if [[ -n "$backup_path" ]] && [[ -f "$backup_path" ]]; then
            log_progress "Restoring: $dest"
            if sudo cp -p "$backup_path" "$dest"; then
                ((restored++))
            else
                log_error "Failed to restore: $dest"
            fi
        elif [[ -f "$dest" ]]; then
            log_progress "Removing: $dest"
            if sudo rm -f "$dest"; then
                ((removed++))
            else
                log_error "Failed to remove: $dest"
            fi
        else
            log_debug "Already absent: $dest"
            ((skipped++))
        fi
    done < <(jq -c '.files[]' "$MANIFEST_FILE" 2>/dev/null)

    echo ""
    log_info "Files restored: $restored"
    log_info "Files removed: $removed"
    log_info "Files skipped: $skipped"
}

# Remove service files (requires sudo)
remove_services() {
    log_section "Removing Service Files"

    local service_files=(
        "/etc/systemd/system/pci-latency.service"
        "/etc/systemd/system/set-min-free-mem.service"
    )

    for service_file in "${service_files[@]}"; do
        if [[ "$DRY_RUN" == "true" ]]; then
            log_info "[DRY-RUN] Would remove: $service_file"
            continue
        fi

        if [[ -f "$service_file" ]]; then
            log_progress "Removing: $service_file"
            sudo rm -f "$service_file"
        else
            log_debug "Already absent: $service_file"
        fi
    done
}

# Remove binaries (requires sudo)
remove_binaries() {
    log_section "Removing Binaries"

    local binaries=(
        "/usr/local/bin/ksmctl"
        "/usr/local/bin/pci-latency"
        "/usr/local/bin/game-performance"
    )

    for bin in "${binaries[@]}"; do
        if [[ "$DRY_RUN" == "true" ]]; then
            log_info "[DRY-RUN] Would remove: $bin"
            continue
        fi

        if [[ -f "$bin" ]]; then
            log_progress "Removing: $bin"
            sudo rm -f "$bin"
        else
            log_debug "Already absent: $bin"
        fi
    done
}

# Clean backup files (user-level operation)
clean_backups() {
    if [[ "$KEEP_BACKUPS" == "true" ]]; then
        log_info "Keeping backup files as requested"
        return 0
    fi

    log_section "Cleaning Backups"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY-RUN] Would remove backup files"
        return 0
    fi

    local count=0
    for backup in "$BACKUP_DIR"/*.bak; do
        if [[ -f "$backup" ]]; then
            rm -f "$backup"
            ((count++))
        fi
    done

    log_info "Cleaned $count backup files"
}

# Reload system services (requires sudo)
reload_system() {
    log_section "Reloading System"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY-RUN] Would reload system services"
        return 0
    fi

    log_progress "Reloading systemd daemon"
    sudo systemctl daemon-reload 2>/dev/null || log_warn "Failed to reload systemd"

    log_progress "Reloading udev rules"
    sudo udevadm control --reload-rules 2>/dev/null || log_warn "Failed to reload udev"

    # Only apply sysctl if we restored a backup
    # Otherwise, just remove our config and let defaults take over
    if [[ -f /etc/sysctl.d/99-debforge.conf ]]; then
        log_progress "Applying sysctl settings (from restored backup)"
        sudo sysctl --system 2>/dev/null || log_warn "Failed to apply sysctl"
    else
        log_progress "Sysctl config removed - defaults will apply on next boot"
    fi
}

# Cleanup state files (user-level operation)
cleanup_state() {
    log_section "Cleaning State Files"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY-RUN] Would remove state files"
        return 0
    fi

    # Remove manifest
    if [[ -f "$MANIFEST_FILE" ]]; then
        rm -f "$MANIFEST_FILE"
        log_progress "Removed manifest"
    fi

    # Remove state marker
    if [[ -f "$STATE_DIR/installed" ]]; then
        rm -f "$STATE_DIR/installed"
        log_progress "Removed state marker"
    fi

    # Remove old state file (from legacy install)
    if [[ -f "$HOME/.local/.debforge.state" ]]; then
        rm -f "$HOME/.local/.debforge.state"
        log_progress "Removed legacy state file"
    fi

    # Remove legacy manifest
    if [[ -f "$HOME/.local/.debforge.manifest" ]]; then
        rm -f "$HOME/.local/.debforge.manifest"
        log_progress "Removed legacy manifest"
    fi
}

# Print summary
print_summary() {
    log_section "Uninstall Complete"

    local summary_items=()
    summary_items+=("DebForge configurations removed")

    if [[ "$KEEP_BACKUPS" == "true" ]]; then
        summary_items+=("Backups preserved in: $BACKUP_DIR")
    else
        summary_items+=("Backups cleaned")
    fi

    summary_items+=("A reboot is recommended")

    log_summary "Summary" "${summary_items[@]}"
}

# Main
main() {
    parse_args "$@"

    # Safety check: don't run as root
    check_not_root

    # Initialize logging
    mkdir -p "$LOG_DIR"
    log_init "uninstall"

    log_section "DebForge Uninstaller"
    log_info "Repository: $REPO_DIR"
    log_info "State: $STATE_DIR"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_warn "DRY-RUN MODE - No changes will be made"
    fi

    # Prompt for uninstall mode (if interactive)
    prompt_uninstall_mode

    # Check manifest (unless force)
    if [[ "$FORCE" != "true" ]] && [[ "$UNINSTALL_MODE" != "debforge-only" ]]; then
        check_manifest
    fi

    # Always stop services and remove DebForge binaries
    stop_services
    remove_binaries

    # Remove system configs based on mode
    if [[ "$UNINSTALL_MODE" == "system" ]] || [[ "$UNINSTALL_MODE" == "full" ]]; then
        restore_files
        remove_services
        reload_system
    fi

    # Remove home configs and state based on mode
    if [[ "$UNINSTALL_MODE" == "full" ]]; then
        remove_home_configs
        remove_home_state  # This clears LOG_FILE internally
    fi

    # Clean backups (unless kept)
    if [[ "$UNINSTALL_MODE" != "debforge-only" ]]; then
        clean_backups
    fi

    # Final cleanup (state marker for non-full modes)
    cleanup_final

    # Print summary (after cleanup to avoid log file issues in full mode)
    print_summary

    # Log success only if LOG_FILE is still set and exists
    if [[ -n "$LOG_FILE" ]] && [[ -f "$LOG_FILE" ]]; then
        log_success "Uninstall completed successfully"
    fi
}

main "$@"
