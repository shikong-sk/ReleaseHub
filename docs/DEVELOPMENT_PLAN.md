# ReleaseHub 统一开发规划

生成时间：2026-06-26
基于：现有代码库完整审查 + 用户原始需求

---

## 一、项目现状总结

### 已完成（v0.1 MVP 主体）

**后端（Go/Gin/GORM/SQLite）：**
- Repository CRUD API（增删改查 + 启停）
- Release 检查（CheckLatest）+ Release/Asset 持久化
- Asset glob/regex 过滤（filter.Matcher）
- Syncer 服务：检查 + 并发下载（max 3）
- 原生 HTTP 下载器 + SHA256 计算
- Local Storage：Put/Open/Delete + SetLatestTag（symlink + latest.json）
- Retention 服务：软删除旧版本，保留最近 N 个
- GitHub Token CRUD API
- Config API
- File browser + download alias
- Task 查询 API
- Scheduler：定时扫描，支持 syncer 模式
- 基础测试：CRUD、过滤、并发同步

**前端（Vue 3/TS/Naive UI）：**
- Dashboard：仓库数/启用数/文件数/失败任务数
- Repositories：列表 + 创建/编辑/启停/删除/检查/同步
- Tasks：列表含错误信息
- Files：浏览 + 下载
- Settings：GitHub Token 管理 + 全局配置展示

**部署：**
- Docker Compose（SQLite）配置
- 前后端 Dockerfile + nginx 反向代理
- README 含本地开发和 Docker 启动说明

### 已识别的遗留缺陷

| # | 缺陷 | 影响 | 涉及文件 |
|---|------|------|----------|
| D1 | `RepositoryPayload` 类型缺少 `githubTokenId` 字段 | 创建仓库时无法选择 Token | `frontend/src/types/repository.ts` |
| D2 | `RepositoryFormDrawer` 没有 Token 选择控件 | 编辑仓库时无法关联 Token | `frontend/src/components/repository/RepositoryFormDrawer.vue` |
| D3 | `submitRepository` 更新时只发部分字段，不含 `githubTokenId` | 编辑后 Token 关联丢失 | `frontend/src/views/RepositoriesView.vue` |
| D4 | Token 删除检查 `github_token_id = ?` 对 nullable 字段不够健壮 | 理论上可能漏匹配 | `backend/internal/api/token_handler.go` |
| D5 | AssetPanel 未区分 failed/pending 状态的视觉差异，无重试按钮 | 失败资产无法直接重试 | `frontend/src/components/repository/AssetPanel.vue` |
| D6 | Docker Compose 未在真实环境验证 | 部署可能有问题 | `docker/compose.sqlite.yml` |
| D7 | 全局并发控制缺失，Scheduler 可能同时触发大量仓库检查 | GitHub API 限流风险 | `backend/internal/services/scheduler/service.go` |
| D8 | GitHub Client 只支持 latest release，不支持全量拉取 | 无法同步历史版本 | `backend/internal/services/github/client.go` |

---

## 二、版本规划总览

```
v0.1 收尾 ←── 当前
  │
v0.2 多存储 + 代理 + 通知
  │
v0.3 过滤增强 + Token 管理 + 多用户认证 + 日志队列
  │
v0.4 下载增强 + 校验 + 保留策略增强 + 重试
  │
v0.5 双向同步 + 手动上传 + 搜索 + 统计
  │
v0.6 多平台 Provider（GitLab/Gitea/Forgejo）
  │
v1.0 插件系统 + RBAC + API Key + Prometheus + OpenAPI + 高可用
```

---

## 三、v0.1 收尾任务清单

**目标：让 MVP 在本地稳定运行，前端可用完整流程闭环。**

### 3.1 前端 — Token 关联（D1/D2/D3）

1. `frontend/src/types/repository.ts`：`RepositoryPayload` 增加 `githubTokenId?: number | null`
2. `RepositoryFormDrawer.vue`：增加 `<NSelect>` 控件，数据源为 `useTokensStore`，选项为 `token.id: token.name (token.tokenHint)`
3. `RepositoriesView.vue`：`submitRepository` 更新时发送完整 payload（含 `githubTokenId`），创建时也发送
4. 编辑模式下，表单回填 `githubTokenId`

### 3.2 后端 — Token 删除安全（D4）

5. `token_handler.go` 删除检查：增加 `AND github_token_id IS NOT NULL` 或改用 `Where("github_token_id = ? AND github_token_id IS NOT NULL", id)` 确保不误匹配 null 行

### 3.3 前端 — Asset 状态增强（D5）

6. `AssetPanel.vue`：failed 资产行显示错误信息 tooltip，增加"重试"按钮（调用 `POST /api/assets/:id/redownload`）
7. pending 资产行显示"待下载"标签而非"下载"按钮

