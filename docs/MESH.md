# WireSocket Mesh 组网方案

本文档描述 WireSocket 的 Mesh 组网功能设计，支持多个 Server 节点组成网络，让 Client 通过单一入口访问所有节点可达的网络。

## 目录

- [概述](#概述)
- [使用场景](#使用场景)
- [架构设计](#架构设计)
- [数据模型](#数据模型)
- [配置说明](#配置说明)
- [API 参考](#api-参考)
- [部署指南](#部署指南)
- [数据流说明](#数据流说明)
- [实现计划](#实现计划)

---

## 概述

### 问题背景

现有 WireSocket 是 Client-Server 1:1 架构：

```text
Client ──TLS──► Server ──► 目标网络
```

当存在多个 Server，且不同 Server 能访问不同的受限网络时，Client 需要分别连接多个 Server，管理复杂。

### 解决方案

Mesh 功能让多个 Server 通过 WireGuard over WebSocket 互联，Client 只需连接一个入口节点，即可访问所有节点可达的网络：

```text
                    ┌─────────────┐
                    │   网络A     │ ← 只有 Server1 能访问
                    └──────▲──────┘
                           │
Client ══WSS══► Server1 ◄══WSS══► Server2 ──► 网络B (只有 Server2 能访问)
                    │                 │
                    └──────WSS────────┘
                           │
                           ▼
                       Server3 ──► 网络C (只有 Server3 能访问)
```

### 核心特性

| 特性 | 说明 |
|------|------|
| **单一入口** | Client 只连一个 Server，自动路由到正确的出口节点 |
| **声明式配置** | 每个节点只需声明自己能访问的网络 |
| **自动同步** | 节点间自动同步路由信息 |
| **零 NAT** | 节点间使用 WireGuard 隧道，无需复杂的 NAT 配置 |
| **复用 Tunnel** | Mesh 连接复用现有 WireGuard over WebSocket 机制 |
| **向后兼容** | 不启用 Mesh 时，系统行为与现有完全一致 |

---

## 使用场景

### 场景一：多出口网络

```text
┌─────────────────────────────────────────────────────────────────┐
│                                                                  │
│  Client A, B, C 都连接 Server1（北京）                           │
│                                                                  │
│  Server1 (北京)     Server2 (上海)     Server3 (香港)            │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────┐            │
│  │ 入口节点    │◄─►│             │◄─►│             │            │
│  │ 10.254.0.1  │   │ 10.254.0.2  │   │ 10.254.0.3  │            │
│  └──────┬──────┘   └──────┬──────┘   └──────┬──────┘            │
│         │                 │                 │                    │
│         ▼                 ▼                 ▼                    │
│    192.168.1.0/24   192.168.2.0/24    172.16.0.0/16             │
│    (北京办公网)     (上海办公网)      (香港数据中心)              │
│                                                                  │
│  路由表（自动生成）:                                              │
│    192.168.1.0/24 → 本地出口                                     │
│    192.168.2.0/24 → via Server2 (10.254.0.2)                    │
│    172.16.0.0/16  → via Server3 (10.254.0.3)                    │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### 场景二：IP 白名单分流

```text
┌─────────────────────────────────────────────────────────────────┐
│                                                                  │
│  某些 API 服务只允许特定 IP 访问：                                │
│    - api.service-a.com 只允许 Server1 的出口 IP                  │
│    - api.service-b.com 只允许 Server2 的出口 IP                  │
│                                                                  │
│  配置：                                                          │
│    Server1 声明 exit-route: api.service-a.com 的 IP 段          │
│    Server2 声明 exit-route: api.service-b.com 的 IP 段          │
│                                                                  │
│  结果：                                                          │
│    Client 连接 Server1，访问不同服务时自动从正确的 Server 出口    │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

---

## 架构设计

### 整体架构

```text
┌──────────────────────────────────────────────────────────────────────┐
│  Server Node                                                          │
│                                                                       │
│  ┌─────────────────────────────────────────────────────────────────┐ │
│  │  Tunnel Server (WSS)                                             │ │
│  │  - 接收 Client 的 WireGuard over WebSocket 连接                  │ │
│  │  - 接收其他 Mesh 节点的 WireGuard over WebSocket 连接            │ │
│  │  - 端口由 tunnel.listen_addr 配置（默认 443）                    │ │
│  └─────────────────────────────────────────────────────────────────┘ │
│                                                                       │
│  ┌─────────────────────────────────────────────────────────────────┐ │
│  │  Tunnel Client (Mesh)                                            │ │
│  │  - 主动连接其他 Mesh 节点的 WSS 端口                             │ │
│  │  - 复用 SDK 的 tunnel client 实现                                │ │
│  └─────────────────────────────────────────────────────────────────┘ │
│                                                                       │
│  ┌─────────────────────────────────────────────────────────────────┐ │
│  │  WireGuard (wg0)                                                 │ │
│  │  - Client Peers: 来自客户端的连接                                │ │
│  │  - Mesh Peers: 来自其他 Server 的连接（通过本地 tunnel 端口）    │ │
│  └─────────────────────────────────────────────────────────────────┘ │
│                                                                       │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐            │
│  │ Mesh Manager  │  │ Route Sync    │  │ Mesh API      │            │
│  │ - Peer 管理   │  │ - 定时同步    │  │ - Admin API   │            │
│  │ - 路由计算    │  │ - 事件推送    │  │ - 节点间 API  │            │
│  └───────────────┘  └───────────────┘  └───────────────┘            │
│                                                                       │
└──────────────────────────────────────────────────────────────────────┘
```

### 网络层设计

| 接口 | 用途          | 传输方式                          | 地址空间               |
|------|---------------|-----------------------------------|------------------------|
| wg0  | Client 连接   | WireGuard over WSS (tunnel 端口)  | 10.0.0.0/24 (可配置)   |
| wg0  | Mesh 节点互联 | WireGuard over WSS (tunnel 端口)  | 10.254.0.0/24 (固定)   |

**关键设计**：Mesh 节点间连接复用现有的 WireGuard over WebSocket 机制，与 Client 连接方式一致：

```text
Client ──WSS──► Server         # 现有（Client 连接）
ServerA ──WSS──► ServerB       # Mesh（Server 间连接，复用相同机制）
```

这样的好处：

- 无需开放额外端口，复用现有 tunnel 端口
- 穿透防火墙，与项目理念一致
- 复用现有 tunnel 代码，减少开发量
- TLS 端口可配置（通过 `tunnel.listen_addr`）

### 组件说明

| 组件 | 职责 |
|------|------|
| **MeshManager** | Mesh 生命周期管理，节点状态维护 |
| **MeshTunnelClient** | 管理到其他节点的 WSS 连接（复用 SDK tunnel） |
| **RouteSync** | 与其他节点同步路由信息 |
| **MeshAPI** | 提供管理 API 和节点间通信 API |

---

## 认证架构

### 设计原则

Mesh 网络采用**入口节点统一认证**模式：

- **入口节点（Gateway）**：处理 Client 认证，管理用户库
- **出口节点（Exit）**：不接受 Client 直连，只处理 Mesh 流量转发
- **节点间认证**：使用共享的 Mesh Token

```text
┌─────────────────────────────────────────────────────────────────────┐
│                                                                      │
│  Client A/B/C ──WSS──► ServerA (Gateway)                            │
│                         │                                            │
│                         │ ✅ Auth: 用户登录、JWT 签发               │
│                         │ ✅ 用户库: users, sessions                │
│                         │ ✅ Client Tunnel: 接收 Client 连接        │
│                         │                                            │
│                         ▼                                            │
│              ┌──────────────────────┐                               │
│              │   Mesh Network       │                               │
│              │                      │                               │
│              │  ServerB (Exit)      │  ← ❌ 不接受 Client 登录      │
│              │  ServerC (Exit)      │  ← ✅ 只处理 Mesh 流量        │
│              │                      │                               │
│              └──────────────────────┘                               │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

### 节点角色

| 角色      | 说明     | Client Auth | Client Tunnel | Mesh Tunnel |
|-----------|----------|-------------|---------------|-------------|
| `gateway` | 入口节点 | ✅ 启用     | ✅ 启用       | ✅ 启用     |
| `exit`    | 出口节点 | ❌ 禁用     | ❌ 禁用       | ✅ 启用     |
| `both`    | 双重角色 | ✅ 启用     | ✅ 启用       | ✅ 启用     |

### 认证流程

```text
1. Client 登录（只与入口节点交互）
   Client ──POST /api/auth/login──► ServerA (Gateway)
                                         │
                                         ▼
                                    验证用户，签发 JWT
                                         │
   Client ◄──────────────────────── { token: "xxx" }

2. Client 获取配置
   Client ──GET /api/config──► ServerA
           Authorization: Bearer xxx
                                    │
                                    ▼
                               返回 WG 配置
                               （自动包含所有 Mesh 路由）

3. Client 连接 VPN
   Client ══WSS══► ServerA ══WSS══► ServerB/C
                   (Gateway)         (Exit)

   ServerB/C 不参与 Client Auth，只转发流量
```

### 节点间认证

Mesh 节点之间使用共享的 **Mesh Token** 认证：

```text
ServerA 添加 ServerB 为 Peer:

1. API 握手
   ServerA ──GET /api/mesh/handshake──► ServerB
            Header: X-Mesh-Token: <shared-token>
                     │
                     ▼
            ServerB 验证 Token
                     │
   ServerA ◄──────── { name, public_key, mesh_ip, exit_routes }

2. 建立 WSS 隧道
   ServerA ══WSS══► ServerB:/mesh
            （WireGuard over WebSocket，公钥认证）
```

---

## 数据模型

### MeshNode 表

Mesh 网络中的节点信息。

```sql
CREATE TABLE mesh_nodes (
    id            INTEGER PRIMARY KEY,
    name          TEXT UNIQUE NOT NULL,      -- 节点名称
    public_key    TEXT UNIQUE NOT NULL,      -- WireGuard 公钥
    private_key   TEXT NOT NULL,             -- WireGuard 私钥（仅本地节点）
    mesh_ip       TEXT UNIQUE NOT NULL,      -- Mesh 内部 IP (10.254.0.x)
    tunnel_url    TEXT,                       -- WSS 隧道地址 (wss://host:port/path)
    api_endpoint  TEXT,                       -- API 地址 (https://host:port)
    is_local      BOOLEAN DEFAULT FALSE,     -- 是否为本地节点
    is_online     BOOLEAN DEFAULT FALSE,     -- 在线状态
    last_seen     DATETIME,                   -- 最后在线时间
    created_at    DATETIME,
    updated_at    DATETIME
);
```

### ExitRoute 表

节点声明的出口路由（该节点可以访问的网络）。

```sql
CREATE TABLE exit_routes (
    id         INTEGER PRIMARY KEY,
    node_id    INTEGER NOT NULL,              -- 所属节点
    cidr       TEXT NOT NULL,                 -- 可访问的网段
    comment    TEXT,                          -- 备注
    enabled    BOOLEAN DEFAULT TRUE,
    priority   INTEGER DEFAULT 100,           -- 优先级（数值越小优先级越高）
    created_at DATETIME,
    FOREIGN KEY (node_id) REFERENCES mesh_nodes(id)
);
```

### Go 结构体定义

```go
// server/internal/database/mesh.go

// MeshNode 代表 Mesh 网络中的一个节点
type MeshNode struct {
    ID          uint       `gorm:"primaryKey" json:"id"`
    Name        string     `gorm:"unique;not null" json:"name"`
    PublicKey   string     `gorm:"unique;not null" json:"public_key"`
    PrivateKey  string     `gorm:"not null" json:"-"`
    MeshIP      string     `gorm:"unique;not null" json:"mesh_ip"`
    TunnelURL   string     `json:"tunnel_url,omitempty"`      // wss://host:port/path
    APIEndpoint string     `json:"api_endpoint,omitempty"`    // https://host:port
    IsLocal     bool       `gorm:"default:false" json:"is_local"`
    IsOnline    bool       `gorm:"default:false" json:"is_online"`
    LastSeen    *time.Time `json:"last_seen,omitempty"`
    CreatedAt   time.Time  `json:"created_at"`
    UpdatedAt   time.Time  `json:"updated_at"`
}

// ExitRoute 声明某个节点可以访问的网络
type ExitRoute struct {
    ID        uint      `gorm:"primaryKey" json:"id"`
    NodeID    uint      `gorm:"not null;index" json:"node_id"`
    CIDR      string    `gorm:"not null" json:"cidr"`
    Comment   string    `json:"comment,omitempty"`
    Enabled   bool      `gorm:"default:true" json:"enabled"`
    Priority  int       `gorm:"default:100" json:"priority"`
    CreatedAt time.Time `json:"created_at"`

    Node MeshNode `gorm:"foreignKey:NodeID" json:"-"`
}
```

---

## 配置说明

### config.yaml 扩展

```yaml
# 现有配置保持不变...

# Tunnel 配置（Mesh 复用此端口）
tunnel:
  enabled: true
  listen_addr: "0.0.0.0:443"       # TLS 端口可配置
  public_host: "vpn.example.com"
  path: "/"
  # Mesh 连接使用的路径（可选，默认 /mesh）
  mesh_path: "/mesh"

# 新增 Mesh 配置段
mesh:
  # 是否启用 Mesh 功能（默认 false，不影响现有功能）
  enabled: false

  # 本节点名称（Mesh 内唯一标识）
  name: "beijing"

  # 节点角色：gateway（入口）、exit（出口）、both（双重）
  # - gateway: 接受 Client 连接，处理用户认证
  # - exit: 只接受 Mesh 连接，不接受 Client 直连
  # - both: 同时支持 Client 和 Mesh 连接
  role: "gateway"

  # Mesh 内部 IP（建议使用 10.254.0.0/24 网段）
  mesh_ip: "10.254.0.1"

  # 节点间认证 Token（所有 Mesh 节点必须使用相同的 Token）
  token: "your-mesh-secret-token"

  # 同步间隔（秒）
  sync_interval: 30

  # 初始对等节点列表（可选，也可通过 API 动态添加）
  peers: []
    # - name: "shanghai"
    #   tunnel_url: "wss://sh.vpn.com:443/mesh"
    #   api_endpoint: "https://sh.vpn.com:8080"
```

**说明**：

- Mesh 连接复用现有的 tunnel 端口，无需额外开放端口
- TLS 端口通过 `tunnel.listen_addr` 配置，不是固定 443
- Mesh 使用单独的 WebSocket 路径（默认 `/mesh`）以区分 Client 连接

### 配置项说明

| 配置项               | 必填 | 默认值      | 说明                              |
|----------------------|------|-------------|-----------------------------------|
| `tunnel.listen_addr` | 否   | 0.0.0.0:443 | Tunnel/Mesh 监听地址              |
| `tunnel.mesh_path`   | 否   | /mesh       | Mesh 连接的 WebSocket 路径        |
| `mesh.enabled`       | 否   | false       | 是否启用 Mesh                     |
| `mesh.name`          | 是*  | -           | 节点名称，Mesh 内唯一             |
| `mesh.role`          | 否   | gateway     | 节点角色: gateway / exit / both   |
| `mesh.mesh_ip`       | 是*  | -           | Mesh 内部 IP (10.254.0.x)         |
| `mesh.token`         | 是*  | -           | 节点间认证 Token                  |
| `mesh.sync_interval` | 否   | 30          | 同步间隔（秒）                    |
| `mesh.peers`         | 否   | []          | 初始对等节点                      |

*启用 Mesh 时必填

---

## API 参考

### 管理 API（需要 Admin 权限）

#### 初始化 Mesh 节点

```http
POST /api/mesh/init
Content-Type: application/json

{
  "name": "beijing",
  "mesh_ip": "10.254.0.1"
}
```

**响应**：

```json
{
  "node": {
    "id": 1,
    "name": "beijing",
    "public_key": "xxx...",
    "mesh_ip": "10.254.0.1",
    "is_local": true
  }
}
```

#### 添加对等节点

```http
POST /api/mesh/peers
Content-Type: application/json

{
  "tunnel_url": "wss://sh.vpn.com:443/mesh",
  "api_endpoint": "https://sh.vpn.com:8080"
}
```

**响应**：

```json
{
  "peer": {
    "id": 2,
    "name": "shanghai",
    "public_key": "yyy...",
    "mesh_ip": "10.254.0.2",
    "is_online": true,
    "exit_routes": ["192.168.2.0/24"]
  }
}
```

#### 获取对等节点列表

```http
GET /api/mesh/peers
```

**响应**：

```json
{
  "peers": [
    {
      "id": 2,
      "name": "shanghai",
      "mesh_ip": "10.254.0.2",
      "tunnel_url": "wss://sh.vpn.com:443/mesh",
      "is_online": true,
      "last_seen": "2024-01-01T12:00:00Z",
      "rtt_ms": 25,
      "exit_routes": ["192.168.2.0/24"]
    }
  ]
}
```

#### 删除对等节点

```http
DELETE /api/mesh/peers/:id
```

#### 添加出口路由

```http
POST /api/mesh/exit-routes
Content-Type: application/json

{
  "cidr": "192.168.1.0/24",
  "comment": "北京办公网络"
}
```

#### 获取出口路由列表

```http
GET /api/mesh/exit-routes
```

#### 删除出口路由

```http
DELETE /api/mesh/exit-routes/:id
```

#### 获取 Mesh 状态

```http
GET /api/mesh/status
```

**响应**：

```json
{
  "enabled": true,
  "local_node": {
    "name": "beijing",
    "mesh_ip": "10.254.0.1",
    "tunnel_url": "wss://bj.vpn.com:443/mesh",
    "client_count": 10
  },
  "peers": [
    {
      "name": "shanghai",
      "mesh_ip": "10.254.0.2",
      "is_online": true,
      "rtt_ms": 25,
      "client_count": 15,
      "exit_routes": ["192.168.2.0/24"]
    }
  ],
  "routing_table": [
    {
      "cidr": "192.168.2.0/24",
      "via_node": "shanghai",
      "via_ip": "10.254.0.2"
    }
  ],
  "total_clients": 25
}
```

#### 手动触发同步

```http
POST /api/mesh/sync
```

### 节点间 API（使用 Mesh Token 认证）

这些 API 由 Mesh 节点间调用，使用 `X-Mesh-Token` Header 认证。

#### 握手

```http
GET /api/mesh/handshake
X-Mesh-Token: your-mesh-secret-token
```

**响应**：

```json
{
  "name": "shanghai",
  "public_key": "yyy...",
  "mesh_ip": "10.254.0.2",
  "tunnel_url": "wss://sh.vpn.com:443/mesh",
  "exit_routes": ["192.168.2.0/24"]
}
```

#### 注册

```http
POST /api/mesh/register
X-Mesh-Token: your-mesh-secret-token
Content-Type: application/json

{
  "name": "beijing",
  "public_key": "xxx...",
  "mesh_ip": "10.254.0.1",
  "tunnel_url": "wss://bj.vpn.com:443/mesh",
  "exit_routes": []
}
```

#### 同步

```http
GET /api/mesh/sync-data
X-Mesh-Token: your-mesh-secret-token
```

**响应**：

```json
{
  "node": {
    "name": "shanghai",
    "mesh_ip": "10.254.0.2"
  },
  "exit_routes": [
    {"cidr": "192.168.2.0/24", "priority": 100}
  ],
  "client_count": 15,
  "timestamp": "2024-01-01T12:00:00Z"
}
```

---

## 部署指南

### 快速开始

**场景**：2 台服务器，ServerA 作为入口，ServerB 能访问内网 192.168.2.0/24。

#### Step 1: 配置 ServerA（入口节点）

```yaml
# /etc/wiresocket/config.yaml

tunnel:
  enabled: true
  listen_addr: "0.0.0.0:443"         # TLS 端口，可自定义
  public_host: "serverA.example.com"
  path: "/"

mesh:
  enabled: true
  name: "serverA"
  role: "gateway"                     # 入口节点，接受 Client 登录
  mesh_ip: "10.254.0.1"
  token: "your-secret-token-change-me"
```

```bash
# 重启服务
sudo systemctl restart wire-socket-server
```

#### Step 2: 配置 ServerB（出口节点）

```yaml
# /etc/wiresocket/config.yaml

tunnel:
  enabled: true
  listen_addr: "0.0.0.0:443"
  public_host: "serverB.example.com"
  path: "/"

mesh:
  enabled: true
  name: "serverB"
  role: "exit"                        # 出口节点，不接受 Client 直连
  mesh_ip: "10.254.0.2"
  token: "your-secret-token-change-me"  # 必须与 ServerA 相同
```

```bash
# 重启服务
sudo systemctl restart wire-socket-server
```

#### Step 3: 建立连接（在 ServerA 上）

```bash
# 添加 ServerB 为对等节点
wsctl mesh peer add \
  --tunnel-url=wss://serverB.example.com:443/mesh \
  --api=https://serverB.example.com:8080
```

#### Step 4: 声明出口路由（在 ServerB 上）

```bash
# 声明 ServerB 能访问的网络
wsctl mesh exit-route add 192.168.2.0/24 --comment="内部网络"
```

#### Step 5: 验证

```bash
# 在 ServerA 上查看状态
wsctl mesh status

# 输出示例：
# Mesh Status: Active
# Local Node: serverA (10.254.0.1)
#   Tunnel URL: wss://serverA.example.com:443/mesh
#
# Peers:
#   serverB (10.254.0.2) - Online - RTT: 15ms
#     Tunnel URL: wss://serverB.example.com:443/mesh
#     Exit Routes: 192.168.2.0/24
#
# Routing Table:
#   192.168.2.0/24  via serverB (10.254.0.2)
```

### 三节点部署示例

```yaml
# ===== ServerA (北京，入口) config.yaml =====
tunnel:
  listen_addr: "0.0.0.0:8443"    # 可以用非 443 端口
  public_host: "bj.vpn.com"

mesh:
  enabled: true
  name: "beijing"
  role: "gateway"                # 入口节点，接受 Client 登录
  mesh_ip: "10.254.0.1"
  token: "shared-mesh-token"

# ===== ServerB (上海，出口) config.yaml =====
tunnel:
  listen_addr: "0.0.0.0:443"
  public_host: "sh.vpn.com"

mesh:
  enabled: true
  name: "shanghai"
  role: "exit"                   # 出口节点，不接受 Client 直连
  mesh_ip: "10.254.0.2"
  token: "shared-mesh-token"

# ===== ServerC (香港，出口) config.yaml =====
tunnel:
  listen_addr: "0.0.0.0:443"
  public_host: "hk.vpn.com"

mesh:
  enabled: true
  name: "hongkong"
  role: "exit"                   # 出口节点，不接受 Client 直连
  mesh_ip: "10.254.0.3"
  token: "shared-mesh-token"
```

```bash
# ===== 建立连接（在 ServerA 上执行）=====
wsctl mesh peer add --tunnel-url=wss://sh.vpn.com:443/mesh --api=https://sh.vpn.com:8080
wsctl mesh peer add --tunnel-url=wss://hk.vpn.com:443/mesh --api=https://hk.vpn.com:8080

# ===== 声明出口路由 =====
# 在 ServerA:
wsctl mesh exit-route add 192.168.1.0/24 --comment="北京办公网"

# 在 ServerB:
wsctl mesh exit-route add 192.168.2.0/24 --comment="上海办公网"

# 在 ServerC:
wsctl mesh exit-route add 172.16.0.0/16 --comment="香港数据中心"
```

### 使用非标准端口

如果 443 端口被占用，可以使用其他端口：

```yaml
tunnel:
  listen_addr: "0.0.0.0:8443"    # 使用 8443 端口
  public_host: "vpn.example.com"
```

添加对等节点时指定端口：

```bash
wsctl mesh peer add \
  --tunnel-url=wss://peer.example.com:8443/mesh \
  --api=https://peer.example.com:8080
```

---

## 数据流说明

### Client 访问远程网络

```text
场景：Client 连接 ServerA (北京)，访问 192.168.2.100 (只有 ServerB 上海能访问)

┌───────────────────────────────────────────────────────────────────────┐
│                                                                        │
│  Step 1: Client 发起请求                                               │
│  ┌─────────────┐                                                      │
│  │ Client      │ dst: 192.168.2.100                                   │
│  │ 10.0.0.5    │─────────────────────────────────────┐                │
│  └─────────────┘                                      │                │
│                                                       ▼                │
│  Step 2: 流量通过 WSS 到达 ServerA                                     │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │ ServerA (wg0: 10.0.0.1, mesh: 10.254.0.1)                        │  │
│  │                                                                   │  │
│  │  路由表查询: 192.168.2.0/24 via 10.254.0.2                       │  │
│  │                                                                   │  │
│  │  转发到 Mesh Peer (通过本地 tunnel client)                        │  │
│  │  127.0.0.1:xxxxx ──► WSS ──► ServerB:443/mesh                    │  │
│  └─────────────────────────────────────────────────────────────────┘  │
│                                      │                                 │
│                                      │ WireGuard over WebSocket        │
│                                      ▼                                 │
│  Step 3: 流量到达 ServerB                                              │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │ ServerB (wg0, mesh: 10.254.0.2)                                  │  │
│  │                                                                   │  │
│  │  路由表查询: 192.168.2.0/24 → 本地出口                           │  │
│  │                                                                   │  │
│  │  eth0 ───────────────────────────────────────────────────────►  │  │
│  └─────────────────────────────────────────────────────────────────┘  │
│                                      │                                 │
│                                      ▼                                 │
│  Step 4: 到达目标                                                      │
│  ┌─────────────┐                                                      │
│  │ 192.168.2.  │                                                      │
│  │ 100         │                                                      │
│  └─────────────┘                                                      │
│                                                                        │
│  返回路径：原路返回                                                     │
│  192.168.2.100 → ServerB → (WSS) → ServerA → (WSS) → Client          │
│                                                                        │
└───────────────────────────────────────────────────────────────────────┘
```

### Mesh 连接工作原理

```text
┌─────────────────────────────────────────────────────────────────────────┐
│  ServerA                                                                 │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │  Tunnel Server (:443)                                              │  │
│  │  ├── /        → 接收 Client 的 WireGuard over WebSocket           │  │
│  │  └── /mesh    → 接收其他 Server 的 WireGuard over WebSocket       │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                          │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │  Mesh Tunnel Clients                                               │  │
│  │  ├── → wss://serverB:443/mesh (本地端口 127.0.0.1:50001)          │  │
│  │  └── → wss://serverC:443/mesh (本地端口 127.0.0.1:50002)          │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                          │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │  WireGuard (wg0)                                                   │  │
│  │  Peers:                                                            │  │
│  │  ├── Client peers (无固定 Endpoint)                               │  │
│  │  ├── ServerB: Endpoint=127.0.0.1:50001, AllowedIPs=10.254.0.2/32  │  │
│  │  └── ServerC: Endpoint=127.0.0.1:50002, AllowedIPs=10.254.0.3/32  │  │
│  └───────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────┘
```

### 为什么不需要 NAT

传统方案需要 NAT 的原因：

- Client IP (10.0.0.5) 对 ServerB 来说是未知来源
- 返回包不知道怎么路由回 Client

Mesh WireGuard 方案：

- ServerA 和 ServerB 通过 WireGuard 隧道连接
- ServerB 的 AllowedIPs 包含 ServerA 的 Client 子网 (10.0.0.0/24)
- 返回包自动通过 WG 隧道回到 ServerA
- **全程在 WireGuard 隧道内路由，无需 NAT！**

---

## 实现计划

### Phase 1: 基础 Mesh 连接

| 任务 | 文件 | 说明 |
|------|------|------|
| 数据库模型 | `internal/database/mesh.go` | MeshNode, ExitRoute 表 |
| Manager 扩展 | `internal/wireguard/manager.go` | AddMeshPeer 方法 |
| Mesh 核心 | `internal/mesh/mesh.go` | MeshManager 结构 |
| Tunnel Client | `internal/mesh/tunnel.go` | 复用 SDK tunnel 连接其他节点 |
| Peer 管理 | `internal/mesh/peer.go` | 连接建立与维护 |
| 配置加载 | `cmd/server/main.go` | Mesh 初始化 |

### Phase 2: API 与同步

| 任务 | 文件 | 说明 |
|------|------|------|
| Admin API | `internal/api/mesh.go` | 管理接口 |
| 节点间 API | `internal/mesh/api.go` | 握手、注册、同步 |
| 路由同步 | `internal/mesh/sync.go` | 定时同步逻辑 |
| Token 认证 | `internal/mesh/auth.go` | X-Mesh-Token 中间件 |

### Phase 3: 工具与 UI

| 任务 | 文件 | 说明 |
|------|------|------|
| wsctl 命令 | `cmd/wsctl/mesh.go` | CLI 管理工具 |
| Admin UI | `internal/admin/static/` | Web 管理界面 |

### Phase 4: Client 集成

| 任务 | 文件 | 说明 |
|------|------|------|
| 配置生成 | `internal/wireguard/config_generator.go` | 合并 Mesh 路由到 Client 配置 |
| API 响应 | `internal/api/router.go` | 扩展 /api/config |

---

## 兼容性说明

### 向后兼容

- `mesh.enabled: false` (默认) 时，系统行为与现有完全一致
- 现有 API 接口不变
- 现有配置文件格式兼容
- 数据库自动迁移，不影响现有表
- Client 无需任何改动

### 版本要求

- WireSocket Server >= 1.0.0 (待实现)
- 所有 Mesh 节点需要运行相同或兼容版本
- Client/SDK 无版本要求（Mesh 对 Client 透明）
