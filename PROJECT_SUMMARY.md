# WireSocket 项目完整总结

**创建日期**: 2025-12-22
**项目名称**: WireSocket

## 项目概览

WireSocket 是一个跨平台 VPN 解决方案，使用：
- **服务器**: Go + WireGuard + wstunnel
- **客户端后端**: Go (系统服务)
- **客户端前端**: Electron
- **数据库**: SQLite (可切换到 PostgreSQL)

## 快速启动指南

### 1. 启动服务器

```bash
# 进入服务器目录
cd server

# 首次运行：初始化数据库
sudo ./wire-socket-server -init-db

# 输出会显示：
# - 默认管理员: admin / admin123 (请立即修改！)
# - 生成的 WireGuard 公钥
# - 将公钥保存到 config.yaml

# 正常启动服务器
sudo ./wire-socket-server

# 服务器会监听在: http://0.0.0.0:8080
```

### 2. 启动 wstunnel 服务器

```bash
# 需要单独运行 wstunnel (另一个终端)
sudo wstunnel server wss://0.0.0.0:443 --restrict-to 127.0.0.1:51820

# 如果没有 wstunnel，安装方法：
# Linux:
wget https://github.com/erebe/wstunnel/releases/latest/download/wstunnel_linux_amd64
chmod +x wstunnel_linux_amd64
sudo mv wstunnel_linux_amd64 /usr/local/bin/wstunnel

# macOS:
wget https://github.com/erebe/wstunnel/releases/latest/download/wstunnel_macos_amd64
chmod +x wstunnel_macos_amd64
sudo mv wstunnel_macos_amd64 /usr/local/bin/wstunnel
```

### 3. 启动客户端后端服务

```bash
# 进入客户端目录
cd client-backend

# 选项 A: 安装为系统服务 (推荐)
sudo ./wire-socket-client -service install
sudo systemctl start WireSocketClient  # Linux
# 或
sudo launchctl load /Library/LaunchDaemons/WireSocketClient.plist  # macOS

# 选项 B: 直接运行 (调试用)
sudo ./wire-socket-client

# 服务会监听在: http://127.0.0.1:41945
```

### 4. 启动 Electron 前端

```bash
# 进入 Electron 目录
cd electron

# 开发模式运行
npm start

# 或构建发布版
npm run build
```

## 重要配置文件

### 服务器配置: `server/config.yaml`

```yaml
server:
  address: "0.0.0.0:8080"

database:
  path: "./vpn.db"

wireguard:
  device_name: "wg0"
  listen_port: 51820
  subnet: "10.0.0.0/24"
  dns: "1.1.1.1,8.8.8.8"
  endpoint: "你的服务器IP:51820"  # 修改这里！
  private_key: ""  # 首次运行后填入生成的密钥
  public_key: ""   # 首次运行后填入生成的密钥

auth:
  jwt_secret: "change-this-to-a-random-secret"  # 生产环境必须修改！
```

### 默认登录凭证

- **用户名**: `admin`
- **密码**: `admin123`
- ⚠️ **首次登录后立即修改密码！**

## 项目结构

```
wire-socket/
├── server/                           # Go 服务器
│   ├── cmd/server/main.go           # 入口点
│   ├── internal/
│   │   ├── database/db.go           # 数据库模型 (自动创建表)
│   │   ├── auth/handler.go          # JWT 认证
│   │   ├── wireguard/
│   │   │   ├── manager.go           # WireGuard 控制
│   │   │   └── config_generator.go  # 动态配置生成
│   │   └── api/router.go            # HTTP API
│   ├── config.yaml                  # 服务器配置
│   ├── go.mod
│   └── vpn.db                       # SQLite 数据库 (自动创建)
│
├── client-backend/                   # Go 客户端服务
│   ├── cmd/client/main.go           # 服务入口
│   ├── internal/
│   │   ├── connection/manager.go    # 连接管理
│   │   ├── wireguard/interface.go   # WireGuard 接口
│   │   ├── wstunnel/client.go       # wstunnel 客户端
│   │   └── api/server.go            # 本地 HTTP API
│   └── go.mod
│
├── electron/                         # Electron 前端
│   ├── src/
│   │   ├── main/index.js            # 主进程
│   │   └── preload/index.js         # 预加载脚本
│   ├── public/index.html            # UI 界面
│   └── package.json
│
├── README.md                         # 完整文档
└── PROJECT_SUMMARY.md               # 本文件
```