### 3.4 后端 — Scheduler 全局并发（D7）

8. `scheduler/service.go`：增加全局 `semaphore chan struct{}`（默认 max 5），控制同时运行的检查/同步任务数
9. 配置化：`RELEASEHUB_SCHEDULER_MAX_CONCURRENT` 环境变量

### 3.5 后端 — 全量 Release 拉取（D8）

10. `github/client.go`：新增 `ListReleases(ctx, owner, repo, token, page, perPage)` 方法
11. `release/checker.go`：新增 `CheckAll(ctx, repositoryID)` 方法，拉取所有 release（分页），存入数据库
12. API：`POST /api/repositories/:id/check-all`
13. 前端：仓库操作栏增加"全量检查"按钮

### 3.6 Docker 验证（D6）

14. 在有 Docker 的环境实际运行 `docker compose -f docker/compose.sqlite.yml up --build`
15. 验证前端可访问、API 可调用、数据持久化

### 3.7 测试补充

16. 后端：Token 删除安全测试（关联仓库时拒绝删除）
17. 后端：Scheduler 全局并发测试
18. 后端：全量 Release 拉取测试（mock GitHub server）
19. 前端：typecheck 通过

### 3.8 收尾验收标准

- [ ] 创建仓库时可选 GitHub Token
- [ ] 编辑仓库时可更换 Token
- [ ] Token 删除时正确拒绝被引用的 Token
- [ ] 失败资产在 UI 中有重试入口
- [ ] Scheduler 不会同时触发超过 N 个仓库检查
- [ ] 全量检查可拉取多页 Release
- [ ] Docker Compose 可一键启动

---

## 四、v0.2 任务清单 — 多存储 + 代理 + 通知

**目标：支持 S3/WebDAV/OpenList 存储、HTTP/SOCKS5 代理、Gotify/Webhook 通知。**

### 4.1 Storage Driver 接口化

1. 抽象 `StorageDriver` 接口：
   ```go
   type StorageDriver interface {
       Put(ctx context.Context, objectPath string, reader io.Reader) (*StoredObject, error)
       Open(ctx context.Context, objectPath string) (*StoredObject, io.ReadCloser, error)
       Delete(ctx context.Context, objectPath string) error
       SetLatestTag(ctx context.Context, provider, owner, repo, tag string) error
       Capabilities() StorageCapabilities
   }
   ```
2. 将现有 `LocalStorage` 改为实现该接口
3. 重构 `asset.Service` 和 `retention.Service`，注入 `StorageDriver` 而非直接依赖 `LocalStorage`

### 4.2 S3 Storage Driver

4. 实现 `S3Storage`（使用 `aws-sdk-go-v2`）
5. 支持 Endpoint / Bucket / Region / Prefix / AccessKey / SecretKey
6. 测试连接 API
7. 契约测试

### 4.3 WebDAV Storage Driver

8. 实现 `WebDAVStorage`（使用 `studio-b12/gowebdav`）
9. 支持 URL / Username / Password / BasePath
10. OpenList 通过 WebDAV 或 S3 接入（无需特殊处理）
11. 契约测试

### 4.4 Storage CRUD API + 前端

12. `Storage` 模型已在数据库中，补充 CRUD API
13. 前端 Storage 管理页面：新增/编辑/删除/测试连接
14. 仓库表单增加"存储目标"选择

### 4.5 Proxy 模型与 Driver

15. 数据库新增 `Proxy` 模型（id, name, type, host, port, username, password）
16. 抽象 `ProxyResolver` 接口
17. 实现 HTTP/HTTPS/SOCKS5 代理支持
18. GitHub Client 下载请求支持代理
19. 仓库表单增加"代理"选择
20. 代理测试连接 API（测试 GitHub API 可达性）

### 4.6 通知系统

21. 数据库新增 `Notification` 模型（id, type, server_url, token, enabled, events）
22. 抽象 `Notifier` 接口
23. 实现 Gotify Notifier
24. 实现 Webhook Notifier
25. 通知触发点：下载完成、下载失败、发现新版本
26. 前端通知管理页面
27. 仓库表单增加"通知渠道"选择（多选）

### 4.7 验收标准

- [ ] 仓库可配置 S3 存储，资产上传到 S3
- [ ] 仓库可配置 WebDAV 存储，资产上传到 WebDAV
- [ ] 仓库可配置代理，GitHub API 请求走代理
- [ ] Gotify 推送下载完成通知
- [ ] Webhook 推送新版本发现通知
- [ ] Storage/Proxy/Notification CRUD 前端页面可用

---

## 五、v0.3 任务清单 — 过滤增强 + Token 管理 + 认证 + 日志

### 5.1 过滤增强

