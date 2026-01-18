# WireSocket SSO 认证方案

本文档描述 WireSocket 的 SSO（单点登录）认证设计，支持 OIDC 和 OAuth 2.0 协议，与现有本地用户认证共存。

## 目录

- [概述](#概述)
- [认证架构](#认证架构)
- [支持的协议](#支持的协议)
- [配置说明](#配置说明)
- [API 参考](#api-参考)
- [客户端集成](#客户端集成)
- [用户管理](#用户管理)
- [实现计划](#实现计划)

---

## 概述

### 设计目标

| 目标 | 说明 |
|------|------|
| **多认证源** | 支持本地用户、OIDC、OAuth 2.0 共存 |
| **企业友好** | 集成 Azure AD、Okta、Google Workspace 等主流 IdP |
| **向后兼容** | 现有本地用户认证方式保持不变 |
| **可扩展** | 预留 SAML 2.0 等协议的扩展接口 |

### 认证方式对比

| 协议 | 适用场景 | 复杂度 | 状态 |
|------|---------|--------|------|
| **Local** | 小团队、测试环境 | 低 | ✅ 已实现 |
| **OIDC** | 企业 IdP（Azure AD、Okta、Keycloak） | 中 | 🎯 优先实现 |
| **OAuth 2.0** | 开发者平台（GitHub、GitLab） | 中 | 🎯 优先实现 |
| **SAML 2.0** | 传统企业系统 | 高 | 📋 预留扩展 |

---

## 认证架构

### 整体架构

```text
┌─────────────────────────────────────────────────────────────────────────┐
│                                                                          │
│  Client                          Gateway Server                          │
│  ┌─────────┐                    ┌─────────────────────────────────────┐ │
│  │         │  1. GET /providers │                                     │ │
│  │         │───────────────────►│  Auth Manager                       │ │
│  │         │◄───────────────────│  ┌─────────────────────────────────┐│ │
│  │         │  返回认证方式列表   │  │ Providers:                      ││ │
│  │         │                    │  │  ├── LocalProvider (内置)       ││ │
│  │         │  2. 选择 SSO       │  │  ├── OIDCProvider (可配置多个)  ││ │
│  │         │───────────────────►│  │  └── OAuth2Provider (可配置多个)││ │
│  │         │                    │  └─────────────────────────────────┘│ │
│  │         │  3. 重定向到 IdP   │                                     │ │
│  │         │◄───────────────────│                                     │ │
│  └────┬────┘                    └─────────────────────────────────────┘ │
│       │                                                                  │
│       │  4. 用户在 IdP 登录                                              │
│       ▼                                                                  │
│  ┌─────────────────┐                                                    │
│  │ Identity        │  Azure AD / Okta / Google / GitHub / ...           │
│  │ Provider (IdP)  │                                                    │
│  └────────┬────────┘                                                    │
│           │                                                              │
│           │  5. 回调 (code)                                              │
│           ▼                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐│
│  │  Gateway Server                                                      ││
│  │                                                                      ││
│  │  6. 用 code 换 token                                                 ││
│  │  7. 获取用户信息                                                     ││
│  │  8. JIT Provisioning (首次登录自动创建用户)                          ││
│  │  9. 签发 WireSocket JWT                                              ││
│  │                                                                      ││
│  └─────────────────────────────────────────────────────────────────────┘│
│           │                                                              │
│           │  10. 返回 JWT + VPN 配置                                     │
│           ▼                                                              │
│  ┌─────────┐                                                            │
│  │ Client  │  正常使用 VPN                                              │
│  └─────────┘                                                            │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### 与 Mesh 的关系

SSO 认证只在 Gateway 节点处理：

```text
┌───────────────────────────────────────────────────────────┐
│                                                            │
│  Client ──SSO Login──► Gateway (ServerA)                  │
│                              │                             │
│                              │ JWT 签发                    │
│                              │                             │
│                              ▼                             │
│                         Mesh Network                       │
│                         ┌─────────┐                        │
│                         │ Exit B  │  ← 不处理认证          │
│                         │ Exit C  │  ← 只转发流量          │
│                         └─────────┘                        │
│                                                            │
└───────────────────────────────────────────────────────────┘
```

---

## 支持的协议

### OIDC (OpenID Connect)

**推荐场景**：企业 IdP 集成

**支持的 IdP**：
- Azure AD / Entra ID
- Okta
- Google Workspace
- Keycloak
- Auth0
- 任何符合 OIDC 标准的 IdP

**流程**：Authorization Code Flow (with PKCE)

```text
1. Client → Gateway: GET /api/auth/sso/azure-ad
2. Gateway → Client: 302 Redirect to Azure AD
3. Client → Azure AD: 用户登录
4. Azure AD → Gateway: Callback with code
5. Gateway → Azure AD: Exchange code for tokens
6. Gateway → Azure AD: Get userinfo (or decode id_token)
7. Gateway → Client: WireSocket JWT
```

### OAuth 2.0

**推荐场景**：开发者平台集成

**支持的 Provider**：
- GitHub
- GitLab
- Bitbucket
- 自定义 OAuth 2.0 服务

**与 OIDC 的区别**：
- OAuth 2.0 只提供授权，不提供标准化的用户信息
- 需要额外调用 userinfo 端点获取用户信息
- 配置时需要指定 userinfo URL 和字段映射

---

## 配置说明

### config.yaml 扩展

```yaml
auth:
  # JWT 配置（保持现有）
  jwt_secret: "change-this-secret-in-production"

  # 允许公开注册（本地用户）
  allow_registration: false

  # SSO 配置
  sso:
    # 是否启用 SSO
    enabled: false

    # SSO 回调基础 URL（必须是外部可访问的地址）
    callback_base_url: "https://vpn.example.com"

    # 认证 Provider 列表
    providers:
      # OIDC Provider 示例：Azure AD
      - id: "azure-ad"
        type: "oidc"
        name: "Microsoft Azure AD"
        enabled: true

        # OIDC Discovery URL（推荐，自动获取配置）
        issuer: "https://login.microsoftonline.com/{tenant-id}/v2.0"

        # OAuth 凭证
        client_id: "your-client-id"
        client_secret: "your-client-secret"

        # 请求的 scope
        scopes: ["openid", "profile", "email"]

        # 用户映射
        mapping:
          # 用哪个 claim 作为用户名
          username: "preferred_username"  # 或 "email", "sub"
          # 用哪个 claim 作为邮箱
          email: "email"
          # 用哪个 claim 判断管理员（可选）
          admin_claim: "groups"
          admin_values: ["VPN-Admins", "IT-Admins"]

      # OIDC Provider 示例：Okta
      - id: "okta"
        type: "oidc"
        name: "Okta"
        enabled: false
        issuer: "https://your-org.okta.com"
        client_id: "xxx"
        client_secret: "xxx"
        scopes: ["openid", "profile", "email", "groups"]
        mapping:
          username: "email"
          email: "email"
          admin_claim: "groups"
          admin_values: ["vpn_admins"]

      # OIDC Provider 示例：Google Workspace
      - id: "google"
        type: "oidc"
        name: "Google"
        enabled: false
        issuer: "https://accounts.google.com"
        client_id: "xxx.apps.googleusercontent.com"
        client_secret: "xxx"
        scopes: ["openid", "profile", "email"]
        mapping:
          username: "email"
          email: "email"
        # 限制只允许特定域名的用户
        allowed_domains: ["your-company.com"]

      # OAuth 2.0 Provider 示例：GitHub
      - id: "github"
        type: "oauth2"
        name: "GitHub"
        enabled: false

        # OAuth 2.0 端点（非 OIDC，需手动配置）
        authorize_url: "https://github.com/login/oauth/authorize"
        token_url: "https://github.com/login/oauth/access_token"
        userinfo_url: "https://api.github.com/user"

        client_id: "xxx"
        client_secret: "xxx"
        scopes: ["user:email", "read:org"]

        # 用户信息字段映射
        mapping:
          username: "login"      # GitHub 用户名
          email: "email"

        # 限制只允许特定组织的成员
        allowed_orgs: ["your-org"]
        orgs_url: "https://api.github.com/user/orgs"

      # OAuth 2.0 Provider 示例：GitLab
      - id: "gitlab"
        type: "oauth2"
        name: "GitLab"
        enabled: false
        authorize_url: "https://gitlab.com/oauth/authorize"
        token_url: "https://gitlab.com/oauth/token"
        userinfo_url: "https://gitlab.com/api/v4/user"
        client_id: "xxx"
        client_secret: "xxx"
        scopes: ["openid", "profile", "email"]
        mapping:
          username: "username"
          email: "email"
```

### 配置项说明

| 配置项 | 必填 | 说明 |
|--------|------|------|
| `sso.enabled` | 否 | 是否启用 SSO，默认 false |
| `sso.callback_base_url` | 是* | SSO 回调地址，必须外部可访问 |
| `providers[].id` | 是 | Provider 唯一标识，用于 URL |
| `providers[].type` | 是 | `oidc` 或 `oauth2` |
| `providers[].name` | 是 | 显示名称 |
| `providers[].issuer` | 是* | OIDC Discovery URL（OIDC 类型必填） |
| `providers[].authorize_url` | 是* | OAuth 2.0 授权 URL（OAuth2 类型必填） |
| `providers[].token_url` | 是* | OAuth 2.0 Token URL（OAuth2 类型必填） |
| `providers[].userinfo_url` | 是* | 用户信息 URL（OAuth2 类型必填） |
| `providers[].client_id` | 是 | OAuth Client ID |
| `providers[].client_secret` | 是 | OAuth Client Secret |
| `providers[].scopes` | 否 | 请求的 scope，默认 `["openid", "profile", "email"]` |
| `providers[].mapping` | 否 | 用户信息字段映射 |
| `providers[].allowed_domains` | 否 | 允许的邮箱域名（白名单） |
| `providers[].allowed_orgs` | 否 | 允许的组织（GitHub/GitLab） |

---

## API 参考

### 获取认证方式列表

```http
GET /api/auth/providers
```

**响应**：

```json
{
  "providers": [
    {
      "id": "local",
      "type": "local",
      "name": "Local Account"
    },
    {
      "id": "azure-ad",
      "type": "oidc",
      "name": "Microsoft Azure AD"
    },
    {
      "id": "github",
      "type": "oauth2",
      "name": "GitHub"
    }
  ]
}
```

### 本地登录（保持现有）

```http
POST /api/auth/login
Content-Type: application/json

{
  "username": "admin",
  "password": "admin123"
}
```

### 发起 SSO 登录

```http
GET /api/auth/sso/{provider_id}
```

**参数**：

| 参数 | 说明 |
|------|------|
| `provider_id` | Provider ID，如 `azure-ad`、`github` |

**Query 参数**（可选）：

| 参数 | 说明 |
|------|------|
| `redirect_uri` | 登录完成后跳转的 URI（用于桌面/移动客户端） |
| `state` | 自定义 state 参数（用于 CSRF 防护） |

**响应**：

```http
HTTP/1.1 302 Found
Location: https://login.microsoftonline.com/xxx/oauth2/v2.0/authorize?
  client_id=xxx&
  redirect_uri=https://vpn.example.com/api/auth/callback/azure-ad&
  response_type=code&
  scope=openid+profile+email&
  state=xxx&
  code_challenge=xxx&
  code_challenge_method=S256
```

### SSO 回调

```http
GET /api/auth/callback/{provider_id}?code=xxx&state=xxx
```

**成功响应**（Web）：

```http
HTTP/1.1 302 Found
Location: /login-success?token=eyJ...
```

**成功响应**（API/客户端）：

如果请求时带了 `Accept: application/json`：

```json
{
  "token": "eyJ...",
  "user": {
    "id": 1,
    "username": "john@company.com",
    "email": "john@company.com",
    "is_admin": false,
    "auth_provider": "azure-ad"
  },
  "config": {
    "private_key": "...",
    "address": "10.0.0.5/24",
    "dns": "1.1.1.1",
    "peers": [...]
  }
}
```

### 获取当前用户信息

```http
GET /api/auth/me
Authorization: Bearer eyJ...
```

**响应**：

```json
{
  "id": 1,
  "username": "john@company.com",
  "email": "john@company.com",
  "is_admin": false,
  "auth_provider": "azure-ad",
  "last_login": "2024-01-01T12:00:00Z"
}
```

---

## 客户端集成

### Desktop Client（Electron）

```javascript
// 1. 获取可用的认证方式
const { providers } = await fetch('/api/auth/providers').then(r => r.json());

// 2. 用户选择 SSO 方式
const selectedProvider = providers.find(p => p.id === 'azure-ad');

// 3. 打开系统浏览器进行 SSO 登录
const { shell } = require('electron');
const callbackUrl = `wiresocket://auth/callback`;
const ssoUrl = `${serverUrl}/api/auth/sso/${selectedProvider.id}?redirect_uri=${encodeURIComponent(callbackUrl)}`;
shell.openExternal(ssoUrl);

// 4. 注册 deep link 处理器接收回调
app.setAsDefaultProtocolClient('wiresocket');
app.on('open-url', (event, url) => {
  // url: wiresocket://auth/callback?token=xxx
  const token = new URL(url).searchParams.get('token');
  // 使用 token 连接 VPN
});
```

### Mobile Client（iOS/Android）

使用 ASWebAuthenticationSession (iOS) 或 Custom Tabs (Android)：

```swift
// iOS 示例
import AuthenticationServices

func startSSO(provider: String) {
    let url = URL(string: "\(serverUrl)/api/auth/sso/\(provider)")!
    let scheme = "wiresocket"

    let session = ASWebAuthenticationSession(url: url, callbackURLScheme: scheme) { callbackURL, error in
        guard let url = callbackURL,
              let token = URLComponents(url: url, resolvingAgainstBaseURL: false)?
                .queryItems?.first(where: { $0.name == "token" })?.value
        else { return }

        // 使用 token 连接 VPN
        self.connectVPN(with: token)
    }
    session.start()
}
```

### CLI Client

```bash
# 1. 获取认证方式
curl https://vpn.example.com/api/auth/providers

# 2. SSO 登录（打开浏览器）
wiresocket login --sso azure-ad

# 这会：
# - 打开浏览器到 SSO 登录页
# - 启动本地 HTTP 服务器等待回调
# - 接收 token 并保存
```

---

## 用户管理

### JIT Provisioning（即时用户创建）

首次 SSO 登录时，系统自动创建本地用户记录：

```text
SSO 登录 → 用户不存在 → 自动创建用户 → 分配 VPN IP → 签发 JWT
```

**自动创建的用户**：

```json
{
  "username": "john@company.com",   // 来自 SSO
  "email": "john@company.com",       // 来自 SSO
  "is_admin": false,                 // 根据 admin_claim 判断
  "auth_provider": "azure-ad",       // 记录来源
  "password_hash": null,             // SSO 用户无密码
  "is_active": true
}
```

### 数据库扩展

```sql
-- 扩展 users 表
ALTER TABLE users ADD COLUMN auth_provider TEXT DEFAULT 'local';
ALTER TABLE users ADD COLUMN external_id TEXT;  -- IdP 中的用户 ID

-- 索引
CREATE INDEX idx_users_auth_provider ON users(auth_provider);
CREATE UNIQUE INDEX idx_users_external_id ON users(auth_provider, external_id);
```

### Go 结构体扩展

```go
// server/internal/database/db.go

type User struct {
    ID           uint       `gorm:"primaryKey" json:"id"`
    Username     string     `gorm:"unique;not null" json:"username"`
    Email        string     `gorm:"unique" json:"email"`
    PasswordHash string     `json:"-"`
    IsActive     bool       `gorm:"default:true" json:"is_active"`
    IsAdmin      bool       `gorm:"default:false" json:"is_admin"`

    // SSO 扩展字段
    AuthProvider string     `gorm:"default:local" json:"auth_provider"`  // local, azure-ad, github, etc.
    ExternalID   string     `json:"-"`                                   // IdP 中的用户 ID

    CreatedAt    time.Time  `json:"created_at"`
    UpdatedAt    time.Time  `json:"updated_at"`
    LastLogin    *time.Time `json:"last_login,omitempty"`
}
```

### 用户管理策略

| 场景 | 行为 |
|------|------|
| SSO 用户首次登录 | 自动创建用户，分配 IP |
| SSO 用户再次登录 | 更新 last_login，复用 IP |
| SSO 用户被 IdP 禁用 | 下次登录失败（IdP 拒绝） |
| 管理员禁用 SSO 用户 | is_active=false，拒绝登录 |
| SSO 用户尝试本地登录 | 拒绝（无密码） |
| 本地用户尝试 SSO 登录 | 创建新的 SSO 用户（不同 auth_provider） |

---

## 实现计划

### Phase 1: 核心框架

| 任务 | 文件 | 说明 |
|------|------|------|
| Provider 接口 | `internal/auth/provider.go` | AuthProvider 接口定义 |
| Local Provider | `internal/auth/local.go` | 重构现有本地认证 |
| Auth Manager | `internal/auth/manager.go` | 管理多个 Provider |
| 配置解析 | `internal/config/sso.go` | SSO 配置结构 |
| DB 扩展 | `internal/database/db.go` | User 模型扩展 |

### Phase 2: OIDC 实现

| 任务 | 文件 | 说明 |
|------|------|------|
| OIDC Provider | `internal/auth/oidc.go` | OIDC 认证实现 |
| PKCE 支持 | `internal/auth/pkce.go` | Code Challenge 生成 |
| Token 验证 | `internal/auth/token.go` | ID Token 解析验证 |
| API 路由 | `internal/api/auth_sso.go` | SSO 相关 API |

### Phase 3: OAuth 2.0 实现

| 任务 | 文件 | 说明 |
|------|------|------|
| OAuth2 Provider | `internal/auth/oauth2.go` | 通用 OAuth 2.0 实现 |
| GitHub 扩展 | `internal/auth/github.go` | GitHub 特定逻辑（组织检查） |
| 用户信息获取 | `internal/auth/userinfo.go` | 通用 userinfo 处理 |

### Phase 4: 客户端支持

| 任务 | 文件 | 说明 |
|------|------|------|
| Electron SSO | `client/frontend/src/main/sso.js` | 桌面端 SSO 流程 |
| Deep Link | `client/frontend/src/main/protocol.js` | 协议处理 |
| CLI 支持 | `cmd/wsctl/login.go` | CLI SSO 登录 |

### 依赖库

```go
// go.mod 新增
require (
    github.com/coreos/go-oidc/v3 v3.9.0  // OIDC 客户端
    golang.org/x/oauth2 v0.15.0          // OAuth 2.0 客户端
)
```

---

## 安全考虑

### PKCE（Proof Key for Code Exchange）

所有 OAuth 流程强制使用 PKCE，防止授权码拦截攻击：

```text
1. 生成 code_verifier (随机字符串)
2. 计算 code_challenge = SHA256(code_verifier)
3. 授权请求带 code_challenge
4. Token 请求带 code_verifier
5. IdP 验证 SHA256(code_verifier) == code_challenge
```

### State 参数

防止 CSRF 攻击：

```text
1. 生成随机 state，存入 session/cookie
2. 授权请求带 state
3. 回调时验证 state 匹配
```

### Token 安全

- SSO 获取的 access_token 仅用于获取用户信息，不存储
- 只签发 WireSocket 自己的 JWT
- JWT 有效期与现有保持一致

### 域名限制

支持限制只允许特定域名的用户：

```yaml
providers:
  - id: "google"
    allowed_domains: ["company.com"]  # 只允许公司邮箱
```

---

## 兼容性说明

### 向后兼容

- 不启用 SSO 时，行为与现有完全一致
- 本地用户认证 API 不变
- 现有客户端无需修改即可使用本地登录

### Mesh 兼容

- SSO 只在 Gateway 节点配置和处理
- Exit 节点无需配置 SSO
- JWT 在 Mesh 网络内通用

---

## 常见 IdP 配置示例

### Azure AD

1. 在 Azure Portal 注册应用
2. 配置重定向 URI: `https://vpn.example.com/api/auth/callback/azure-ad`
3. 获取 Application ID 和 Client Secret

```yaml
- id: "azure-ad"
  type: "oidc"
  name: "Microsoft"
  issuer: "https://login.microsoftonline.com/{tenant-id}/v2.0"
  client_id: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  client_secret: "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
  scopes: ["openid", "profile", "email"]
  mapping:
    username: "preferred_username"
    email: "email"
```

### Okta

1. 在 Okta Admin 创建 OIDC 应用
2. 配置 Sign-in redirect URI

```yaml
- id: "okta"
  type: "oidc"
  name: "Okta"
  issuer: "https://your-org.okta.com"
  client_id: "xxx"
  client_secret: "xxx"
  scopes: ["openid", "profile", "email", "groups"]
  mapping:
    username: "email"
    admin_claim: "groups"
    admin_values: ["VPN Admins"]
```

### GitHub

1. 在 GitHub Settings > Developer settings 创建 OAuth App
2. 配置 Authorization callback URL

```yaml
- id: "github"
  type: "oauth2"
  name: "GitHub"
  authorize_url: "https://github.com/login/oauth/authorize"
  token_url: "https://github.com/login/oauth/access_token"
  userinfo_url: "https://api.github.com/user"
  client_id: "xxx"
  client_secret: "xxx"
  scopes: ["user:email", "read:org"]
  mapping:
    username: "login"
    email: "email"
  allowed_orgs: ["your-org"]
  orgs_url: "https://api.github.com/user/orgs"
```
