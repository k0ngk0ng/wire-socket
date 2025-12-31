# WireSocket 客户端架构说明

## 整体架构

```
┌─────────────────────────────────────────────────────────────┐
│                     WireSocket Client                         │
│                                                               │
│  ┌─────────────────────────────────────────────────────┐    │
│  │           Electron Frontend (UI)                    │    │
│  │  - React/HTML界面                                   │    │
│  │  - 用户交互                                         │    │
│  │  - 状态显示                                         │    │
│  └──────────────────────┬──────────────────────────────┘    │
│                         │ HTTP (localhost:41945)             │
│                         ↓                                     │
│  ┌─────────────────────────────────────────────────────┐    │
│  │      Client Backend Service (Go)                    │    │
│  │  - 连接管理                                         │    │
│  │  - 配置管理                                         │    │
│  │  - 本地API服务器                                    │    │
│  └──────────┬─────────────────────┬────────────────────┘    │
│             │                     │                           │
│             │                     │                           │
│  ┌──────────▼─────────┐  ┌───────▼────────────┐             │
│  │  WireGuard         │  │   wstunnel         │             │
│  │  Interface         │  │   Client           │             │
│  │  - wg0/utun        │  │   Process          │             │
│  │  - 加密/解密       │  │   - WebSocket      │             │
│  └──────────┬─────────┘  └────────┬───────────┘             │
│             │                      │                          │
└─────────────┼──────────────────────┼──────────────────────────┘
              │                      │
              │ (encrypted)          │ (WSS/WS)
              ↓                      ↓
        ┌─────────────────────────────────────┐
        │           Network                    │
        │  ┌──────────────┐  ┌──────────────┐ │
        │  │ Direct UDP   │  │  WebSocket   │ │
        │  │ (fallback)   │  │  (primary)   │ │
        │  └──────────────┘  └──────────────┘ │
        └─────────────────────────────────────┘
                      ↓
        ┌─────────────────────────────────────┐
        │         VPN Server                   │
        └─────────────────────────────────────┘
```

## 打包架构

### 资源文件结构

```
安装包
├── Electron App
│   ├── index.html (UI)
│   ├── main.js (主进程)
│   ├── preload.js (预加载)
│   └── renderer.js (渲染进程)
│
└── Resources/
    └── bin/
        ├── wire-socket-client     (Go 后端服务)
        ├── wstunnel              (WebSocket 隧道)
        └── wireguard-go*         (WireGuard 用户空间实现)
                                  (* macOS/Windows)
```

### 平台差异

#### macOS (.app bundle)
```
WireSocket.app/
├── Contents/
│   ├── MacOS/
│   │   └── WireSocket (Electron)
│   ├── Resources/
│   │   └── bin/
│   │       ├── wire-socket-client
│   │       ├── wstunnel
│   │       └── wireguard-go
│   └── Info.plist
```

#### Windows (安装目录)
```
C:\Program Files\WireSocket\
├── WireSocket.exe (Electron)
├── resources/
│   └── bin/
│       ├── wire-socket-client.exe
│       ├── wstunnel.exe
│       ├── wireguard.exe
│       └── wintun.dll
```

#### Linux (AppImage/安装目录)
```
/opt/WireSocket/
├── wiresocket (Electron)
└── resources/
    └── bin/
        ├── wire-socket-client
        └── wstunnel
```

## 运行时流程

### 1. 应用启动

```
用户启动应用
    ↓
Electron 主进程启动
    ↓
检查客户端后端服务状态
    ↓
如果未运行：启动 wire-socket-client
    ↓
启动 Electron 窗口
    ↓
加载 UI
```

### 2. 连接VPN

```
用户点击"连接"
    ↓
前端 → HTTP POST → 后端 (/api/connect)
    ↓
后端服务处理
    ├─→ 创建 WireGuard 接口
    │   ├─→ 生成密钥对
    │   ├─→ 配置接口 (IP, DNS)
    │   └─→ 设置路由
    │
    ├─→ 启动 wstunnel 客户端
    │   ├─→ 查找 wstunnel 二进制
    │   ├─→ 启动 WebSocket 隧道
    │   └─→ 连接到服务器
    │
    └─→ 返回连接状态
        ↓
前端更新 UI
    ├─→ 显示"已连接"
    ├─→ 显示分配的 IP
    └─→ 开始显示流量统计
```

### 3. 流量统计更新

```
定时器 (每秒)
    ↓
前端 → HTTP GET → 后端 (/api/status)
    ↓
后端查询 WireGuard 接口统计
    ↓
计算速率 (bytes/sec)
    ↓
返回 JSON {rx_bytes, tx_bytes, rx_speed, tx_speed}
    ↓
前端更新显示
```

### 4. 断开连接

```
用户点击"断开"
    ↓
前端 → HTTP POST → 后端 (/api/disconnect)
    ↓
后端服务处理
    ├─→ 停止 wstunnel 客户端
    └─→ 删除 WireGuard 接口
        ↓
返回断开状态
    ↓
前端更新 UI → 显示"未连接"
```

## 组件通信

### IPC 通信 (Electron)

```
Renderer Process ←→ Preload Script ←→ Main Process
      (UI)             (Bridge)         (后台)
        │                  │                │
        │  window.api.*    │   ipcRenderer  │
        │ ─────────────→   │ ─────────────→ │
        │                  │                │
        │    callback      │   ipcMain      │
        │ ←───────────────  │ ←───────────── │
```

