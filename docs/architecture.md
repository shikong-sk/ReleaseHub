# 架构设计

本文描述 ReleaseHub 的系统架构、核心模块和设计决策。

## 系统架构概览

ReleaseHub 采用经典的前后端分离架构：

- **后端**：单进程 Go 应用，内嵌 HTTP API、Scheduler 和后台任务执行
- **前端**：Vue 3 SPA，Nginx 反向代理
- **数据层**：SQLite（GORM AutoMigrate），本地文件系统 / S3 / WebDAV 存储

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
           ├── 下载：Provider → io.Pipe → StorageDriver.Put
           ├── SHA256 校验
           ├── 通知：download_ok / download_err
           └── Retention Service 清理旧版本
```

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

### 为什么用 SQLite

- 降低自托管部署门槛，适合 NAS 和个人服务器
- 单文件便于备份和迁移
- 写并发能力有限，通过任务队列和仓库级锁控制冲突

### 为什么单进程

- MVP 阶段不需要分布式调度
- Go 的并发模型足够处理几百个仓库的同步
- 部署简单，单二进制 + 静态前端

### 为什么驱动接口化

- 后续接入 S3/WebDAV/OpenList/GitLab/Gitea 时只需增加实现
- 核心 Syncer/Retention/Notify 流程不因存储或平台变更而修改
- 便于独立测试（mock 驱动）

## 技术债与已知限制

| 项目 | 说明 | 规划版本 |
| --- | --- | --- |
| SQLite 写并发 | 单写锁，高并发同步需排队 | v1.0（PostgreSQL） |
| S3 简化签名 | 当前为 HTTP Basic Auth，非 AWS V4 | v0.5 收尾 |
| WebDAV/S3 全量上传 | 内部读取完整内容再上传 | v0.5（分片） |
| 无 OpenAPI 文档 | API 较多但无自动生成文档 | v1.0 |
| 前端趋势图未集成 | 后端 API 已就绪，前端未调用 | v0.5 |
| aria2 可选后端 | 代码存在但未接入调度 | v0.6 |
| 软删除已迁移 | v0.4 已完成硬删除迁移，`DeletedAt` 字段已清理 | 已完成 |

## 相关文档

- [架构决策记录 ADR 0001](adr/0001-architecture.md)
- [开发规划](DEVELOPMENT_PLAN.md)
- [功能审查清单](FUNCTIONAL_REVIEW_TODO.md)
