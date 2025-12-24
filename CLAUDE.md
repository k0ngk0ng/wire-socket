# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

WireSocket is a cross-platform VPN solution with three main components:
- **Server** (Go): HTTP API + WireGuard + built-in WebSocket tunnel
- **Client Backend** (Go): System service managing WireGuard interfaces and WebSocket tunnel client
- **Client Frontend** (Electron): Desktop UI for VPN management

## Build Commands

### Server
```bash
cd server
go mod tidy
go build -o wire-socket-server cmd/server/main.go
```

### Client Backend
```bash
cd client/backend
go mod tidy
go build -o wire-socket-client cmd/client/main.go
```

### Client Frontend
```bash
cd client/frontend
npm install
npm start              # Development mode
npm run build          # Build distribution packages
npm run build:mac      # macOS package
npm run build:win      # Windows package
npm run build:linux    # Linux package
```

## Running the System

### First-Time Setup
```bash
# Initialize database (creates admin/admin123 user)
cd server
sudo ./wire-socket-server -init-db
```

### Start All Services
The system requires 3 components running simultaneously:

1. **WireSocket Server** (includes built-in tunnel on port 443)
   ```bash
   cd server
   sudo ./wire-socket-server
   ```

2. **WireSocket Client Service**
   ```bash
   cd client/backend
   sudo ./wire-socket-client

   # Or install as system service:
   sudo ./wire-socket-client -service install
   sudo systemctl start WireSocketClient  # Linux
   sudo launchctl load /Library/LaunchDaemons/WireSocketClient.plist  # macOS
   ```

3. **Client Frontend**
   ```bash
   cd client/frontend
   npm start
   ```

## Architecture

### Server Architecture

The server is organized into internal packages:

- **`internal/database/`**: GORM models (User, Server, AllocatedIP, Session)
  - Uses SQLite by default (`vpn.db`)
  - Can switch to PostgreSQL via config
  - Auto-migrates schema on startup

- **`internal/auth/`**: JWT authentication handlers
  - Issues JWT tokens on login
  - Validates tokens via middleware
  - Session tracking for revocation

- **`internal/wireguard/`**: WireGuard management
  - `manager.go`: Controls WireGuard via backend abstraction (supports kernel and userspace modes)
  - `config_generator.go`: Dynamically generates peer configs
  - Allocates IPs from subnet pool
  - Adds/removes peers dynamically

- **`pkg/wireguard/`**: Shared WireGuard backend abstraction
  - `backend.go`: Interface definitions for kernel/userspace backends
  - `kernel.go`: Kernel WireGuard backend using wgctrl
  - `userspace.go`: Pure Go userspace WireGuard implementation
  - `platform_*.go`: Platform-specific helpers (Linux, macOS, Windows)

- **`internal/api/`**: HTTP API routes (Gin framework)
  - POST `/api/auth/register` - User registration
  - POST `/api/auth/login` - Authentication + config generation
  - GET `/api/config` - WireGuard config (authenticated)
  - GET `/api/status` - Connection status

**Important**: Server must run with sudo/root privileges. Supports both kernel and userspace WireGuard modes.

### Client Backend Architecture

The client backend runs as a system service (Windows Service/macOS LaunchDaemon/Linux systemd):

- **`internal/connection/`**: Connection state management
  - Manages connection lifecycle (disconnected → connecting → connected)
  - Stores server profiles locally
  - Tracks traffic statistics (RX/TX bytes)

- **`internal/wireguard/`**: WireGuard interface operations
  - Uses userspace WireGuard by default (no WireGuard installation required)
  - Applies peer configurations from server
  - Monitors traffic statistics

- **`internal/wstunnel/`**: Built-in WebSocket tunnel client
  - Pure Go implementation (no external binary)
  - Tunnels WireGuard UDP over WebSocket
  - Handles reconnection on failure

- **`internal/api/`**: Local HTTP API (port 41945)
  - POST `/api/connect` - Connect to VPN
  - POST `/api/disconnect` - Disconnect
  - GET `/api/status` - Connection status + stats
  - GET/POST `/api/servers` - Manage saved servers

**Key dependency**: `github.com/kardianos/service` for cross-platform service management. Uses userspace WireGuard by default (no WireGuard installation required on client).

### Electron Frontend Architecture

- **`src/main/index.js`**: Main process
  - IPC handlers for backend communication
  - System tray integration
  - Window management

- **`src/preload/index.js`**: Preload script
  - Exposes safe IPC channels to renderer
  - Bridge between main and renderer

- **`public/index.html`**: UI
  - Communicates with client backend on localhost:41945
  - Shows connection status, traffic stats, server list

### Traffic Flow

