#!/bin/bash

# Script to download/build WireGuard components
# For macOS and Windows, we use wireguard-go (userspace implementation)
# For Linux, we rely on kernel WireGuard (requires wireguard-tools to be installed)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESOURCES_DIR="$SCRIPT_DIR/../resources/bin"

echo "📦 Setting up WireGuard components..."

# Check if go is installed
if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed. Please install Go to build wireguard-go"
    echo "   Visit: https://golang.org/dl/"
    exit 1
fi

# Clone and build wireguard-go for macOS
echo "🍎 Building wireguard-go for macOS..."
TEMP_DIR=$(mktemp -d)
cd "$TEMP_DIR"
git clone https://git.zx2c4.com/wireguard-go
cd wireguard-go

# Build for macOS AMD64
echo "  Building for AMD64..."
GOOS=darwin GOARCH=amd64 go build -o "$RESOURCES_DIR/darwin/wireguard-go" -v

# Build for macOS ARM64
echo "  Building for ARM64..."
GOOS=darwin GOARCH=arm64 go build -o "$RESOURCES_DIR/darwin/wireguard-go-arm64" -v

# Build for Windows
echo "🪟 Building wireguard-go for Windows..."
GOOS=windows GOARCH=amd64 go build -o "$RESOURCES_DIR/win32/wireguard.exe" -v

# Download wintun driver for Windows
echo "🪟 Downloading wintun driver for Windows..."
WINTUN_VERSION="0.14.1"
curl -L "https://www.wintun.net/builds/wintun-${WINTUN_VERSION}.zip" -o /tmp/wintun.zip
unzip -o /tmp/wintun.zip -d /tmp/wintun
cp /tmp/wintun/wintun/bin/amd64/wintun.dll "$RESOURCES_DIR/win32/"
rm -rf /tmp/wintun /tmp/wintun.zip

# Clean up
cd "$SCRIPT_DIR"
rm -rf "$TEMP_DIR"

# For Linux, create a README about wireguard-tools
cat > "$RESOURCES_DIR/linux/WIREGUARD-README.txt" << 'EOF'
WireGuard on Linux
==================

On Linux, WireSocket uses the kernel's built-in WireGuard support.
The installation script will check for and install wireguard-tools if needed.

Required package: wireguard-tools (provides wg and wg-quick commands)

The installer will attempt to install this automatically using your system's
package manager (apt, yum, dnf, or pacman).
EOF

echo "✅ WireGuard components ready!"
echo ""
echo "📋 Built files:"
ls -lh "$RESOURCES_DIR/darwin/" 2>/dev/null || true
ls -lh "$RESOURCES_DIR/win32/" 2>/dev/null || true
cat "$RESOURCES_DIR/linux/WIREGUARD-README.txt"
