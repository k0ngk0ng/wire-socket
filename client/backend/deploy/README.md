# WireSocket VPN Client - Linux CLI Installation

## Quick Start

### 1. Download and Install Binary

```bash
# Download the binary (adjust version as needed)
sudo curl -L -o /usr/local/bin/wire-socket-client \
  https://github.com/k0ngk0ng/wire-socket/releases/latest/download/wire-socket-client-linux-amd64

sudo chmod +x /usr/local/bin/wire-socket-client
```

### 2. Create Configuration

```bash
sudo mkdir -p /etc/wire-socket
sudo nano /etc/wire-socket/client.json
```

Add your server credentials:
```json
{
  "server": "https://vpn.example.com",
  "username": "your-username",
  "password": "your-password"
}
```

Secure the config file:
```bash
sudo chmod 600 /etc/wire-socket/client.json
```

### 3. Test Connection

```bash
# Manual test (foreground)
sudo wire-socket-client connect

# Or with explicit parameters
sudo wire-socket-client connect -server https://vpn.example.com -user myuser -pass mypass
```

### 4. Install as Systemd Service

```bash
# Copy service file
sudo curl -L -o /etc/systemd/system/wire-socket-client.service \
  https://raw.githubusercontent.com/k0ngk0ng/wire-socket/main/client/backend/deploy/wire-socket-client.service

# Reload systemd
sudo systemctl daemon-reload

# Enable and start
sudo systemctl enable wire-socket-client
sudo systemctl start wire-socket-client

# Check status
sudo systemctl status wire-socket-client
```

## CLI Usage

```
WireSocket Client - VPN Client

Usage:
  wire-socket-client [options]

Service Mode (for GUI frontend):
  wire-socket-client                     Run as service (for systemd/launchd)
  wire-socket-client -service install    Install as system service
  wire-socket-client -service uninstall  Uninstall system service

CLI Mode (direct VPN connection):
  wire-socket-client connect -server <addr> -user <user> -pass <pass>
  wire-socket-client connect -config /path/to/config.json
  wire-socket-client connect -config /path/to/config.json -daemon

Options:
  -version        Show version and exit
  -help           Show this help message
```

## Daemon Mode

The `-daemon` flag enables:
- Automatic reconnection on connection loss
- Status logging every 60 seconds
- Graceful shutdown on SIGTERM/SIGINT

## Troubleshooting

### Check logs
```bash
sudo journalctl -u wire-socket-client -f
```

### Verify network
```bash
# Check if TUN interface exists
ip addr show wg0

# Check routes
ip route | grep wg0
```

### Manual disconnect
```bash
sudo systemctl stop wire-socket-client
```

## Uninstall

```bash
sudo systemctl stop wire-socket-client
sudo systemctl disable wire-socket-client
sudo rm /etc/systemd/system/wire-socket-client.service
sudo rm /usr/local/bin/wire-socket-client
sudo rm -rf /etc/wire-socket
sudo systemctl daemon-reload
```
