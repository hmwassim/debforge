#!/usr/bin/env bash
# manifest.sh - JSON manifest tracking for DebForge
# Manages installation state with rich metadata
#
# Note: Manifest is stored in user's home directory (~/.local/share/debforge/manifest.json)

set -euo pipefail

# Configuration - User-level state directory
STATE_DIR="${STATE_DIR:-$HOME/.local/share/debforge}"
MANIFEST_FILE="$STATE_DIR/manifest.json"

# Ensure state directory exists
_manifest_init() {
    mkdir -p "$STATE_DIR"
}

# Get current timestamp in ISO 8601 format
_manifest_timestamp() {
    date -u '+%Y-%m-%dT%H:%M:%SZ'
}

# Get system info
_manifest_system_info() {
    local os="" codename="" arch=""

    if [[ -f /etc/os-release ]]; then
        # shellcheck disable=SC1091
        source /etc/os-release
        os="${ID:-unknown}"
        codename="${VERSION_CODENAME:-unknown}"
    fi

    arch="$(dpkg --print-architecture 2>/dev/null || echo 'unknown')"

    echo "{\"os\": \"$os\", \"codename\": \"$codename\", \"architecture\": \"$arch\"}"
}

# Initialize a new manifest
manifest_create() {
    _manifest_init

    local version="${1:-1.0}"
    local system_info
    system_info=$(_manifest_system_info)

    cat > "$MANIFEST_FILE" << EOF
{
    "version": "$version",
    "created_at": "$(_manifest_timestamp)",
    "updated_at": "$(_manifest_timestamp)",
    "system": $system_info,
    "files": [],
    "home_configs": [],
    "binaries": [],
    "services": []
}
EOF
}

# Check if manifest exists
manifest_exists() {
    [[ -f "$MANIFEST_FILE" ]]
}

# Read manifest value using jq
manifest_get() {
    local path="$1"
    if ! manifest_exists; then
        echo "null"
        return 1
    fi
    jq -r "$path" "$MANIFEST_FILE" 2>/dev/null || echo "null"
}

# Add a file entry to the manifest
manifest_add_file() {
    local dest="$1"
    local source="$2"
    local permissions="$3"
    local type="${4:-config}"
    local checksum="${5:-}"
    local backup="${6:-}"

    if ! manifest_exists; then
        manifest_create
    fi

    # Calculate checksum if not provided
    if [[ -z "$checksum" ]] && [[ -f "$dest" ]]; then
        checksum="sha256:$(sha256sum "$dest" | cut -d' ' -f1)"
    fi

    local entry
    entry=$(cat << EOF
{
    "dest": "$dest",
    "source": "$source",
    "permissions": "$permissions",
    "type": "$type",
    "checksum": "$checksum",
    "backup": "$backup",
    "status": "applied",
    "installed_at": "$(_manifest_timestamp)"
}
EOF
)

    # Use jq to add entry to files array
    local tmp_file
    tmp_file=$(mktemp)
    jq --argjson entry "$entry" '.files += [$entry] | .updated_at = "'$(_manifest_timestamp)'"' \
        "$MANIFEST_FILE" > "$tmp_file" && mv "$tmp_file" "$MANIFEST_FILE"
}

# Add a binary entry
manifest_add_binary() {
    local dest="$1"
    local source="$2"
    local permissions="${3:-0755}"
    local checksum="${4:-}"
    local backup="${5:-}"

    manifest_add_file "$dest" "$source" "$permissions" "binary" "$checksum" "$backup"
}

# Add a service entry
manifest_add_service() {
    local name="$1"
    local path="$2"
    local enabled="${3:-true}"

    if ! manifest_exists; then
        manifest_create
    fi

    local entry
    entry=$(cat << EOF
{
    "name": "$name",
    "path": "$path",
    "enabled": $enabled,
    "status": "installed",
    "installed_at": "$(_manifest_timestamp)"
}
EOF
)

    local tmp_file
    tmp_file=$(mktemp)
    jq --argjson entry "$entry" '.services += [$entry] | .updated_at = "'$(_manifest_timestamp)'"' \
        "$MANIFEST_FILE" > "$tmp_file" && mv "$tmp_file" "$MANIFEST_FILE"
}

