#!/bin/bash

# Build script for WireSocket Android app
# Prerequisites: Go, Android SDK, NDK

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
SDK_DIR="$PROJECT_ROOT/sdk"
ANDROID_DIR="$SCRIPT_DIR"
LIBS_DIR="$ANDROID_DIR/app/libs"

echo "🔨 Building WireSocket Android App"
echo ""

# Check for gomobile
if ! command -v gomobile &> /dev/null; then
    echo "📦 Installing gomobile..."
    go install golang.org/x/mobile/cmd/gomobile@latest
    go install golang.org/x/mobile/cmd/gobind@latest
fi

# Initialize gomobile if needed
echo "🔧 Initializing gomobile..."
gomobile init

# Build the mobile SDK
echo "📱 Building mobile SDK (.aar)..."
mkdir -p "$LIBS_DIR"

cd "$SDK_DIR"
gomobile bind -v \
    -target=android \
    -androidapi=24 \
    -javapkg=com.wiresocket \
    -o "$LIBS_DIR/mobile.aar" \
    ./mobile

echo "✅ SDK built: $LIBS_DIR/mobile.aar"

# Build the Android app
echo ""
echo "🏗️ Building Android app..."
cd "$ANDROID_DIR"

if [ -f "./gradlew" ]; then
    ./gradlew assembleDebug
else
    echo "⚠️ Gradle wrapper not found. Please run from Android Studio or create gradlew"
    echo "   You can open the project in Android Studio: $ANDROID_DIR"
fi

echo ""
echo "✅ Build complete!"
echo ""
echo "📱 APK location: $ANDROID_DIR/app/build/outputs/apk/debug/app-debug.apk"
