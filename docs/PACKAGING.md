# WireSocket 客户端打包指南

本指南说明如何使用 Tauri 2 为 macOS、Windows 和 Linux 构建 WireSocket 桌面客户端安装包。

## 前置要求

1. Node.js 18+
   ```bash
   node --version
   npm --version
   ```

2. Go 1.22+
   ```bash
   go version
   ```

3. Rust 工具链
   ```bash
   rustc --version
   cargo --version
   ```

4. 平台构建工具
   - macOS: Xcode Command Line Tools
   - Linux: Tauri/WebKitGTK 依赖和系统构建工具
   - Windows: Microsoft C++ Build Tools

## 快速构建

```bash
cd client/frontend
npm install
npm run build
```

这会执行：

1. `npm run prepare`: 准备平台二进制资源并交叉编译 Go 客户端后端。
2. `tauri build`: 使用 Tauri 2 打包当前平台桌面应用。

## 构建特定平台

```bash
cd client/frontend
npm run build:mac
npm run build:win
npm run build:linux
```

Tauri 桌面应用通常需要在目标操作系统上打包。GitHub Actions 会分别在 macOS、Windows、Linux runner 上构建对应安装包。

macOS 默认构建 `.app`，CI 会将 `.app` 压缩为 zip 发布。`npm run build:mac:dmg` 可在本机 GUI 会话中尝试构建 DMG；该步骤依赖 Finder AppleScript，在 headless/自动化环境中不稳定。

构建产物位于 `client/frontend/src-tauri/target/release/bundle/`。

## 打包内容

安装包包含：

- Tauri 2 桌面壳和静态 UI
- Go 客户端后端服务 `wire-socket-client`
- macOS `wireguard-go`
- Windows `wireguard.exe` 和 `wintun.dll`
- Linux 后端二进制和 WireGuard 说明文件

WebSocket tunnel 已内置在 Go 后端中，不再打包外部 `wstunnel`。

## 图标

源图标位于 `client/assets/icon-1024.png`。重新生成 Tauri 图标：

```bash
cd client/frontend
npm run tauri -- icon ../assets/icon-1024.png
```

生成文件位于 `client/frontend/src-tauri/icons/`。

## 代码签名

### macOS

在 `src-tauri/tauri.conf.json` 中已启用 hardened runtime，并使用 `src-tauri/entitlements.plist`。实际签名和公证需要配置 Apple Developer 证书与 Tauri 支持的签名环境变量。

### Windows

Windows 安装包使用 Tauri NSIS bundle。生产发布时建议配置代码签名证书，避免安装时出现未知发布者警告。

## 常见问题

### `cargo` 或 `rustc` 未找到

安装 Rust：

```bash
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
```

安装后重新打开终端，确认 `cargo --version` 可用。

### 后端二进制未找到

运行：

```bash
cd client/frontend
npm run prepare
```

### Linux 缺少 WebKitGTK 依赖

按 Tauri 官方 Linux 依赖说明安装 WebKitGTK、ayatana appindicator、librsvg 等系统包，然后重试 `npm run build:linux`。

### 需要减小安装包体积

当前 Tauri release profile 已启用：

- `opt-level = "s"`
- `lto = "thin"`
- `codegen-units = 1`
- `strip = "symbols"`

如果还需要进一步减小体积，优先检查 Go 后端二进制和平台 WireGuard 资源大小。
