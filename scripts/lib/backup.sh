#!/usr/bin/env bash
# backup.sh - Backup and restore functions for DebForge
# Handles safe backup of existing files before modification
#
# Note: Backups are stored in user's home directory (~/.local/share/debforge/backups)

set -euo pipefail

# Configuration - User-level state directory
STATE_DIR="${STATE_DIR:-$HOME/.local/share/debforge}"
BACKUP_DIR="${BACKUP_DIR:-$STATE_DIR/backups}"

# Ensure backup directory exists
_backup_init() {
    mkdir -p "$BACKUP_DIR"
}

# Generate a safe backup filename
_backup_filename() {
    local dest="$1"
    # Replace / with _ for flat structure, preserve extension
    local basename
    basename=$(echo "$dest" | tr '/' '_' | sed 's/^_//')
    echo "${basename}.bak"
}

# Get full backup path for a destination
backup_path() {
    local dest="$1"
    local filename
    filename=$(_backup_filename "$dest")
    echo "$BACKUP_DIR/$filename"
}

# Create backup of a file (requires sudo for system files)
# Returns: backup path on success, empty on no-op
backup_file() {
    local dest="$1"
    local force="${2:-false}"

    _backup_init

    # If file doesn't exist, no backup needed
    if [[ ! -f "$dest" ]] && [[ ! -d "$dest" ]]; then
        echo ""
        return 0
    fi

    local backup_dest
    backup_dest=$(backup_path "$dest")

    # If backup already exists
    if [[ -f "$backup_dest" ]]; then
        if [[ "$force" == "true" ]]; then
            rm -f "$backup_dest"
        else
            # Backup of backup with timestamp
            backup_dest="${backup_dest}.$(date +%Y%m%d-%H%M%S)"
        fi
    fi

    # Create backup (use sudo for system files)
    if [[ -f "$dest" ]]; then
        sudo cp -p "$dest" "$backup_dest"
    elif [[ -d "$dest" ]]; then
        sudo cp -rp "$dest" "$backup_dest"
    fi

    echo "$backup_dest"
}

# Restore a file from backup (requires sudo for system destinations)
# Returns: 0 on success, 1 on failure
restore_file() {
    local dest="$1"
    local backup_src="${2:-}"

    # If no backup source specified, look in backup dir
    if [[ -z "$backup_src" ]]; then
        backup_src=$(backup_path "$dest")
    fi

    if [[ ! -f "$backup_src" ]]; then
        echo "Backup not found: $backup_src" >&2
        return 1
    fi

    # Ensure destination directory exists
    local dest_dir
    dest_dir=$(dirname "$dest")
    sudo mkdir -p "$dest_dir"

    # Restore (use sudo for system destinations)
    sudo cp -p "$backup_src" "$dest"
    return 0
}

# Remove a backup file (user-level operation)
remove_backup() {
    local dest="$1"
    local backup_src
    backup_src=$(backup_path "$dest")

    if [[ -f "$backup_src" ]]; then
        rm -f "$backup_src"
        return 0
    fi
    return 1
}

# Check if backup exists for a file
backup_exists() {
    local dest="$1"
    local backup_src
    backup_src=$(backup_path "$dest")
    [[ -f "$backup_src" ]]
}

# List all backups (user-level operation)
list_backups() {
    _backup_init
    find "$BACKUP_DIR" -type f -name "*.bak" 2>/dev/null | sort
}

# Get backup info (original path, timestamp, size)
backup_info() {
    local backup_src="$1"

    if [[ ! -f "$backup_src" ]]; then
        echo "Backup not found: $backup_src" >&2
        return 1
    fi

    local size timestamp
    size=$(stat -c %s "$backup_src" 2>/dev/null || echo "unknown")
    timestamp=$(stat -c %Y "$backup_src" 2>/dev/null || echo "unknown")

    echo "path: $backup_src"
    echo "size: $size bytes"
    echo "timestamp: $timestamp"
}