1. 支持 Glob 多模式（逗号/换行分隔，已支持）
2. 支持 Regex 命名捕获组提取版本号
3. 支持按文件大小过滤（min/max）
4. 过滤预览 API：提交过滤规则 → 返回最新 Release 匹配的资产列表
5. 前端过滤规则编辑器增加预览面板

### 5.2 GitHub Token 增强

6. Token 健康检查（验证 Token 有效性、Rate Limit 状态）
7. Token 使用统计（哪些仓库在用）
8. Token 轮换：支持同一仓库配置多个 Token（自动选择 rate limit 充足的）
9. Token 分组管理

### 5.3 用户认证

10. 数据库新增 `User` 模型
11. JWT 认证中间件
12. 登录/登出 API
13. 前端登录页面
14. 路由守卫

### 5.4 日志与任务队列

15. 结构化任务日志（独立 `task_logs` 表）
16. 任务重试策略（指数退避）
17. 任务取消 API
18. 任务批量操作（重试所有失败任务）

### 5.5 验收标准

- [ ] 过滤规则可在前端预览匹配结果
- [ ] Token 健康状态可见
- [ ] 用户需登录才能访问管理页面
- [ ] 失败任务可批量重试

---

## 六、v0.4 任务清单 — 下载增强 + 校验 + 保留策略 + 重试

### 6.1 下载增强

1. 大文件流式下载（不缓存到内存）
2. 断点续传（记录已下载字节数，HTTP Range 请求）
3. aria2 RPC 可选后端（通过配置启用）
4. 下载进度实时推送（WebSocket 或 SSE）
5. 下载速度限制（Rate Limit）

### 6.2 SHA256 校验增强

6. 下载前查询 GitHub Release 的 asset checksum（如果 release body 或 `.sha256` 文件存在）
7. 自动比对本地计算 vs 远程校验和
8. 校验不匹配时标记资产为 `failed`，自动重试

### 6.3 保留策略增强

9. 按时间保留（保留最近 N 天的 Release）
10. 按数量 + 时间混合策略
11. 保留策略预览 API（dry-run，返回将被删除的 Release 列表）
12. 手动触发清理 API

### 6.4 失败重试

13. 下载失败自动重试（可配置重试次数和间隔）
14. 重试计数和退避策略
15. GitHub API 429/5xx 自动退避

### 6.5 验收标准

- [ ] 大文件不占用内存
- [ ] 下载中断后可续传
- [ ] SHA256 与远程校验和比对
- [ ] 保留策略可预览
- [ ] 下载失败自动重试 3 次

---

## 七、v0.5 任务清单 — 双向同步 + 上传 + 搜索 + 统计

### 7.1 双向同步

1. 检测本地存储中已有但数据库未记录的资产（reconcile）
2. 手动上传资产到指定仓库/Release
3. 存储容量统计与告警

### 7.2 搜索增强

4. 全文搜索 Release body
5. 按仓库/Tag/日期/状态组合筛选
6. 前端高级搜索面板

### 7.3 统计面板

7. ECharts 统计图：同步趋势、存储使用、下载速度
8. 仓库健康度总览
9. Dashboard 增强

### 7.4 验收标准

- [ ] 本地存储与数据库可一键对账
- [ ] 可手动上传资产
- [ ] 可搜索 Release body
- [ ] Dashboard 展示趋势图

---

## 八、v0.6 任务清单 — 多平台 Provider

### 8.1 Release Provider 接口化

1. 抽象 `ReleaseProvider` 接口：
   ```go
   type ReleaseProvider interface {
       ListReleases(ctx, owner, repo, token, page, perPage) ([]ProviderRelease, error)
       GetLatestRelease(ctx, owner, repo, token) (*ProviderRelease, error)
       GetAssetDownloadURL(ctx, owner, repo, asset, token) (string, error)
   }
   ```

### 8.2 GitLab Release Provider

2. 实现 GitLab Release Provider
3. 支持 GitLab Token
4. 支持 Self-hosted GitLab

### 8.3 Gitea/Forgejo Release Provider

5. 实现 Gitea/Forgejo Release Provider
6. 支持 Gitea Token
7. 兼容 Forgejo API

### 8.4 Docker Hub Provider（可选）

8. 调研 Docker Hub Registry API
9. 如果可行，实现 Docker Hub Tag 同步

### 8.5 前端适配

10. 仓库表单 Provider 选择
11. Provider 对应的认证配置
12. Provider 特有的 UI 差异处理

### 8.6 验收标准

- [ ] 仓库可选 GitLab/Gitea/Forgejo 作为 Provider
- [ ] GitLab Release 可自动同步
- [ ] Gitea/Forgejo Release 可自动同步

---

## 九、v1.0 任务清单 — 插件系统 + RBAC + API Key + 可观测性