## 数据库信息

### 使用 SQLite (默认)

- **位置**: `server/vpn.db`
- **自动创建**: 首次运行自动创建所有表
- **表结构**:
  - `users` - 用户账户
  - `servers` - VPN 服务器配置
  - `allocated_ips` - IP 地址分配
  - `sessions` - JWT 会话

### 查看数据库内容

```bash
# 安装 sqlite3
brew install sqlite3  # macOS
sudo apt install sqlite3  # Linux

# 查看数据库
cd server
sqlite3 vpn.db

# SQLite 命令
.tables                     # 列出所有表
SELECT * FROM users;        # 查看用户
SELECT * FROM allocated_ips; # 查看 IP 分配
.quit
```

### 切换到 PostgreSQL

修改 `server/config.yaml`:
```yaml
database:
  driver: "postgres"
  dsn: "host=localhost user=vpn password=vpn dbname=vpn port=5432 sslmode=disable"
```

## API 端点

### 服务器 API (端口 8080)

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| POST | `/api/auth/register` | 注册用户 | 否 |
| POST | `/api/auth/login` | 登录 | 否 |
| POST | `/api/auth/refresh` | 刷新令牌 | 是 |
| GET | `/api/config` | 获取 WireGuard 配置 | 是 |
| GET | `/api/servers` | 列出服务器 | 是 |
| GET | `/api/status` | 查看状态 | 是 |

### 客户端本地 API (端口 41945)

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/connect` | 连接 VPN |
| POST | `/api/disconnect` | 断开 VPN |
| GET | `/api/status` | 查看连接状态 |
| GET | `/api/servers` | 列出已保存服务器 |
| POST | `/api/servers` | 添加服务器配置 |

## 常见问题排查

### 问题 1: "Failed to configure WireGuard device"

**原因**: WireGuard 未安装或未加载

**解决方案**:
```bash
# Linux
sudo apt install wireguard-tools  # Debian/Ubuntu
sudo yum install wireguard-tools   # CentOS/RHEL
sudo modprobe wireguard            # 加载内核模块

# macOS
brew install wireguard-tools

# 检查是否安装成功
which wg
```

### 问题 2: "Permission denied"

**原因**: VPN 操作需要 root 权限

**解决方案**:
```bash
# 必须使用 sudo 运行
sudo ./vpn-server
sudo ./vpn-client
```

### 问题 3: "wstunnel binary not found"

**原因**: wstunnel 未安装

**解决方案**:
```bash
# 下载并安装 wstunnel
wget https://github.com/erebe/wstunnel/releases/latest/download/wstunnel_linux_amd64
chmod +x wstunnel_linux_amd64
sudo mv wstunnel_linux_amd64 /usr/local/bin/wstunnel

# 验证安装
which wstunnel
wstunnel --version
```

### 问题 4: "Connection failed" / "Authentication failed"

**检查清单**:
1. 服务器是否正在运行? `ps aux | grep vpn-server`
2. wstunnel 服务器是否运行? `ps aux | grep wstunnel`
3. 防火墙是否开放端口?
   ```bash
   sudo ufw allow 8080  # API 端口
   sudo ufw allow 443   # wstunnel
   sudo ufw allow 51820/udp  # WireGuard
   ```
4. 客户端后端服务是否运行? `curl http://127.0.0.1:41945/health`
5. 用户名密码是否正确? (默认: admin/admin123)

### 问题 5: 数据库相关错误

**解决方案**:
```bash
# 删除旧数据库重新初始化
cd server
sudo rm vpn.db
sudo ./wire-socket-server -init-db
```

