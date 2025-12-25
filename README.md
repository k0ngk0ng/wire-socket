# WireSocket

A cross-platform VPN solution using WireGuard over WebSockets.

## Features

- **Cross-Platform**: macOS, Windows, Linux
- **Zero Dependencies**: Pure Go userspace WireGuard - no kernel modules or external tools required
- **Secure**: WireGuard encryption with WebSocket tunneling
- **All-in-One**: Client bundles all dependencies
- **Modern UI**: Electron desktop app with system tray

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

**Deployment Options** (see [server/deploy/](server/deploy/)):
- **systemd** - Linux service
- **Docker** - Container deployment
- **docker-compose** - Multi-container setup

### Client

Download from [Releases](../../releases) or use built package in `client/dist/`.

1. Launch WireSocket
2. Enter server address (e.g., `your-server-ip:8080`)
3. Login (default: `admin` / `admin123`)
4. Click "Connect"

## Documentation

- [CLAUDE.md](CLAUDE.md) - Development guide
- [server/deploy/](server/deploy/) - Server deployment options
- [client/frontend/PACKAGING.md](client/frontend/PACKAGING.md) - Client packaging details

## Security

- Change default password immediately
- Set strong JWT secret in `config.yaml`
- Use HTTPS in production

## License

MIT
