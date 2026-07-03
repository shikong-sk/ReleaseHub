# ReleaseHub

ReleaseHub 是面向 GitHub / GitLab / Gitea / Forgejo Releases 的 Artifact 管理平台，自动同步多个仓库的 Release Assets，提供类似 Sonarr/Radarr 的 Web 管理体验。

## 功能概览

**仓库管理**
- 多平台 Provider 支持：GitHub、GitLab、Gitea、Forgejo（含自托管实例）
- 仓库 CRUD、启停控制、定时同步间隔配置
- Provider API Base URL 可配置（自托管 GitLab/Gitea/Forgejo）

**Release 同步**
- 检查最新 Release 或全量拉取所有 Release
- 手动触发同步（检查 + 下载）
- Asset Glob/Regex 过滤、按文件大小过滤
- SHA256 校验
- 流式下载，不缓存完整文件到内存
- 失败资产自动重试（指数退避）

**存储**
- 本地存储：`provider/owner/repo/tag/` 目录结构 + `latest` 映射
- S3 存储（兼容 MinIO 等 S3 协议存储）
- WebDAV 存储
- 多存储支持：每个仓库可关联多个存储目标，资产会分发到所选的每个存储
- 启动时自动创建默认本地存储并回填存量资产的 `storageId`

**保留与清理**
- 保留最近 N 个版本，自动清理旧版本
- Retention 按仓库选择存储驱动

**代理**
- HTTP/HTTPS/SOCKS5 代理支持
- 代理连通性测试与延迟展示
- 每个仓库可独立配置代理

**通知**
- Gotify / Webhook / Email / Telegram 通知
- 按事件类型订阅：下载完成、下载失败、发现新版本、同步完成/失败
- 通知渠道可启用/禁用

**认证与权限**
- JWT 认证，支持运行时动态启停（设置页面 → 全局配置）
- 默认管理员账号
- RBAC：admin / operator / viewer 三级角色
- API Key 认证，支持 scope 权限控制（`*` / `read` / `write` / `admin` / 细粒度 scope）
- 修改密码

**任务与日志**
- 任务状态查询（pending / running / succeeded / failed / canceled）
- 结构化任务日志（TaskLog）
- 任务重试与退避

**文件管理**
- Web 浏览、搜索、下载已同步文件
- 文件树懒加载浏览（后端分层 + 前端 NTree）
- 存储对账（Reconcile）：默认 dry-run 安全预检，可双向检测存储与数据库的不一致并支持安全修复模式
- 手动上传资产
- 文件删除

**Dashboard**
- 仓库数、启用数、文件数、失败任务数概览
- 失败任务告警
- 统计趋势数据（API 已就绪）

**Scheduler**
- 定时扫描，全局并发控制
- 可配置扫描间隔与最大并发数

## 技术栈

| 层 | 技术 |
| --- | --- |
| 后端 | Go 1.25、Gin、GORM、SQLite、Viper、Zap |
| 前端 | Vue 3、TypeScript、Vite、Pinia、Vue Router、Naive UI |
| 部署 | Docker Compose、Nginx 反向代理 |

## 快速开始

### Docker 部署（推荐）

使用预构建镜像一键部署：

```bash
# 拉取镜像
docker pull ghcr.io/shikong-sk/releasehub:latest

# 启动
docker run -d \
  --name releasehub \
  -p 5180:80 \
  -v releasehub-data:/data \
  -e RELEASEHUB_APP_ENV=production \
  releasehub
```

访问 `http://localhost:5180`，默认无需认证。

#### Docker Compose 示例

**基础部署**（适合内网，认证可在设置页面按需开启）：

```yaml
services:
  releasehub:
    image: ghcr.io/shikong-sk/releasehub:latest
    environment:
      RELEASEHUB_APP_ENV: production
      RELEASEHUB_HTTP_HOST: 0.0.0.0
      RELEASEHUB_HTTP_PORT: 8080
      RELEASEHUB_DATABASE_DSN: /data/releasehub.db
      RELEASEHUB_STORAGE_DATA_DIR: /data/releases
    volumes:
      - data:/data
    ports:
      - "5180:80"
    restart: unless-stopped

volumes:
  data:
```

**生产部署**（自托管 GitHub 镜像源 + 绑定存储路径，认证在设置页面开启）：

