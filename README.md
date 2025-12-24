# WireSocket

A modern, cross-platform VPN solution using WireGuard tunneled over WebSockets (via wstunnel) with an Electron desktop client and Go server.

## Features

- **Cross-Platform**: Supports macOS, Windows, and Linux
- **Secure**: WireGuard encryption with WebSocket tunneling
- **Modern UI**: Electron-based desktop application
- **Auto-Reconnect**: Automatic reconnection on network changes
- **Traffic Statistics**: Real-time upload/download monitoring
- **Multi-Server Support**: Save and manage multiple VPN servers
- **All-in-One Package**: Client bundles all dependencies (no manual setup)

## Architecture

### Components

1. **Server (Go)** - Deployed on your VPN server
   - HTTP API for authentication and config generation
   - Dynamic WireGuard configuration with IP allocation
   - Database (SQLite/PostgreSQL) for user management
   - Works with wstunnel server for WebSocket tunneling

2. **Client (Electron + Go)** - Installed on user devices
   - Modern desktop UI with system tray integration
   - Bundled backend service for WireGuard management
   - Includes wstunnel and wireguard-go binaries
   - Auto-installs as system service on first run

### Traffic Flow

```
User Application
  ↓
WireGuard Interface (encrypted)
  ↓
wstunnel client (WebSocket encapsulation)
  ↓
Internet (WSS/WS)
  ↓
wstunnel server (WebSocket decapsulation)
  ↓
WireGuard Server (decrypted)
  ↓
Internet
```

## Quick Start

### Server Setup

**Prerequisites:** Go 1.21+, root privileges

```bash
# Clone and build
git clone <repository-url> wire-socket
cd wire-socket/server
go mod tidy
go build -o wire-socket-server cmd/server/main.go

# Configure (edit config.yaml with your server IP)
# Initialize database (creates admin/admin123 user)
sudo ./wire-socket-server -init-db

# Start server
sudo ./wire-socket-server

# Start wstunnel server (in separate terminal)
sudo wstunnel server wss://0.0.0.0:443 --restrict-to 127.0.0.1:51820
```

### Client Installation

Download the pre-built package for your platform from [Releases](../../releases), or build from source:

```bash
cd client/frontend
npm install
npm run build:mac    # macOS (.dmg, .zip)
npm run build:win    # Windows (.exe, portable)
npm run build:linux  # Linux (.AppImage, .deb, .rpm)
```

Output files are in `client/dist/` directory.

The package includes everything needed:
- Electron desktop UI
- Go backend service
- wstunnel binary
- WireGuard components (wireguard-go)

### Using the Client

1. Install and launch WireSocket
2. Enter server address (e.g., `your-server-ip:8080`)
3. Enter username and password (default: `admin` / `admin123`)
4. Click "Connect to VPN"
5. View assigned IP, traffic stats, and connection duration
6. Click "Disconnect" to terminate

## Configuration

### Server Configuration

Edit `server/config.yaml`:

```yaml
server:
  address: "0.0.0.0:8080"

wireguard:
  device_name: "wg0"
  listen_port: 51820
  subnet: "10.0.0.0/24"
  dns: "1.1.1.1,8.8.8.8"
  endpoint: "your-server-ip:51820"

auth:
  jwt_secret: "change-this-to-a-random-secret"
```

### Client Configuration

Client settings are stored in:
- **Linux/macOS**: `~/.wire-socket/`
- **Windows**: `%USERPROFILE%\.wire-socket\`

## Troubleshooting

### Server Issues

**Problem**: "Failed to configure WireGuard device"

**Solution**: Ensure WireGuard kernel module is loaded:
```bash
sudo modprobe wireguard  # Linux
```

**Problem**: "Permission denied"

**Solution**: Run server with sudo/admin privileges

### Client Issues

**Problem**: "Connection failed" or "Authentication failed"

**Solution**:
- Verify server is running and accessible
- Check firewall rules (ports 8080, 443, 51820)
- Verify credentials are correct

### Checking Service Status

```bash
# Linux
sudo systemctl status WireSocketClient

# macOS
sudo launchctl list | grep WireSocketClient

# Windows
sc query WireSocketClient
```

### Viewing Logs

Server:
```bash
sudo ./wire-socket-server  # Logs to stdout
```

Client:
```bash
# Linux
journalctl -u WireSocketClient -f

# macOS
tail -f /var/log/system.log | grep WireSocket
```

## Security Considerations

1. **Change default credentials** immediately after initialization
2. **Use HTTPS** for the server API (configure TLS in config.yaml)
3. **Generate strong JWT secret** (32+ random characters)
4. **Keep wstunnel updated** to latest version
5. **Restrict server firewall** to necessary ports only
6. **Use strong passwords** for VPN accounts

## Development

### Project Structure

```
wire-socket/
├── server/               # Go server
│   ├── cmd/server/       # Entry point
│   ├── internal/         # Internal packages
│   └── config.yaml       # Configuration
└── client/
    └── frontend/         # Electron app (includes Go backend)
        ├── src/main/     # Main process
        ├── src/preload/  # Preload scripts
        └── public/       # HTML/assets
```

### Building from Source

**Server:**
```bash
cd server
go build -o wire-socket-server cmd/server/main.go
```

**Client:**
```bash
cd client/frontend
npm install
npm run build
```

See [client/frontend/PACKAGING.md](client/frontend/PACKAGING.md) for detailed build instructions.

## API Documentation

### Server API

- **POST /api/auth/register** - Register new user (admin only)
- **POST /api/auth/login** - Authenticate user, returns JWT + config
- **GET /api/config** - Get WireGuard configuration (authenticated)
- **GET /api/servers** - List available servers
- **GET /api/status** - Get user's connection status

### Client Local API (localhost:41945)

- **POST /api/connect** - Connect to VPN
- **POST /api/disconnect** - Disconnect from VPN
- **GET /api/status** - Get connection status and stats
- **GET /api/servers** - List saved server profiles
- **POST /api/servers** - Add new server profile

## License

MIT License - See LICENSE file for details

## Contributing

Contributions are welcome! Please:
1. Fork the repository
2. Create a feature branch
3. Commit your changes
4. Push to the branch
5. Open a Pull Request

## Support

For issues, questions, or feature requests, please open an issue on GitHub.

## Acknowledgments

- [WireGuard](https://www.wireguard.com/) - Fast, modern VPN protocol
- [wstunnel](https://github.com/erebe/wstunnel) - WebSocket tunneling
- [Electron](https://www.electronjs.org/) - Desktop app framework
- [Gin](https://gin-gonic.com/) - Go web framework
