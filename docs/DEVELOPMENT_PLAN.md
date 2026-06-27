# ReleaseHub 统一开发规划

更新时间：2026-06-27

---

## 一、项目现状总结

### 已完成

**后端（Go/Gin/GORM/SQLite）：**
- Repository CRUD API（增删改查 + 启停）
- Release 检查（CheckLatest + CheckAll）+ Release/Asset 持久化
- Asset glob/regex 过滤（filter.Matcher）
- Syncer 服务：检查 + 并发下载
- 流式 HTTP 下载器 + SHA256 计算（不缓存完整文件到内存）
- Storage Driver 接口化：Local / S3 / WebDAV
- Storage DriverFactory：按仓库动态选择存储驱动
- Proxy 模型与 Factory：HTTP/HTTPS/SOCKS5 代理支持
- Retention 服务：按仓库选择存储驱动，保留最近 N 个版本
- GitHub Token CRUD API + 健康检查 + Rate Limit 查询
- Notification CRUD + Notifier 接口：Gotify/Webhook/Email/Telegram
- Notify Service：按事件类型派发通知（new_release/download_ok/download_err/sync_success/sync_failed）
- User 模型 + JWT 认证 + RBAC（admin/operator/viewer）
- APIKey 模型 + scope 权限控制 + APIKeyOrAuth 中间件
- AuthorizeRequest 中间件：角色矩阵 + scope 判断
- Config API + 运行时配置更新
- File browser + download + tree
- Task 查询 API + TaskLog 结构化日志
- Search API
- Stats API（dashboard + trend）
- Filter preview API
- Upload API（按仓库选择存储驱动）
- Reconcile API（按仓库选择存储驱动）
- Release Provider 接口化：GitHub/GitLab/Gitea/Forgejo
- Scheduler：定时扫描，全局并发控制，失败资产重试
- 基础测试：CRUD、过滤、并发同步、存储工厂、权限

**前端（Vue 3/TS/Naive UI）：**
- Dashboard：仓库数/启用数/文件数/失败任务数 + 失败告警
- Repositories：列表 + 创建/编辑/启停/删除/检查/同步/全量检查/历史
- RepositoryFormDrawer：Provider 选择、Token 关联、Storage/Proxy 选择、API Base URL
- AssetPanel：状态区分、重试按钮、下载文件链接、删除按钮
- ReleaseHistoryDrawer：Release 历史浏览
- Tasks：列表含错误信息 + TaskLog Drawer
- Files：浏览 + 下载 + 存储对账 + 删除
- Settings：GitHub Token 管理（含健康检查状态）+ 全局配置 + API Key 管理
- Storages：存储管理（创建/编辑/删除/测试连接）
- Proxies：代理管理（创建/编辑/删除/测试连接）
- Notifications：通知管理
- Users：用户管理（admin）
- Login：登录页 + 路由守卫 + 401 自动跳转
- RBAC：菜单按角色过滤、操作按钮按权限隐藏

**部署：**
- Docker Compose（SQLite）配置
- 前后端 Dockerfile + nginx 反向代理
- README 含本地开发和 Docker 启动说明

### 当前版本状态

| 版本 | 状态 | 说明 |
| --- | --- | --- |
| v0.1 | ✅ 已完成 | MVP 主体 + 收尾 |
| v0.2 | ✅ 已完成 | 多存储 + 代理 + 通知 |
| v0.3 | ✅ 已完成 | 认证 + RBAC + API Key scope + 任务日志 + Token 健康 |
| v0.4 | 🔧 进行中 | 流式下载已优化，断点续传/SHA256 远程比对/重试退避待完善 |
| v0.5 | 📋 规划中 | 双向同步 + 搜索增强 + Dashboard 趋势图 |
| v0.6 | 📋 规划中 | GitLab/Gitea/Forgejo Provider 完整接入 |
| v1.0 | 📋 规划中 | PostgreSQL + OpenAPI + 插件系统 + Prometheus |

