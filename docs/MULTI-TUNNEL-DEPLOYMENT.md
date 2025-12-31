# Multi-Tunnel Architecture Deployment Guide

## Architecture Overview

```
                    ┌─────────────────┐
                    │   Auth Service  │
                    │  (Central Auth) │
                    │    :8080        │
                    └────────┬────────┘
                             │
           ┌─────────────────┼─────────────────┐
           │                 │                 │
           ▼                 ▼                 ▼
    ┌─────────────┐   ┌─────────────┐   ┌─────────────┐
    │   Tunnel    │   │   Tunnel    │   │   Tunnel    │
    │   hk-01     │   │   jp-01     │   │   us-01     │
    │  Hong Kong  │   │   Tokyo     │   │  New York   │
    └─────────────┘   └─────────────┘   └─────────────┘
           │                 │                 │
           └─────────────────┴─────────────────┘
                             │
                    ┌────────┴────────┐
                    │     Clients     │
                    │ (Multi-connect) │
                    └─────────────────┘
```

## Components

| Component | Description | Default Port |
|-----------|-------------|--------------|
| `wire-socket-auth` | Central authentication & user management | 8080 |
| `wire-socket-tunnel` | Edge tunnel node (WireGuard + WebSocket) | 8080 (API), 443 (WS), 51820 (WG) |
| `wsctl` (auth) | CLI for auth service management | - |
| `wsctl` (tunnel) | CLI for tunnel node management | - |

---

## 1. Deploy Auth Service

### 1.1 Build

```bash
cd auth
go build -o wire-socket-auth cmd/auth/main.go
go build -o wsctl cmd/wsctl/main.go
```

### 1.2 Configure

Create `config.yaml`:

```yaml
server:
  address: "0.0.0.0:8080"

database:
  path: "./auth.db"  # SQLite, or PostgreSQL DSN

auth:
  jwt_secret: "your-secure-random-secret-at-least-32-chars"
  master_token: "secret-token-for-tunnel-registration"  # Tunnels use this to register
```

### 1.3 Initialize Database

```bash
sudo ./wire-socket-auth -init-db
# Creates admin/admin123 user
```

### 1.4 Run as Service

**systemd (Linux):**

```bash
cat > /etc/systemd/system/wire-socket-auth.service << 'EOF'
[Unit]
Description=WireSocket Auth Service
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/wiresocket/auth
ExecStart=/opt/wiresocket/auth/wire-socket-auth -config config.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable wire-socket-auth
systemctl start wire-socket-auth
```

### 1.5 Admin UI

Access: `http://auth-server:8080/admin`

- Login with `admin` / `admin123`
- **Change the default password immediately!**

---

## 2. Deploy Tunnel Service

### 2.1 Build

```bash
cd tunnel
go build -o wire-socket-tunnel cmd/tunnel/main.go
go build -o wsctl cmd/wsctl/main.go
```

### 2.2 Generate WireGuard Keys

```bash
./wire-socket-tunnel -gen-key
# Output:
# Private Key: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
# Public Key:  yyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy
```

### 2.3 Configure

Create `config.yaml`:

```yaml
tunnel:
  id: "hk-01"                    # Unique ID for this tunnel
  name: "Hong Kong"              # Display name
  region: "asia"                 # Region code
  token: ""                      # Will be set after registration
  master_token: "secret-token-for-tunnel-registration"  # Same as auth service

auth:
  url: "http://auth-server:8080" # Auth service URL

server:
  address: "0.0.0.0:8080"        # Local API

database:
  path: "./tunnel.db"

wireguard:
  device_name: "wg0"
  mode: "userspace"              # or "kernel"
  listen_port: 51820
  subnet: "10.0.0.0/24"          # Each tunnel should have unique subnet!
  dns: "1.1.1.1"
  endpoint: "tunnel.example.com:51820"  # Public WireGuard endpoint
  private_key: "xxxxx"           # From -gen-key
  public_key: "yyyyy"            # From -gen-key

ws_tunnel:
  enabled: true
  listen_addr: "0.0.0.0:443"
  public_host: "tunnel.example.com"
  path: "/ws"
  # tls_cert: "/path/to/cert.pem"  # Optional: for WSS
  # tls_key: "/path/to/key.pem"
```

### 2.4 Register with Auth Service

```bash
./wire-socket-tunnel -register
# Output: Successfully registered with auth service
```

