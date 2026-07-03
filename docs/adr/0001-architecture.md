# ADR 0001：ReleaseHub 第一版架构

## 状态

Accepted

## 背景

ReleaseHub 的目标是长期管理多个 GitHub 仓库的 Release Assets，同步流程包括仓库配置、Release 检查、资产过滤、下载、校验、存储、清理与通知。

项目需要先达到本地稳定运行，再逐步扩展到多存储、多通知、多 Provider 和平台化能力。

## 决策

第一版采用以下架构：

- 后端使用 Go + Gin。
- 数据层使用 GORM，默认 SQLite，后续兼容 PostgreSQL。
- 前端使用 Vue 3 + TypeScript + Vite + Naive UI。
- 默认部署使用 Docker Compose。
- 后端业务模块放在 `backend/internal/`，避免未稳定内部 API 被外部依赖。
- MVP 默认使用 Local Storage 和原生 HTTP 下载，保留 Storage Driver、Downloader、Release Provider、Notifier 的接口边界。

## 原因

- SQLite 降低自托管部署门槛，适合 NAS 和个人服务器。
- Go 单二进制便于部署，后台任务、下载和 API 服务可以先在单进程内稳定运行。
- Vue + Naive UI 适合管理后台，能够快速交付高信息密度界面。
- 驱动化边界能减少后续接入 S3/WebDAV/OpenList/GitLab/Gitea 时对核心流程的破坏。

## 后果

- 第一版会优先保证单机可靠性，不提前引入 Redis、aria2 或复杂 RBAC。
- SQLite 写并发能力有限，需要通过任务队列和 Repository 级锁控制冲突。
- 后续要支持 PostgreSQL 时，迁移和查询必须避免 SQLite 专属行为。

## 后续

- ADR 0002：任务状态机与调度策略。📋 待编写
- ADR 0003：Storage Driver 契约。📋 待编写
- ADR 0004：Release Provider 契约。📋 待编写
- ADR 0005：Token 与 Secret 管理。📋 待编写
- ADR 0006：通知系统架构。📋 待编写
- ADR 0007：认证与授权。📋 待编写

> 截至 v1.0，上述 ADR 尚未正式编号编写，相关设计散见于代码注释与本文档。
