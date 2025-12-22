#!/bin/bash

# Script to build client backend for all platforms

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
CLIENT_BACKEND_DIR="$PROJECT_ROOT/client/backend"
RESOURCES_DIR="$SCRIPT_DIR/../resources/bin"

echo "🔨 Checking client backend builds..."

cd "$CLIENT_BACKEND_DIR"

# Get source modification time
get_source_mtime() {
    find "$CLIENT_BACKEND_DIR" -name "*.go" -type f -exec stat -f "%m" {} \; 2>/dev/null | sort -rn | head -1 || \
    find "$CLIENT_BACKEND_DIR" -name "*.go" -type f -printf "%T@\n" 2>/dev/null | sort -rn | head -1 || \
    echo "0"
}

# Get binary modification time
get_bin_mtime() {
    if [ -f "$1" ]; then
        stat -f "%m" "$1" 2>/dev/null || stat -c "%Y" "$1" 2>/dev/null || echo "0"
    else
        echo "0"
    fi
}

SOURCE_MTIME=$(get_source_mtime)

# Build for macOS AMD64
BIN_MTIME=$(get_bin_mtime "$RESOURCES_DIR/darwin/wire-socket-client")
if [ "$SOURCE_MTIME" -gt "$BIN_MTIME" ] 2>/dev/null || [ ! -f "$RESOURCES_DIR/darwin/wire-socket-client" ]; then
    echo "🍎 Building for macOS AMD64..."
    GOOS=darwin GOARCH=amd64 go build -o "$RESOURCES_DIR/darwin/wire-socket-client" cmd/client/main.go
    chmod +x "$RESOURCES_DIR/darwin/wire-socket-client"
else
    echo "✓ macOS AMD64 build is up to date"
fi

# Build for macOS ARM64
BIN_MTIME=$(get_bin_mtime "$RESOURCES_DIR/darwin/wire-socket-client-arm64")
if [ "$SOURCE_MTIME" -gt "$BIN_MTIME" ] 2>/dev/null || [ ! -f "$RESOURCES_DIR/darwin/wire-socket-client-arm64" ]; then
    echo "🍎 Building for macOS ARM64..."
    GOOS=darwin GOARCH=arm64 go build -o "$RESOURCES_DIR/darwin/wire-socket-client-arm64" cmd/client/main.go
    chmod +x "$RESOURCES_DIR/darwin/wire-socket-client-arm64"
else
    echo "✓ macOS ARM64 build is up to date"
fi

# Build for Linux AMD64
BIN_MTIME=$(get_bin_mtime "$RESOURCES_DIR/linux/wire-socket-client")
if [ "$SOURCE_MTIME" -gt "$BIN_MTIME" ] 2>/dev/null || [ ! -f "$RESOURCES_DIR/linux/wire-socket-client" ]; then
    echo "🐧 Building for Linux AMD64..."
    GOOS=linux GOARCH=amd64 go build -o "$RESOURCES_DIR/linux/wire-socket-client" cmd/client/main.go
    chmod +x "$RESOURCES_DIR/linux/wire-socket-client"
else
    echo "✓ Linux AMD64 build is up to date"
fi

# Build for Windows AMD64
BIN_MTIME=$(get_bin_mtime "$RESOURCES_DIR/win32/wire-socket-client.exe")
if [ "$SOURCE_MTIME" -gt "$BIN_MTIME" ] 2>/dev/null || [ ! -f "$RESOURCES_DIR/win32/wire-socket-client.exe" ]; then
    echo "🪟 Building for Windows AMD64..."
    GOOS=windows GOARCH=amd64 go build -o "$RESOURCES_DIR/win32/wire-socket-client.exe" cmd/client/main.go
else
    echo "✓ Windows AMD64 build is up to date"
fi

echo "✅ All builds ready!"
echo ""
echo "📋 Built files:"
ls -lh "$RESOURCES_DIR/darwin/"
ls -lh "$RESOURCES_DIR/linux/"
ls -lh "$RESOURCES_DIR/win32/"
