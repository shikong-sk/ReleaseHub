# 架构设计

本文描述 ReleaseHub 的系统架构、核心模块和设计决策。

## 系统架构概览

ReleaseHub 采用经典的前后端分离架构：

- **后端**：单进程 Go 应用，内嵌 HTTP API、Scheduler 和后台任务执行
- **前端**：Vue 3 SPA，Nginx 反向代理
- **数据层**：SQLite / PostgreSQL / MySQL（GORM AutoMigrate），本地文件系统 / S3 / WebDAV 存储

```
┌─────────────┐     ┌─────────────────────────────────────┐
│   Browser    │────▶│  Nginx (反向代理)                    │
└─────────────┘     │   ├── / → Vue SPA                    │
                    │   └── /api → Go Backend              │
                    └──────────────┬──────────────────────┘
                                   │
                    ┌──────────────▼──────────────────────┐
                    │  Go Backend (单进程)                  │
                    │  ┌─────────────────────────────────┐ │
                    │  │ Gin HTTP Server                  │ │
                    │  │  ├── 公开路由（health, login）    │ │
                    │  │  └── 认证路由（JWT/APIKey + RBAC）│ │
                    │  ├─────────────────────────────────┤ │
                    │  │ 业务服务                          │ │
                    │  │  ├── Repository Service           │ │
                    │  │  ├── Release Checker              │ │
                    │  │  ├── Asset Service（下载/打开/删除）│ │
                    │  │  ├── Syncer（编排检查+下载）       │ │
                    │  │  ├── Scheduler（定时调度）         │ │
                    │  │  ├── Retention Service            │ │
                    │  │  ├── Notify Service               │ │
                    │  │  ├── Filter Service               │ │
                    │  │  └── TaskLog Service               │ │
                    │  ├─────────────────────────────────┤ │
                    │  │ 驱动/接口                          │ │
                    │  │  ├── Storage Driver               │ │
                    │  │  │   ├── LocalStorage             │ │
                    │  │  │   ├── S3Storage                 │ │
                    │  │  │   └── WebDAVStorage             │ │
                    │  │  ├── Release Provider              │ │
                    │  │  │   ├── GitHub                    │ │
                    │  │  │   ├── GitLab                    │ │
                    │  │  │   ├── Gitea                     │ │
                    │  │  │   └── Forgejo                   │ │
                    │  │  ├── Notifier                      │ │
                    │  │  │   ├── Gotify                    │ │
                    │  │  │   ├── Webhook                   │ │
                    │  │  │   ├── Email                     │ │
                    │  │  │   └── Telegram                  │ │
                    │  │  └── Proxy Factory                 │ │
                    │  ├─────────────────────────────────┤ │
                    │  │ 数据层                            │ │
                    │  │  ├── GORM（SQLite）               │ │
                    │  │  └── Storage Driver Factory       │ │
                    │  └─────────────────────────────────┘ │
                    └─────────────────────────────────────┘
```

## 核心数据模型

```
User ───────────────────────────────────────┐
APIKey ──── UserID (nullable)               │
                                            │
GitHubToken                                 │
                                            │
Storage ──── isDefault                      │
                                            │
Proxy                                       │
                                            │
Notification ──── events                    │
                                            │
Repository ──── Provider + Owner + Repo (UK)  │
            ├── GitHubTokenID (FK)          │
            ├── StorageID (FK，主存储)       │
            ├── RepositoryStorage (多对多)   │ —— 一个仓库可关联多个存储，资产分发到每个存储
            └── ProxyID (FK)                │
                                            │
Release ──── RepositoryID + Tag (UK)        │
         ├── IsLatest                       │
         ├── IsPinned                       │
         └── SyncStatus                    │
                                            │
Asset ──── ReleaseID + Name (UK)           │
        ├── Status                         │
        └── SHA256                          │
                                            │
Task ──── Type + Status                     │
      ├── RepositoryID                      │
      ├── ReleaseID                         │
      ├── AssetID                           │
      ├── Attempt / MaxAttempts              │
      └── ScheduledAt                       │
                                            │
TaskLog ──── TaskID + Timestamp            │
```

## 核心业务流程

### 同步流程

