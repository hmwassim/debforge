#!/usr/bin/env bash
# status.sh - Show DebForge configuration status
# Displays installed files, their state, and verification results
#
# Usage: ./status.sh [OPTIONS]
# Note: Runs entirely as normal user - no sudo needed

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
VERIFY=false
JSON_OUTPUT=false
VERBOSE=false

# Colors (for non-JSON output)
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Parse arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --verify|-v)
                VERIFY=true
                shift
                ;;
            --json)
                JSON_OUTPUT=true
                LOG_QUIET=true
                shift
                ;;
            --verbose)
                VERBOSE=true
                shift
                ;;
            --quiet|-q)
                LOG_QUIET=true
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
DebForge Status - Configuration Status Viewer

Usage: $(basename "$0") [OPTIONS]

Options:
  --verify, -v    Verify installed files (checksums, permissions)
  --json          Output status as JSON
  --verbose       Show detailed information
  --quiet, -q     Suppress non-essential output
  --help, -h      Show this help message

Examples:
  $(basename "$0")                  # Show installation status
  $(basename "$0") --verify         # Verify all installed files
  $(basename "$0") --json           # Output as JSON for scripting

Note: This script runs as normal user - no sudo needed.
EOF
}

# Check if DebForge is installed
check_installed() {
    local installed=false

    if manifest_exists; then
        installed=true
    fi

    # Also check legacy state
    if [[ -f "$HOME/.local/.debforge.state" ]]; then
        installed=true
    fi

    # Check for key files
    if [[ -f /etc/sysctl.d/99-debforge.conf ]]; then
        installed=true
    fi

    echo "$installed"
}

# Get status summary (JSON)
get_summary() {
    if ! manifest_exists; then
        echo '{"installed": false, "reason": "no_manifest"}'
        return
    fi

    local total applied modified failed missing
    total=$(manifest_count 2>/dev/null || echo "0")
    applied=$(manifest_count_status "applied" 2>/dev/null || echo "0")
    modified=$(manifest_count_status "modified" 2>/dev/null || echo "0")
    failed=$(manifest_count_status "failed" 2>/dev/null || echo "0")

    # Count missing files
    missing=0
    if manifest_exists; then
        while IFS= read -r dest; do
            [[ -z "$dest" ]] && continue
            if [[ ! -f "$dest" ]]; then
                ((missing++))
            fi
        done < <(jq -r '.files[].dest' "$MANIFEST_FILE" 2>/dev/null)
    fi

    local created updated
    created=$(manifest_get '.created_at' 2>/dev/null || echo "unknown")
    updated=$(manifest_get '.updated_at' 2>/dev/null || echo "unknown")

    cat << EOF
{
    "installed": true,
    "total_files": $total,
    "applied": $applied,
    "modified": $modified,
    "failed": $failed,
    "missing": $missing,
    "created_at": "$created",
    "updated_at": "$updated"
}
EOF
}

