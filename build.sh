#!/bin/bash

# WireSocket Unified Build Script
# Builds server, client backend, and client frontend (Electron app with bundled backend)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_header() {
    echo ""
    echo -e "${BLUE}============================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}============================================${NC}"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

# Default values
BUILD_SERVER=false
BUILD_CLIENT=false
BUILD_ALL=false
PLATFORM=""
SKIP_DEPS=false

# Help message
show_help() {
    cat << EOF
WireSocket Build Script

Usage: $0 [OPTIONS]

Options:
    -s, --server        Build server only
    -c, --client        Build client (frontend + bundled backend)
    -a, --all           Build everything (server + client)
    -p, --platform      Target platform for client (mac|win|linux|all)
                        Default: current platform
    --skip-deps         Skip npm install and go mod tidy
    -h, --help          Show this help message

Examples:
    $0 --all                    # Build everything for current platform
    $0 --server                 # Build server only
    $0 --client --platform mac  # Build client for macOS
    $0 --all --platform all     # Build everything for all platforms

Output:
    Server:   ./server/dist/wire-socket-server
    Client:   ./client/dist/
EOF
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -s|--server)
            BUILD_SERVER=true
            shift
            ;;
        -c|--client)
            BUILD_CLIENT=true
            shift
            ;;
        -a|--all)
            BUILD_ALL=true
            shift
            ;;
        -p|--platform)
            PLATFORM="$2"
            shift 2
            ;;
        --skip-deps)
            SKIP_DEPS=true
            shift
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        *)
            print_error "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

# If no build target specified, show help
if [[ "$BUILD_SERVER" == "false" && "$BUILD_CLIENT" == "false" && "$BUILD_ALL" == "false" ]]; then
    show_help
    exit 0
fi

# If --all is specified, build both
if [[ "$BUILD_ALL" == "true" ]]; then
    BUILD_SERVER=true
    BUILD_CLIENT=true
fi

# Detect current platform for client build
detect_platform() {
    case "$(uname -s)" in
        Darwin*)    echo "mac" ;;
        Linux*)     echo "linux" ;;
        MINGW*|MSYS*|CYGWIN*) echo "win" ;;
        *)          echo "unknown" ;;
    esac
}

if [[ -z "$PLATFORM" ]]; then
    PLATFORM=$(detect_platform)
fi

# Check prerequisites
check_prerequisites() {
    print_header "Checking Prerequisites"

    local missing=false

    # Check Go
    if command -v go &> /dev/null; then
        print_success "Go $(go version | awk '{print $3}')"
    else
        print_error "Go not found. Please install Go: https://golang.org/dl/"
        missing=true
    fi

    # Check Node.js (for client build)
    if [[ "$BUILD_CLIENT" == "true" ]]; then
        if command -v node &> /dev/null; then
            print_success "Node.js $(node -v)"
        else
            print_error "Node.js not found. Please install Node.js: https://nodejs.org/"
            missing=true
        fi

        if command -v npm &> /dev/null; then
            print_success "npm $(npm -v)"
        else
            print_error "npm not found"
            missing=true
        fi
    fi

    if [[ "$missing" == "true" ]]; then
        exit 1
    fi
}

# Build server
build_server() {
    print_header "Building Server"

    cd "$SCRIPT_DIR/server"

    # Create dist directory
    mkdir -p dist

    if [[ "$SKIP_DEPS" == "false" ]]; then
        echo "Running go mod tidy..."
        go mod tidy
    fi

    echo "Compiling server..."
    go build -o dist/wire-socket-server cmd/server/main.go

    print_success "Server built: ./server/dist/wire-socket-server"

    # Show binary info
    if command -v file &> /dev/null; then
        file dist/wire-socket-server
    fi
    ls -lh dist/wire-socket-server
}

