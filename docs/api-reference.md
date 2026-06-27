# API 参考

所有 API 基础路径为 `/api`。启用认证后，核心 API 需要 `Authorization: Bearer <token>` 或 `X-API-Key: <key>` 请求头。

## 健康检查

| 方法 | 路径 | 说明 | 认证 |
| --- | --- | --- | --- |
| GET | `/api/health` | 服务健康状态 | 否 |
| GET | `/api/metrics` | 基础指标 | 否 |

## 认证

| 方法 | 路径 | 说明 | 认证 |
| --- | --- | --- | --- |
| POST | `/api/auth/login` | 登录，返回 JWT | 否 |
| GET | `/api/auth/me` | 当前用户信息 | JWT |
| POST | `/api/auth/change-password` | 修改密码 | JWT |

### POST /api/auth/login

请求体：

```json
{
  "username": "admin",
  "password": "admin"
}
```

响应：

```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "user": { "id": 1, "username": "admin", "role": "admin" }
}
```

## 仓库

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/repositories` | 仓库列表 |
| POST | `/api/repositories` | 创建仓库 |
| GET | `/api/repositories/:id` | 仓库详情 |
| PATCH | `/api/repositories/:id` | 更新仓库 |
| DELETE | `/api/repositories/:id` | 删除仓库 |
| POST | `/api/repositories/:id/check` | 检查最新 Release |
| POST | `/api/repositories/:id/check-all` | 全量检查所有 Release |
| POST | `/api/repositories/:id/sync` | 同步最新 Release（检查 + 下载） |
| POST | `/api/repositories/:id/sync-tag` | 同步指定 Tag 的 Release |
| GET | `/api/repositories/:id/releases` | 仓库的 Release 列表 |
| GET | `/api/repositories/:id/remote-tags` | 远程仓库的 Tag 列表 |

### 仓库创建/更新字段

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| provider | string | 否 | 默认 `github`，可选 `gitlab`/`gitea`/`forgejo` |
| owner | string | 是 | 仓库所有者 |
| repo | string | 是 | 仓库名 |
| enabled | bool | 否 | 是否启用 |
| githubTokenId | number/null | 否 | 关联的 Token ID |
| storageId | number/null | 否 | 存储目标 ID |
| proxyId | number/null | 否 | 代理 ID |
| providerApiBaseUrl | string | 否 | 自托管实例地址 |
| intervalSeconds | number | 否 | 同步间隔，默认 1800 |
| filterMode | string | 否 | `glob` 或 `regex` |
| assetIncludePatterns | string | 否 | 包含规则 |
| assetExcludePatterns | string | 否 | 排除规则 |
| retentionKeepLatest | number | 否 | 保留最近 N 版，默认 5 |

## Release

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/releases/:id` | Release 详情 |
| DELETE | `/api/releases/:id` | 删除 Release |
| POST | `/api/releases/:id/pin` | 置顶 Release |
| POST | `/api/releases/:id/unpin` | 取消置顶 |
| GET | `/api/releases/:id/assets` | Release 的资产列表 |

## Asset

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/assets/:id/download` | 下载资产 |
| POST | `/api/assets/:id/redownload` | 重新下载资产 |
| DELETE | `/api/assets/:id` | 删除资产 |
| GET | `/api/assets/:id/file` | 获取资产文件（直接下载） |
| POST | `/api/assets/upload` | 手动上传资产 |

### 资产状态

| 状态 | 说明 |
| --- | --- |
| pending | 待下载 |
| skipped | 被过滤规则跳过 |
| downloading | 下载中 |
| downloaded | 已下载 |
| verified | 已校验 |
| failed | 下载失败 |
| deleted | 已删除 |

## 任务

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/tasks` | 任务列表 |
| GET | `/api/tasks/:id` | 任务详情 |
| GET | `/api/tasks/:id/logs` | 任务日志 |

### 任务状态

| 状态 | 说明 |
| --- | --- |
| pending | 待执行 |
| running | 执行中 |
| succeeded | 成功 |
| failed | 失败 |
| canceled | 已取消 |

