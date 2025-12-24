# WireSocket

A modern, cross-platform VPN solution using WireGuard tunneled over WebSockets (via wstunnel) with an Electron frontend and Go backend.

## Features

- **Cross-Platform**: Supports macOS, Windows, and Linux
- **Secure**: WireGuard encryption with WebSocket tunneling
- **Modern UI**: Electron-based desktop application
- **Auto-Reconnect**: Automatic reconnection on network changes
- **Traffic Statistics**: Real-time upload/download monitoring
- **Multi-Server Support**: Save and manage multiple VPN servers
- **System Service**: Runs as a system service with proper privileges

## Architecture

### Components

1. **Server (Go)**
   - HTTP API for authentication and config generation
   - Dynamic WireGuard configuration with IP allocation
   - Database (SQLite/PostgreSQL) for user management
   - Integration with wstunnel server

2. **Client Backend (Go)**
   - System service (Windows Service, macOS LaunchDaemon, Linux systemd)
   - WireGuard interface management
   - wstunnel client integration
   - Local HTTP API for Electron frontend

3. **Client Frontend (Electron)**
   - Modern, user-friendly interface
   - Login and connection management
   - Real-time status updates
   - System tray integration

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

## Prerequisites

### Server

- Go 1.21 or higher
- WireGuard kernel module or wireguard-go
- wstunnel binary ([download](https://github.com/erebe/wstunnel/releases))
- Root/admin privileges

### Client

- Go 1.21 or higher (for building)
- Node.js 18+ and npm (for Electron)
- WireGuard tools
- wstunnel binary
- Root/admin privileges (for VPN operations)

## Quick Start

### Server Setup

```bash
# Clone and build
git clone <repository-url> wire-socket
cd wire-socket/server
go mod tidy
go build -o wire-socket-server cmd/server/main.go

# Configure (edit config.yaml with your settings)
# Initialize database (creates admin/admin123 user)
sudo ./wire-socket-server -init-db

# Start server
sudo ./wire-socket-server

# Start wstunnel server (in separate terminal)
sudo wstunnel server wss://0.0.0.0:443 --restrict-to 127.0.0.1:51820
```

### Client Setup

```bash
# Build client backend
cd client/backend
go mod tidy
go build -o wire-socket-client cmd/client/main.go

# Install and start as system service
sudo ./wire-socket-client -service install

# Linux
sudo systemctl start WireSocketClient && sudo systemctl enable WireSocketClient

# macOS
sudo launchctl load /Library/LaunchDaemons/WireSocketClient.plist

# Windows (as Administrator)
# .\wire-socket-client.exe -service install
# net start WireSocketClient

# Build and run frontend
cd ../frontend
npm install
npm start
```

### Using the Client

1. Launch the Electron app
2. Enter server address (e.g., `your-server-ip:8080`)
3. Enter username and password (default: `admin` / `admin123`)
4. Click "Connect to VPN"
5. View assigned IP, traffic stats, and connection duration
6. Click "Disconnect" to terminate

## Building Distribution Packages

WireSocket supports one-click packaging with all required dependencies bundled (no manual WireGuard or wstunnel installation needed):

```bash
cd client/frontend

# Build for all platforms (auto-downloads dependencies)
npm run build

# Build for specific platform
npm run build:mac    # macOS (.dmg, .zip)
npm run build:win    # Windows (.exe, portable)
npm run build:linux  # Linux (.AppImage, .deb, .rpm)
```

**Package includes:**
- Electron frontend UI
- Go client backend service
- wstunnel binary (all platforms)
- WireGuard components (wireguard-go)
- Auto service installation scripts

Output files are located in `client/dist/` directory.

See [client/frontend/PACKAGING.md](client/frontend/PACKAGING.md) for detailed packaging instructions.

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
- **Linux/macOS**: `~/.vpn-client/`
- **Windows**: `%USERPROFILE%\.vpn-client\`

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

**Problem**: "Failed to create WireGuard interface"

**Solution**:
- Ensure client backend service is running with admin privileges
- Check WireGuard tools are installed
- On Linux: `sudo apt install wireguard-tools`
- On macOS: `brew install wireguard-tools`

**Problem**: "wstunnel binary not found"

**Solution**: Install wstunnel:
```bash
# Download from https://github.com/erebe/wstunnel/releases
# Place in /usr/local/bin/ or add to PATH
```

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

Client backend:
```bash
# Linux
journalctl -u WireSocketClient -f

# macOS
tail -f /var/log/system.log | grep VPN
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
├── server/               # Go server backend
│   ├── cmd/server/       # Server entry point
│   ├── internal/         # Internal packages
│   └── config.yaml       # Server configuration
└── client/               # Client application
    ├── backend/          # Go client backend
    │   ├── cmd/client/   # Client entry point
    │   └── internal/     # Internal packages
    └── frontend/         # Electron frontend
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

**Client Backend:**
```bash
cd client/backend
go build -o wire-socket-client cmd/client/main.go
```

**Client Frontend:**
```bash
cd client/frontend
npm install
npm run build
```

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
