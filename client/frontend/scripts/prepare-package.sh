#!/bin/bash

# Master script to prepare all resources for packaging

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "🚀 Preparing WireSocket for packaging..."
echo ""

# Step 1: Download binaries
echo "=========================================="
echo "Step 1/4: Downloading third-party binaries"
echo "=========================================="
"$SCRIPT_DIR/download-binaries.sh"
echo ""

# Step 2: Download/build WireGuard components
echo "=========================================="
echo "Step 2/4: Setting up WireGuard components"
echo "=========================================="
"$SCRIPT_DIR/download-wireguard.sh"
echo ""

# Step 3: Build backend
echo "=========================================="
echo "Step 3/4: Building client backend"
echo "=========================================="
"$SCRIPT_DIR/build-backend.sh"
echo ""

# Step 4: Apply build configuration
echo "=========================================="
echo "Step 4/4: Applying build configuration"
echo "=========================================="
"$SCRIPT_DIR/apply-config.sh"
echo ""

echo "✅ All preparation completed!"
echo ""
echo "Now you can run:"
echo "  npm run build       # Build for the current platform"
echo "  npm run build:mac   # Build for macOS on a macOS host"
echo "  npm run build:win   # Build for Windows on a Windows host"
echo "  npm run build:linux # Build for Linux on a Linux host"
