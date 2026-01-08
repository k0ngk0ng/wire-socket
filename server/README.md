# WireSocket Server

WireGuard VPN server with user authentication and management.

## Features

- WireGuard VPN with kernel or userspace mode
- User authentication and session management
- Automatic IP allocation
- NAT and routing management
- Admin API for user management
- CLI tool (`wsctl`) for administration

## Quick Start

### Build

```bash
cd server
go build -o wire-socket-server ./cmd/server
go build -o wsctl ./cmd/wsctl
```

### Run

```bash
# Generate initial config
./wire-socket-server init

# Start server
sudo ./wire-socket-server
```

## Configuration

Default config location: `/etc/wire-socket/config.yaml`

```yaml
# Server settings
listen_port: 51820
api_port: 8080
admin_port: 8081

# Network settings
subnet: "10.8.0.0/24"
dns: "1.1.1.1"

# WireGuard mode: "kernel" or "userspace"
wireguard_mode: "kernel"

# Database
database_path: "/var/lib/wire-socket/wire-socket.db"
```

## Architecture

```
server/
├── cmd/
│   ├── server/          # Main server binary
│   └── wsctl/           # Admin CLI tool
├── internal/
│   ├── api/             # HTTP API routes
│   ├── auth/            # Authentication handler
│   ├── authservice/     # Auth service (standalone mode)
│   ├── tunnelservice/   # Tunnel service (standalone mode)
│   ├── database/        # SQLite database
│   ├── wireguard/       # WireGuard manager
│   ├── nat/             # NAT/iptables management
│   └── route/           # Route management
└── deploy/              # Deployment configs
```

## Components

### WireGuard Manager (`internal/wireguard/`)

Manages WireGuard interface with support for:
- **Kernel mode**: Uses kernel WireGuard module via `wg` command
- **Userspace mode**: Pure Go implementation using wireguard-go

### Auth Service (`internal/authservice/`)

Handles user authentication:
- Username/password login
- Session token generation
- WireGuard key pair generation for clients

### Tunnel Service (`internal/tunnelservice/`)

Manages VPN tunnels:
- Peer registration and IP allocation
- Traffic statistics
- Session cleanup

### Database (`internal/database/`)

SQLite database for:
- User accounts
- Active sessions
- Peer configurations

## API Endpoints

### Client API (`:8080`)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/auth` | POST | Authenticate and get WireGuard config |
| `/api/status` | GET | Get connection status |
| `/api/disconnect` | POST | Disconnect session |

### Admin API (`:8081`)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/admin/users` | GET | List users |
| `/admin/users` | POST | Create user |
| `/admin/users/:id` | DELETE | Delete user |
| `/admin/sessions` | GET | List active sessions |
| `/admin/stats` | GET | Server statistics |

## wsctl CLI

```bash
# User management
wsctl user add <username> <password>
wsctl user list
wsctl user delete <username>

# Session management
wsctl session list
wsctl session kick <session_id>

# Server info
wsctl status
wsctl stats
```

## Deployment

See [docs/DEPLOY.md](../docs/DEPLOY.md) for detailed deployment instructions:

- Systemd service
- Docker container
- Docker Compose

## Requirements

- Linux (kernel mode requires WireGuard module)
- Root/sudo for network operations
- Ports: 51820/UDP (WireGuard), 8080/TCP (API), 8081/TCP (Admin)

## License

MIT