---

## 二、v0.4 任务清单 — 下载增强 + 校验 + 保留策略增强 + 重试

**目标：完善下载可靠性，支持断点续传和远程校验和比对。**

### 4.1 断点续传

1. Asset 模型增加 `downloadBytes` 字段，记录已下载字节数
2. 下载中断后恢复时使用 HTTP Range 请求
3. 下载进度实时推送（WebSocket 或 SSE）

### 4.2 SHA256 校验增强

4. 下载前查询 GitHub Release 的 asset checksum（如果 release body 或 `.sha256` 文件存在）
5. Asset 模型增加 `expectedSha256` 字段
6. 自动比对本地计算 vs 远程校验和
7. 校验不匹配时标记资产为 `failed`，自动重试

### 4.3 保留策略增强

8. 按时间保留（保留最近 N 天的 Release）
9. 按数量 + 时间混合策略
10. 保留策略预览 API（dry-run，返回将被删除的 Release 列表）
11. 手动触发清理 API

### 4.4 失败重试完善

12. 下载失败自动重试（可配置重试次数和间隔）
13. 重试计数和退避策略完善
14. GitHub API 429/5xx 自动退避
15. RetryDownload 主动 sleep 或调度延迟任务

### 4.5 下载速度限制

16. 下载速度限制（Rate Limit）

### 4.6 aria2 RPC 可选后端

17. aria2 RPC 接入调度（代码已存在，需接入 Syncer）

### 4.7 验收标准

- [ ] 大文件不占用内存
- [ ] 下载中断后可续传
- [ ] SHA256 与远程校验和比对
- [ ] 保留策略可预览
- [ ] 下载失败自动重试 3 次
- [ ] 429 响应自动退避

---

## 三、v0.5 任务清单 — 双向同步 + 搜索增强 + 统计

### 5.1 双向同步

1. 检测本地存储中已有但数据库未记录的资产（reconcile 增强）
2. 手动上传资产到指定仓库/Release
3. 存储容量统计与告警

### 5.2 搜索增强

4. 全文搜索 Release body
5. 按仓库/Tag/日期/状态组合筛选
6. 前端高级搜索面板

### 5.3 统计面板

7. 前端调用 `getTrendStats` API
8. 引入 ECharts 或轻量图表库
9. 展示 Release 检查趋势和下载量趋势
10. 仓库健康度总览
11. Dashboard 增强

### 5.4 验收标准

- [ ] 本地存储与数据库可一键对账
- [ ] 可手动上传资产
- [ ] 可搜索 Release body
- [ ] Dashboard 展示趋势图

---

## 四、v0.6 任务清单 — 多平台 Provider 完整接入

### 6.1 Provider 接入业务

1. 将 release checker 改为依赖 `ReleaseProvider` 接口
2. 仓库新增 provider API base URL/token 解析规则
3. 前端仓库表单增加 provider 选择（已有，需验证完整流程）
4. 分别补 GitHub/GitLab/Gitea/Forgejo 的 latest/list/download URL 测试

### 6.2 GitLab Release Provider

5. 完善 GitLab Release Provider 实现
6. 支持 GitLab Token
7. 支持 Self-hosted GitLab

### 6.3 Gitea/Forgejo Release Provider

8. 完善 Gitea/Forgejo Release Provider 实现
9. 支持 Gitea Token
10. 兼容 Forgejo API

### 6.4 验收标准

- [ ] 仓库可选 GitLab/Gitea/Forgejo 作为 Provider
- [ ] GitLab Release 可自动同步
- [ ] Gitea/Forgejo Release 可自动同步
- [ ] 自托管实例可配置 API Base URL

---

## 五、v1.0 任务清单 — PostgreSQL + OpenAPI + 插件系统 + 可观测性

### 5.1 PostgreSQL 支持

