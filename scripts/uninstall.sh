#!/usr/bin/env bash
# uninstall.sh - DebForge uninstaller
# Removes DebForge configurations with optional cleanup levels
#
# Usage: ./uninstall.sh [OPTIONS]
# Note: Script handles sudo internally - do NOT run with sudo prefix

set -euo pipefail

# ─────────────────────────────────────────────────────────────────────────────
# Configuration
# ─────────────────────────────────────────────────────────────────────────────

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

STATE_DIR="$HOME/.local/share/debforge"
MANIFEST_FILE="$STATE_DIR/manifest.json"
BACKUP_DIR="$STATE_DIR/backups"

# ─────────────────────────────────────────────────────────────────────────────
# Source libraries
# ─────────────────────────────────────────────────────────────────────────────

source "$SCRIPT_DIR/lib/logger.sh"
source "$SCRIPT_DIR/lib/manifest.sh"
source "$SCRIPT_DIR/lib/backup.sh"

# ─────────────────────────────────────────────────────────────────────────────
# Global state
# ─────────────────────────────────────────────────────────────────────────────

DRY_RUN=false
FORCE=false
MODE="interactive"  # interactive, configs-only, full
KEEP_BACKUPS=false

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
            --keep-backups)
                KEEP_BACKUPS=true
                shift
                ;;
            --configs-only)
                MODE="configs-only"
                shift
                ;;
            --full)
                MODE="full"
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
DebForge Uninstaller - Remove DebForge configurations

Usage: $(basename "$0") [OPTIONS]

Options:
  --dry-run, -n       Preview changes without applying
  --force, -f         Force removal without confirmation
  --keep-backups      Keep backup files after uninstall
  --configs-only      Remove configs only (keep state, backups)
  --full              Full cleanup (remove everything)
  --quiet, -q         Suppress output
  --verbose, -v       Enable debug output
  --help, -h          Show this help message

Uninstall Modes:
  interactive         Prompt for uninstall mode (default)
  configs-only        Remove deployed configs, keep state and backups
  full                Full cleanup (state directory, backups, everything)

Examples:
  $(basename "$0")                      # Interactive (prompts you)
  $(basename "$0") --configs-only       # Remove configs only
  $(basename "$0") --full               # Full cleanup
  $(basename "$0") --dry-run --force    # Preview as non-interactive

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

prompt_mode() {
    if [[ "$MODE" != "interactive" ]]; then
        return 0
    fi

    if [[ "$DRY_RUN" == "true" ]] || [[ "$FORCE" == "true" ]]; then
        MODE="full"
        return 0
    fi

    echo ""
    echo "How do you want to uninstall DebForge?"
    echo ""
    echo "  1) Remove configs only"
    echo "     → Remove deployed system configs"
    echo "     → Restore from backups if available"
    echo "     → Keep ~/. local/share/debforge (state, logs, backups)"
    echo ""
    echo "  2) Full cleanup"
    echo "     → Remove everything (configs + state directory)"
    echo "     → Clear logs and manifest"
    echo "     → Optionally keep backups\""
    echo ""

    while true; do
        read -rp "Select option (1-2, or enter for #1): " choice
        choice="${choice:-1}"

        case "$choice" in
            1)
                MODE="configs-only"
                break
                ;;
            2)
                MODE="full"
                break
                ;;
            *)
                echo "Please enter 1 or 2"
                ;;
        esac
    done

    echo ""
    if [[ "$MODE" == "configs-only" ]]; then
        log_info "Selected: Remove configs only"
    else
        log_info "Selected: Full cleanup"
    fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 1: Check manifest
# ─────────────────────────────────────────────────────────────────────────────

