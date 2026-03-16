#!/usr/bin/env bash
# verify.sh - Verification functions for DebForge
# Validates installed files, permissions, checksums, and service states
#
# Note: Uses sudo internally for system-level checks

set -euo pipefail

# Verify file exists
verify_exists() {
    local path="$1"
    [[ -e "$path" ]]
}

# Verify file is a regular file
verify_file() {
    local path="$1"
    [[ -f "$path" ]]
}

# Verify directory exists
verify_dir() {
    local path="$1"
    [[ -d "$path" ]]
}

# Verify file permissions
verify_permissions() {
    local path="$1"
    local expected_perms="$2"

    if [[ ! -e "$path" ]]; then
        return 1
    fi

    local actual_perms
    actual_perms=$(stat -c %a "$path" 2>/dev/null)

    # Compare octal permissions (last 3 digits)
    [[ "${actual_perms: -3}" == "${expected_perms: -3}" ]]
}

# Verify file ownership
verify_ownership() {
    local path="$1"
    local expected_owner="${2:-root}"
    local expected_group="${3:-root}"

    if [[ ! -e "$path" ]]; then
        return 1
    fi

    local actual_owner actual_group
    actual_owner=$(stat -c %U "$path" 2>/dev/null)
    actual_group=$(stat -c %G "$path" 2>/dev/null)

    [[ "$actual_owner" == "$expected_owner" ]] && [[ "$actual_group" == "$expected_group" ]]
}

# Verify file checksum
verify_checksum() {
    local path="$1"
    local expected_checksum="$2"

    if [[ ! -f "$path" ]]; then
        return 1
    fi

    # Parse checksum format: "sha256:abc123..." or just "abc123..."
    local expected_hash
    if [[ "$expected_checksum" == sha256:* ]]; then
        expected_hash="${expected_checksum#sha256:}"
    else
        expected_hash="$expected_checksum"
    fi

    local actual_hash
    actual_hash=$(sha256sum "$path" | cut -d' ' -f1)

    [[ "$actual_hash" == "$expected_hash" ]]
}

# Verify systemd service is installed
verify_service_installed() {
    local service="$1"
    sudo systemctl list-unit-files "$service" &>/dev/null || \
    [[ -f "/etc/systemd/system/$service" ]] || \
    [[ -f "/usr/lib/systemd/system/$service" ]]
}

# Verify systemd service is enabled
verify_service_enabled() {
    local service="$1"
    sudo systemctl is-enabled "$service" &>/dev/null
}

# Verify systemd service is active
verify_service_active() {
    local service="$1"
    sudo systemctl is-active "$service" &>/dev/null
}

# Verify sysctl setting
verify_sysctl() {
    local key="$1"
    local expected_value="$2"

    local actual_value
    actual_value=$(sudo sysctl -n "$key" 2>/dev/null || echo "")

    [[ "$actual_value" == "$expected_value" ]]
}

# Verify udev rules are loaded
verify_udev_rules() {
    local rule_file="$1"
    [[ -f "$rule_file" ]]
}