This registers the tunnel node with the auth service. The auth service will issue a token that the tunnel uses for future API calls.

### 2.5 Run as Service

**systemd (Linux):**

```bash
cat > /etc/systemd/system/wire-socket-tunnel.service << 'EOF'
[Unit]
Description=WireSocket Tunnel Service
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/wiresocket/tunnel
ExecStart=/opt/wiresocket/tunnel/wire-socket-tunnel -config config.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable wire-socket-tunnel
systemctl start wire-socket-tunnel
```

### 2.6 Admin UI

Access: `http://tunnel-server:8080/admin`

- No login required (local management only)
- Manage routes and NAT rules

---

## 3. Firewall Rules

### Auth Service

```bash
# API only
ufw allow 8080/tcp
```

### Tunnel Service

```bash
# API (internal)
ufw allow 8080/tcp

# WebSocket tunnel
ufw allow 443/tcp

# WireGuard UDP
ufw allow 51820/udp
```

---

## 4. Multiple Tunnel Nodes

Each tunnel node needs a **unique subnet** to avoid IP conflicts:

| Tunnel ID | Region | Subnet |
|-----------|--------|--------|
| hk-01 | Hong Kong | 10.0.0.0/24 |
| jp-01 | Tokyo | 10.1.0.0/24 |
| us-01 | New York | 10.2.0.0/24 |
| eu-01 | Frankfurt | 10.3.0.0/24 |

---

## 5. User Access Control

### Via Auth Admin UI

1. Go to `http://auth-server:8080/admin`
2. Click on a user → "Manage Tunnel Access"
3. Select which tunnels the user can access

### Via wsctl CLI

```bash
cd auth

# List users
./wsctl user list

# Get user's tunnel access
./wsctl user tunnels 1

# Set user's tunnel access (user ID 1 can access hk-01 and jp-01)
./wsctl user set-tunnels 1 hk-01,jp-01
```

---

## 6. Migration from Single Server

If you're migrating from the old single `wire-socket-server`:

### 6.1 Export Users

```bash
# On old server
sqlite3 vpn.db "SELECT username, email, password_hash, is_admin FROM users" > users.csv
```

### 6.2 Import to Auth Service

```bash
# On auth server
cd auth
./wsctl user create <username> <email> <password> [--admin]
```

Or use the admin UI to create users.

### 6.3 Deploy Tunnel Node

The old server becomes a tunnel node:

1. Keep the same WireGuard keys
2. Create tunnel config with same subnet
3. Register with auth service
4. Update client connection settings

---

## 7. Client Configuration

Clients can now connect to multiple tunnels simultaneously via the **Settings → Tunnels** tab.

Each tunnel connection requires:
- Tunnel ID (e.g., `hk-01`)
- Server Address (e.g., `tunnel.hk.example.com:8080`)
- Username
- Password

---

## 8. Monitoring

### Auth Service Logs

```bash
journalctl -u wire-socket-auth -f
```

### Tunnel Service Logs

```bash
journalctl -u wire-socket-tunnel -f
```

### Check WireGuard Peers

```bash
# On tunnel node
./wsctl peer list
```

### Check Registered Tunnels

```bash
# On auth server
./wsctl tunnel list
```

---

## 9. Troubleshooting

### Tunnel Registration Failed

```
Error: failed to register with auth service
```

- Check `master_token` matches between auth and tunnel configs
- Verify auth service is reachable: `curl http://auth-server:8080/health`

### Client Can't Connect

1. Check user has access to the tunnel: Auth Admin UI → Users → Tunnel Access
2. Verify tunnel is registered: `./wsctl tunnel list` on auth server
3. Check tunnel status: `curl http://tunnel-server:8080/health`

### WireGuard Not Working

```bash
# Check interface
ip link show wg0

# Check if using kernel or userspace mode
ps aux | grep wireguard

# Check listening port
ss -ulnp | grep 51820
```

---

## 10. Security Checklist

- [ ] Change default admin password
- [ ] Set strong `jwt_secret` (32+ random chars)
- [ ] Set unique `master_token` for tunnel registration
- [ ] Use HTTPS for auth service in production
- [ ] Use WSS (TLS) for WebSocket tunnels
- [ ] Restrict auth admin UI access by IP if possible
- [ ] Regular backup of `auth.db`