```
User App → WireGuard Interface (encrypted)
         → Built-in tunnel client (WebSocket wrapper)
         → Internet (WSS/WS)
         → Built-in tunnel server (unwraps WebSocket)
         → WireGuard Server (decrypts)
         → Internet
```

This allows WireGuard to traverse restrictive firewalls that only allow HTTP/HTTPS.

## Key Configuration Files

### `server/config.yaml`
```yaml
server:
  address: "0.0.0.0:8080"

database:
  path: "./vpn.db"  # Or PostgreSQL DSN

wireguard:
  device_name: "wg0"
  mode: "userspace"         # "userspace" (default, no deps) or "kernel" (requires wireguard-tools)
  listen_port: 51820
  subnet: "10.0.0.0/24"
  dns: "1.1.1.1,8.8.8.8"
  endpoint: "your-server-ip:51820"  # MUST be set correctly
  private_key: ""  # Generated on first run
  public_key: ""   # Generated on first run

auth:
  jwt_secret: "change-this"  # MUST be changed in production

tunnel:
  enabled: true              # Built-in WebSocket tunnel
  listen_addr: "0.0.0.0:443"
  # tls_cert: ""             # Optional: for WSS
  # tls_key: ""
```

## Important Implementation Details

### WireGuard Key Management
- Server generates keypair on first run with `-init-db`
- Client keys are generated per-connection
- Keys use `wgtypes.GeneratePrivateKey()` from `golang.zx2c4.com/wireguard/wgctrl/wgtypes`

### IP Allocation
- Server allocates IPs from configured subnet
- IP assignment tracked in `allocated_ips` table
- Must avoid IP conflicts (check before allocation)

### Service Privileges
- Both server and client backend MUST run with elevated privileges
- WireGuard operations require root/admin access
- Use `sudo` or run as system service

### Cross-Platform Service Installation
```bash
# Install service
sudo ./vpn-client -service install

# Uninstall service
sudo ./vpn-client -service uninstall

# Service control varies by platform:
# Linux: systemctl start/stop WireSocketClient
# macOS: launchctl load/unload WireSocketClient.plist
# Windows: net start/stop WireSocketClient
```

## Database Schema

Tables auto-created on first run:
- `users`: User accounts (username, email, password_hash)
- `servers`: VPN server configs
- `allocated_ips`: IP allocations to users (with public keys)
- `sessions`: JWT session tracking

Query database (SQLite):
```bash
cd server
sqlite3 vpn.db
.tables
SELECT * FROM users;
.quit
```

## Testing Connection

```bash
# Check server API
curl http://localhost:8080/health

# Check client backend API
curl http://127.0.0.1:41945/health

# View WireGuard interfaces
sudo wg show

# Check running processes
ps aux | grep -E "wire-socket-server|wire-socket-client"
```

## Common Development Tasks

### Adding New API Endpoints
- Server: Add routes in `internal/api/router.go`
- Client: Add handlers in `internal/api/server.go`
- Both use Gin framework: `router.GET()`, `router.POST()`, etc.

### Modifying Database Schema
- Update models in `internal/database/db.go`
- GORM auto-migrates on startup
- For major changes, consider migration scripts

### Changing WireGuard Configuration
- Server: Modify `internal/wireguard/config_generator.go`
- Client: Modify `internal/wireguard/interface.go`
- Use wgctrl API: `client.ConfigureDevice()`

### Updating Electron UI
- Edit `public/index.html` for layout
- API calls use `fetch()` to `http://127.0.0.1:41945`
- IPC communication via `src/preload/index.js`

## Debugging

### Server Logs
```bash
cd server
sudo ./wire-socket-server  # Logs to stdout
```

### Client Logs
```bash
# Linux
journalctl -u WireSocketClient -f

# macOS
tail -f /var/log/system.log | grep WireSocket

# Direct run (for debugging)
cd client/backend
sudo ./wire-socket-client  # Logs to stdout
```

### Common Issues

**"Failed to configure WireGuard device"**
- If using `mode: "kernel"`: Install WireGuard tools: `sudo apt install wireguard-tools` (Linux) or `brew install wireguard-tools` (macOS)
- If using `mode: "userspace"` (default): Ensure you have root/sudo privileges for TUN device creation
- On Linux with kernel mode: Load kernel module: `sudo modprobe wireguard`

**"Permission denied"**
- Run with sudo: `sudo ./wire-socket-server` or `sudo ./wire-socket-client`

**"Connection failed"**
- Ensure server is running: `ps aux | grep wire-socket-server`
- Verify ports open: 8080 (API), 443 (tunnel), 51820 (WireGuard UDP)
- Check credentials: default is admin/admin123

## Security Notes

- Change default admin password immediately after first login
- Generate strong JWT secret (32+ random characters)
- Use HTTPS/TLS in production (configure in config.yaml)
- Private keys stored encrypted at rest
- JWT tokens have expiration
