#!/bin/bash

# Post-install script for Linux
# This script runs after the package is installed

set -e

echo "🔧 Setting up WireSocket..."

# Check if wireguard-tools is installed
if ! command -v wg &> /dev/null; then
    echo "📦 Installing wireguard-tools..."

    # Detect package manager and install wireguard-tools
    if command -v apt-get &> /dev/null; then
        apt-get update
        apt-get install -y wireguard-tools
    elif command -v yum &> /dev/null; then
        yum install -y wireguard-tools
    elif command -v dnf &> /dev/null; then
        dnf install -y wireguard-tools
    elif command -v pacman &> /dev/null; then
        pacman -S --noconfirm wireguard-tools
    else
        echo "⚠️  Could not detect package manager. Please install wireguard-tools manually."
        echo "   Example: sudo apt install wireguard-tools"
    fi
fi

# Get the installation directory
INSTALL_DIR="/opt/WireSocket"
BIN_DIR="$INSTALL_DIR/resources/bin"

# Set executable permissions
chmod +x "$BIN_DIR/wire-socket-client" 2>/dev/null || true
chmod +x "$BIN_DIR/wstunnel" 2>/dev/null || true

# Create systemd service file
cat > /etc/systemd/system/wiresocket-client.service << EOF
[Unit]
Description=WireSocket VPN Client Service
After=network.target

[Service]
Type=simple
ExecStart=$BIN_DIR/wire-socket-client
Restart=on-failure
User=root

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd and enable service
systemctl daemon-reload
systemctl enable wiresocket-client.service

echo "✅ WireSocket installed successfully!"
echo ""
echo "To start the service manually:"
echo "  sudo systemctl start wiresocket-client"
echo ""
echo "The service will start automatically on boot."
