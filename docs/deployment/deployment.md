# 部署指南

本文介绍 ReleaseHub 的各种部署方式、生产环境配置建议和运维要点。

## 部署架构

```
┌─────────────┐     ┌─────────────────────────────────────┐
│   Browser    │────▶│  Nginx (容器内反向代理)              │
└─────────────┘     │   ├── / → Vue SPA 静态文件            │
                    │   └── /api → backend:8080            │
                    └──────────────┬──────────────────────┘
                                   │
                    ┌──────────────▼──────────────────────┐
                    │  Go Backend (单进程)                  │
                    │   ├── Gin HTTP API (:8080)           │
                    │   ├── Scheduler 后台扫描              │
                    │   └── Syncer 同步编排                 │
                    ├─────────────────────────────────────┤
                    │  SQLite + 本地文件存储               │
                    │   (或 S3 / WebDAV 远程存储)           │
                    └─────────────────────────────────────┘
```

容器编排采用 Docker Compose，对外只暴露前端 Nginx 端口（默认 8088），后端 API 不直接对外。

## 方式一：Docker Compose（推荐）

### 最简启动

```bash
git clone <repo-url> && cd ReleaseHub
docker compose -f docker/compose.sqlite.yml up --build -d
```

启动后访问 `http://localhost:8088`。

- 前端容器：Nginx 监听 80，对外映射到 8088
- 后端容器：Gin 监听 8080，仅容器内网可达
- 数据持久化：`./data` 目录挂载到后端容器的 `/data`，包含 SQLite 数据库和本地存储的 Release 文件

### 启用认证

生产环境务必启用认证。启动后在 Web 管理界面的「设置 → 全局配置」中开启认证开关，或通过 API：

```bash
curl -X PUT http://localhost:8080/api/config \
  -H 'Content-Type: application/json' \
  -d '{"authEnabled": true}'
```

同时设置 JWT 密钥环境变量：

```yaml
services:
  backend:
    environment:
      RELEASEHUB_APP_JWT_SECRET: your-strong-random-secret
```

> 启用认证后请立即登录修改默认管理员密码。

### 自定义端口

修改 `docker/compose.sqlite.yml` 中 frontend 的端口映射：

```yaml
services:
  frontend:
    ports:
      - "9000:80"   # 改为你想要的对外端口
```

## 方式二：本地开发部署

适合二次开发和调试，前后端独立运行。

```bash
# 后端
cd backend
go mod tidy
go run ./cmd/releasehub        # 默认监听 :8080

# 前端
cd frontend
npm install
npm run dev                    # Vite 监听 :5173，自动代理 /api 到 :8080
```

访问 `http://localhost:5173`。

## 方式三：单后端二进制 + 前端静态文件

生产部署也可不依赖 Docker，直接编译后端并托管前端静态文件：

```bash
# 构建前端
cd frontend && npm install && npm run build      # 产物在 frontend/dist

# 构建后端
cd backend && go build -o releasehub ./cmd/releasehub

# 运行（用 Nginx 或其他反向代理把 / 指到 dist，/api 反代到 :8080）
./releasehub
```

## 数据持久化与备份

| 内容 | 位置 | 说明 |
| --- | --- | --- |
| SQLite 数据库 | `data/releasehub.db` | 元数据、配置、任务记录（PostgreSQL 部署时对应外部数据库） |
| 本地资产 | `data/releases/<provider>/<owner>/<repo>/<tag>/` | 已下载的 Release 文件 |
| `latest` 映射 | `data/releases/<provider>/<owner>/<repo>/latest` | 指向最新版本的符号链接或文件 |

备份建议：

1. 定期备份 `data/releasehub.db`（SQLite 单文件，停机或使用 `.backup` 命令）
2. 如果使用 S3/WebDAV，按云厂商的桶备份策略执行
3. 升级前务必备份数据库，启动时会自动执行 AutoMigrate

## 反向代理（Nginx）

参考 `docker/nginx.conf`：

