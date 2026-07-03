# 配置参考

ReleaseHub 通过环境变量进行配置，所有变量以 `RELEASEHUB_` 为前缀，层级用 `_` 分隔。

## 应用

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `RELEASEHUB_APP_ENV` | `development` | 运行环境，`production` 时 Gin 进入 ReleaseMode |
| `RELEASEHUB_APP_JWT_SECRET` | `""` | JWT 签名密钥，启用认证时必须设置 |

## HTTP 服务

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `RELEASEHUB_HTTP_HOST` | `0.0.0.0` | API 监听地址 |
| `RELEASEHUB_HTTP_PORT` | `8080` | API 监听端口 |

## 数据库

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `RELEASEHUB_DATABASE_DRIVER` | `sqlite` | 数据库类型，支持 `sqlite`、`postgres` 和 `mysql` |
| `RELEASEHUB_DATABASE_DSN` | `data/releasehub.db` | 数据库路径（SQLite）或连接字符串（PostgreSQL/MySQL） |

数据库使用 GORM AutoMigrate 自动建表，无需手动执行迁移脚本。

## 存储

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `RELEASEHUB_STORAGE_DATA_DIR` | `data/releases` | 本地存储的根目录 |

除了本地存储外，还支持 S3 和 WebDAV 存储驱动，这些通过 Web 界面配置而非环境变量：

**S3 存储（兼容 MinIO）：**
- Endpoint：S3 服务端地址
- Bucket：桶名
- Region：区域
- AccessKey / SecretKey：认证信息
- BasePath：桶内路径前缀

**WebDAV 存储：**
- RemoteURL：WebDAV 服务地址
- Username / Password：认证信息
- BasePath：路径前缀

每个仓库可独立选择存储目标。未指定存储的仓库使用默认存储（标记为 `isDefault` 的存储配置，或本地存储）。

## GitHub API

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `RELEASEHUB_GITHUB_API_BASE_URL` | `https://api.github.com` | GitHub API 基础地址 |

自托管 GitHub Enterprise 时可修改此地址。


## 下载

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `RELEASEHUB_DOWNLOAD_MAX_SPEED_BYTES` | `0` | 下载速度限制（字节/秒），0 表示不限速 |
| `RELEASEHUB_DOWNLOAD_ARIA2_RPC` | `""` | aria2 JSON-RPC 端点，空则不使用 aria2 |
| `RELEASEHUB_DOWNLOAD_ARIA2_SECRET` | `""` | aria2 RPC 密钥 |
| `RELEASEHUB_DOWNLOAD_ARIA2_HTTP` | `""` | aria2 文件服务地址 |

## Scheduler

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `RELEASEHUB_SCHEDULER_ENABLED` | `true` | 是否启用定时扫描 |
| `RELEASEHUB_SCHEDULER_TICK_SECONDS` | `60` | 扫描间隔，最小 10 秒 |
| `RELEASEHUB_SCHEDULER_MAX_CONCURRENT` | `5` | 同时运行的检查/同步任务上限 |

Scheduler 在每个 tick 周期中：
1. 扫描所有 `enabled=true` 且到达同步间隔的仓库
2. 通过全局 semaphore 控制并发数
3. 依次执行检查 + 下载
4. 扫描失败资产并触发重试


## Syncer（同步器）

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `RELEASEHUB_SYNCER_MAX_CONCURRENT_TASKS` | `2` | 任务队列并发执行数（同时进行的仓库同步数） |
| `RELEASEHUB_SYNCER_MAX_CONCURRENT_DOWNLOADS` | `3` | 单任务内资产下载并发数 |

Syncer 配置支持运行时动态更新，在 Web 管理界面的「设置 → 全局配置」中调整，无需重启。

## 认证

认证开关支持运行时动态切换，在 Web 管理界面的「设置 → 全局配置」中开启或关闭，无需重启服务。

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `RELEASEHUB_AUTH_DEFAULT_ADMIN` | `admin` | 初始管理员用户名 |
| `RELEASEHUB_AUTH_DEFAULT_PASSWORD` | `admin` | 初始管理员密码 |

启用认证后：
- 所有核心 API 需要 JWT Token 或 API Key 认证
- `/api/health`、`/api/metrics`、`/api/auth/login`、`/api/config` 保持公开
- RBAC 角色矩阵：
  - **admin**：全部权限，含用户管理、存储/代理/通知配置
  - **operator**：可读写仓库/Release/资产，不可管理基础设施
  - **viewer**：只读访问所有资源

### API Key Scope

| Scope | 权限 |
| --- | --- |
| `*` | 全部权限 |
| `read` | 所有资源的读取权限 |
| `write` | 所有资源的写入权限 |
| `admin` | 包含 read + write + 管理资源 |
| `repo:read` / `repo:write` | 仓库的读/写 |
| `asset:download` | 资产下载 |
| `release:read` / `release:write` | Release 的读/写 |

## 仓库级配置

以下配置在仓库表单中设置，不属于环境变量：

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| Provider | `github` | Release 来源平台 |
| Provider API Base URL | 空（使用默认） | 自托管实例地址 |
| 同步间隔 | `1800`（30 分钟） | 该仓库的检查间隔（秒） |
| 过滤模式 | `glob` | `glob` 或 `regex` |
| 包含规则 | 空 | 匹配规则，逗号或换行分隔 |
| 排除规则 | 空 | 排除规则，逗号或换行分隔 |
| 保留最近 N 版 | `5` | 保留策略 |
| 关联 Token | 空 | 用于 API 认证的 GitHub Token |
| 存储目标 | 默认存储 | 资产存储位置 |
| 代理 | 空 | API 和下载请求使用的代理 |

## 运行时配置更新

以下配置可通过 `PUT /api/config` 在运行时更新，无需重启：

- `schedulerEnabled`
- `schedulerTickSeconds`
- `schedulerMaxConcurrent`
- `githubApiBaseUrl`

更新 scheduler 相关配置后会触发 Scheduler 热重载（tick 间隔和并发数即时生效）。

## Docker Compose 配置示例

```yaml
services:
  backend:
    environment:
      RELEASEHUB_APP_ENV: production
      RELEASEHUB_HTTP_HOST: 0.0.0.0
      RELEASEHUB_HTTP_PORT: 8080
      RELEASEHUB_DATABASE_DSN: /data/releasehub.db
      RELEASEHUB_STORAGE_DATA_DIR: /data/releases
      RELEASEHUB_APP_JWT_SECRET: your-secret-here
      RELEASEHUB_AUTH_DEFAULT_ADMIN: admin
      RELEASEHUB_AUTH_DEFAULT_PASSWORD: changeme
    volumes:
      - ./data:/data
```
