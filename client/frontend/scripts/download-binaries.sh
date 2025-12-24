#!/bin/bash

# Script to check required binaries for WireSocket
# Note: wstunnel is no longer needed - tunnel functionality is built-in

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESOURCES_DIR="$SCRIPT_DIR/../resources/bin"

echo "📦 Checking binaries for WireSocket..."
echo ""
echo "ℹ️  Note: wstunnel is no longer required - tunnel is now built-in"
echo ""

echo "📋 Current binary files:"
echo ""
echo "=== macOS ==="
ls -lh "$RESOURCES_DIR/darwin/" 2>/dev/null || echo "  (empty)"
echo ""
echo "=== Linux ==="
ls -lh "$RESOURCES_DIR/linux/" 2>/dev/null || echo "  (empty)"
echo ""
echo "=== Windows ==="
ls -lh "$RESOURCES_DIR/win32/" 2>/dev/null || echo "  (empty)"
echo ""

echo "✅ Binary check complete!"