## 文件

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/files` | 文件列表 |
| GET | `/api/files/tree` | 文件树 |
| GET | `/api/files/download?assetId=:id` | 下载文件 |

## GitHub Token

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/tokens` | Token 列表 |
| POST | `/api/tokens` | 创建 Token |
| GET | `/api/tokens/:id` | Token 详情 |
| DELETE | `/api/tokens/:id` | 删除 Token |
| GET | `/api/tokens/:id/health` | Token 健康检查 |
| GET | `/api/tokens/:id/rate-limit` | Token Rate Limit 信息 |

## 存储

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/storages` | 存储列表 |
| POST | `/api/storages` | 创建存储 |
| GET | `/api/storages/:id` | 存储详情 |
| PATCH | `/api/storages/:id` | 更新存储 |
| DELETE | `/api/storages/:id` | 删除存储 |
| POST | `/api/storages/:id/test` | 测试存储连接 |

### 存储类型

- `local`：本地文件系统
- `s3`：S3 兼容存储
- `webdav`：WebDAV 存储

## 代理

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/proxies` | 代理列表 |
| POST | `/api/proxies` | 创建代理 |
| GET | `/api/proxies/:id` | 代理详情 |
| PATCH | `/api/proxies/:id` | 更新代理 |
| DELETE | `/api/proxies/:id` | 删除代理 |
| POST | `/api/proxies/:id/test` | 测试代理连通性 |

### 代理类型

- `http`：HTTP 代理
- `https`：HTTPS 代理
- `socks5`：SOCKS5 代理

## 通知

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/notifications` | 通知列表 |
| POST | `/api/notifications` | 创建通知 |
| GET | `/api/notifications/:id` | 通知详情 |
| PATCH | `/api/notifications/:id` | 更新通知 |
| DELETE | `/api/notifications/:id` | 删除通知 |
| POST | `/api/notifications/:id/test` | 发送测试通知 |

### 通知类型

- `gotify`：Gotify 推送
- `webhook`：Webhook 回调
- `email`：邮件通知
- `telegram`：Telegram Bot

### 事件类型

- `new_release`：发现新版本
- `download_ok`：下载成功
- `download_err`：下载失败
- `sync_success`：同步完成
- `sync_failed`：同步失败

## API Key

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/apikeys` | API Key 列表 |
| POST | `/api/apikeys` | 创建 API Key |
| DELETE | `/api/apikeys/:id` | 删除 API Key |

### 创建 API Key 请求体

```json
{
  "name": "ci-pipeline",
  "scope": "repo:read,asset:download"
}
```

响应会一次性返回完整的 Key 值，后续不再展示。

## 用户（管理员）

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/users` | 用户列表 |
| POST | `/api/users` | 创建用户 |
| PATCH | `/api/users/:id` | 更新用户 |
| DELETE | `/api/users/:id` | 删除用户 |

### 用户角色

- `admin`：全部权限
- `operator`：可读写，不可管理基础设施
- `viewer`：只读

## 搜索

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/search?q=keyword` | 全文搜索 |

## 统计

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/stats/dashboard` | Dashboard 统计数据 |
| GET | `/api/stats/trend` | 趋势时间序列 |

## 过滤预览

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/filter/preview` | 预览过滤规则匹配结果 |

请求体：

```json
{
  "owner": "gohugoio",
  "repo": "hugo",
  "filterMode": "glob",
  "includePatterns": "*.tar.gz",
  "excludePatterns": "*.sha256"
}
```

## 上传

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/assets/upload` | 手动上传资产 |

`multipart/form-data` 表单，字段包括文件、仓库 ID、Tag 等。

## 对账

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/reconcile` | 存储与数据库对账 |

返回存储中缺失的文件、数据库中缺失的记录和孤立资产。

## 配置

| 方法 | 路径 | 说明 | 认证 |
| --- | --- | --- | --- |
| GET | `/api/config` | 当前配置（含 authEnabled） | 否 |
| PUT | `/api/config` | 更新运行时配置 | 是 |

### 可更新的配置项

```json
{
  "schedulerEnabled": true,
  "schedulerTickSeconds": 120,
  "schedulerMaxConcurrent": 3,
  "githubApiBaseUrl": "https://api.github.com"
}
```

## 通用响应格式

成功：

```json
{ "data": ... }
```

错误：

```json
{ "error": "错误描述" }
```

列表接口支持分页参数 `page` 和 `pageSize`（默认 20）。
