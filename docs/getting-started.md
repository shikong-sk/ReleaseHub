# 快速上手

本文帮助你从零启动 ReleaseHub 并完成基本配置。

## 前置条件

- Docker & Docker Compose（推荐方式），或本地安装 Go 1.25+ / Node.js 22+
- 至少一个 GitHub Personal Access Token（如果需要同步私有仓库或避免 API 限流）

## 一、部署

### 方式一：Docker Compose（推荐）

```bash
git clone <repo-url> && cd ReleaseHub
docker compose -f docker/compose.sqlite.yml up --build -d
```

启动后访问 `http://localhost:8088`。

数据持久化在 `./data` 目录，包含 SQLite 数据库和本地存储的 Release 文件。

### 方式二：本地开发

分别启动后端和前端：

```bash
# 终端 1 — 后端
cd backend
go mod tidy
go run ./cmd/releasehub

# 终端 2 — 前端
cd frontend
npm install
npm run dev
```

- 后端 API：`http://localhost:8080`
- 前端页面：`http://localhost:5173`（自动代理 `/api` 请求到后端）

## 二、启用认证（可选）

默认不启用认证，适合内网部署。如需启用：

1. 设置环境变量：

```bash
export RELEASEHUB_AUTH_ENABLED=true
export RELEASEHUB_APP_JWT_SECRET=your-random-secret
```

2. 重启服务后访问 `http://localhost:8088`，使用默认账号登录：
   - 用户名：`admin`
   - 密码：`admin`

3. 登录后请立即修改密码。

## 三、添加 GitHub Token

1. 进入 **设置** 页面。
2. 在 **GitHub Token** 面板点击"添加"。
3. 填写名称和 Token 值，保存。

Token 值在保存后不会再次展示，仅显示 hint（如 `ghp_****abcd`）。

## 四、添加仓库

1. 进入 **仓库** 页面，点击"新增仓库"。
2. 填写：
   - **Provider**：GitHub / GitLab / Gitea / Forgejo
   - **Owner**：仓库所有者
   - **Repo**：仓库名
   - **GitHub Token**（可选）：关联上一步创建的 Token
   - **存储目标**（可选）：选择 S3/WebDAV 或使用默认本地存储
   - **代理**（可选）：选择代理配置
3. 保存后可点击"检查最新"或"全量检查"拉取 Release 信息。

## 五、同步资产

- **手动同步**：在仓库列表点击"立即同步"，检查并下载最新 Release 的资产。
- **自动同步**：Scheduler 默认每 60 秒扫描所有启用的仓库，自动检查和同步。

### 过滤规则

在仓库表单中配置：
- **过滤模式**：Glob 或 Regex
- **包含规则**：如 `*.tar.gz` 或 `linux-amd64`
- **排除规则**：如 `*.sha256` 或 `*.sig`

### 保留策略

每个仓库可配置保留最近 N 个版本，旧版本的资产会被自动清理。

## 六、浏览和下载文件

- **文件页面**：浏览所有已同步的文件，支持搜索和下载。
- **Release 历史**：在仓库列表点击"历史"查看所有 Release 及其资产。
- **存储对账**：检测存储与数据库的不一致（管理员功能）。

## 七、通知配置

1. 进入 **通知** 页面，点击"添加"。
2. 选择类型：Gotify / Webhook / Email / Telegram。
3. 填写服务器地址和 Token。
4. 选择要订阅的事件类型。
5. 保存并启用。

## 八、代理配置

如需通过代理访问 GitHub API 或下载资产：

1. 进入 **代理** 页面，点击"添加"。
2. 选择类型：HTTP / HTTPS / SOCKS5。
3. 填写地址、端口和认证信息。
4. 点击"测试"验证连通性。
5. 在仓库表单中关联代理。

## 下一步

- [完整配置参考](configuration.md)
- [API 文档](api-reference.md)
- [架构设计](architecture.md)
