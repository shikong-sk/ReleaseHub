# ReleaseHub

ReleaseHub 是面向 GitHub Releases 的 Artifact 管理平台，目标是长期自动同步几十到几百个仓库的 Release Assets，并提供接近 Sonarr/Radarr 的 Web 管理体验。

当前状态：项目骨架与第一版本地运行闭环。已包含 Go API 服务、SQLite 初始化、健康检查、Vue 管理台壳、Docker 本地启动配置。

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
| `RELEASEHUB_GITHUB_API_BASE_URL` | `https://api.github.com` | GitHub API 地址，测试或代理场景可覆盖 |
| `RELEASEHUB_SCHEDULER_ENABLED` | `true` | 是否启用定时检查 |
| `RELEASEHUB_SCHEDULER_TICK_SECONDS` | `60` | Scheduler 扫描间隔，最小 10 秒 |

## 当前 API

```text
GET /api/health
GET /api/repositories
POST /api/repositories
GET /api/repositories/:id
PATCH /api/repositories/:id
DELETE /api/repositories/:id
POST /api/repositories/:id/check
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
```

当前已支持 API 与数据库健康检查、GitHub 仓库配置的增删改查、latest Release 检查、Release/Asset 入库与查询、按过滤规则同步资产、资产下载/重新下载/删除、本地存储 latest 映射、SHA256 计算、基础保留策略、本地文件读取、任务查询和文件浏览。

