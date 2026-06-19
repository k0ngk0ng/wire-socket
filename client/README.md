# WireSocket Client

WireSocket 客户端应用，包含后端服务和前端界面。

## 📁 目录结构

```
client/
├── backend/          # Go 后端服务
│   ├── cmd/          # 入口程序
│   │   └── client/   # 客户端主程序
│   ├── internal/     # 内部包
│   │   ├── api/      # HTTP API 服务器
│   │   ├── connection/  # 连接管理
│   │   ├── wireguard/   # WireGuard 接口管理
│   │   └── wstunnel/    # 内置 WebSocket tunnel 管理
│   ├── go.mod
│   └── go.sum
│
└── frontend/         # Tauri 2 桌面应用
    ├── public/       # 静态 UI
    ├── src-tauri/    # Tauri Rust 入口和配置
    ├── resources/    # 打包资源
    │   └── bin/      # 各平台二进制文件
    ├── scripts/      # 构建脚本
    └── package.json
```

## 🚀 快速开始

### 后端服务

```bash
cd backend
go mod tidy
go build -o wire-socket-client cmd/client/main.go

# 运行服务
sudo ./wire-socket-client
```

### 前端应用

```bash
cd frontend
npm install

# 开发模式
npm start

# 构建安装包
npm run build
```

## 📦 功能组件

### Backend (Go)

**核心功能**：
- 系统服务管理（支持 Windows Service、macOS LaunchDaemon、Linux systemd）
- WireGuard 接口创建和管理
- 内置 WebSocket tunnel 管理
- 本地 HTTP API 服务器（监听 localhost:41945）
- 连接状态和流量统计

**API 端点**：
- `POST /api/connect` - 连接 VPN
- `POST /api/disconnect` - 断开 VPN
- `GET /api/status` - 获取连接状态和统计
- `GET /api/servers` - 列出已保存的服务器
- `POST /api/servers` - 添加服务器配置

**权限要求**：
- 需要 root/管理员权限运行
- 用于创建网络接口和配置路由

### Frontend (Tauri 2)

**核心功能**：
- 接近原生风格的桌面界面
- 服务器配置管理
- 实时连接状态显示
- 流量统计可视化
- 系统托盘集成
- Deep link SSO 回调

**技术栈**：
- Tauri 2
- Rust
- HTML/CSS/JavaScript
- 通过 Tauri command 转发到本地后端 HTTP API

## 🔧 开发指南

### 后端开发

**添加新功能**：
1. 在 `internal/` 下创建新包
2. 在 `cmd/client/main.go` 中集成
3. 更新 API 路由（如需要）

**测试**：
```bash
cd backend
go test ./...
```

**调试**：
```bash
# 直接运行，查看日志输出
sudo go run cmd/client/main.go
```

### 前端开发

**开发模式**：
```bash
cd frontend
npm start
```

**修改 UI**：
- 编辑 `public/index.html`
- 修改 CSS 样式
- JavaScript 逻辑内联在静态页面中
- 桌面能力在 `src-tauri/src/lib.rs`

**调试**：
- 使用系统 WebView 开发者工具
- 查看控制台日志

## 📦 打包和发布

### 构建当前平台

```bash
cd frontend
npm run build
```

这会自动：
1. 准备 WireGuard 组件和平台资源
2. 交叉编译后端服务
3. 打包 Tauri 应用
4. 生成当前平台的安装包

输出位置：`frontend/src-tauri/target/release/bundle/`

### 构建特定平台

```bash
npm run build:mac     # macOS
npm run build:win     # Windows
npm run build:linux   # Linux
```

详细说明请参考 [../docs/PACKAGING.md](../docs/PACKAGING.md)。

## 🔄 架构和通信

### 组件交互

```
┌─────────────────┐
│  Tauri UI       │
│  (WebView)      │
└────────┬────────┘
         │ HTTP (localhost:41945)
         ↓
┌─────────────────┐
│  Backend API    │
│  (Go Service)   │
└────────┬────────┘
         │
    ┌────┴────┐
    │         │
    ↓         ↓
┌────────┐  ┌──────────┐
│WireGuard│  │Built-in  │
│Interface│  │WS Tunnel │
└────────┘  └──────────┘
```

### 数据流

1. **用户操作** → Tauri UI
2. **HTTP 请求** → Backend API (localhost:41945)
3. **管理操作** → WireGuard + 内置 WebSocket tunnel
4. **网络流量** → VPN 服务器

## 🛠️ 故障排除

### 后端服务无法启动

**问题**: "Permission denied"
**解决**: 使用 sudo 运行
```bash
sudo ./wire-socket-client
```

**问题**: "Failed to create WireGuard interface"
**解决**: 安装 WireGuard 工具
```bash
# macOS
brew install wireguard-tools

# Linux
sudo apt install wireguard-tools
```

### 前端无法连接后端

**问题**: "Connection refused" 到 localhost:41945
**解决**: 确保后端服务正在运行
```bash
# 检查服务状态
curl http://localhost:41945/health
```

### 后端二进制未找到

**问题**: "backend binary not found"
**解决**: 在前端目录运行 `npm run prepare` 生成打包资源。

## 📚 相关文档

- **前端详细文档**: [frontend/README.md](frontend/README.md)
- **打包指南**: [../docs/PACKAGING.md](../docs/PACKAGING.md)
- **架构说明**: [../docs/ARCHITECTURE.md](../docs/ARCHITECTURE.md)
- **项目总览**: [../README.md](../README.md)
- **开发指南**: [../AGENTS.md](../AGENTS.md)

## 🤝 贡献

欢迎贡献代码！请：
1. Fork 项目
2. 创建功能分支
3. 提交更改
4. 推送到分支
5. 创建 Pull Request

## 📝 许可证

MIT License - 查看 LICENSE 文件了解详情
