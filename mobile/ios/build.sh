#!/bin/bash

# Build script for WireSocket iOS app
# Prerequisites: Go, Xcode, gomobile

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
SDK_DIR="$PROJECT_ROOT/sdk"
IOS_DIR="$SCRIPT_DIR"
FRAMEWORKS_DIR="$IOS_DIR/WireSocket/Frameworks"

echo "🔨 Building WireSocket iOS App"
echo ""

# Check for gomobile
if ! command -v gomobile &> /dev/null; then
    echo "📦 Installing gomobile..."
    go install golang.org/x/mobile/cmd/gomobile@latest
    go install golang.org/x/mobile/cmd/gobind@latest
fi

# Check for Xcode
if ! xcode-select -p &> /dev/null; then
    echo "❌ Xcode command line tools not found"
    echo "   Run: xcode-select --install"
    exit 1
fi

# Initialize gomobile if needed
echo "🔧 Initializing gomobile..."
gomobile init

# Build the mobile SDK
echo "📱 Building mobile SDK (.xcframework)..."
mkdir -p "$FRAMEWORKS_DIR"

cd "$SDK_DIR"
gomobile bind -v \
    -target=ios \
    -o "$FRAMEWORKS_DIR/Mobile.xcframework" \
    ./mobile

echo "✅ SDK built: $FRAMEWORKS_DIR/Mobile.xcframework"

# Build the iOS app
echo ""
echo "🏗️ Building iOS app..."
cd "$IOS_DIR/WireSocket"

xcodebuild -project WireSocket.xcodeproj \
    -scheme WireSocket \
    -configuration Debug \
    -sdk iphonesimulator \
    -destination 'platform=iOS Simulator,name=iPhone 15' \
    build

echo ""
echo "✅ Build complete!"
echo ""
echo "📱 To run on simulator, open in Xcode:"
echo "   open $IOS_DIR/WireSocket/WireSocket.xcodeproj"