# Comprehensive file verification
verify_file_install() {
    local path="$1"
    local expected_perms="${2:-}"
    local expected_checksum="${3:-}"

    local errors=()
    local warnings=()

    # Check existence
    if ! verify_file "$path"; then
        errors+=("File does not exist: $path")
    else
        # Check permissions if specified
        if [[ -n "$expected_perms" ]]; then
            if ! verify_permissions "$path" "$expected_perms"; then
                local actual_perms
                actual_perms=$(stat -c %a "$path")
                warnings+=("Permissions mismatch: expected $expected_perms, got $actual_perms")
            fi
        fi

        # Check checksum if specified
        if [[ -n "$expected_checksum" ]]; then
            if ! verify_checksum "$path" "$expected_checksum"; then
                errors+=("Checksum verification failed")
            fi
        fi
    fi

    # Output results
    if [[ ${#errors[@]} -gt 0 ]]; then
        for err in "${errors[@]}"; do
            echo "ERROR: $err"
        done
        return 1
    fi

    if [[ ${#warnings[@]} -gt 0 ]]; then
        for warn in "${warnings[@]}"; do
            echo "WARNING: $warn"
        done
    fi

    echo "OK: $path"
    return 0
}

# Verify all files from manifest
verify_manifest() {
    local manifest_file="${1:-}"

    # Source manifest library if not already loaded
    if ! declare -f manifest_get &>/dev/null; then
        local lib_dir
        lib_dir="$(dirname "${BASH_SOURCE[0]}")"
        # shellcheck source=/dev/null
        source "$lib_dir/manifest.sh"
    fi

    if ! manifest_exists; then
        echo "No manifest found"
        return 1
    fi

    local failed=0
    local passed=0

    # Get all file destinations from manifest
    while IFS= read -r dest; do
        [[ -z "$dest" ]] && continue

        if verify_file "$dest"; then
            ((passed++))
            echo "✓ $dest"
        else
            ((failed++))
            echo "✗ $dest (missing)"
        fi
    done < <(jq -r '.files[].dest' "$MANIFEST_FILE" 2>/dev/null)

    echo ""
    echo "Verification complete: $passed passed, $failed failed"

    [[ $failed -eq 0 ]]
}

# Pre-flight system checks
preflight_checks() {
    local errors=0
    local warnings=0

    echo "Running pre-flight checks..."
    echo ""

    # Check if running as root (we don't want that)
    if [[ $EUID -eq 0 ]]; then
        echo "ERROR: This script should NOT be run as root"
        echo "  Run without sudo - it will use sudo internally where needed"
        ((errors++))
    fi

    # Check OS version
    if [[ -f /etc/os-release ]]; then
        # shellcheck disable=SC1091
        source /etc/os-release
        if [[ "${VERSION_CODENAME:-}" != "trixie" ]]; then
            echo "WARNING: Target OS is Debian Trixie, detected: ${VERSION_CODENAME:-unknown}"
            ((warnings++))
        else
            echo "✓ OS version: Debian $VERSION_CODENAME"
        fi
    else
        echo "WARNING: Cannot detect OS (no /etc/os-release)"
        ((warnings++))
    fi

    # Check required tools
    local required_tools=("jq" "systemctl" "udevadm" "install" "sudo")
    for tool in "${required_tools[@]}"; do
        if command -v "$tool" &>/dev/null; then
            echo "✓ Tool available: $tool"
        else
            echo "ERROR: Required tool missing: $tool"
            ((errors++))
        fi
    done

    # Check disk space (need at least 100MB free)
    local free_space
    free_space=$(df -m / | awk 'NR==2 {print $4}')
    if [[ "$free_space" -lt 100 ]]; then
        echo "ERROR: Insufficient disk space (need 100MB, have ${free_space}MB)"
        ((errors++))
    else
        echo "✓ Disk space: ${free_space}MB free"
    fi

    # Check if jq is available for JSON handling
    if ! command -v jq &>/dev/null; then
        echo "Installing jq for JSON handling..."
        sudo apt-get update -qq && sudo apt-get install -y -qq jq
    fi

    echo ""
    echo "Pre-flight complete: $errors errors, $warnings warnings"

    [[ $errors -eq 0 ]]
}

# Post-install verification
post_install_verify() {
    echo ""
    echo "Running post-install verification..."
    echo ""

    local passed=0
    local failed=0

    # Verify sysctl configs
    if [[ -f /etc/sysctl.d/99-debforge.conf ]]; then
        echo "✓ Sysctl config installed"
        ((passed++))
    else
        echo "✗ Sysctl config missing"
        ((failed++))
    fi

    # Verify udev rules
    local udev_count
    udev_count=$(find /usr/lib/udev/rules.d -name "*debforge*" -o -name "20-audio-pm.rules" -o -name "30-zram.rules" 2>/dev/null | wc -l)
    if [[ "$udev_count" -gt 0 ]]; then
        echo "✓ Udev rules installed: $udev_count files"
        ((passed++))
    else
        echo "✗ Udev rules missing"
        ((failed++))
    fi

    # Verify binaries
    for bin in ksmctl pci-latency game-performance; do
        if command -v "$bin" &>/dev/null || [[ -f "/usr/local/bin/$bin" ]]; then
            echo "✓ Binary installed: $bin"
            ((passed++))
        else
            echo "✗ Binary missing: $bin"
            ((failed++))
        fi
    done

    echo ""
    echo "Verification complete: $passed passed, $failed failed"

    [[ $failed -eq 0 ]]
}
