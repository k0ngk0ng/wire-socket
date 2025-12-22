#!/bin/bash

# Script to download all required binaries for WireSocket
# This includes wstunnel for all platforms

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESOURCES_DIR="$SCRIPT_DIR/../resources/bin"

echo "📦 Checking binaries for WireSocket..."

# wstunnel version to download
WSTUNNEL_VERSION="v10.1.4"
WSTUNNEL_VERSION_NUM="${WSTUNNEL_VERSION#v}"  # Remove 'v' prefix for filenames

# Download wstunnel for macOS
if [ -f "$RESOURCES_DIR/darwin/wstunnel" ]; then
    echo "✓ wstunnel for macOS already exists, skipping..."
else
    echo "⬇️  Downloading wstunnel for macOS..."
    curl -L "https://github.com/erebe/wstunnel/releases/download/${WSTUNNEL_VERSION}/wstunnel_${WSTUNNEL_VERSION_NUM}_darwin_amd64.tar.gz" -o /tmp/wstunnel-darwin.tar.gz
    tar -xzf /tmp/wstunnel-darwin.tar.gz -C /tmp
    mv /tmp/wstunnel "$RESOURCES_DIR/darwin/wstunnel"
    chmod +x "$RESOURCES_DIR/darwin/wstunnel"
    rm /tmp/wstunnel-darwin.tar.gz
fi

# Download wstunnel for macOS ARM64
if [ -f "$RESOURCES_DIR/darwin/wstunnel-arm64" ]; then
    echo "✓ wstunnel for macOS ARM64 already exists, skipping..."
else
    echo "⬇️  Downloading wstunnel for macOS ARM64..."
    curl -L "https://github.com/erebe/wstunnel/releases/download/${WSTUNNEL_VERSION}/wstunnel_${WSTUNNEL_VERSION_NUM}_darwin_arm64.tar.gz" -o /tmp/wstunnel-darwin-arm64.tar.gz
    tar -xzf /tmp/wstunnel-darwin-arm64.tar.gz -C /tmp
    mv /tmp/wstunnel "$RESOURCES_DIR/darwin/wstunnel-arm64"
    chmod +x "$RESOURCES_DIR/darwin/wstunnel-arm64"
    rm /tmp/wstunnel-darwin-arm64.tar.gz
fi

# Download wstunnel for Linux
if [ -f "$RESOURCES_DIR/linux/wstunnel" ]; then
    echo "✓ wstunnel for Linux already exists, skipping..."
else
    echo "⬇️  Downloading wstunnel for Linux..."
    curl -L "https://github.com/erebe/wstunnel/releases/download/${WSTUNNEL_VERSION}/wstunnel_${WSTUNNEL_VERSION_NUM}_linux_amd64.tar.gz" -o /tmp/wstunnel-linux.tar.gz
    tar -xzf /tmp/wstunnel-linux.tar.gz -C /tmp
    mv /tmp/wstunnel "$RESOURCES_DIR/linux/wstunnel"
    chmod +x "$RESOURCES_DIR/linux/wstunnel"
    rm /tmp/wstunnel-linux.tar.gz
fi

# Download wstunnel for Windows
if [ -f "$RESOURCES_DIR/win32/wstunnel.exe" ]; then
    echo "✓ wstunnel for Windows already exists, skipping..."
else
    echo "⬇️  Downloading wstunnel for Windows..."
    curl -L "https://github.com/erebe/wstunnel/releases/download/${WSTUNNEL_VERSION}/wstunnel_${WSTUNNEL_VERSION_NUM}_windows_amd64.tar.gz" -o /tmp/wstunnel-windows.tar.gz
    tar -xzf /tmp/wstunnel-windows.tar.gz -C /tmp
    mv /tmp/wstunnel.exe "$RESOURCES_DIR/win32/wstunnel.exe"
    rm /tmp/wstunnel-windows.tar.gz
fi

echo "✅ All binaries ready!"
echo ""
echo "📋 Binary files:"
ls -lh "$RESOURCES_DIR/darwin/"
ls -lh "$RESOURCES_DIR/linux/"
ls -lh "$RESOURCES_DIR/win32/"
