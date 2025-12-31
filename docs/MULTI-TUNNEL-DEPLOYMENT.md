# Multi-Tunnel Architecture Deployment Guide

## Upgrade from v0.5.4 to v0.6.1

v0.6.1 introduces multi-tunnel architecture. You have two deployment options:

### Option A: Keep Single Server (No Changes)

Continue using `wire-socket-server` as before. No migration needed.

### Option B: Deploy Multi-Tunnel Architecture

Migrate to central auth + distributed tunnel nodes.

---

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

| Component | Binary | Description | Default Port |
|-----------|--------|-------------|--------------|
| Auth Service | `wire-socket-auth` | Central authentication & user management | 8080 |
| Tunnel Service | `wire-socket-tunnel` | Edge tunnel node (WireGuard + WebSocket) | 8080 (API), 443 (WS), 51820 (WG) |
| CLI Tool | `wsctl` | Auto-detects mode from config | - |

---

## 1. Deploy Auth Service

### 1.1 Download Binary

```bash
# From GitHub Release v0.6.1
wget https://github.com/k0ngk0ng/wire-socket/releases/download/v0.6.1/wire-socket-auth-linux-amd64
wget https://github.com/k0ngk0ng/wire-socket/releases/download/v0.6.1/wsctl-linux-amd64

chmod +x wire-socket-auth-linux-amd64 wsctl-linux-amd64
mv wire-socket-auth-linux-amd64 /opt/wiresocket/auth/wire-socket-auth
mv wsctl-linux-amd64 /opt/wiresocket/auth/wsctl
```

Or build from source:

```bash
cd server
go build -o wire-socket-auth ./cmd/auth
go build -o wsctl ./cmd/wsctl
```

### 1.2 Configure

Create `/opt/wiresocket/auth/config.yaml`:

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
cd /opt/wiresocket/auth
sudo ./wire-socket-auth -init-db
# Creates admin/admin123 user
```

### 1.4 Run as Service

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

### 2.1 Download Binary

```bash
wget https://github.com/k0ngk0ng/wire-socket/releases/download/v0.6.1/wire-socket-tunnel-linux-amd64
wget https://github.com/k0ngk0ng/wire-socket/releases/download/v0.6.1/wsctl-linux-amd64

chmod +x wire-socket-tunnel-linux-amd64 wsctl-linux-amd64
mv wire-socket-tunnel-linux-amd64 /opt/wiresocket/tunnel/wire-socket-tunnel
mv wsctl-linux-amd64 /opt/wiresocket/tunnel/wsctl
```

Or build from source:

```bash
cd server
go build -o wire-socket-tunnel ./cmd/tunnel
go build -o wsctl ./cmd/wsctl
```

### 2.2 Generate WireGuard Keys

```bash
./wire-socket-tunnel -gen-key
# Output:
# Private Key: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
# Public Key:  yyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy
```

### 2.3 Configure

Create `/opt/wiresocket/tunnel/config.yaml`:

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

### 2.5 Run as Service

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
ufw allow 8080/tcp
```

### Tunnel Service

```bash
ufw allow 8080/tcp    # API (internal)
ufw allow 443/tcp     # WebSocket tunnel
ufw allow 51820/udp   # WireGuard UDP
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

### Via wsctl CLI (on auth server)

```bash
cd /opt/wiresocket/auth

# List users
./wsctl user list

# Get user's tunnel access
./wsctl user tunnels 1

# Set user's tunnel access (user ID 1 can access hk-01 and jp-01)
./wsctl user set-tunnels 1 hk-01,jp-01

# List registered tunnels
./wsctl tunnel list
```

### Via wsctl CLI (on tunnel node)

```bash
cd /opt/wiresocket/tunnel

# List routes
./wsctl route list

# List NAT rules
./wsctl nat list

# List connected peers
./wsctl peer list
```

---

## 6. Migration from v0.5.4 Single Server

### 6.1 Export Users

```bash
# On old v0.5.4 server
sqlite3 vpn.db "SELECT username, email, password_hash, is_admin FROM users" > users.csv
```

### 6.2 Deploy Auth Service

Follow Section 1 above to deploy auth service.

### 6.3 Import Users

```bash
# On auth server
cd /opt/wiresocket/auth
./wsctl user create <username> <email> <password> [--admin]
```

Or use the admin UI to create users.

### 6.4 Convert Old Server to Tunnel Node

Your old v0.5.4 server becomes a tunnel node:

1. Stop the old server:
   ```bash
   systemctl stop wire-socket-server
   ```

2. Keep the same WireGuard keys from old config

3. Create tunnel config with same subnet

4. Register with auth service:
   ```bash
   ./wire-socket-tunnel -register
   ```

5. Start tunnel service:
   ```bash
   systemctl start wire-socket-tunnel
   ```

6. Update client connection settings to point to the tunnel

---

## 7. Client Configuration

Clients v0.6.1+ can connect to multiple tunnels simultaneously via **Settings → Tunnels**.

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

### Check WireGuard Peers (on tunnel node)

```bash
./wsctl peer list
```

### Check Registered Tunnels (on auth server)

```bash
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
