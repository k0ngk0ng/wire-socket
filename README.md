# WireSocket

A cross-platform VPN solution using WireGuard over WebSockets.

## Features

- **Cross-Platform**: macOS, Windows, Linux
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
sudo wstunnel server wss://0.0.0.0:443 --restrict-to 127.0.0.1:51820
```

### Client

Download from [Releases](../../releases) or use built package in `client/dist/`.

1. Launch WireSocket
2. Enter server address (e.g., `your-server-ip:8080`)
3. Login (default: `admin` / `admin123`)
4. Click "Connect"

## Documentation

- [CLAUDE.md](CLAUDE.md) - Development guide
- [client/frontend/PACKAGING.md](client/frontend/PACKAGING.md) - Packaging details

## Security

- Change default password immediately
- Set strong JWT secret in `config.yaml`
- Use HTTPS in production

## License

MIT