# Print human-readable summary
print_summary() {
    echo -e "${CYAN}${BOLD}DebForge Status${NC}"
    echo ""

    if [[ "$(check_installed)" != "true" ]]; then
        echo -e "${YELLOW}DebForge is not installed${NC}"
        echo ""
        echo "To install: $REPO_DIR/scripts/install.sh"
        return
    fi

    if ! manifest_exists; then
        echo -e "${YELLOW}Installation detected but manifest missing${NC}"
        echo "Some legacy installations may not have a manifest."
        echo ""
        echo "Key files found:"
        if [[ -f /etc/sysctl.d/99-debforge.conf ]]; then
            echo -e "  ${GREEN}✓${NC} /etc/sysctl.d/99-debforge.conf"
        fi
        if [[ -f /usr/local/bin/ksmctl ]]; then
            echo -e "  ${GREEN}✓${NC} /usr/local/bin/ksmctl"
        fi
        if [[ -f /usr/local/bin/pci-latency ]]; then
            echo -e "  ${GREEN}✓${NC} /usr/local/bin/pci-latency"
        fi
        if [[ -f /usr/local/bin/game-performance ]]; then
            echo -e "  ${GREEN}✓${NC} /usr/local/bin/game-performance"
        fi
        echo ""
        echo "Consider re-running install.sh to create a manifest."
        return
    fi

    local total applied modified failed missing
    total=$(manifest_count)
    applied=$(manifest_count_status "applied")
    modified=$(manifest_count_status "modified")
    failed=$(manifest_count_status "failed")

    # Count missing
    missing=0
    while IFS= read -r dest; do
        [[ -z "$dest" ]] && continue
        if [[ ! -f "$dest" ]]; then
            ((missing++))
        fi
    done < <(jq -r '.files[].dest' "$MANIFEST_FILE" 2>/dev/null)

    echo -e "${GREEN}DebForge is installed${NC}"
    echo ""
    echo "Statistics:"
    echo -e "  Total files:  ${CYAN}$total${NC}"
    echo -e "  Applied:      ${GREEN}$applied${NC}"
    echo -e "  Modified:     ${YELLOW}$modified${NC}"
    echo -e "  Failed:       ${RED}$failed${NC}"
    echo -e "  Missing:      ${RED}$missing${NC}"
    echo ""
    echo "Installation info:"
    echo "  Created: $(manifest_get '.created_at')"
    echo "  Updated: $(manifest_get '.updated_at')"
    echo "  Manifest: $MANIFEST_FILE"
    echo "  Backups: $BACKUP_DIR"
    echo ""
}

# Print file status table
print_files() {
    if ! manifest_exists; then
        return
    fi

    echo -e "${CYAN}${BOLD}Installed Files${NC}"
    echo ""
    printf "%-8s %-50s %s\n" "STATUS" "FILE" "TYPE"
    printf "%s\n" "$(printf '=%.0s' {1..70})"

    while IFS= read -r file_json; do
        [[ -z "$file_json" ]] && continue

        local dest status file_type exists
        dest=$(echo "$file_json" | jq -r '.dest')
        status=$(echo "$file_json" | jq -r '.status')
        file_type=$(echo "$file_json" | jq -r '.type')

        # Check if file exists
        if [[ -f "$dest" ]]; then
            exists="✓"
        else
            exists="✗"
        fi

        # Color code status
        local status_color
        case "$status" in
            applied) status_color="$GREEN" ;;
            modified) status_color="$YELLOW" ;;
            failed) status_color="$RED" ;;
            *) status_color="$NC" ;;
        esac

        printf "%b%-8s%b %-50s %s\n" "$status_color" "$exists $status" "$NC" "$dest" "$file_type"
    done < <(jq -c '.files[]' "$MANIFEST_FILE" 2>/dev/null)

    echo ""
}

