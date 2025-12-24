#!/bin/bash
# WireSocket Server Installation Script for systemd

set -e

INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/wire-socket"
DATA_DIR="/var/lib/wire-socket"
SERVICE_FILE="/etc/systemd/system/wire-socket-server.service"

echo "Installing WireSocket Server..."

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "Please run as root (sudo)"
    exit 1
fi

# Create directories
mkdir -p "$CONFIG_DIR" "$DATA_DIR"
chmod 700 "$CONFIG_DIR" "$DATA_DIR"

# Copy binary
if [ -f "./wire-socket-server" ]; then
    cp ./wire-socket-server "$INSTALL_DIR/"
    chmod +x "$INSTALL_DIR/wire-socket-server"
    echo "Binary installed to $INSTALL_DIR/wire-socket-server"
else
    echo "Error: wire-socket-server binary not found in current directory"
    exit 1
fi

# Copy config if not exists
if [ ! -f "$CONFIG_DIR/config.yaml" ]; then
    if [ -f "./config.yaml" ]; then
        cp ./config.yaml "$CONFIG_DIR/"
        echo "Config copied to $CONFIG_DIR/config.yaml"
        echo "Please edit $CONFIG_DIR/config.yaml before starting the service"
    else
        echo "Warning: config.yaml not found, please create $CONFIG_DIR/config.yaml"
    fi
else
    echo "Config already exists at $CONFIG_DIR/config.yaml"
fi

# Install systemd service
cp ./deploy/systemd/wire-socket-server.service "$SERVICE_FILE"
systemctl daemon-reload
echo "Systemd service installed"

echo ""
echo "Installation complete!"
echo ""
echo "Next steps:"
echo "  1. Edit config: sudo nano $CONFIG_DIR/config.yaml"
echo "  2. Initialize DB: sudo $INSTALL_DIR/wire-socket-server -config $CONFIG_DIR/config.yaml -init-db"
echo "  3. Start service: sudo systemctl start wire-socket-server"
echo "  4. Enable on boot: sudo systemctl enable wire-socket-server"
echo "  5. Check status: sudo systemctl status wire-socket-server"
echo ""