```yaml
services:
  releasehub:
    # image: ghcr.io/shikong-sk/releasehub:latest
    image: ghcr.nju.edu.cn/shikong-sk/releasehub:latest   # GitHub 镜像加速
    environment:
      RELEASEHUB_APP_ENV: production
      RELEASEHUB_HTTP_HOST: 0.0.0.0
      RELEASEHUB_HTTP_PORT: 8080
      RELEASEHUB_DATABASE_DSN: /data/releasehub.db
      RELEASEHUB_STORAGE_DATA_DIR: /data/releases
      RELEASEHUB_GITHUB_API_BASE_URL: https://api.github.com
      RELEASEHUB_SCHEDULER_ENABLED: "true"
      RELEASEHUB_SCHEDULER_TICK_SECONDS: "60"
      RELEASEHUB_SCHEDULER_MAX_CONCURRENT: "5"
      RELEASEHUB_APP_JWT_SECRET: your-random-secret-key-here
      RELEASEHUB_AUTH_DEFAULT_ADMIN: admin
      RELEASEHUB_AUTH_DEFAULT_PASSWORD: admin
    volumes:
      - data:/data
    ports:
      - "5180:80"
    restart: unless-stopped

volumes:
  data:
    driver: local
    driver_opts:
      type: none
      o: bind
      device: /mnt/storage/release   # 替换为你的实际存储路径
```

> **提示**：
> - 容器内 nginx 监听 **80** 端口，后端监听 **8080**（容器内部），对外只需映射 `80`
> - `volumes.data.driver_opts` 可将数据直接绑定到宿主机目录，避免 Docker 卷管理开销
> - 启用认证后务必修改默认管理员密码和 `RELEASEHUB_APP_JWT_SECRET`
> - 如使用 GitHub 镜像站（如 `ghcr.nju.edu.cn`），替换 image 地址即可

#### 从源码构建

```bash
docker compose -f docker/compose.sqlite.yml up --build
```

启动后访问 `http://localhost:8080`，数据持久化到 `./data`。

### 本地开发

**后端：**

```bash
cd backend
go mod tidy
go test ./...
go run ./cmd/releasehub
```

API 地址：`http://localhost:8080/api/health`

**前端：**

```bash
cd frontend
npm install
npm run dev
```

前端地址：`http://localhost:5173`（Vite 自动代理 `/api` 到后端 `:8080`）

## 配置

完整配置参考见 [docs/configuration.md](docs/configuration.md)。

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `RELEASEHUB_APP_ENV` | `development` | 运行环境 |
| `RELEASEHUB_HTTP_HOST` | `0.0.0.0` | API 监听地址 |
| `RELEASEHUB_HTTP_PORT` | `8080` | API 监听端口 |
| `RELEASEHUB_DATABASE_DRIVER` | `sqlite` | 数据库类型（`sqlite`、`postgres` 或 `mysql`） |
| `RELEASEHUB_DATABASE_DSN` | `data/releasehub.db` | 数据库路径（SQLite）或连接字符串（PostgreSQL） |
| `RELEASEHUB_STORAGE_DATA_DIR` | `data/releases` | 本地资产存储目录 |
| `RELEASEHUB_GITHUB_API_BASE_URL` | `https://api.github.com` | GitHub API 地址 |
| `RELEASEHUB_SCHEDULER_ENABLED` | `true` | 是否启用定时检查 |
| `RELEASEHUB_SCHEDULER_TICK_SECONDS` | `60` | Scheduler 扫描间隔（最小 10 秒） |
| `RELEASEHUB_SCHEDULER_MAX_CONCURRENT` | `5` | Scheduler 最大并发同步数 |
| `RELEASEHUB_SYNCER_MAX_CONCURRENT_TASKS` | `2` | 任务队列并发执行数 |
| `RELEASEHUB_SYNCER_MAX_CONCURRENT_DOWNLOADS` | `3` | 单任务内资产下载并发数 |
| `RELEASEHUB_DOWNLOAD_MAX_SPEED_BYTES` | `0` | 下载速度限制（字节/秒，0=不限） |
| `RELEASEHUB_APP_JWT_SECRET` | `""` | JWT 签名密钥（启用认证时必须设置） |
| `RELEASEHUB_AUTH_DEFAULT_ADMIN` | `admin` | 默认管理员用户名 |
| `RELEASEHUB_AUTH_DEFAULT_PASSWORD` | `admin` | 默认管理员密码 |

## API 概览

完整 API 文档见 [docs/api-reference.md](docs/api-reference.md)。

| 分组 | 接口 |
| --- | --- |
| 健康检查 | `GET /api/health`、`GET /api/metrics` |
| 认证 | `POST /api/auth/login`、`GET /api/auth/me`、`POST /api/auth/change-password` |
| 仓库 | CRUD + `check` / `check-all` / `sync` |
| Release | `GET /api/repositories/:id/releases`、`GET /api/releases/:id`、`GET /api/releases/:id/assets` |
| Asset | `download` / `redownload` / `delete` / `file` |
| 任务 | `GET /api/tasks`、`GET /api/tasks/:id` |
| 文件 | `GET /api/files`、`GET /api/files/download` |
| Token | CRUD + `health` / `rate-limit` |
| 存储 | CRUD + 测试连接 |
| 代理 | CRUD + 测试连接 |
| 通知 | CRUD |
| API Key | CRUD |
| 用户 | CRUD（admin） |
| 搜索 | `GET /api/search` |
| 统计 | `GET /api/stats/dashboard`、`GET /api/stats/trend` |
| 过滤预览 | `POST /api/filter/preview` |
| 保留策略 | `GET /api/repositories/:id/retention-preview`、`POST /api/repositories/:id/cleanup` |
| 上传 | `POST /api/upload` |
| 对账 | `POST /api/reconcile`（含孤儿数据检测与清理） |
| 配置 | `GET /api/config`、`PUT /api/config` |

