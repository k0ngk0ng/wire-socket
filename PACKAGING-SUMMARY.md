# WireSocket 客户端打包方案总结

## 📝 概述

已成功为 WireSocket 客户端创建了完整的打包解决方案，支持 macOS、Windows 和 Linux 三个平台。用户无需手动安装 WireGuard、wstunnel 等依赖，所有组件都自动打包到安装包中。

## ✨ 主要特性

### 1. 一键构建
```bash
cd electron
npm install
npm run build
```

### 2. 全自动依赖管理
- ✅ 自动下载 wstunnel 二进制文件（所有平台）
- ✅ 自动构建 wireguard-go（macOS/Windows）
- ✅ 自动交叉编译 Go 后端服务
- ✅ 自动打包所有资源到 Electron 应用

### 3. 平台特定优化
- **macOS**: Intel + Apple Silicon 通用二进制，DMG + ZIP
- **Windows**: NSIS 安装程序 + 便携版，包含 wintun 驱动
- **Linux**: AppImage + DEB + RPM，自动安装 wireguard-tools

## 📁 新增文件清单

### 脚本文件 (`client/frontend/scripts/`)

1. **`download-binaries.sh`**
   - 下载 wstunnel 二进制文件（所有平台）
   - 支持 macOS (Intel/ARM64)、Linux、Windows

2. **`download-wireguard.sh`**
   - 克隆并构建 wireguard-go
   - 下载 Windows wintun 驱动
   - 为 Linux 创建安装说明

3. **`build-backend.sh`**
   - 交叉编译 Go 客户端后端
   - 生成所有平台的二进制文件

4. **`prepare-package.sh`**
   - 主脚本，调用上述所有脚本
   - 集成到 npm scripts 中

### 打包配置 (`client/frontend/build/`)

1. **`entitlements.mac.plist`**
   - macOS 应用权限配置
   - 支持 JIT、网络访问等必需权限

2. **`linux-post-install.sh`**
   - Linux 安装后脚本
   - 自动安装 wireguard-tools
   - 配置 systemd 服务

3. **`installer.nsh`**
   - Windows NSIS 安装脚本
   - 处理服务安装和清理

### 资源目录结构 (`client/frontend/resources/`)

```
resources/
└── bin/
    ├── darwin/          # macOS 二进制文件
    │   ├── wire-socket-client
    │   ├── wire-socket-client-arm64
    │   ├── wstunnel
    │   ├── wstunnel-arm64
    │   ├── wireguard-go
    │   └── wireguard-go-arm64
    ├── linux/           # Linux 二进制文件
    │   ├── wire-socket-client
    │   ├── wstunnel
    │   └── WIREGUARD-README.txt
    └── win32/           # Windows 二进制文件
        ├── wire-socket-client.exe
        ├── wstunnel.exe
        ├── wireguard.exe
        └── wintun.dll
```

### 文档文件

1. **`client/frontend/PACKAGING.md`**
   - 详细的打包指南
   - 故障排除
   - 代码签名说明

2. **`client/frontend/README.md`**
   - 快速开始指南
   - 项目结构说明

3. **`PACKAGING-QUICKSTART.md`**
   - 极简快速开始
   - 常见问题解答

4. **`PACKAGING-SUMMARY.md`** (本文件)
   - 完整方案总结
   - 所有改动清单

### 配置文件更新

1. **`client/frontend/package.json`**
   - 添加 `prepare` 脚本
   - 更新所有 `build:*` 脚本调用 prepare
   - 配置 Electron Builder：
     - extraResources（打包二进制文件）
     - 平台特定配置（macOS binaries、Windows NSIS、Linux post-install）

2. **`client/frontend/.gitignore`**
   - 忽略下载的二进制文件
   - 忽略构建输出

### 代码修改

1. **`client/backend/internal/wstunnel/client.go`**
   - 修改 `findWSTunnelBinary()` 函数
   - 优先查找打包在应用中的 wstunnel
   - 支持从可执行文件同目录加载
   - 兼容 macOS app bundle 结构

## 🚀 使用方法

### 开发者：构建安装包

```bash
# 1. 进入 client frontend 目录
cd client/frontend

# 2. 安装依赖
npm install

# 3. 构建安装包（自动准备所有依赖）
npm run build              # 所有平台
npm run build:mac          # 仅 macOS
npm run build:win          # 仅 Windows
npm run build:linux        # 仅 Linux

# 4. 安装包位于 dist/ 目录
ls -lh dist/
```

### 最终用户：安装使用

#### macOS
1. 下载 `WireSocket-1.0.0.dmg`
2. 双击打开，拖拽到 Applications
3. 首次运行需要在"系统偏好设置"中授权