# Verify files (user-level reads)
verify_files() {
    if ! manifest_exists; then
        echo -e "${YELLOW}No manifest found - cannot verify${NC}"
        return 1
    fi

    echo -e "${CYAN}${BOLD}File Verification${NC}"
    echo ""

    local passed=0 failed=0

    while IFS= read -r file_json; do
        [[ -z "$file_json" ]] && continue

        local dest expected_perms expected_checksum
        dest=$(echo "$file_json" | jq -r '.dest')
        expected_perms=$(echo "$file_json" | jq -r '.permissions')
        expected_checksum=$(echo "$file_json" | jq -r '.checksum // empty')

        local status="PASS"
        local issues=()

        # Check existence
        if [[ ! -f "$dest" ]]; then
            status="FAIL"
            issues+=("missing")
        else
            # Check permissions
            if [[ -n "$expected_perms" ]]; then
                local actual_perms
                actual_perms=$(stat -c %a "$dest" 2>/dev/null)
                if [[ "${actual_perms: -3}" != "${expected_perms: -3}" ]]; then
                    issues+=("perms:$actual_perms")
                fi
            fi

            # Check checksum
            if [[ -n "$expected_checksum" ]]; then
                local expected_hash actual_hash
                if [[ "$expected_checksum" == sha256:* ]]; then
                    expected_hash="${expected_checksum#sha256:}"
                else
                    expected_hash="$expected_checksum"
                fi
                actual_hash=$(sha256sum "$dest" | cut -d' ' -f1)
                if [[ "$actual_hash" != "$expected_hash" ]]; then
                    issues+=("checksum")
                fi
            fi

            if [[ ${#issues[@]} -gt 0 ]]; then
                status="WARN"
            fi
        fi

        # Output
        local color
        case "$status" in
            PASS) color="$GREEN"; ((passed++)) ;;
            WARN) color="$YELLOW"; ((passed++)) ;;
            FAIL) color="$RED"; ((failed++)) ;;
        esac

        printf "%b%-6s%b %s" "$color" "$status" "$NC" "$dest"
        if [[ ${#issues[@]} -gt 0 ]]; then
            printf " (%s)" "$(IFS=,; echo "${issues[*]}")"
        fi
        echo ""

    done < <(jq -c '.files[]' "$MANIFEST_FILE" 2>/dev/null)

    echo ""
    echo "Verification complete:"
    echo -e "  ${GREEN}Passed: $passed${NC}"
    echo -e "  ${RED}Failed: $failed${NC}"

    [[ $failed -eq 0 ]]
}

# Check service status (uses sudo for systemctl queries)
print_services() {
    echo -e "${CYAN}${BOLD}Service Status${NC}"
    echo ""

    local services=(
        "pci-latency.service:PCI Latency Tuning"
        "set-min-free-mem.service:Min Free Memory"
        "earlyoom.service:Early OOM Killer"
    )

    printf "%-30s %-15s %s\n" "SERVICE" "ACTIVE" "ENABLED"
    printf "%s\n" "$(printf '=%.0s' {1..60})"

    for service_info in "${services[@]}"; do
        local service="${service_info%%:*}"
        local desc="${service_info##*:}"

        local active="inactive"
        local enabled="disabled"

        if sudo systemctl is-active "$service" &>/dev/null; then
            active="active"
        fi

        if sudo systemctl is-enabled "$service" &>/dev/null; then
            enabled="enabled"
        fi

        local active_color enabled_color
        [[ "$active" == "active" ]] && active_color="$GREEN" || active_color="$YELLOW"
        [[ "$enabled" == "enabled" ]] && enabled_color="$GREEN" || enabled_color="$YELLOW"

        printf "%-30s %b%-15s%b %b%s%b\n" "$service" \
            "$active_color" "$active" "$NC" \
            "$enabled_color" "$enabled" "$NC"
    done

    echo ""
}

# Show backup status
print_backups() {
    if ! manifest_exists; then
        return
    fi

    echo -e "${CYAN}${BOLD}Backup Status${NC}"
    echo ""

    local backup_count=0
    local backup_size=0

    if [[ -d "$BACKUP_DIR" ]]; then
        backup_count=$(find "$BACKUP_DIR" -type f -name "*.bak" 2>/dev/null | wc -l)
        backup_size=$(du -sh "$BACKUP_DIR" 2>/dev/null | cut -f1)
    fi

    echo "  Backup directory: $BACKUP_DIR"
    echo "  Backups stored: $backup_count files"
    echo "  Total size: ${backup_size:-0}"
    echo ""
}

# Main
main() {
    parse_args "$@"

    if [[ "$JSON_OUTPUT" == "true" ]]; then
        get_summary
        exit 0
    fi

    print_summary

    if manifest_exists; then
        print_files

        if [[ "$VERIFY" == "true" ]]; then
            verify_files
        fi

        print_services
        print_backups
    fi

    echo -e "${CYAN}Commands:${NC}"
    echo "  Install:   $REPO_DIR/scripts/install.sh"
    echo "  Uninstall: $REPO_DIR/scripts/uninstall.sh"
    echo "  Verify:    $REPO_DIR/scripts/status.sh --verify"
}

main "$@"