```
Scheduler / 手动触发
    │
    ▼
Syncer.SyncRepository(repo)
    │
    ├──▶ Release Checker
    │      │
    │      ├── Provider.ListReleases / GetLatestRelease
    │      ├── 过滤：已有 Release 跳过
    │      ├── 持久化新 Release
    │      ├── 通知：new_release
    │      └── 更新 repo.last_check_at / last_status
    │
    └──▶ Asset Downloader
           │
           ├── 遍历 Release 的 Assets
           ├── Filter.Matcher 过滤
           ├── 已存在 / 已跳过 → 跳过
           ├── 下载：Downloader.For(repo) → HTTP 直连 / aria2 离线 → StorageDriver.Put
           ├── SHA256 校验
           ├── 通知：download_ok / download_err
           └── Retention Service 清理旧版本
```

### Release 查询的降级策略（HTML Fallback）

Release Checker 默认通过 Provider 调 API。当 API 不可用时按下面的顺序降级：

```
resolveProvider（API）
    │
    ├── 成功 → 使用 API 数据
    │
    └── 失败且错误属于「限流(403/429) / 资源不存在或无权限(404)」
           │
           ├── 仓库已配置 Token 或非 GitHub → 直接失败（私有仓库/其他平台必须走 API）
           │
           └── 未配置 Token 的 GitHub 公开仓库
                  ├── /{owner}/{repo}/releases.atom                       取最新 tag
                  ├── /{owner}/{repo}/releases/expanded_assets/{tag}      取全部资产
                  └── 限流命中后进入 30 分钟冷却期，期间直接走网页抓取
```

实现分布：
- `backend/internal/services/github/htmlfallback.go`：网页端点抓取与 HTML 解析（`HTMLFetcher`、`ParseExpandedAssets`）
- `backend/internal/services/provider/github_html.go`：把抓取结果适配为 `ReleaseProvider`（`ListAllReleases` 明确不支持）
- `backend/internal/services/release/checker.go`：`releaseProviderFor` / `htmlFallbackProvider` 决定是否降级

关键约束：网页端点固定只返回最近 10 个 Release，因此**全量同步（CheckAll）永远走 API**；
私有仓库网页端点不接受 PAT 鉴权，同样只能走 API。

### 下载子系统和 Downloader 接口

`Asset Downloader` 通过统一的 `downloader.Downloader` 接口抽象，按仓库配置选择实现：

- `HTTPDownloader`：HTTP 直连下载，受 `download.max_speed_bytes` 全局限速（基于 `golang.org/x/time/rate` token bucket），支持仓库级代理。
- `Aria2Downloader`：通过 aria2 JSON-RPC 提交离线下载任务，轮询完成后从本地完成目录读出文件（`os.Open`），不受全局限速与仓库级代理作用。

选择逻辑位于 `asset.Service.downloaderForRepository`：当 `download.aria2_rpc` 非空时返回 `Aria2Downloader`，否则返回 `HTTPDownloader`。Syncer 自动同步与手动下载任务共用同一选择路径，确保两者语义一致。

`Aria2Downloader` 采用惰性缓存：按 `aria2DownloaderKey{rpc,secret,http,dir}` 缓存单个实例，任一 aria2 配置变更后下一次下载即缓存未命中并重建，无需显式调用 setter。其余热更新字段（`max_speed_bytes`、`authEnabled`、`githubApiBaseUrl`、`*RetentionDays`）通过共享的 `*config.Config` 指针读取，消费侧下次访问即生效；scheduler 与 syncer 并发字段则通过显式 setter 重建 ticker / 信号量。

### 下载 User-Agent

两种下载器与网页降级抓取统一使用 `config.Config.DownloadUserAgent()` 解析出的 UA：

- `HTTPDownloader`：`userAgent func() string` 字段，请求前求值（`resolveUserAgent`），空值回退 `ReleaseHub`。
- `Aria2Downloader`：下载时以 aria2 `user-agent` 选项下发（非 header），空值不下发以保留 daemon 默认。
- `HTMLFetcher`：注入同一 provider，空值回退内置 `ReleaseHub-HTMLFallback`。

三者都用函数而非字符串注入，因此配置热更新后下一次请求即生效，无需重建实例；GitHub API 与各 Provider 请求保持固定 `ReleaseHub` 标识，不受该配置影响。

仓库级覆盖：`Repository.downloadUserAgentPreset` 非空（且不是空值的 `custom`）时优先于全局配置，解析函数为 `asset.Service.userAgentForRepository` 与 `release.CheckService.newHTMLProvider` 内的同名逻辑。`preset=custom` 但 UA 为空时仍回退全局，保证「留空即跟随全局」单一语义。为避免仓库级 UA 造成 Transport 泛滥，`asset.Service` 以 UA 字符串为键缓存 `HTTPDownloader`（`uaDownloaders`），与全局 UA 相同的仓库直接复用共享 `downloader`；aria2 下载器的缓存键额外包含 UA，避免不同 UA 复用同一实例。