# Add a home config entry (user-level configs in ~/.config, ~/.local)
manifest_add_home_config() {
    local dest="$1"
    local source="$2"
    local permissions="${3:-0644}"
    local checksum="${4:-}"
    local backup="${5:-}"

    if ! manifest_exists; then
        manifest_create
    fi

    # Calculate checksum if not provided
    if [[ -z "$checksum" ]] && [[ -f "$dest" ]]; then
        checksum="sha256:$(sha256sum "$dest" | cut -d' ' -f1)"
    fi

    local entry
    entry=$(cat << EOF
{
    "dest": "$dest",
    "source": "$source",
    "permissions": "$permissions",
    "type": "home_config",
    "checksum": "$checksum",
    "backup": "$backup",
    "status": "applied",
    "installed_at": "$(_manifest_timestamp)"
}
EOF
)

    local tmp_file
    tmp_file=$(mktemp)
    jq --argjson entry "$entry" '.home_configs += [$entry] | .updated_at = "'$(_manifest_timestamp)'"' \
        "$MANIFEST_FILE" > "$tmp_file" && mv "$tmp_file" "$MANIFEST_FILE"
}

# Get home config count
manifest_count_home_configs() {
    manifest_get ".home_configs | length"
}

# Get all home configs
manifest_get_home_configs() {
    jq -c '.home_configs[]' "$MANIFEST_FILE" 2>/dev/null
}

# Update file status
manifest_update_status() {
    local dest="$1"
    local status="$2"

    if ! manifest_exists; then
        return 1
    fi

    local tmp_file
    tmp_file=$(mktemp)
    jq --arg dest "$dest" --arg status "$status" \
        '(.files[] | select(.dest == $dest)).status = $status | .updated_at = "'$(_manifest_timestamp)'"' \
        "$MANIFEST_FILE" > "$tmp_file" && mv "$tmp_file" "$MANIFEST_FILE"
}

# Remove a file entry from manifest
manifest_remove_file() {
    local dest="$1"

    if ! manifest_exists; then
        return 1
    fi

    local tmp_file
    tmp_file=$(mktemp)
    jq --arg dest "$dest" \
        '.files = [.files[] | select(.dest != $dest)] | .updated_at = "'$(_manifest_timestamp)'"' \
        "$MANIFEST_FILE" > "$tmp_file" && mv "$tmp_file" "$MANIFEST_FILE"
}

# Get all files with a specific status
manifest_get_by_status() {
    local status="$1"
    manifest_get ".files[] | select(.status == \"$status\")"
}

# Get file count by status
manifest_count_status() {
    local status="$1"
    manifest_get "[.files[] | select(.status == \"$status\")] | length"
}

# Get total file count
manifest_count() {
    manifest_get ".files | length"
}

# Validate manifest JSON
manifest_validate() {
    if ! manifest_exists; then
        echo "Manifest file not found: $MANIFEST_FILE"
        return 1
    fi

    if ! jq empty "$MANIFEST_FILE" 2>/dev/null; then
        echo "Manifest JSON is invalid"
        return 1
    fi

    echo "Manifest is valid"
    return 0
}

# Print manifest summary
manifest_summary() {
    if ! manifest_exists; then
        echo "No manifest found"
        return 1
    fi

    local total applied modified failed
    total=$(manifest_count)
    applied=$(manifest_count_status "applied")
    modified=$(manifest_count_status "modified")
    failed=$(manifest_count_status "failed")

    echo "Manifest Summary:"
    echo "  Total files: $total"
    echo "  Applied: $applied"
    echo "  Modified: $modified"
    echo "  Failed: $failed"
    echo ""
    echo "Created: $(manifest_get '.created_at')"
    echo "Updated: $(manifest_get '.updated_at')"
    echo "Location: $MANIFEST_FILE"
}

# Destroy manifest (for uninstall)
manifest_destroy() {
    if manifest_exists; then
        rm -f "$MANIFEST_FILE"
    fi
}
