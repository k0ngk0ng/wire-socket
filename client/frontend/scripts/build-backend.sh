#!/bin/bash

# Script to build client backend for all platforms

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
CLIENT_BACKEND_DIR="$PROJECT_ROOT/client/backend"
RESOURCES_DIR="$SCRIPT_DIR/../resources/bin"

echo "🔨 Building client backend for all platforms..."

cd "$CLIENT_BACKEND_DIR"

# Build for macOS AMD64
echo "🍎 Building for macOS AMD64..."
GOOS=darwin GOARCH=amd64 go build -o "$RESOURCES_DIR/darwin/wire-socket-client" cmd/client/main.go
chmod +x "$RESOURCES_DIR/darwin/wire-socket-client"

# Build for macOS ARM64
echo "🍎 Building for macOS ARM64..."
GOOS=darwin GOARCH=arm64 go build -o "$RESOURCES_DIR/darwin/wire-socket-client-arm64" cmd/client/main.go
chmod +x "$RESOURCES_DIR/darwin/wire-socket-client-arm64"

# Build for Linux AMD64
echo "🐧 Building for Linux AMD64..."
GOOS=linux GOARCH=amd64 go build -o "$RESOURCES_DIR/linux/wire-socket-client" cmd/client/main.go
chmod +x "$RESOURCES_DIR/linux/wire-socket-client"

# Build for Windows AMD64
echo "🪟 Building for Windows AMD64..."
GOOS=windows GOARCH=amd64 go build -o "$RESOURCES_DIR/win32/wire-socket-client.exe" cmd/client/main.go

echo "✅ All builds completed successfully!"
echo ""
echo "📋 Built files:"
ls -lh "$RESOURCES_DIR/darwin/"
ls -lh "$RESOURCES_DIR/linux/"
ls -lh "$RESOURCES_DIR/win32/"
