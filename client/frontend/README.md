# WireSocket Electron 客户端

WireSocket 的跨平台桌面客户端，基于 Electron 构建。

## 快速开始

### 开发模式

```bash
npm install
npm start
```

### 构建安装包

```bash
# 构建所有平台
npm run build

# 构建特定平台
npm run build:mac     # macOS
npm run build:win     # Windows
npm run build:linux   # Linux
```

构建的安装包将输出到 `dist/` 目录。

## 📦 安装包说明

所有安装包都包含了必需的依赖：
- ✅ 客户端后端服务（Go）
- ✅ wstunnel（WebSocket 隧道）
- ✅ WireGuard 组件
- ✅ 无需手动安装任何依赖

### 平台特定安装包

#### macOS
- **WireSocket.dmg**: 磁盘映像，拖拽安装
- **WireSocket-mac.zip**: ZIP 压缩包

#### Windows
- **WireSocket Setup.exe**: 标准安装程序
- **WireSocket.exe**: 便携版（无需安装）

#### Linux
- **WireSocket.AppImage**: 通用格式（推荐）
- **wiresocket.deb**: Debian/Ubuntu
- **wiresocket.rpm**: RedHat/Fedora/CentOS

## 📖 详细文档

查看 [PACKAGING.md](./PACKAGING.md) 了解：
- 详细的构建步骤
- 故障排除
- 代码签名
- CI/CD 配置

## 🔧 开发

### 项目结构

```
electron/
├── src/
│   ├── main/          # 主进程
│   └── preload/       # 预加载脚本
├── public/            # 静态资源
├── resources/         # 打包资源
│   └── bin/           # 各平台的二进制文件
│       ├── darwin/    # macOS
│       ├── linux/     # Linux
│       └── win32/     # Windows
├── scripts/           # 构建脚本
└── build/             # 打包配置
```

### 可用脚本

- `npm start`: 启动开发服务器
- `npm run prepare`: 准备打包资源（下载依赖、构建后端）
- `npm run build`: 构建所有平台的安装包
- `npm run build:mac`: 仅构建 macOS
- `npm run build:win`: 仅构建 Windows
- `npm run build:linux`: 仅构建 Linux

## 🐛 问题排查

### "wstunnel binary not found"

运行准备脚本：
```bash
npm run prepare
```

### "Go not installed"

安装 Go: https://golang.org/dl/

### Electron Builder 错误

清理并重新安装：
```bash
rm -rf node_modules dist
npm install
npm run build
```

## 📝 许可证

MIT
