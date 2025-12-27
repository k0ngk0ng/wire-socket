#!/bin/bash

# Apply build configuration to HTML template

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FRONTEND_DIR="$(dirname "$SCRIPT_DIR")"
CONFIG_FILE="$FRONTEND_DIR/build.config.json"
EXAMPLE_CONFIG="$FRONTEND_DIR/build.config.example.json"
HTML_FILE="$FRONTEND_DIR/public/index.html"

echo "📝 Applying build configuration..."

# Use build.config.json if exists, otherwise use example
if [ ! -f "$CONFIG_FILE" ]; then
    if [ -f "$EXAMPLE_CONFIG" ]; then
        echo "   Using build.config.example.json (copy to build.config.json to customize)"
        CONFIG_FILE="$EXAMPLE_CONFIG"
    else
        echo "⚠️  No config found, using defaults"
        DEFAULT_SERVER="vpn.example.com"
        DEFAULT_USERNAME=""
        DEFAULT_PASSWORD=""
        CONFIG_FILE=""
    fi
fi

if [ -n "$CONFIG_FILE" ]; then
    # Read config values using node (cross-platform JSON parsing)
    DEFAULT_SERVER=$(node -e "console.log(require('$CONFIG_FILE').defaultServer || 'vpn.example.com')")
    DEFAULT_USERNAME=$(node -e "console.log(require('$CONFIG_FILE').defaultUsername || '')")
    DEFAULT_PASSWORD=$(node -e "console.log(require('$CONFIG_FILE').defaultPassword || '')")
    echo "   Server: $DEFAULT_SERVER"
fi

# Apply replacements to HTML
if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS sed requires empty string for -i
    sed -i '' "s|{{DEFAULT_SERVER}}|$DEFAULT_SERVER|g" "$HTML_FILE"
    sed -i '' "s|{{DEFAULT_USERNAME}}|$DEFAULT_USERNAME|g" "$HTML_FILE"
    sed -i '' "s|{{DEFAULT_PASSWORD}}|$DEFAULT_PASSWORD|g" "$HTML_FILE"
else
    # Linux sed
    sed -i "s|{{DEFAULT_SERVER}}|$DEFAULT_SERVER|g" "$HTML_FILE"
    sed -i "s|{{DEFAULT_USERNAME}}|$DEFAULT_USERNAME|g" "$HTML_FILE"
    sed -i "s|{{DEFAULT_PASSWORD}}|$DEFAULT_PASSWORD|g" "$HTML_FILE"
fi

echo "✅ Build configuration applied"