## 系统要求

### 服务器端
- **操作系统**: Linux (推荐 Ubuntu 20.04+)
- **Go**: 1.21+
- **WireGuard**: 内核模块或 wireguard-go
- **权限**: root/sudo
- **端口**: 8080 (HTTP API), 443 (wstunnel), 51820 (WireGuard)

### 客户端
- **操作系统**: macOS 11+, Windows 10+, Linux
- **Go**: 1.21+ (仅构建时)
- **Node.js**: 18+ (Electron)
- **WireGuard**: 客户端工具
- **权限**: 管理员/root (VPN 操作)

## 重新构建项目

### 服务器
```bash
cd server
go mod tidy
go build -o wire-socket-server cmd/server/main.go
```

### 客户端后端
```bash
cd client-backend
go mod tidy
go build -o wire-socket-client cmd/client/main.go
```

### Electron 前端
```bash
cd electron
npm install
npm run build  # 构建发布版
```

## 生产环境部署清单

- [ ] 修改 `config.yaml` 中的 `jwt_secret`
- [ ] 修改默认管理员密码
- [ ] 配置 HTTPS/TLS 证书 (Let's Encrypt)
- [ ] 设置防火墙规则
- [ ] 配置 systemd 服务自动启动
- [ ] 备份数据库文件 `vpn.db`
- [ ] 设置日志轮转
- [ ] 监控服务器状态

## 关键命令速查

```bash
# 构建所有组件
cd server && go build -o wire-socket-server cmd/server/main.go && cd ..
cd client-backend && go build -o wire-socket-client cmd/client/main.go && cd ..
cd electron && npm install && cd ..

# 启动服务器 (需要 4 个终端)
# 终端 1: WireSocket 服务器
cd server && sudo ./wire-socket-server

# 终端 2: wstunnel
sudo wstunnel server wss://0.0.0.0:443 --restrict-to 127.0.0.1:51820

# 终端 3: WireSocket 客户端后端
cd client-backend && sudo ./wire-socket-client

# 终端 4: Electron 前端
cd electron && npm start

# 查看服务状态
ps aux | grep wire-socket
ps aux | grep wstunnel
curl http://127.0.0.1:41945/health
curl http://localhost:8080/health

# 查看日志
# 服务器: 直接在终端查看
# 客户端服务 (Linux): journalctl -u WireSocketClient -f
# 客户端服务 (macOS): tail -f /var/log/system.log | grep WireSocket
```

## 技术栈总结

| 组件 | 技术 | 版本 | 说明 |
|------|------|------|------|
| 服务器后端 | Go | 1.21+ | HTTP API + WireGuard 管理 |
| 客户端后端 | Go | 1.21+ | 系统服务 + VPN 控制 |
| 前端框架 | Electron | 28+ | 跨平台桌面应用 |
| VPN 协议 | WireGuard | - | 现代 VPN 协议 |
| 隧道工具 | wstunnel | - | WebSocket 隧道 |
| 数据库 | SQLite | 3 | 嵌入式数据库 |
| ORM | GORM | 1.25+ | Go ORM 框架 |
| Web 框架 | Gin | 1.9+ | Go HTTP 框架 |
| 认证 | JWT | - | JSON Web Tokens |

## 联系与支持

- **项目名称**: WireSocket
- **完整文档**: `README.md`
- **Claude 指南**: `CLAUDE.md`
- **配置文件**: `server/config.yaml`
- **数据库**: `server/vpn.db`

## 下次打开时恢复工作

1. **进入项目目录**:
   ```bash
   cd wire-socket
   ls -la
   ```

2. **阅读本文件**: `cat PROJECT_SUMMARY.md`

3. **查看完整文档**: `cat README.md`

4. **按"快速启动指南"部分启动服务**

---

**保存日期**: 2025-12-22
**项目名称**: WireSocket
**项目状态**: ✅ 完成 - 所有核心功能已实现
