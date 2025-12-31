# WireSocket 客户端打包指南

本指南说明如何为 macOS、Windows 和 Linux 构建完整的 WireSocket 客户端安装包。

## 📋 前置要求

在开始构建之前，确保已安装：

1. **Node.js** (v16+)
   ```bash
   node --version  # 检查版本
   ```

2. **Go** (v1.19+)
   ```bash
   go version  # 检查版本
   ```

3. **Git**
   ```bash
   git --version
   ```

4. **构建工具**
   - macOS: Xcode Command Line Tools
     ```bash
     xcode-select --install
     ```
   - Linux: build-essential
     ```bash
     sudo apt install build-essential  # Debian/Ubuntu
     ```
   - Windows: 暂不需要额外工具

## 🚀 快速开始

### 一键构建所有平台

```bash
cd electron
npm install
npm run build
```

这将：
1. 下载所有必需的二进制文件（wstunnel）
2. 构建 WireGuard 组件
3. 编译客户端后端（Go）
4. 打包 Electron 应用

### 构建特定平台

```bash
# 仅构建 macOS
npm run build:mac

# 仅构建 Windows
npm run build:win

# 仅构建 Linux
npm run build:linux
```

## 📦 打包内容

每个安装包都包含：

### 核心组件
- **Electron 前端**: 用户界面
- **客户端后端**: Go 编写的系统服务（wire-socket-client）
- **wstunnel**: WebSocket 隧道工具
- **WireGuard 组件**: 平台特定的 WireGuard 工具

### 平台差异

#### macOS (.dmg / .zip)
- 包含 Intel 和 Apple Silicon 的通用二进制文件
- wireguard-go（用户空间实现）
- 自动签名和公证（需要配置）

#### Windows (.exe / portable)
- NSIS 安装程序
- 包含 wintun 驱动
- wireguard-go for Windows
- 需要管理员权限安装

#### Linux (.AppImage / .deb / .rpm)
- 支持多种发行版
- 安装后自动安装 wireguard-tools（如果缺失）
- 自动配置 systemd 服务

## 🔧 详细步骤

### 步骤 1: 准备依赖

```bash
cd electron
npm install
```

### 步骤 2: 下载和构建二进制文件

```bash
npm run prepare
```

这个命令会运行 `scripts/prepare-package.sh`，包括：

1. **下载 wstunnel**
   - macOS (AMD64 + ARM64)
   - Linux (AMD64)
   - Windows (AMD64)

2. **构建 WireGuard 组件**
   - 克隆 wireguard-go
   - 为所有平台编译
   - 下载 Windows wintun 驱动

3. **构建客户端后端**
   - 为所有平台交叉编译 Go 程序
   - 生成平台特定的二进制文件

### 步骤 3: 打包应用

```bash
# 打包所有平台
npm run build

# 或者指定平台
npm run build:mac
npm run build:win
npm run build:linux
```

## 📁 输出文件

构建完成后，安装包位于 `electron/dist/` 目录：

```
dist/
├── WireSocket-1.0.0.dmg              # macOS 磁盘映像
├── WireSocket-1.0.0-mac.zip          # macOS ZIP
├── WireSocket Setup 1.0.0.exe        # Windows 安装程序
├── WireSocket 1.0.0.exe              # Windows 便携版
├── WireSocket-1.0.0.AppImage         # Linux AppImage
├── wiresocket_1.0.0_amd64.deb        # Debian/Ubuntu
└── wiresocket-1.0.0.x86_64.rpm       # RedHat/Fedora
```

## 🔐 代码签名（可选但推荐）

### macOS 签名

1. 获取 Apple Developer 证书
2. 配置环境变量：
   ```bash
   export CSC_LINK=/path/to/certificate.p12
   export CSC_KEY_PASSWORD=certificate_password
   export APPLE_ID=your@apple.id
   export APPLE_ID_PASSWORD=app-specific-password
   ```
3. 构建时自动签名和公证

### Windows 签名

1. 获取代码签名证书
2. 配置环境变量：
   ```bash
   export CSC_LINK=/path/to/certificate.pfx
   export CSC_KEY_PASSWORD=certificate_password
   ```

## 🐛 故障排除

### 问题: "wireguard-go build failed"

**解决方案**: 确保 Go 已正确安装，并且可以访问 git.zx2c4.com

```bash
go version
git clone https://git.zx2c4.com/wireguard-go  # 测试连接
```

### 问题: "wstunnel download failed"

**解决方案**: 检查网络连接，或手动下载到对应目录：
- `electron/resources/bin/darwin/wstunnel`
- `electron/resources/bin/linux/wstunnel`
- `electron/resources/bin/win32/wstunnel.exe`

### 问题: Electron Builder 失败

**解决方案**: 清理并重新安装依赖

```bash
cd electron
rm -rf node_modules dist
npm install
npm run build
```

### 问题: Linux 构建需要 Docker

如果在非 Linux 系统上构建 Linux 包，可能需要 Docker：

```bash
# 安装 Docker Desktop
# 然后运行
npm run build:linux
```

Electron Builder 会自动使用 Docker 容器构建 Linux 包。

## 📝 自定义配置

### 修改版本号

编辑 `electron/package.json`:

```json
{
  "version": "1.0.0"
}
```

### 修改应用图标

替换以下文件：
- macOS: `electron/public/icon.icns`
- Windows: `electron/public/icon.ico`
- Linux: `electron/public/icon.png`

### 修改应用名称

编辑 `electron/package.json`:

```json
{
  "name": "wire-socket",
  "productName": "WireSocket"
}
```

## 🚢 发布

### GitHub Releases

1. 创建 Git tag:
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```

2. 上传 `dist/` 目录中的文件到 GitHub Release

### 自动发布（CI/CD）

可以配置 GitHub Actions 自动构建和发布。创建 `.github/workflows/release.yml`:

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

jobs:
  release:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [macos-latest, ubuntu-latest, windows-latest]
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-node@v3
      - uses: actions/setup-go@v4
      - run: cd electron && npm install
      - run: cd electron && npm run build
      - uses: softprops/action-gh-release@v1
        with:
          files: electron/dist/*
```

## ⚙️ 安装后配置

### macOS
安装后，应用会请求权限：
- **网络扩展权限**: 用于创建 VPN 连接
- **管理员权限**: 用于配置网络接口

用户可能需要在"系统偏好设置 > 安全性与隐私"中批准。

### Windows
- 安装需要管理员权限
- Windows Defender 可能会警告，需要允许
- 客户端服务会自动安装为 Windows 服务

### Linux
- `.deb` / `.rpm`: 安装后会自动：
  - 安装 wireguard-tools（如果缺失）
  - 配置 systemd 服务
  - 设置开机自启

- `.AppImage`:
  - 不需要安装，直接运行
  - 首次运行时会提示安装系统服务（需要 sudo）

## 📚 更多资源

- [Electron Builder 文档](https://www.electron.build/)
- [WireGuard 官方网站](https://www.wireguard.com/)
- [wstunnel GitHub](https://github.com/erebe/wstunnel)

## 🤝 贡献

如果遇到打包问题或有改进建议，欢迎提交 Issue 或 Pull Request。
