# 🚀 WireSocket 快速启动指南

**项目名称**: WireSocket - WireGuard over WebSocket tunneling

## ⚡ 一键启动（开发环境）

### 方法 1: 使用脚本（推荐）

创建启动脚本 `start-all.sh`:
```bash
#!/bin/bash

# 启动服务器
echo "启动 WireSocket 服务器..."
cd server
sudo ./wire-socket-server &
SERVER_PID=$!

# 等待服务器启动
sleep 2

# 启动 wstunnel（需要单独终端）
echo "请在新终端运行: sudo wstunnel server wss://0.0.0.0:443 --restrict-to 127.0.0.1:51820"

# 启动客户端后端
cd ../client-backend
sudo ./wire-socket-client &
CLIENT_PID=$!

# 启动 Electron
cd ../electron
npm start &
ELECTRON_PID=$!

echo "所有服务已启动！"
echo "服务器 PID: $SERVER_PID"
echo "客户端 PID: $CLIENT_PID"
echo "Electron PID: $ELECTRON_PID"
```

### 方法 2: 手动启动（4个终端）

**终端 1 - WireSocket 服务器:**
```bash
cd server
sudo ./wire-socket-server
```

**终端 2 - wstunnel 服务器:**
```bash
sudo wstunnel server wss://0.0.0.0:443 --restrict-to 127.0.0.1:51820
```

**终端 3 - WireSocket 客户端:**
```bash
cd client-backend
sudo ./wire-socket-client
```

**终端 4 - Electron 前端:**
```bash
cd electron
npm start
```

## 🔑 默认登录

- **服务器地址**: `localhost:8080`
- **用户名**: `admin`
- **密码**: `admin123`

## 🛠️ 首次运行必做

### 1️⃣ 初始化数据库（只需一次）
```bash
cd server
sudo ./wire-socket-server -init-db
```

### 2️⃣ 安装 wstunnel
```bash
# macOS
wget https://github.com/erebe/wstunnel/releases/latest/download/wstunnel_macos_amd64
chmod +x wstunnel_macos_amd64
sudo mv wstunnel_macos_amd64 /usr/local/bin/wstunnel

# Linux
wget https://github.com/erebe/wstunnel/releases/latest/download/wstunnel_linux_amd64
chmod +x wstunnel_linux_amd64
sudo mv wstunnel_linux_amd64 /usr/local/bin/wstunnel
```

### 3️⃣ 修改配置（可选）
编辑 `server/config.yaml`:
```yaml
wireguard:
  endpoint: "你的服务器IP:51820"  # 改成实际IP

auth:
  jwt_secret: "改成随机字符串"  # 生产环境必须改
```

## 📁 重要文件位置

```
wire-socket/
├── PROJECT_SUMMARY.md          ← 完整项目文档（中文）
├── README.md                   ← 详细使用说明（英文）
├── CLAUDE.md                   ← Claude Code 开发指南
├── QUICK_START.md              ← 本文件
│
├── server/
│   ├── wire-socket-server      ← 服务器可执行文件
│   ├── config.yaml             ← 服务器配置（需要修改）
│   └── vpn.db                  ← 数据库（自动创建）
│
├── client-backend/
│   └── wire-socket-client      ← 客户端服务可执行文件
│
└── electron/
    ├── public/index.html       ← UI 界面
    └── package.json            ← 前端配置
```

## 🔍 检查服务状态

```bash
# 检查所有进程
ps aux | grep -E "wire-socket|wstunnel|electron"

# 测试服务器 API
curl http://localhost:8080/health

# 测试客户端 API
curl http://127.0.0.1:41945/health

# 查看 WireGuard 接口
sudo wg show
```

## 🐛 常见错误速查

| 错误信息 | 解决方案 |
|---------|---------|
| `Permission denied` | 使用 `sudo` 运行 |
| `wstunnel not found` | 按上面步骤安装 wstunnel |
| `Failed to configure WireGuard` | `sudo apt install wireguard-tools` |
| `Connection refused` | 检查服务器是否启动 |
| `Authentication failed` | 检查用户名密码 (admin/admin123) |

## 🔄 重新构建

```bash
# 构建服务器
cd server && go build -o wire-socket-server cmd/server/main.go && cd ..

# 构建客户端
cd client-backend && go build -o wire-socket-client cmd/client/main.go && cd ..

# 安装 Electron 依赖
cd electron && npm install && cd ..
```

## 📊 数据库管理

```bash
# 查看数据库
cd server
sqlite3 vpn.db

# 常用 SQL 命令
.tables                      # 列出表
SELECT * FROM users;         # 查看用户
SELECT * FROM allocated_ips; # 查看 IP 分配
.quit                        # 退出
```

## 🎯 下次启动流程

1. **进入项目目录:**
   ```bash
   cd wire-socket
   ```

2. **阅读本文件:**
   ```bash
   cat QUICK_START.md
   ```

3. **按"一键启动"部分启动所有服务**

4. **打开浏览器访问 Electron 应用或运行 `npm start`**

---

💡 **提示**:
- 完整中文文档: `PROJECT_SUMMARY.md`
- 英文说明: `README.md`
- Claude Code 开发指南: `CLAUDE.md`
