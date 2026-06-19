# WireSocket Tauri 客户端

WireSocket 的跨平台桌面客户端，基于 Tauri 2 构建。前端使用静态 HTML/CSS/JavaScript，桌面能力由 Rust/Tauri 负责，客户端后端仍然是 Go 系统服务。

## 快速开始

### 开发模式

```bash
npm install
npm start
```

`npm start` 会运行 `tauri dev`。本机需要安装 Rust 工具链和平台 WebView 依赖。

### 构建安装包

```bash
# 构建当前平台安装包
npm run build

# 构建特定平台
npm run build:mac
npm run build:win
npm run build:linux
```

构建前会运行 `npm run prepare`，用于准备 Go 后端服务和平台二进制资源。

## 打包内容

安装包包含：

- Tauri 2 桌面壳
- Go 客户端后端服务 `wire-socket-client`
- WireGuard 平台组件
- Windows `wintun.dll`

WebSocket tunnel 已经内置在 Go 后端中，不再需要外部 `wstunnel` 二进制。

## 项目结构

```text
frontend/
├── public/             # 静态 UI
├── resources/
│   └── bin/            # npm run prepare 生成的平台二进制资源
│       ├── darwin/
│       ├── linux/
│       └── win32/
├── scripts/            # 构建和资源准备脚本
├── src-tauri/          # Tauri 2 Rust 入口、配置、权限和图标
└── package.json
```

## 可用脚本

- `npm start`: 启动 Tauri 开发模式
- `npm run prepare`: 准备打包资源，包含跨平台后端构建
- `npm run build`: 构建当前平台安装包
- `npm run build:mac`: 在 macOS 上构建 `.app`
- `npm run build:mac:dmg`: 在 macOS GUI 会话中构建 DMG
- `npm run build:win`: 在 Windows 上构建 NSIS 安装包
- `npm run build:linux`: 在 Linux 上构建 AppImage/deb/rpm
- `npm run tauri -- <command>`: 直接运行 Tauri CLI

## 桌面能力

Tauri 侧负责：

- 自动检测、安装、启动 Go 后端服务
- 系统托盘和隐藏到托盘
- `wiresocket://` deep link SSO 回调
- 与本地后端 API 的命令桥接

UI 侧只负责界面状态和用户交互，默认调用 Tauri command；浏览器预览时会退回到 `http://127.0.0.1:41945`。

## 问题排查

### `cargo` 或 `rustc` 未找到

安装 Rust 工具链：

```bash
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
```

### 后端服务未找到

运行资源准备：

```bash
npm run prepare
```

### Tauri 构建失败

清理前端依赖和 Tauri 产物后重试：

```bash
rm -rf node_modules src-tauri/target
npm install
npm run build
```

## 许可证

MIT