取消语义对两种下载器对称：`downloadWithAttempt` 为每次尝试构造 `context.WithCancel`，`cancelFn` 注册到 `cancelMap[taskID]`；`CancelDownloadTask` 触发取消后，HTTP 下载经 `http.Client` 中止，aria2 下载经轮询 `ctx.Done` 并调用 aria2 `Remove` 回收任务，复制阶段的 `context.Canceled` 被包装后由 `errors.Is(downloadErr, context.Canceled)` 统一识别。

### 存储驱动选择

每个仓库可关联一个或多个存储目标（通过 `RepositoryStorage` 多对多关系）。存储驱动选择逻辑：

1. 如果仓库关联了多个存储（`storageIds` 非空），资产会分发写入到每个存储目标
2. 如果仓库配置了单个 `storageId`，使用该存储
3. 如果存在标记 `isDefault` 的 Storage，使用默认存储
4. 回退到本地存储（`storage.data_dir`）

启动时会自动创建默认本地存储并回填存量资产的 `storageId`（幂等迁移）。

### 代理选择

每个仓库可指定代理。代理选择逻辑：

1. 如果仓库配置了 `proxyId`，查找对应 Proxy 记录创建 transport
2. 否则使用直连

### 认证与权限

```
请求 → APIKeyOrAuth 中间件
         │
         ├── X-API-Key 头 → 查询 API Key 表 → 验证启用状态
         │
         └── Authorization: Bearer → 验证 JWT → 注入 userID/role

      → AuthorizeRequest 中间件
         │
         ├── 根据 route resource + HTTP method 映射权限
         ├── API Key: 按 scope 判断
         └── JWT: 按 role 矩阵判断
              ├── admin: 全部通过
              ├── operator: 写操作通过，管理资源拒绝
              └── viewer: 仅 GET 通过
```

## 目录结构设计原则

后端采用 `internal/` 布局，防止外部包依赖未稳定的内部 API：

- `api/`：HTTP handler，只做参数绑定和响应序列化
- `services/`：业务逻辑，按领域拆分子包
- `models/`：GORM 模型定义
- `middleware/`：Gin 中间件
- `config/`：配置加载

驱动（Storage / Provider / Notifier）通过接口抽象，新驱动只需实现接口并注册到工厂/注册表，不修改核心流程。

## 设计决策

详细的架构决策记录见 [docs/adr/](adr/)。

### 为什么默认用 SQLite

- 降低自托管部署门槛，适合 NAS 和个人服务器
- 单文件便于备份和迁移
- 写并发能力有限，通过任务队列和仓库级锁控制冲突
- v1.0 已支持 PostgreSQL 和 MySQL，高并发或规模化场景可选择切换

### 为什么单进程

- MVP 阶段不需要分布式调度
- Go 的并发模型足够处理几百个仓库的同步
- 部署简单，单二进制 + 静态前端

### 为什么驱动接口化

- 后续接入 S3/WebDAV/OpenList/GitLab/Gitea 时只需增加实现
- 核心 Syncer/Retention/Notify 流程不因存储或平台变更而修改
- 便于独立测试（mock 驱动）

## 技术债与已知限制

| 项目 | 说明 | 状态 |
| --- | --- | --- |
| S3 签名 | 当前为简化实现，部分 S3 兼容存储可能不兼容 | 待优化 |
| WebDAV/S3 全量上传 | 内部读取完整内容再上传，大文件可能占用较多内存 | 待优化 |
| 通知发送为同步 fan-out | 未进入队列，暂无重试机制 | 待优化 |
| 无国际化（i18n） | 界面和文档目前仅有中文 | 待规划 |
| 多平台 Provider 完整集成 | Release Checker 仍部分依赖 GitHub 专用类型 | 待完善 |
| HTML 降级的数据精度 | 网页端点的资产大小是页面显示值（四舍五入），且无 prerelease/draft 标记 | 可接受 |

## 相关文档

- [架构决策记录 ADR 0001](adr/0001-architecture.md)
- [开发规划](DEVELOPMENT_PLAN.md)
- [功能审查清单](FUNCTIONAL_REVIEW_TODO.md)