# Clean old backups (older than N days) - user-level operation
clean_old_backups() {
    local days="${1:-30}"
    _backup_init

    local count=0
    while IFS= read -r backup; do
        if [[ -f "$backup" ]]; then
            rm -f "$backup"
            ((count++))
        fi
    done < <(find "$BACKUP_DIR" -type f -name "*.bak" -mtime +"$days" 2>/dev/null)

    echo "Cleaned $count old backups"
}

# Verify backup integrity (compare checksums) - user-level reads
verify_backup() {
    local dest="$1"
    local backup_src="${2:-}"

    if [[ -z "$backup_src" ]]; then
        backup_src=$(backup_path "$dest")
    fi

    if [[ ! -f "$backup_src" ]]; then
        echo "Backup not found" >&2
        return 1
    fi

    if [[ ! -f "$dest" ]]; then
        echo "Original file not found" >&2
        return 1
    fi

    local src_sum backup_sum
    src_sum=$(sha256sum "$dest" | cut -d' ' -f1)
    backup_sum=$(sha256sum "$backup_src" | cut -d' ' -f1)

    if [[ "$src_sum" == "$backup_sum" ]]; then
        echo "Backup verified: checksums match"
        return 0
    else
        echo "Backup warning: checksums differ" >&2
        echo "  Original: $src_sum"
        echo "  Backup:   $backup_sum"
        return 1
    fi
}

# === HOME DIRECTORY BACKUPS ===
# Home configs don't need sudo - user owns ~/.config

# Backup a home config file (no sudo needed)
backup_home_config() {
    local dest="$1"
    local force="${2:-false}"

    _backup_init

    # If file doesn't exist, no backup needed
    if [[ ! -f "$dest" ]]; then
        echo ""
        return 0
    fi

    local backup_dest
    backup_dest=$(backup_path "$dest")

    # If backup already exists
    if [[ -f "$backup_dest" ]]; then
        if [[ "$force" == "true" ]]; then
            rm -f "$backup_dest"
        else
            backup_dest="${backup_dest}.$(date +%Y%m%d-%H%M%S)"
        fi
    fi

    # Create backup (no sudo needed for home dir)
    cp -p "$dest" "$backup_dest"
    echo "$backup_dest"
}

# Restore a home config file (no sudo needed)
restore_home_config() {
    local dest="$1"
    local backup_src="${2:-}"

    # If no backup source specified, look in backup dir
    if [[ -z "$backup_src" ]]; then
        backup_src=$(backup_path "$dest")
    fi

    if [[ ! -f "$backup_src" ]]; then
        echo "Backup not found: $backup_src" >&2
        return 1
    fi

    # Ensure destination directory exists (no sudo needed)
    local dest_dir
    dest_dir=$(dirname "$dest")
    mkdir -p "$dest_dir"

    # Restore (no sudo needed for home dir)
    cp -p "$backup_src" "$dest"
    return 0
}

# Remove a home config file (no sudo needed)
remove_home_config() {
    local dest="$1"

    if [[ -f "$dest" ]]; then
        rm -f "$dest"
        return 0
    fi
    return 1
}

# Backup entire home config directory
backup_home_dir() {
    local dir="$1"
    local backup_name
    backup_name=$(echo "$dir" | tr '/' '_' | sed 's/^_//')

    _backup_init

    if [[ ! -d "$dir" ]]; then
        echo ""
        return 0
    fi

    local backup_dest="$BACKUP_DIR/${backup_name}.dir.bak"

    if [[ -d "$backup_dest" ]]; then
        backup_dest="${backup_dest}.$(date +%Y%m%d-%H%M%S)"
    fi

    cp -rp "$dir" "$backup_dest"
    echo "$backup_dest"
}

# Restore entire home config directory
restore_home_dir() {
    local dir="$1"
    local backup_src="${2:-}"

    if [[ -z "$backup_src" ]]; then
        backup_src=$(backup_path "$dir")
    fi

    if [[ ! -d "$backup_src" ]]; then
        echo "Backup not found: $backup_src" >&2
        return 1
    fi

    # Remove current dir and restore
    rm -rf "$dir"
    cp -rp "$backup_src" "$dir"
    return 0
}
