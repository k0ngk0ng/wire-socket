# WireSocket Server - Docker Deployment

## Quick Start

### Build Image

```bash
# From repository root
docker build -t wire-socket-server -f server/Dockerfile .
```

### Run Container

```bash
# Create data directory
mkdir -p ./data

# Copy and edit config
cp server/config.yaml ./data/config.yaml
# Edit ./data/config.yaml as needed

# Initialize database (first time only)
docker run --rm -v $(pwd)/data:/data wire-socket-server -config /data/config.yaml -init-db

# Run server
docker run -d \
  --name wire-socket-server \
  --cap-add NET_ADMIN \
  --device /dev/net/tun \
  -p 8080:8080 \
  -p 443:443 \
  -v $(pwd)/data:/data \
  wire-socket-server
```

### Required Capabilities

The container needs:
- `NET_ADMIN` capability for network configuration
- `/dev/net/tun` device for WireGuard TUN interface

### Ports

| Port | Protocol | Description |
|------|----------|-------------|
| 8080 | TCP | HTTP API |
| 443 | TCP | WebSocket tunnel |
| 51820 | UDP | WireGuard (if not using tunnel) |

### Volumes

| Path | Description |
|------|-------------|
| `/data` | Config and database storage |
| `/etc/wireguard` | WireGuard config persistence |

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `GIN_MODE` | `release` | Gin framework mode |

### Health Check

The container includes a health check that queries `http://localhost:8080/health`.
