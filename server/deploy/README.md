# WireSocket Server Deployment

This directory contains deployment configurations for various environments.

## Deployment Options

| Method | Use Case | Directory |
|--------|----------|-----------|
| **systemd** | Linux servers, VPS | `systemd/` |
| **Docker** | Single container | `docker/` |
| **docker-compose** | Local/dev environment | `docker/` |

## Quick Start

### Option 1: systemd (Linux)

```bash
# Build the binary
cd server && go build -o wire-socket-server ./cmd/server

# Install
sudo ./deploy/systemd/install.sh

# Configure and start
sudo nano /etc/wire-socket/config.yaml
sudo systemctl start wire-socket-server
sudo systemctl enable wire-socket-server
```

### Option 2: Docker

```bash
# Build image
docker build -t wire-socket-server -f Dockerfile .

# Run
docker run -d \
  --name wire-socket-server \
  --cap-add NET_ADMIN \
  --device /dev/net/tun \
  -p 8080:8080 -p 443:443 \
  -v $(pwd)/data:/data \
  wire-socket-server
```

### Option 3: docker-compose

```bash
cd deploy/docker

# Copy and edit config
mkdir -p data
cp config.yaml.example data/config.yaml
nano data/config.yaml

# Start
docker-compose up -d
```

## Configuration

All deployment methods use the same `config.yaml` format:

```yaml
server:
  address: "0.0.0.0:8080"

database:
  path: "/data/vpn.db"  # Adjust path as needed

wireguard:
  device_name: "wg0"
  mode: "userspace"     # or "kernel"
  listen_port: 51820
  subnet: "10.0.0.0/24"
  dns: "1.1.1.1,8.8.8.8"
  endpoint: "your-public-ip:51820"  # IMPORTANT!

auth:
  jwt_secret: "change-this!"  # IMPORTANT!

tunnel:
  enabled: true
  listen_addr: "0.0.0.0:443"
```

## Ports

| Port | Protocol | Description |
|------|----------|-------------|
| 8080 | TCP | HTTP API |
| 443 | TCP | WebSocket tunnel |
| 51820 | UDP | WireGuard (direct) |

## Requirements

### All Methods
- Root/admin privileges (for TUN device creation)

### systemd
- Linux with systemd
- Go 1.21+ (for building)

### Docker/docker-compose
- Docker 20.10+
- docker-compose 2.0+ (for compose)

## First Run

After deployment, initialize the database:

```bash
# systemd
sudo wire-socket-server -config /etc/wire-socket/config.yaml -init-db

# Docker
docker exec wire-socket-server /app/wire-socket-server -config /data/config.yaml -init-db
```

This creates the default admin user:
- Username: `admin`
- Password: `admin123`

**Change this password immediately!**

## Server Management (wsctl)

Use the `wsctl` CLI tool for server-side administration:

```bash
# User management
wsctl user list
wsctl user create alice alice@example.com secret123 --admin
wsctl user update 1 --admin=true
wsctl user delete 2

# Route management
wsctl route list
wsctl route create 192.168.1.0/24 --comment="Internal network"
wsctl route apply

# NAT rule management
wsctl nat list
wsctl nat create masquerade --interface=eth0
wsctl nat create snat --interface=wg0 --source=10.0.0.0/24 --dest=192.168.1.0/24 --to-source=192.168.1.1
wsctl nat create dnat --interface=eth0 --protocol=tcp --port=8080 --to-dest=10.0.0.5:80
wsctl nat create tcpmss --interface=wg0 --source=10.0.0.0/24 --mss=1360  # MTU fix
wsctl nat apply
```

### TCPMSS for MTU Issues

When running WireGuard over WebSocket tunnels, you may encounter MTU issues. Use TCPMSS rules to clamp MSS:

```bash
# Prevent MTU issues on multi-link tunnels
wsctl nat create tcpmss --interface=wg0 --source=10.233.64.0/18 --mss=1360
wsctl nat create tcpmss --interface=wg0 --source=100.100.0.0/16 --mss=1360
wsctl nat apply
```

This generates iptables rules like:
```
iptables -t mangle -A POSTROUTING -o wg0 -s 10.233.64.0/18 -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --set-mss 1360
```

**Note:** NAT rules can be updated without restarting the server - just run `wsctl nat apply`.
