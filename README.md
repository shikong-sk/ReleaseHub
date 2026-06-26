# ReleaseHub

ReleaseHub 是面向 GitHub Releases 的 Artifact 管理平台，目标是长期自动同步几十到几百个仓库的 Release Assets，并提供接近 Sonarr/Radarr 的 Web 管理体验。

## 技术栈

- 后端：Go、Gin、GORM、SQLite、Viper、Zap
- 前端：Vue 3、TypeScript、Vite、Pinia、Vue Router、Naive UI
- 部署：Docker Compose，默认使用 SQLite 与本地数据目录

## 本地开发

### 后端

```bash
cd backend
go mod tidy
go test ./...
go run ./cmd/releasehub
```

默认 API 地址：

```text
http://localhost:8080/api/health
```

### 前端

```bash
cd frontend
npm install
npm run typecheck
npm run build
npm run dev
```

默认前端地址：

```text
http://localhost:5173
```

Vite 开发服务会把 `/api` 代理到 `http://localhost:8080`。

## Docker Compose

```bash
docker compose -f docker/compose.sqlite.yml up --build
```

默认访问：

```text
http://localhost:8088
```

默认数据目录：

```text
./data
```

## 环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `RELEASEHUB_APP_ENV` | `development` | 运行环境 |
| `RELEASEHUB_HTTP_HOST` | `0.0.0.0` | API 监听地址 |
| `RELEASEHUB_HTTP_PORT` | `8080` | API 监听端口 |
| `RELEASEHUB_DATABASE_DRIVER` | `sqlite` | 数据库类型 |
| `RELEASEHUB_DATABASE_DSN` | `data/releasehub.db` | SQLite 数据库路径 |
| `RELEASEHUB_STORAGE_DATA_DIR` | `data/releases` | 本地资产存储目录 |
| `RELEASEHUB_GITHUB_API_BASE_URL` | `https://api.github.com` | GitHub API 地址 |
| `RELEASEHUB_SCHEDULER_ENABLED` | `true` | 是否启用定时检查 |
| `RELEASEHUB_SCHEDULER_TICK_SECONDS` | `60` | Scheduler 扫描间隔，最小 10 秒 |
| `RELEASEHUB_SCHEDULER_MAX_CONCURRENT` | `5` | Scheduler 最大并发同步数 |

## 当前 API

```text
GET /api/health
GET /api/repositories
POST /api/repositories
GET /api/repositories/:id
PATCH /api/repositories/:id
DELETE /api/repositories/:id
POST /api/repositories/:id/check
POST /api/repositories/:id/check-all
POST /api/repositories/:id/sync
GET /api/repositories/:id/releases
GET /api/releases/:id
GET /api/releases/:id/assets
POST /api/assets/:id/download
POST /api/assets/:id/redownload
DELETE /api/assets/:id
GET /api/assets/:id/file
GET /api/tasks
GET /api/tasks/:id
GET /api/files
GET /api/files/download?assetId=:id
GET /api/tokens
POST /api/tokens
GET /api/tokens/:id
DELETE /api/tokens/:id
GET /api/config
```

## 功能

- 添加/编辑/删除 GitHub 仓库，支持关联 GitHub Token
- 手动检查最新 Release、全量检查所有 Release
- 手动同步（检查 + 下载）
- Asset glob/regex 过滤
- SHA256 校验
- 本地存储：`github/owner/repo/tag/` 目录结构 + `latest` 映射
- 保留策略：保留最近 N 个版本，自动清理旧版本
- 失败资产重试
- Scheduler 定时同步，全局并发控制
- Web 浏览、搜索、下载已同步文件
- GitHub Token 管理（创建/删除，Token 值不暴露）
- 任务状态与错误信息可查询

## 路线图

完整规划见 [docs/DEVELOPMENT_PLAN.md](docs/DEVELOPMENT_PLAN.md)。

| 版本 | 功能 |
| --- | --- |
| v0.1 MVP | 仓库管理、Release 检查与全量拉取、资产过滤与下载、本地存储、SHA256、保留策略、Scheduler、Web 管理、Docker 部署 |
| v0.2 | 多存储（S3/WebDAV）、代理、通知（Gotify/Webhook） |
| v0.3 | 过滤增强、Token 健康、认证、日志队列 |
| v0.4 | 流式下载、断点续传、aria2 RPC、SHA256 远程比对 |
| v0.5 | 双向同步、搜索、ECharts 统计 |
| v0.6 | GitLab/Gitea/Forgejo Provider |
| v1.0 | 插件系统、RBAC、API Key、Prometheus、OpenAPI |