### HTTP API (后端服务)

```
Electron Frontend ←─────→ Client Backend Service
                HTTP/JSON
            (localhost:41945)

API 端点：
- POST /api/connect
- POST /api/disconnect
- GET  /api/status
- GET  /api/servers
- POST /api/servers
```

### 进程管理

```
Electron Main Process
    │
    ├─→ Spawn: wire-socket-client (如果未运行)
    │      │
    │      ├─→ Spawn: wstunnel client (连接时)
    │      │
    │      └─→ Manage: WireGuard interface
    │
    └─→ Electron Renderer Process (UI)
```

## 二进制文件查找顺序

### wstunnel 查找

```go
findWSTunnelBinary() {
    1. 检查可执行文件同目录
       ./wstunnel (Linux/macOS)
       ./wstunnel.exe (Windows)

    2. macOS App Bundle
       ../Resources/bin/wstunnel

    3. 系统 PATH
       exec.LookPath("wstunnel")

    4. 常见位置
       /usr/local/bin/wstunnel
       /usr/bin/wstunnel
       C:\Program Files\wstunnel\wstunnel.exe
}
```

### WireGuard 工具

#### Linux
- 使用内核 WireGuard (需要 `ip` 命令)
- 依赖系统安装的 `wireguard-tools`

#### macOS
- 使用打包的 `wireguard-go` (用户空间实现)
- 位于 Resources/bin/wireguard-go

#### Windows
- 使用打包的 `wireguard.exe` + `wintun.dll`
- 位于 resources/bin/

## 构建流程架构

```
开发者运行: npm run build
    ↓
┌─────────────────────────────────┐
│   Step 1: Prepare Dependencies  │
│                                  │
│  ┌─────────────────────────┐    │
│  │  Download wstunnel      │    │
│  │  - macOS (x64/arm64)    │    │
│  │  - Linux (x64)          │    │
│  │  - Windows (x64)        │    │
│  └─────────────────────────┘    │
│                                  │
│  ┌─────────────────────────┐    │
│  │  Build wireguard-go     │    │
│  │  - Clone from Git       │    │
│  │  - Cross-compile        │    │
│  │  - Download wintun.dll  │    │
│  └─────────────────────────┘    │
│                                  │
│  ┌─────────────────────────┐    │
│  │  Build Client Backend   │    │
│  │  - Go cross-compile     │    │
│  │  - All platforms        │    │
│  └─────────────────────────┘    │
└─────────────────────────────────┘
    ↓
┌─────────────────────────────────┐
│   Step 2: Package with Electron │
│                                  │
│  electron-builder                │
│    ↓                             │
│  ┌─────────────────────────┐    │
│  │  Collect resources      │    │
│  │  - Electron files       │    │
│  │  - Binary files         │    │
│  │  - Assets               │    │
│  └─────────────────────────┘    │
│    ↓                             │
│  ┌─────────────────────────┐    │
│  │  Create installers      │    │
│  │  - macOS: DMG/ZIP       │    │
│  │  - Windows: NSIS/EXE    │    │
│  │  - Linux: AppImage/DEB  │    │
│  └─────────────────────────┘    │
└─────────────────────────────────┘
    ↓
    输出到 dist/ 目录
```

## 安全考虑

### 1. 权限要求

- **WireGuard 操作**: 需要 root/管理员权限
- **服务安装**: 需要 root/管理员权限
- **网络配置**: 需要系统权限

### 2. 进程隔离

```
Electron (普通权限)
    ↓ IPC/HTTP
Client Backend (特权进程)
    ↓ Spawn
wstunnel (继承权限)
    ↓ Network
WireGuard (内核/用户空间)
```

### 3. 数据存储

- **配置文件**: 用户目录 (~/.wire-socket/)
- **日志文件**: 系统日志
- **私钥**: 内存中临时存储，用完即删

## 性能优化

### 1. 二进制文件优化

- Go 编译: `-ldflags "-s -w"` (移除调试信息)
- wireguard-go: 优化构建
- Electron: asar 打包

### 2. 启动优化

- 延迟加载非关键组件
- 并行初始化
- 缓存配置

### 3. 网络优化

- wstunnel: 复用 WebSocket 连接
- WireGuard: UDP 优化
- 统计信息: 定时采样，减少查询频率

## 故障恢复

### 1. 连接断开

```
检测到连接断开
    ↓
重试机制 (3次)
    ├─→ 成功 → 恢复正常
    └─→ 失败 → 通知用户
```

### 2. 进程崩溃

```
检测到后端服务崩溃
    ↓
自动重启服务
    ├─→ 成功 → 尝试重连
    └─→ 失败 → 提示用户重启应用
```

### 3. 网络切换

```
检测到网络变化
    ↓
断开现有连接
    ↓
等待网络稳定
    ↓
自动重连
```

## 总结

这个架构实现了：

✅ **模块化设计** - 各组件职责清晰
✅ **跨平台兼容** - 统一接口，平台差异在底层处理
✅ **易于维护** - 清晰的通信机制和错误处理
✅ **用户友好** - 自动化程度高，无需手动配置
✅ **安全可靠** - 适当的权限隔离和错误恢复