```nginx
server {
    listen 80;
    root /usr/share/nginx/html;
    index index.html;

    location /api/ {
        proxy_pass http://backend:8080/api/;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location / {
        try_files $uri $uri/ /index.html;
    }
}
```

如需 HTTPS，在外层 Nginx/Traefik 配置 TLS 证书即可。

## 资源规划建议

| 仓库规模 | 并发同步数 | 内存 | 说明 |
| --- | --- | --- | --- |
| < 50 个仓库 | 默认 5 | 256MB+ | 个人/NAS 部署 |
| 50–200 个仓库 | 5–10 | 512MB+ | 小团队 |
| > 200 个仓库 | 10+ | 1GB+ | 建议使用 PostgreSQL 替代 SQLite |

流式下载不缓存完整文件到内存，内存占用主要取决于并发下载数。

## 升级与迁移

ReleaseHub 通过 GORM AutoMigrate 自动处理表结构变更，无需手动迁移脚本。已知的一次性数据迁移已内置在启动流程中：

- `SeedDefaultStorage`：自动创建默认本地存储
- `BackfillAssetStorageID`：回填存量资产的 `storageId`
- `MigrateDropDeletedAt`：从软删除迁移到硬删除（v0.4 完成）

升级步骤：

1. 备份 `data/releasehub.db`
2. 拉取新版本镜像或代码
3. `docker compose -f docker/compose.sqlite.yml up --build -d` 重建容器
4. 启动时会自动执行迁移


## 方式四：PostgreSQL / MySQL 部署

v1.0 起支持 PostgreSQL 和 MySQL，适合多实例或高并发场景：

```bash
# 设置环境变量启用 PostgreSQL
export RELEASEHUB_DATABASE_DRIVER=postgres
export RELEASEHUB_DATABASE_DSN="host=127.0.0.1 port=5432 user=releasehub password=secret dbname=releasehub sslmode=disable"

# 启动后端
cd backend && go run ./cmd/releasehub
```

MySQL 示例：

```bash
export RELEASEHUB_DATABASE_DRIVER=mysql
export RELEASEHUB_DATABASE_DSN="releasehub:secret@tcp(127.0.0.1:3306)/releasehub?charset=utf8mb4&parseTime=True&loc=Local"
```

Docker Compose 示例（PostgreSQL）：

```yaml
services:
  releasehub:
    image: ghcr.io/shikong-sk/releasehub:latest
    environment:
      RELEASEHUB_DATABASE_DRIVER: postgres
      RELEASEHUB_DATABASE_DSN: "host=db port=5432 user=releasehub password=secret dbname=releasehub sslmode=disable"
      # 其他配置...
    depends_on:
      - db
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: releasehub
      POSTGRES_PASSWORD: secret
      POSTGRES_DB: releasehub
    volumes:
      - pgdata:/var/lib/postgresql/data

volumes:
  pgdata:
```

Docker Compose 示例（MySQL）：

```yaml
services:
  releasehub:
    image: ghcr.io/shikong-sk/releasehub:latest
    environment:
      RELEASEHUB_DATABASE_DRIVER: mysql
      RELEASEHUB_DATABASE_DSN: "releasehub:secret@tcp(db:3306)/releasehub?charset=utf8mb4&parseTime=True&loc=Local"
      # 其他配置...
    depends_on:
      - db
  db:
    image: mysql:8.0
    environment:
      MYSQL_ROOT_PASSWORD: root
      MYSQL_USER: releasehub
      MYSQL_PASSWORD: secret
      MYSQL_DATABASE: releasehub
    volumes:
      - mysqldata:/var/lib/mysql

volumes:
  mysqldata:
```

> 从 SQLite 迁移到 PostgreSQL/MySQL 时，启动时 GORM AutoMigrate 会自动建表。跨数据库的存量数据迁移需自行处理导出导入。

## 下一步

- [完整配置参考](../configuration.md)
- [用户指南](../user-guide/user-guide.md)
- [架构设计](../architecture.md)