## 项目结构

```
ReleaseHub/
├── backend/                  # Go 后端
│   ├── cmd/releasehub/       # 入口
│   └── internal/
│       ├── api/              # HTTP handler 与路由
│       ├── config/           # 配置加载
│       ├── database/         # 数据库初始化
│       ├── middleware/       # 认证、权限中间件
│       ├── models/           # GORM 模型
│       └── services/         # 业务服务
│           ├── asset/        # 资产下载与管理
│           ├── downloader/   # HTTP 下载器
│           ├── filter/       # Glob/Regex 过滤
│           ├── github/       # GitHub API 客户端
│           ├── health/       # 健康检查
│           ├── notify/       # 通知服务
│           ├── provider/     # 多平台 Provider
│           ├── proxy/        # 代理工厂
│           ├── release/      # Release 检查
│           ├── repository/   # 仓库管理
│           ├── retention/    # 保留策略与清理
│           ├── scheduler/    # 定时调度
│           ├── storage/      # 存储驱动
│           ├── syncer/       # 同步编排
│           └── tasklog/      # 任务日志
├── frontend/                 # Vue 3 前端
│   └── src/
│       ├── api/              # API 封装
│       ├── components/       # 组件
│       ├── layouts/          # 布局
│       ├── router/           # 路由
│       ├── stores/           # Pinia 状态
│       ├── types/            # TypeScript 类型
│       └── views/            # 页面
├── docker/                   # Docker 配置
│   ├── Dockerfile            # 统一构建镜像（前端 + 后端 + nginx）
│   ├── Dockerfile.backend    # 仅后端（开发用）
│   ├── Dockerfile.frontend   # 仅前端（开发用）
│   ├── compose.sqlite.yml
│   ├── nginx.conf
│   └── start.sh              # 容器入口：nginx + backend
└── docs/                     # 文档
    ├── getting-started.md
    ├── configuration.md
    ├── api-reference.md
    ├── architecture.md
    ├── DEVELOPMENT_PLAN.md
    ├── FUNCTIONAL_REVIEW_TODO.md
    └── adr/
        └── 0001-architecture.md
```

## 文档

- [快速上手](docs/getting-started.md)
- [完整配置参考](docs/configuration.md)
- [API 文档](docs/api-reference.md)
- [架构设计](docs/architecture.md)
- [部署指南](docs/deployment/deployment.md)
- [用户指南](docs/user-guide/user-guide.md)
- [开发规划](docs/DEVELOPMENT_PLAN.md)
- [功能审查清单](docs/FUNCTIONAL_REVIEW_TODO.md)
- [架构决策记录](docs/adr/0001-architecture.md)

## 路线图

详细规划见 [docs/DEVELOPMENT_PLAN.md](docs/DEVELOPMENT_PLAN.md)。

| 版本 | 状态 | 功能 |
| --- | --- | --- |
| v0.1 | ✅ 已完成 | 仓库管理、Release 检查与全量拉取、资产过滤与下载、本地存储、SHA256、保留策略、Scheduler、Web 管理、Docker 部署 |
| v0.2 | ✅ 已完成 | 多存储（S3/WebDAV）、代理、通知（Gotify/Webhook/Email/Telegram） |
| v0.3 | ✅ 已完成 | 认证、RBAC、API Key scope、任务日志、Token 健康检查、过滤预览 |
| v0.4 | ✅ 已完成 | 多存储分发、失败重试、硬删除迁移、文件树浏览、存储对账、置顶/固定版本、按 Tag 同步、级联删除、孤儿数据清理、Docker 统一镜像、CI 自动发布 |
| v0.5 | ✅ 已完成 | 断点续传、SHA256 远程比对与自动填充、保留策略增强、Dashboard 趋势图、429/5xx 自动退避 |
| v0.6 | ✅ 已完成 | GitLab/Gitea/Forgejo Provider 完整接入、搜索增强（Release body 全文搜索、组合筛选） |
| v0.7 | ✅ 已完成 | 下载速度限制（RateLimitedWriter）、aria2 RPC 配置接入 |
| v1.0 | ✅ 已完成 | PostgreSQL/MySQL 支持、Prometheus 指标导出、OpenAPI/Swagger UI |

## 许可证

[MIT License](LICENSE)