### 9.1 插件系统

1. 插件目录结构约定
2. 插件加载器（Go plugin 或 WASM）
3. 插件 API：注册 Storage Driver / Notifier / Provider
4. 插件管理页面

### 9.2 RBAC

5. 角色模型（admin / operator / viewer）
6. 权限矩阵
7. 资源级权限控制
8. 用户管理页面

### 9.3 API Key

9. API Key 模型（用于外部系统集成）
10. API Key 认证中间件
11. API Key 权限范围（scope）

### 9.4 可观测性

12. Prometheus 指标导出（同步次数、下载大小、API 请求、错误率）
13. 健康检查增强（存储连通性、GitHub API 可达性）
14. 结构化日志（JSON 格式，可对接 Loki）

### 9.5 OpenAPI 文档

15. Swagger 注解补全
16. 自动生成 OpenAPI 3.0 文档
17. Swagger UI 集成

### 9.6 高可用部署

18. PostgreSQL 正式支持 + 迁移脚本
19. Redis 可选（任务队列、缓存）
20. 多实例部署指南

### 9.7 验收标准

- [ ] 插件可独立开发并加载
- [ ] 非管理员用户只能查看不能修改
- [ ] API Key 可用于外部系统调用
- [ ] Prometheus 可采集指标
- [ ] OpenAPI 文档自动生成
- [ ] PostgreSQL + Redis 生产部署文档

---

## 十、数据库迁移规划

当前模型中 `Storage` 表已存在但未使用。后续版本需要：

| 版本 | 新增/变更 |
|------|----------|
| v0.1 收尾 | 无变更 |
| v0.2 | `Proxy` 表、`Notification` 表、`Repository` 增加 `proxy_id`/`notification_ids` |
| v0.3 | `User` 表、`TaskLog` 表 |
| v0.4 | `Asset` 增加 `download_bytes`/`expected_sha256`/`retry_count` |
| v0.5 | 无新表，`Asset` 增加索引 |
| v0.6 | `Repository.provider` 字段从默认 github 变为可配置 |
| v1.0 | `APIKey` 表、`Role`/`Permission` 表 |

迁移策略：GORM AutoMigrate 为主，破坏性变更用独立迁移脚本。

---

## 十一、技术债跟踪

| 债项 | 来源 | 建议处理版本 |
|------|------|-------------|
| GitHub Client 手写 HTTP 而非用 go-github SDK | MVP 快速闭环 | v0.3 |
| 下载全部缓存到内存（bytes.Buffer） | MVP 快速闭环 | v0.4 |
| 前端无路由守卫/认证 | MVP 不需要 | v0.3 |
| 无 OpenAPI/Swagger 文档 | 优先级低 | v1.0 |
| 无国际化（i18n） | 暂时只有中文 | v0.5+ |
| 测试覆盖不够全面 | 持续改进 | 每个版本 |
| 无优雅关闭时的任务中断处理 | 非紧急 | v0.4 |

---

## 十二、ADR 规划

| 编号 | 标题 | 建议版本 |
|------|------|---------|
| 0001 | 第一版架构 | ✅ 已完成 |
| 0002 | 任务状态机与调度策略 | v0.1 收尾 |
| 0003 | Storage Driver 契约 | v0.2 |
| 0004 | Release Provider 契约 | v0.6 |
| 0005 | Token 与 Secret 管理 | v0.3 |
| 0006 | 通知系统架构 | v0.2 |
| 0007 | 认证与授权 | v0.3 |
| 0008 | 插件系统设计 | v1.0 |

---

## 十三、执行优先级原则

1. **闭环优先**：每个版本结束前，该版本的所有功能必须在 Web UI 中可操作、有测试、有文档。
2. **安全优先**：Token/认证相关缺陷优先修复，不留安全隐患。
3. **可观测优先**：任务状态、错误信息、日志在每个版本都要有改善。
4. **向后兼容**：数据库迁移必须可回滚，API 变更必须版本化。
5. **驱动化优先**：新功能通过 Driver 接口扩展，不修改核心业务逻辑。

---

## 十四、建议的初始提交信息

```
feat: ReleaseHub v0.1 MVP — GitHub Release 同步管理平台

包含：
- 后端：Repository/Release/Asset/Task 模型与 CRUD API
- 后端：GitHub Release 检查、资产过滤、并发下载、SHA256 校验
- 后端：Local Storage、Retention 保留策略、Scheduler 定时同步
- 后端：GitHub Token 管理、Config API、File Browser
- 前端：Dashboard、仓库管理、任务列表、文件浏览、设置页
- 部署：Docker Compose (SQLite) + 前后端 Dockerfile
- 测试：Repository CRUD、过滤、并发同步集成测试
```
