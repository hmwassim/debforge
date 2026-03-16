#!/usr/bin/env bash
# logger.sh - Logging functions for DebForge
# Provides consistent logging with file and stdout output
#
# Note: State is stored in user's home directory (~/.local/share/debforge/logs)

set -euo pipefail

# Configuration - User-level state directory
STATE_DIR="${STATE_DIR:-$HOME/.local/share/debforge}"
LOG_DIR="${LOG_DIR:-$STATE_DIR/logs}"
LOG_FILE="${LOG_FILE:-}"
LOG_QUIET="${LOG_QUIET:-false}"
LOG_VERBOSE="${LOG_VERBOSE:-false}"

# Colors (disabled if not a terminal)
if [[ -t 1 ]]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[0;33m'
    BLUE='\033[0;34m'
    MAGENTA='\033[0;35m'
    CYAN='\033[0;36m'
    WHITE='\033[0;37m'
    BOLD='\033[1m'
    NC='\033[0m' # No Color
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    MAGENTA=''
    CYAN=''
    WHITE=''
    BOLD=''
    NC=''
fi

# Initialize logging - call this at script start
log_init() {
    local script_name="${1:-script}"

    # Ensure STATE_DIR and LOG_DIR are set
    STATE_DIR="${STATE_DIR:-$HOME/.local/share/debforge}"
    LOG_DIR="${LOG_DIR:-$STATE_DIR/logs}"

    # Create directories (ignore errors if directory is being removed)
    mkdir -p "$STATE_DIR" "$LOG_DIR" 2>/dev/null || true

    # Set LOG_FILE
    LOG_FILE="$LOG_DIR/${script_name}-$(date +%Y%m%d-%H%M%S).log"
    if touch "$LOG_FILE" 2>/dev/null; then
        log_info "Logging started: $LOG_FILE"
    else
        # If we can't create the log file, use /tmp as fallback
        LOG_FILE="/tmp/debforge-${script_name}-$(date +%Y%m%d-%H%M%S).log"
        if touch "$LOG_FILE" 2>/dev/null; then
            log_info "Logging started (fallback): $LOG_FILE"
        else
            # Last resort: no file logging
            LOG_FILE=""
        fi
    fi
}

# Get timestamp
_log_timestamp() {
    date '+%Y-%m-%d %H:%M:%S'
}

# Log to file (always)
_log_file() {
    local level="$1"
    local msg="$2"
    # Only log to file if LOG_FILE is set AND the file or its directory exists
    if [[ -n "$LOG_FILE" ]]; then
        local log_dir
        log_dir=$(dirname "$LOG_FILE" 2>/dev/null || echo "")
        if [[ -n "$log_dir" ]] && [[ -d "$log_dir" ]]; then
            echo "[$(_log_timestamp)] [$level] $msg" >> "$LOG_FILE" 2>/dev/null || true
        fi
    fi
}

# Log to stdout (if not quiet)
_log_stdout() {
    local color="$1"
    local msg="$2"
    if [[ "$LOG_QUIET" != "true" ]]; then
        echo -e "${color}${msg}${NC}"
    fi
}

# Info message
log_info() {
    local msg="$*"
    _log_file "INFO" "$msg"
    _log_stdout "$BLUE" "ℹ $msg"
}

# Success message
log_success() {
    local msg="$*"
    _log_file "SUCCESS" "$msg"
    _log_stdout "$GREEN" "✓ $msg"
}

# Warning message
log_warn() {
    local msg="$*"
    _log_file "WARN" "$msg"
    _log_stdout "$YELLOW" "⚠ $msg"
}

# Error message
log_error() {
    local msg="$*"
    _log_file "ERROR" "$msg"
    _log_stdout "$RED" "✗ $msg" >&2
}

# Debug message (only if verbose)
log_debug() {
    local msg="$*"
    _log_file "DEBUG" "$msg"
    if [[ "$LOG_VERBOSE" == "true" ]]; then
        _log_stdout "$WHITE" "  $msg"
    fi
}

# Section header
log_section() {
    local msg="$*"
    _log_file "SECTION" "$msg"
    _log_stdout "$CYAN" ""
    _log_stdout "$CYAN" "${BOLD}==> $msg${NC}"
    _log_stdout "$CYAN" ""
}

# Step message (numbered step)
log_step() {
    local step="$1"
    local total="$2"
    local msg="$3"
    _log_file "STEP" "[$step/$total] $msg"
    _log_stdout "$MAGENTA" "    [$step/$total] $msg"
}

# Progress indicator (for long operations)
log_progress() {
    local msg="$*"
    _log_file "PROGRESS" "$msg"
    _log_stdout "$WHITE" "    → $msg"
}

# Print a summary box
log_summary() {
    local title="$1"
    shift
    local items=("$@")

    _log_file "SUMMARY" "$title"
    _log_stdout "$CYAN" ""
    _log_stdout "$CYAN" "${BOLD}╔══════════════════════════════════════════════════════════╗${NC}"
    _log_stdout "$CYAN" "${BOLD}║${NC} ${BOLD}$title${NC}"
    _log_stdout "$CYAN" "${BOLD}╠══════════════════════════════════════════════════════════╠${NC}"

    for item in "${items[@]}"; do
        _log_stdout "$CYAN" "${BOLD}║${NC}   $item"
        _log_file "SUMMARY_ITEM" "$item"
    done

    _log_stdout "$CYAN" "${BOLD}╚══════════════════════════════════════════════════════════╝${NC}"
    _log_stdout "$CYAN" ""
}

# Fatal error - log and exit
log_fatal() {
    local msg="$*"
    local exit_code="${2:-1}"
    _log_file "FATAL" "$msg"
    _log_stdout "$RED" "${BOLD}✗ FATAL: $msg${NC}" >&2
    exit "$exit_code"
}
