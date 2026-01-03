# WireSocket

A cross-platform VPN solution using WireGuard over WebSockets.

## Features

- **Cross-Platform**: macOS, Windows, Linux
- **Zero Dependencies**: Pure Go userspace WireGuard - no kernel modules or external tools required
- **Secure**: WireGuard encryption with WebSocket tunneling
- **All-in-One**: Client bundles all dependencies
- **Modern UI**: Electron desktop app with system tray
- **Auto Service Install**: Automatically installs and starts backend service with privilege escalation
- **Admin Tools**: Web UI and `wsctl` CLI for managing users, routes, and NAT rules
- **MTU Handling**: TCPMSS clamping support for multi-link tunnels

## Quick Start

### Build

```bash
./build.sh --all            # Build server + client
./build.sh --server         # Server only
./build.sh --client -p mac  # Client for macOS (or linux/win/all)
```

### Server

```bash
# Edit server/config.yaml (set your server IP and JWT secret)
sudo ./server/dist/wire-socket-server -init-db   # First time only
sudo ./server/dist/wire-socket-server
```

The server includes a built-in WebSocket tunnel (port 443) and userspace WireGuard - no external dependencies needed.

**WireGuard Mode** (in `config.yaml`):
- `mode: "userspace"` - Pure Go implementation (default, no WireGuard installation required)
- `mode: "kernel"` - Uses kernel WireGuard (requires wireguard-tools)

**Server Management** with `wsctl`:
```bash
# User management
wsctl user list
wsctl user create alice alice@example.com secret123 --admin

# NAT rules (including TCPMSS for MTU issues)
wsctl nat create masquerade --interface=eth0
wsctl nat create tcpmss --interface=wg0 --source=10.0.0.0/24 --mss=1360
wsctl nat apply

# Routes
wsctl route create 192.168.1.0/24 --comment="Internal network"
wsctl route apply
```

**Deployment Options** (see [server/deploy/](server/deploy/)):
- **systemd** - Linux service
- **Docker** - Container deployment
- **docker-compose** - Multi-container setup

### Client

Download from [Releases](../../releases) or use built package in `client/dist/`.

1. Launch WireSocket
2. Enter server address (e.g., `https://vpn.example.com` or `your-server-ip:8080`)
3. Login (default: `admin` / `admin123`)
4. Click "Connect"

**First Launch**: The app will request administrator password to install the VPN service. This only happens once.

**nginx Reverse Proxy**: If using nginx, configure WebSocket proxy:
```nginx
location /tunnel {
    proxy_pass http://127.0.0.1:8443;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_read_timeout 86400;
}
```

And add to server `config.yaml`:
```yaml
tunnel:
  public_host: "vpn.example.com"
  path: "/tunnel"
```

## Documentation

See [docs/](docs/) for full documentation:

- [CLAUDE.md](docs/CLAUDE.md) - Development guide for Claude Code
- [ARCHITECTURE.md](docs/ARCHITECTURE.md) - System architecture
- [DEPLOY.md](docs/DEPLOY.md) - Server deployment (systemd, Docker)
- [DOCKER.md](docs/DOCKER.md) - Docker deployment
- [PACKAGING.md](docs/PACKAGING.md) - Client packaging

## Security

- Change default password immediately
- Set strong JWT secret in `config.yaml`
- Use HTTPS in production

## License

MIT
