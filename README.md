# WireSocket

A cross-platform VPN solution using WireGuard over WebSockets.

## Features

- **Cross-Platform**: macOS, Windows, Linux
- **Secure**: WireGuard encryption with WebSocket tunneling
- **All-in-One**: Client bundles all dependencies
- **Modern UI**: Electron desktop app with system tray

## Quick Start

### Server

```bash
cd server
go build -o wire-socket-server cmd/server/main.go
sudo ./wire-socket-server -init-db   # First time only
sudo ./wire-socket-server

# In separate terminal
sudo wstunnel server wss://0.0.0.0:443 --restrict-to 127.0.0.1:51820
```

Edit `server/config.yaml` to set your server IP and JWT secret.

### Client

Download from [Releases](../../releases), or build:

```bash
./build.sh --client --platform mac   # or linux/win/all
```

Output: `client/dist/`

### Usage

1. Launch WireSocket
2. Enter server address (e.g., `your-server-ip:8080`)
3. Login (default: `admin` / `admin123`)
4. Click "Connect"

## Build

```bash
./build.sh --help           # Show all options
./build.sh --all            # Build server + client
./build.sh --server         # Server only
./build.sh --client         # Client only
./build.sh --client -p all  # Client for all platforms
```

## Documentation

- [CLAUDE.md](CLAUDE.md) - Development guide
- [client/frontend/PACKAGING.md](client/frontend/PACKAGING.md) - Packaging details

## Security

- Change default password immediately
- Set strong JWT secret in `config.yaml`
- Use HTTPS in production

## License

MIT
