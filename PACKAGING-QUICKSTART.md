# 快速开始：构建 WireSocket 安装包

本指南帮助你快速构建包含所有依赖的 WireSocket 客户端安装包。

## 🎯 目标

构建一个完整的客户端安装包，包含：
- Electron 前端应用
- Go 后端服务
- wstunnel（无需手动安装）
- WireGuard 组件（无需手动安装）

## ⚡ 快速步骤

### 1. 确保已安装必需工具

```bash
# 检查 Node.js (需要 v16+)
node --version

# 检查 Go (需要 v1.19+)
go version

# 检查 Git
git --version
```

如果缺少任何工具，请先安装：
- **Node.js**: https://nodejs.org/
- **Go**: https://golang.org/dl/
- **Git**: https://git-scm.com/downloads

### 2. 进入 client frontend 目录

```bash
cd wire-socket/client/frontend
```

### 3. 安装 npm 依赖

```bash
npm install
```

### 4. 一键构建

```bash
# 构建所有平台
npm run build

# 或者只构建当前平台
npm run build:mac     # macOS only
npm run build:win     # Windows only
npm run build:linux   # Linux only
```

### 5. 获取安装包

构建完成后，在 `client/frontend/dist/` 目录查找安装包：

**macOS:**
- `WireSocket-1.0.0.dmg`
- `WireSocket-1.0.0-mac.zip`

**Windows:**
- `WireSocket Setup 1.0.0.exe`
- `WireSocket 1.0.0.exe` (便携版)

**Linux:**
- `WireSocket-1.0.0.AppImage`
- `wiresocket_1.0.0_amd64.deb`
- `wiresocket-1.0.0.x86_64.rpm`

## 🔍 构建过程说明

运行 `npm run build` 时会自动执行以下步骤：

1. **下载 wstunnel 二进制文件**
   - macOS (Intel + Apple Silicon)
   - Linux (AMD64)
   - Windows (AMD64)

2. **构建 WireGuard 组件**
   - 从源码构建 wireguard-go
   - 下载 Windows wintun 驱动

3. **编译客户端后端**
   - 为所有平台交叉编译 Go 程序
   - 生成优化的二进制文件

4. **打包 Electron 应用**
   - 将所有组件打包到安装包中
   - 创建平台特定的安装程序

## ⏱️ 预计时间

首次构建（下载所有依赖）：
- macOS: ~5-10 分钟
- Linux: ~5-10 分钟
- Windows: ~5-10 分钟

后续构建（依赖已缓存）：
- ~2-3 分钟

## 🐛 常见问题

### 问题 1: "Go not found"

**解决方案**: 安装 Go
```bash
# macOS
brew install go

# Linux
sudo apt install golang  # Debian/Ubuntu
sudo yum install golang  # CentOS/RHEL

# Windows
# 从 https://golang.org/dl/ 下载安装包
```

### 问题 2: "npm install failed"

**解决方案**: 清理并重试
```bash
rm -rf node_modules package-lock.json
npm install
```

### 问题 3: "wireguard-go build failed"

**解决方案**: 检查 Git 连接
```bash
# 测试是否能访问 Git 仓库
git clone https://git.zx2c4.com/wireguard-go /tmp/test-wg
rm -rf /tmp/test-wg
```

如果无法访问，可以手动下载并放置：
1. 访问 https://git.zx2c4.com/wireguard-go/
2. 下载源码
3. 手动构建并放置到 `client/frontend/resources/bin/{platform}/`

### 问题 4: "Permission denied"

**解决方案**:
```bash
# 给脚本添加执行权限
chmod +x client/frontend/scripts/*.sh

# 重新运行
npm run build
```

### 问题 5: "Electron Builder failed"

**解决方案**: 清理并重新构建
```bash
rm -rf dist node_modules resources/bin
npm install
npm run build
```

## 🎨 自定义构建

### 只准备依赖（不打包）

```bash
npm run prepare
```

这会下载所有依赖和构建后端，但不会创建安装包。

### 修改版本号

编辑 `package.json`:
```json
{
  "version": "1.0.1"
}
```

### 更换应用图标

替换以下文件：
- macOS: `public/icon.icns`
- Windows: `public/icon.ico`
- Linux: `public/icon.png`

## 📚 更多帮助

- **详细打包文档**: [client/frontend/PACKAGING.md](client/frontend/PACKAGING.md)
- **项目架构**: [CLAUDE.md](CLAUDE.md)
- **完整文档**: [README.md](README.md)

## ✅ 验证安装包

### macOS
```bash
# 打开 DMG
open dist/WireSocket-1.0.0.dmg

# 或直接运行 ZIP 中的应用
unzip dist/WireSocket-1.0.0-mac.zip
open WireSocket.app
```

### Windows
```bash
# 运行安装程序
dist/"WireSocket Setup 1.0.0.exe"

# 或运行便携版
dist/"WireSocket 1.0.0.exe"
```

### Linux
```bash
# AppImage（推荐，无需安装）
chmod +x dist/WireSocket-1.0.0.AppImage
./dist/WireSocket-1.0.0.AppImage

# 或安装 DEB
sudo dpkg -i dist/wiresocket_1.0.0_amd64.deb

# 或安装 RPM
sudo rpm -i dist/wiresocket-1.0.0.x86_64.rpm
```

## 🚀 发布到生产环境

1. **测试安装包**
   - 在干净的系统上测试安装
   - 验证所有功能正常工作

2. **签名（推荐）**
   - macOS: 使用 Apple Developer 证书
   - Windows: 使用代码签名证书

3. **上传到 GitHub Release**
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   # 在 GitHub 上创建 Release 并上传 dist/ 中的文件
   ```

## 💡 提示

- 首次构建会下载约 100-200MB 的依赖
- 依赖会被缓存，后续构建更快
- 可以在 CI/CD 中使用相同的命令自动化构建
- 交叉编译只需要在一台机器上完成

现在你可以开始构建了！🎉
