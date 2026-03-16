#!/usr/bin/env bash
# configs_apply.sh - Legacy wrapper for DebForge installer
# This script is kept for backwards compatibility.
# New users should use: scripts/install.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "⚠ Warning: configs_apply.sh is deprecated"
echo "  Please use: $SCRIPT_DIR/install.sh"
echo ""

# Pass all arguments to the new installer
exec "$SCRIPT_DIR/install.sh" "$@"