phase_check_manifest() {
    log_section "Phase 1: Check Manifest"

    if ! manifest_exists; then
        if [[ "$FORCE" != "true" ]]; then
            log_error "No manifest found at $MANIFEST_FILE"
            log_info "Cannot uninstall without manifest - it tracks what was installed"
            log_info "Use --force to attempt removal anyway"
            exit 1
        fi

        log_warn "Manifest not found, proceeding with --force"
        return 0
    fi

    log_success "Manifest found, installation tracked"
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 2: Remove configs
# ─────────────────────────────────────────────────────────────────────────────

phase_remove_configs() {
    log_section "Phase 2: Remove Deployed Configurations"

    if ! manifest_exists; then
        log_warn "No manifest, skipping config removal"
        return 0
    fi

    local restored=0
    local removed=0

    # Get files from manifest
    while IFS= read -r file_json; do
        [[ -z "$file_json" ]] && continue

        local dest backup
        dest=$(echo "$file_json" | jq -r '.dest // empty')
        backup=$(echo "$file_json" | jq -r '.backup // empty')

        [[ -z "$dest" ]] && continue

        if [[ "$DRY_RUN" == "true" ]]; then
            if [[ -n "$backup" ]] && [[ -f "$backup" ]]; then
                log_info "[DRY-RUN] Would restore: $dest"
            elif [[ -f "$dest" ]]; then
                log_info "[DRY-RUN] Would remove: $dest"
            fi
            continue
        fi

        # Restore from backup or remove
        if [[ -n "$backup" ]] && [[ -f "$backup" ]]; then
            log_progress "Restoring: $dest"
            sudo mkdir -p "$(dirname "$dest")"
            if sudo cp -p "$backup" "$dest"; then
                ((restored++))
            else
                log_warn "Failed to restore: $dest"
            fi
        elif [[ -f "$dest" ]]; then
            log_progress "Removing: $dest"
            if sudo rm -f "$dest"; then
                ((removed++))
            else
                log_warn "Failed to remove: $dest"
            fi
        fi
    done < <(jq -c '.files[]?' "$MANIFEST_FILE" 2>/dev/null || echo "")

    # Remove systemd services
    for service in /etc/systemd/system/{pci-latency,set-min-free-mem}.service; do
        if [[ -f "$service" ]]; then
            if [[ "$DRY_RUN" == "true" ]]; then
                log_info "[DRY-RUN] Would remove: $service"
            else
                log_progress "Removing service: $(basename "$service")"
                sudo rm -f "$service"
                ((removed++))
            fi
        fi
    done

    # Remove earlyoom config
    if [[ -f /etc/default/earlyoom ]]; then
        if [[ "$DRY_RUN" == "true" ]]; then
            log_info "[DRY-RUN] Would remove: /etc/default/earlyoom"
        else
            log_progress "Removing: /etc/default/earlyoom"
            sudo rm -f /etc/default/earlyoom
            ((removed++))
        fi
    fi

    echo ""
    log_info "Configs restored: $restored"
    log_info "Configs removed: $removed"
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 3: Reload systemd
# ─────────────────────────────────────────────────────────────────────────────

phase_reload_systemd() {
    log_section "Phase 3: Reload systemd"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY-RUN] Would reload systemd"
        return 0
    fi

    log_progress "Reloading systemd"
    sudo systemctl daemon-reload || log_warn "systemd reload failed"

    log_progress "Stopping earlyoom service"
    sudo systemctl stop earlyoom.service 2>/dev/null || true
    sudo systemctl disable earlyoom.service 2>/dev/null || true

    log_progress "Restarting systemd-journald"
    sudo systemctl restart systemd-journald 2>/dev/null || true

    log_success "systemd reloaded"
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 4: Clean state (optional)
# ─────────────────────────────────────────────────────────────────────────────

phase_clean_state() {
    if [[ "$MODE" != "full" ]]; then
        log_section "Phase 4: Clean State (skipped)"
        log_info "To remove state directory, use: --full"
        return 0
    fi

    log_section "Phase 4: Clean State Directory"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY-RUN] Would remove: $STATE_DIR"
        return 0
    fi

    # Backups
    if [[ "$KEEP_BACKUPS" != "true" ]] && [[ -d "$BACKUP_DIR" ]]; then
        log_progress "Removing backups: $BACKUP_DIR"
        rm -rf "$BACKUP_DIR"
    fi

    # State dir
    if [[ -d "$STATE_DIR" ]]; then
        log_progress "Removing state directory: $STATE_DIR"
        rm -rf "$STATE_DIR"
    fi

    log_success "State cleaned"
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 5: Summary
# ─────────────────────────────────────────────────────────────────────────────

phase_summary() {
    log_section "Uninstall Summary"

    local summary=()
    summary+=("Mode: $MODE")
    summary+=("Dry-run: $DRY_RUN")

    if [[ "$MODE" == "configs-only" ]]; then
        summary+=("State preserved: $STATE_DIR")
        summary+=("Backups preserved: $BACKUP_DIR")
    fi

    if [[ "$MODE" == "full" ]]; then
        if [[ "$KEEP_BACKUPS" == "true" ]]; then
            summary+=("Backups preserved: $BACKUP_DIR")
        else
            summary+=("Backups removed")
        fi
    fi

    log_summary "DebForge Uninstall" "${summary[@]}"

    if [[ "$DRY_RUN" != "true" ]]; then
        echo ""
        if [[ "$MODE" == "configs-only" ]]; then
            echo "Configs removed. Your system settings have been restored or cleaned."
            echo ""
            echo "To fully uninstall (including state): $SCRIPT_DIR/undo.sh --full"
        else
            echo "Full cleanup complete. DebForge has been completely removed."
        fi
    fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Main
# ─────────────────────────────────────────────────────────────────────────────

main() {
    parse_args "$@"
    check_not_root

    mkdir -p "$BACKUP_DIR"
    log_init "uninstall"

    log_section "DebForge Uninstaller"
    log_info "Repository: $REPO_DIR"
    log_info "State: $STATE_DIR"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_warn "DRY-RUN MODE - No changes will be made"
        echo ""
    fi

    # Get mode
    prompt_mode

    # Run phases
    phase_check_manifest
    phase_remove_configs
    phase_reload_systemd
    phase_clean_state
    phase_summary

    log_success "Uninstall completed!"
}

main "$@"
