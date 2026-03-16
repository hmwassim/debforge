#!/usr/bin/env bash
# configs_undo.sh - Legacy wrapper for DebForge uninstaller
# This script is kept for backwards compatibility.
# New users should use: scripts/uninstall.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "⚠ Warning: configs_undo.sh is deprecated"
echo "  Please use: $SCRIPT_DIR/uninstall.sh"
echo ""

# Pass all arguments to the new uninstaller
exec "$SCRIPT_DIR/uninstall.sh" "$@"
