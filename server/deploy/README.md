# WireSocket Server Deployment

This directory contains deployment configurations for various environments.

## Deployment Options

| Method | Use Case | Directory |
|--------|----------|-----------|
| **systemd** | Linux servers, VPS | `systemd/` |
| **Docker** | Single container | `docker/` |
| **docker-compose** | Local/dev environment | `docker/` |
| **Kubernetes Helm** | Production clusters | `helm/` |

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

### Option 4: Kubernetes Helm

```bash
cd deploy/helm

# Install
helm install wire-socket ./wire-socket \
  --namespace wire-socket \
  --create-namespace \
  --set config.wireguard.endpoint="your-server:51820" \
  --set config.auth.jwtSecret="your-secret"
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

### Kubernetes
- Kubernetes 1.19+
- Helm 3.0+
- Privileged pods or NET_ADMIN capability

## First Run

After deployment, initialize the database:

```bash
# systemd
sudo wire-socket-server -config /etc/wire-socket/config.yaml -init-db

# Docker
docker exec wire-socket-server /app/wire-socket-server -config /data/config.yaml -init-db

# Kubernetes
kubectl exec -it deploy/wire-socket -n wire-socket -- \
  /app/wire-socket-server -config /etc/wire-socket/config.yaml -init-db
```

This creates the default admin user:
- Username: `admin`
- Password: `admin123`

**Change this password immediately!**