1. 引入 `gorm.io/driver/postgres`
2. 增加 `database.type=postgres` 配置
3. 补 SQLite/PostgreSQL 迁移一致性测试
4. 迁移脚本

### 5.2 OpenAPI 文档

5. Swagger 注解补全或维护 OpenAPI YAML
6. 自动生成 OpenAPI 3.0 文档
7. Swagger UI 集成
8. CI 中校验 OpenAPI 文件同步

### 5.3 插件系统

9. 插件目录结构约定
10. 插件加载器（Go plugin 或 WASM）
11. 插件 API：注册 Storage Driver / Notifier / Provider
12. 插件管理页面

### 5.4 RBAC 增强

13. 资源级权限控制
14. 用户管理页面增强

### 5.5 API Key 增强

15. API Key 编辑 scope
16. API Key 禁用/启用

### 5.6 可观测性

17. Prometheus 指标导出（同步次数、下载大小、API 请求、错误率）
18. 健康检查增强（存储连通性、GitHub API 可达性）
19. 结构化日志（JSON 格式，可对接 Loki）

### 5.7 高可用部署

20. Redis 可选（任务队列、缓存）
21. 多实例部署指南

### 5.8 验收标准

- [ ] PostgreSQL 可作为数据库运行
- [ ] OpenAPI 文档自动生成
- [ ] 插件可独立开发并加载
- [ ] Prometheus 可采集指标
- [ ] PostgreSQL + Redis 生产部署文档

---

## 六、数据库迁移规划

| 版本 | 新增/变更 |
|------|----------|
| v0.4 | `Asset` 增加 `download_bytes`/`expected_sha256`/`retry_count` |
| v0.5 | `Asset` 增加索引 |
| v0.6 | `Repository.provider` 字段从默认 github 变为可配置（已完成） |
| v1.0 | PostgreSQL 支持 |

迁移策略：GORM AutoMigrate 为主，破坏性变更用独立迁移脚本。

---

## 七、技术债跟踪

| 债项 | 来源 | 建议处理版本 |
|------|------|-------------|
| S3 简化 HTTP Basic Auth 实现 | MVP 快速闭环 | v0.2 收尾 |
| WebDAV/S3 全量上传 | MVP 快速闭环 | v0.4 |
| GitHub Client 手写 HTTP 而非用 go-github SDK | MVP 快速闭环 | v0.3 |
| 无 OpenAPI/Swagger 文档 | 优先级低 | v1.0 |
| 无国际化（i18n） | 暂时只有中文 | v0.5+ |
| 测试覆盖不够全面 | 持续改进 | 每个版本 |
| 无优雅关闭时的任务中断处理 | 非紧急 | v0.4 |
| 前端趋势图未集成 | API 已就绪 | v0.5 |
| aria2 RPC 未接入调度 | 代码已存在 | v0.4 |

---

## 八、ADR 规划

| 编号 | 标题 | 状态 |
|------|------|------|
| 0001 | 第一版架构 | ✅ 已完成 |
| 0002 | 任务状态机与调度策略 | 📋 待编写 |
| 0003 | Storage Driver 契约 | 📋 待编写 |
| 0004 | Release Provider 契约 | 📋 待编写 |
| 0005 | Token 与 Secret 管理 | 📋 待编写 |
| 0006 | 通知系统架构 | 📋 待编写 |
| 0007 | 认证与授权 | 📋 待编写 |
| 0008 | 插件系统设计 | 📋 待编写 |

---

## 九、执行优先级原则

1. **闭环优先**：每个版本结束前，该版本的所有功能必须在 Web UI 中可操作、有测试、有文档。
2. **安全优先**：Token/认证相关缺陷优先修复，不留安全隐患。
3. **可观测优先**：任务状态、错误信息、日志在每个版本都要有改善。
4. **向后兼容**：数据库迁移必须可回滚，API 变更必须版本化。
5. **驱动化优先**：新功能通过 Driver 接口扩展，不修改核心业务逻辑。
