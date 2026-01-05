#!/bin/bash

# Script to build standalone Linux CLI package

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLIENT_BACKEND_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PROJECT_ROOT="$(cd "$CLIENT_BACKEND_DIR/../.." && pwd)"
DEPLOY_DIR="$CLIENT_BACKEND_DIR/deploy"
DIST_DIR="$PROJECT_ROOT/dist/linux-cli"

# Get version from git tag or default
VERSION="${VERSION:-$(git describe --tags --always 2>/dev/null || echo "dev")}"

echo "🔨 Building WireSocket Linux CLI v${VERSION}"

mkdir -p "$DIST_DIR"
cd "$CLIENT_BACKEND_DIR"

# Build for Linux AMD64
echo "🐧 Building for Linux AMD64..."
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.Version=${VERSION}" \
    -o "$DIST_DIR/wire-socket-client-linux-amd64" ./cmd/client/

# Build for Linux ARM64
echo "🐧 Building for Linux ARM64..."
GOOS=linux GOARCH=arm64 go build -ldflags="-s -w -X main.Version=${VERSION}" \
    -o "$DIST_DIR/wire-socket-client-linux-arm64" ./cmd/client/

# Copy deployment files
echo "📦 Copying deployment files..."
cp "$DEPLOY_DIR/wire-socket-client.service" "$DIST_DIR/"
cp "$DEPLOY_DIR/client.json.example" "$DIST_DIR/"
cp "$DEPLOY_DIR/README.md" "$DIST_DIR/"

# Create install script
cat > "$DIST_DIR/install.sh" << 'INSTALL_EOF'
#!/bin/bash
set -e

# Detect architecture
ARCH=$(uname -m)
case $ARCH in
    x86_64)  BINARY="wire-socket-client-linux-amd64" ;;
    aarch64) BINARY="wire-socket-client-linux-arm64" ;;
    arm64)   BINARY="wire-socket-client-linux-arm64" ;;
    *)       echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "Installing WireSocket VPN Client..."

# Install binary
sudo install -m 755 "$SCRIPT_DIR/$BINARY" /usr/local/bin/wire-socket-client

# Create config directory
sudo mkdir -p /etc/wire-socket
if [ ! -f /etc/wire-socket/client.json ]; then
    sudo cp "$SCRIPT_DIR/client.json.example" /etc/wire-socket/client.json
    sudo chmod 600 /etc/wire-socket/client.json
    echo "Created /etc/wire-socket/client.json - please edit with your credentials"
fi

# Install systemd service
sudo cp "$SCRIPT_DIR/wire-socket-client.service" /etc/systemd/system/
sudo systemctl daemon-reload

echo ""
echo "Installation complete!"
echo ""
echo "Next steps:"
echo "  1. Edit config:   sudo nano /etc/wire-socket/client.json"
echo "  2. Test connect:  sudo wire-socket-client connect"
echo "  3. Enable service: sudo systemctl enable --now wire-socket-client"
INSTALL_EOF
chmod +x "$DIST_DIR/install.sh"

# Create tarball
echo "📦 Creating tarball..."
cd "$PROJECT_ROOT/dist"
tar -czvf "wire-socket-client-linux-${VERSION}.tar.gz" -C linux-cli .

echo ""
echo "✅ Build complete!"
echo ""
echo "📋 Output files:"
ls -lh "$DIST_DIR/"
echo ""
echo "📦 Tarball: $PROJECT_ROOT/dist/wire-socket-client-linux-${VERSION}.tar.gz"
