#!/bin/bash

# Post-install script for Linux
# This script runs after the package is installed

set -e

echo "Setting up WireSocket..."

# Get the installation directory
INSTALL_DIR="/opt/WireSocket"
BIN_DIR="$INSTALL_DIR/resources/bin"

# Set executable permissions
chmod +x "$BIN_DIR/wire-socket-client" 2>/dev/null || true

# Create data directory for the service
mkdir -p /var/lib/wiresocket
chmod 755 /var/lib/wiresocket

# Create systemd service file using the backend's service installer
# The backend binary handles service installation properly
if [ -f "$BIN_DIR/wire-socket-client" ]; then
    "$BIN_DIR/wire-socket-client" -service install 2>/dev/null || true
fi

# If the service file doesn't exist, create it manually
if [ ! -f /etc/systemd/system/WireSocketClient.service ]; then
    cat > /etc/systemd/system/WireSocketClient.service << EOF
[Unit]
Description=WireSocket VPN Client Service
After=network.target

[Service]
Type=simple
ExecStart=$BIN_DIR/wire-socket-client
Restart=on-failure
RestartSec=5
User=root

[Install]
WantedBy=multi-user.target
EOF
fi

# Reload systemd and enable service
systemctl daemon-reload
systemctl enable WireSocketClient.service 2>/dev/null || true

# Start the service
systemctl start WireSocketClient.service 2>/dev/null || true

echo "WireSocket installed successfully!"
echo ""
echo "The VPN client service has been installed and started."
echo "You can manage it with: sudo systemctl {start|stop|status} WireSocketClient"
