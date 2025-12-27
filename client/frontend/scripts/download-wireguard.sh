#!/bin/bash

# Script to download/build WireGuard components
# For macOS and Windows, we use wireguard-go (userspace implementation)
# For Linux, we rely on kernel WireGuard (requires wireguard-tools to be installed)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESOURCES_DIR="$SCRIPT_DIR/../resources/bin"

echo "📦 Checking WireGuard components..."

# Check if all WireGuard binaries exist
NEED_BUILD=false
if [ ! -f "$RESOURCES_DIR/darwin/wireguard-go" ]; then
    NEED_BUILD=true
fi
if [ ! -f "$RESOURCES_DIR/darwin/wireguard-go-arm64" ]; then
    NEED_BUILD=true
fi
if [ ! -f "$RESOURCES_DIR/win32/wireguard.exe" ]; then
    NEED_BUILD=true
fi
if [ ! -f "$RESOURCES_DIR/win32/wintun.dll" ]; then
    NEED_BUILD=true
fi

if [ "$NEED_BUILD" = false ]; then
    echo "✓ All WireGuard components already exist, skipping build..."
else
    # Check if go is installed
    if ! command -v go &> /dev/null; then
        echo "❌ Go is not installed. Please install Go to build wireguard-go"
        echo "   Visit: https://golang.org/dl/"
        exit 1
    fi

    # Clone and build wireguard-go
    TEMP_DIR=$(mktemp -d)
    cd "$TEMP_DIR"

    if [ ! -f "$RESOURCES_DIR/darwin/wireguard-go" ] || [ ! -f "$RESOURCES_DIR/darwin/wireguard-go-arm64" ] || [ ! -f "$RESOURCES_DIR/win32/wireguard.exe" ]; then
        echo "🍎 Building wireguard-go for macOS..."
        git clone https://git.zx2c4.com/wireguard-go
        cd wireguard-go

        # Build for macOS AMD64
        if [ ! -f "$RESOURCES_DIR/darwin/wireguard-go" ]; then
            echo "  Building for AMD64..."
            GOOS=darwin GOARCH=amd64 go build -o "$RESOURCES_DIR/darwin/wireguard-go" -v
        else
            echo "  ✓ wireguard-go (AMD64) already exists"
        fi

        # Build for macOS ARM64
        if [ ! -f "$RESOURCES_DIR/darwin/wireguard-go-arm64" ]; then
            echo "  Building for ARM64..."
            GOOS=darwin GOARCH=arm64 go build -o "$RESOURCES_DIR/darwin/wireguard-go-arm64" -v
        else
            echo "  ✓ wireguard-go (ARM64) already exists"
        fi

        # Build for Windows
        if [ ! -f "$RESOURCES_DIR/win32/wireguard.exe" ]; then
            echo "🪟 Building wireguard-go for Windows..."
            GOOS=windows GOARCH=amd64 go build -o "$RESOURCES_DIR/win32/wireguard.exe" -v
        else
            echo "  ✓ wireguard.exe already exists"
        fi

        cd "$TEMP_DIR"
    fi

    # Download wintun driver for Windows
    if [ ! -f "$RESOURCES_DIR/win32/wintun.dll" ]; then
        echo "🪟 Downloading wintun driver for Windows..."
        WINTUN_VERSION="0.14.1"
        curl -L "https://www.wintun.net/builds/wintun-${WINTUN_VERSION}.zip" -o /tmp/wintun.zip
        unzip -o /tmp/wintun.zip -d /tmp/wintun
        cp /tmp/wintun/wintun/bin/amd64/wintun.dll "$RESOURCES_DIR/win32/"
        rm -rf /tmp/wintun /tmp/wintun.zip
    else
        echo "✓ wintun.dll already exists"
    fi

    # Clean up
    cd "$SCRIPT_DIR"
    rm -rf "$TEMP_DIR"
fi

# For Linux, create a README about wireguard-tools
mkdir -p "$RESOURCES_DIR/linux"
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
