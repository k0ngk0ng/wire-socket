# WireSocket 客户端架构说明

## 整体架构

```text
┌─────────────────────────────────────────────────────────────┐
│                     WireSocket Client                       │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐    │
│  │              Tauri 2 Frontend                       │    │
│  │  - 静态 HTML/CSS/JavaScript UI                      │    │
│  │  - 系统托盘、deep link、窗口生命周期                 │    │
│  │  - 自动安装和启动后端服务                            │    │
│  └──────────────────────┬──────────────────────────────┘    │
│                         │ Tauri command / local HTTP         │
│                         ↓                                    │
│  ┌─────────────────────────────────────────────────────┐    │
│  │              Client Backend Service (Go)            │    │
│  │  - 本地 API: 127.0.0.1:41945                         │    │
│  │  - 连接状态和配置管理                                │    │
│  │  - WireGuard 接口管理                                │    │
│  │  - 内置 WebSocket tunnel                             │    │
│  └──────────┬─────────────────────┬────────────────────┘    │
│             │                     │                         │
│  ┌──────────▼─────────┐  ┌────────▼────────────┐            │
│  │  WireGuard         │  │ Built-in WS Tunnel  │            │
│  │  Interface         │  │ UDP over WS/WSS     │            │
│  └──────────┬─────────┘  └────────┬────────────┘            │
└─────────────┼──────────────────────┼────────────────────────┘
              │                      │
              │ encrypted UDP        │ WS/WSS
              ↓                      ↓
        ┌─────────────────────────────────────┐
        │             VPN Server              │
        └─────────────────────────────────────┘
```

## 打包架构

```text
client/frontend/
├── public/                  # 静态 UI
├── src-tauri/               # Tauri 2 Rust 入口和配置
│   ├── src/lib.rs           # commands、tray、deep link、服务启动
│   ├── tauri.conf.json      # bundle/resources/window/security
│   ├── capabilities/        # Tauri 权限
│   └── icons/               # 应用图标
├── resources/bin/           # npm run prepare 生成
│   ├── darwin/
│   │   ├── wire-socket-client
│   │   ├── wire-socket-client-arm64
│   │   ├── wireguard-go
│   │   └── wireguard-go-arm64
│   ├── linux/
│   │   └── wire-socket-client
│   └── win32/
│       ├── wire-socket-client.exe
│       ├── wireguard.exe
│       └── wintun.dll
└── scripts/
```

WebSocket tunnel 已内置到 Go 后端，不再需要外部 `wstunnel` 二进制。

## 运行时流程

### 应用启动

```text
用户启动 WireSocket
    ↓
Tauri 初始化窗口、托盘、deep link
    ↓
检测后端服务 /health
    ↓
未运行时通过系统提权安装或启动 wire-socket-client
    ↓
UI 通过 Tauri command 调用本地后端 API
```

### 连接 VPN

```text
用户点击连接
    ↓
Tauri command: connect(credentials)
    ↓
Go backend: POST /api/connect
    ↓
创建 WireGuard 接口并获取服务端配置
    ↓
启动内置 WebSocket tunnel
    ↓
状态通过 /api/status 轮询回到 UI
```

## 进程模型

- Tauri 应用以普通用户权限运行。
- Go 后端服务以 root/管理员权限运行，用于创建 TUN/WireGuard 接口和配置路由。
- Tauri 负责在首次启动或版本变化时触发服务安装/重启。
- 前端不直接操作系统网络能力，所有高权限操作都在 Go 后端中完成。

## 通信方式

Tauri commands：

- `check_service`
- `connect`
- `disconnect`
- `get_status`
- `get_route_settings`
- `update_route_settings`
- `change_password`
- `sso_get_providers`
- `sso_login`
- `sso_connect_with_token`

本地后端 API：

- `GET /health`
- `POST /api/connect`
- `POST /api/disconnect`
- `GET /api/status`
- `GET /api/routes/settings`
- `PUT /api/routes/settings`
- `POST /api/change-password`

## 空闲连接保活

WebSocket tunnel 两端都会使用 ping/pong 保活。客户端和服务端都必须保证同一条 WebSocket 连接只有一个并发 writer；数据帧、ping 和 pong 都要串行写入，避免 gorilla/websocket 在无业务数据时因为并发控制帧写入导致连接异常关闭。

## 体积策略

迁移到 Tauri 2 后，不再随应用打包 Chromium。桌面壳使用系统 WebView，主要体积来自：

- Go 后端服务二进制
- WireGuard 平台组件
- Tauri Rust 壳

`src-tauri/Cargo.toml` 的 release profile 已使用偏体积优化的设置。