# Build client (frontend + bundled backend)
build_client() {
    print_header "Building Client (Frontend + Backend Bundle)"

    cd "$SCRIPT_DIR/client/frontend"

    # Install npm dependencies
    if [[ "$SKIP_DEPS" == "false" ]]; then
        echo "Installing npm dependencies..."
        npm install
    fi

    # Build backend for target platform(s)
    print_header "Building Client Backend"

    cd "$SCRIPT_DIR/client/backend"

    if [[ "$SKIP_DEPS" == "false" ]]; then
        go mod tidy
    fi

    RESOURCES_DIR="$SCRIPT_DIR/client/frontend/resources/bin"
    mkdir -p "$RESOURCES_DIR/darwin" "$RESOURCES_DIR/linux" "$RESOURCES_DIR/win32"

    case "$PLATFORM" in
        mac)
            echo "Building backend for macOS..."
            GOOS=darwin GOARCH=amd64 go build -o "$RESOURCES_DIR/darwin/wire-socket-client" cmd/client/main.go
            GOOS=darwin GOARCH=arm64 go build -o "$RESOURCES_DIR/darwin/wire-socket-client-arm64" cmd/client/main.go
            chmod +x "$RESOURCES_DIR/darwin/wire-socket-client" "$RESOURCES_DIR/darwin/wire-socket-client-arm64"
            print_success "macOS backend built (amd64 + arm64)"
            ;;
        linux)
            echo "Building backend for Linux..."
            GOOS=linux GOARCH=amd64 go build -o "$RESOURCES_DIR/linux/wire-socket-client" cmd/client/main.go
            chmod +x "$RESOURCES_DIR/linux/wire-socket-client"
            print_success "Linux backend built (amd64)"
            ;;
        win)
            echo "Building backend for Windows..."
            GOOS=windows GOARCH=amd64 go build -o "$RESOURCES_DIR/win32/wire-socket-client.exe" cmd/client/main.go
            print_success "Windows backend built (amd64)"
            ;;
        all)
            echo "Building backend for all platforms..."
            GOOS=darwin GOARCH=amd64 go build -o "$RESOURCES_DIR/darwin/wire-socket-client" cmd/client/main.go
            GOOS=darwin GOARCH=arm64 go build -o "$RESOURCES_DIR/darwin/wire-socket-client-arm64" cmd/client/main.go
            GOOS=linux GOARCH=amd64 go build -o "$RESOURCES_DIR/linux/wire-socket-client" cmd/client/main.go
            GOOS=windows GOARCH=amd64 go build -o "$RESOURCES_DIR/win32/wire-socket-client.exe" cmd/client/main.go
            chmod +x "$RESOURCES_DIR/darwin/wire-socket-client" "$RESOURCES_DIR/darwin/wire-socket-client-arm64" "$RESOURCES_DIR/linux/wire-socket-client" 2>/dev/null || true
            print_success "All platform backends built"
            ;;
        *)
            print_error "Unknown platform: $PLATFORM"
            exit 1
            ;;
    esac

    # Build Electron app
    print_header "Building Electron App"

    cd "$SCRIPT_DIR/client/frontend"

    case "$PLATFORM" in
        mac)
            echo "Building Electron app for macOS..."
            npm run build:mac
            ;;
        linux)
            echo "Building Electron app for Linux..."
            npm run build:linux
            ;;
        win)
            echo "Building Electron app for Windows..."
            npm run build:win
            ;;
        all)
            echo "Building Electron app for all platforms..."
            npm run build
            ;;
    esac

    print_success "Client built: ./client/dist/"

    echo ""
    echo "Distribution packages:"
    ls -lh "$SCRIPT_DIR/client/dist/" 2>/dev/null || true
}

# Main execution
main() {
    echo ""
    echo -e "${GREEN}╔══════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║       WireSocket Build System            ║${NC}"
    echo -e "${GREEN}╚══════════════════════════════════════════╝${NC}"

    check_prerequisites

    if [[ "$BUILD_SERVER" == "true" ]]; then
        build_server
    fi

    if [[ "$BUILD_CLIENT" == "true" ]]; then
        build_client
    fi

    print_header "Build Complete!"

    echo ""
    echo "Build Summary:"
    echo "─────────────────────────────────────────────"

    if [[ "$BUILD_SERVER" == "true" ]]; then
        echo "  Server:   ./server/dist/wire-socket-server"
    fi

    if [[ "$BUILD_CLIENT" == "true" ]]; then
        echo "  Client:   ./client/dist/"
        echo ""
        echo "Note: The client package includes the backend service."
        echo "      Backend is bundled at: <app>/Contents/Resources/bin/"
    fi

    echo ""
}

main
