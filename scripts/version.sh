#!/bin/bash
# Get version from git tag or commit
# Usage: ./scripts/version.sh

# Try to get version from git tag
VERSION=$(git describe --tags --exact-match 2>/dev/null)

if [ -n "$VERSION" ]; then
    # Remove 'v' prefix if present
    VERSION="${VERSION#v}"
else
    # No tag, try to get tag + commits
    VERSION=$(git describe --tags 2>/dev/null)
    if [ -n "$VERSION" ]; then
        VERSION="${VERSION#v}"
    else
        # No tags at all, use commit hash
        COMMIT=$(git rev-parse --short HEAD 2>/dev/null)
        if [ -n "$COMMIT" ]; then
            # Read base version from VERSION file
            if [ -f "VERSION" ]; then
                BASE_VERSION=$(cat VERSION | tr -d '[:space:]')
                VERSION="${BASE_VERSION}-${COMMIT}"
            else
                VERSION="0.0.0-${COMMIT}"
            fi
        else
            VERSION="dev"
        fi
    fi
fi

echo "$VERSION"