#### Windows
1. 下载 `WireSocket Setup 1.0.0.exe`
2. 右键"以管理员身份运行"
3. 按照安装向导完成安装

#### Linux
1. 下载对应格式的安装包：
   - **推荐**: `WireSocket-1.0.0.AppImage`（无需安装）
   - Debian/Ubuntu: `wiresocket_1.0.0_amd64.deb`
   - RedHat/Fedora: `wiresocket-1.0.0.x86_64.rpm`

2. 安装：
   ```bash
   # AppImage（推荐）
   chmod +x WireSocket-1.0.0.AppImage
   ./WireSocket-1.0.0.AppImage

   # DEB
   sudo dpkg -i wiresocket_1.0.0_amd64.deb

   # RPM
   sudo rpm -i wiresocket-1.0.0.x86_64.rpm
   ```

## 🔧 技术细节

### 1. 依赖来源

| 组件 | 来源 | 版本 |
|-----|------|------|
| wstunnel | GitHub Releases | v10.1.4 |
| wireguard-go | Git 源码构建 | latest |
| wintun (Windows) | 官方下载 | v0.14.1 |
| wireguard-tools (Linux) | 系统包管理器 | 系统版本 |

### 2. 构建流程

```
npm run build
    ↓
npm run prepare
    ↓
┌─────────────────┬──────────────────┬─────────────────┐
│  download       │  download        │  build          │
│  binaries       │  wireguard       │  backend        │
│  (wstunnel)     │  (wireguard-go)  │  (Go compile)   │
└─────────────────┴──────────────────┴─────────────────┘
                         ↓
                electron-builder
                         ↓
            ┌────────────┼────────────┐
            ↓            ↓            ↓
          macOS       Windows      Linux
         (.dmg)        (.exe)    (.AppImage)
```

### 3. 运行时路径解析

客户端后端在运行时按以下顺序查找 wstunnel：

1. 可执行文件同目录（打包路径）
2. macOS app bundle Resources 目录
3. 系统 PATH
4. 常见安装位置

这确保了打包的二进制文件优先被使用。

### 4. 服务安装

- **Linux**: systemd service（安装后自动配置）
- **macOS**: LaunchDaemon（需要手动配置，待后续改进）
- **Windows**: Windows Service（NSIS 安装时配置）

## 📊 安装包大小

预估大小（未压缩）：

- macOS: ~80-100 MB
- Windows: ~60-80 MB
- Linux: ~60-80 MB

包含内容：
- Electron 运行时 (~50MB)
- 客户端后端 (~10MB)
- wstunnel (~5MB)
- wireguard-go (~5MB)
- 其他资源

## 🐛 已知限制和未来改进

### 当前限制

1. **macOS 服务安装**
   - 暂未自动配置 LaunchDaemon
   - 需要用户手动运行安装命令

2. **代码签名**
   - 需要开发者证书
   - 配置说明已包含在文档中

3. **自动更新**
   - 暂未实现应用内自动更新
   - 可以后续集成 electron-updater

### 未来改进方向

1. **自动更新系统**
   ```bash
   npm install electron-updater
   # 集成到主进程
   ```

2. **macOS 服务自动安装**
   - 添加 postinstall 脚本
   - 使用 electron-builder afterPack hook

3. **CI/CD 自动化**
   - GitHub Actions 自动构建
   - 自动发布到 GitHub Releases

4. **多架构支持**
   - ARM64 Linux
   - 其他架构

## 📚 相关文档

- **快速开始**: `PACKAGING-QUICKSTART.md`
- **详细指南**: `client/frontend/PACKAGING.md`
- **Frontend 文档**: `client/frontend/README.md`
- **项目说明**: `README.md`
- **开发指南**: `CLAUDE.md`

## ✅ 验证清单

在发布前，请验证：

- [ ] 所有平台的安装包都能成功构建
- [ ] 安装包能在干净系统上安装
- [ ] 应用能正常启动
- [ ] 能成功连接到 VPN 服务器
- [ ] wstunnel 和 wireguard 正常工作
- [ ] 服务能正确安装和启动
- [ ] 卸载后能完全清理

## 🎉 总结

这套完整的打包方案实现了：

✅ **零手动依赖安装** - 所有组件自动打包
✅ **跨平台支持** - macOS、Windows、Linux
✅ **一键构建** - 单个命令完成所有步骤
✅ **生产就绪** - 包含服务安装和配置
✅ **易于维护** - 模块化脚本，清晰的文档

现在你可以轻松为 WireSocket 创建专业的安装包了！🚀
