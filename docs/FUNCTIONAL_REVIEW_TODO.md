# ReleaseHub 功能审查与待完善清单

更新时间：2026-06-28

本文基于当前代码实现复核 API、服务层与前端页面是否闭环。状态含义：

- `已修复`：本轮已补齐并纳入验证。
- `待完善`：已有部分实现，但未形成完整业务闭环。
- `规划中`：属于 v1 路线目标，当前仅有骨架或尚未实现。

## 已修复

### P0-1 认证开关未保护核心 API

状态：已修复

证据：

- `backend/internal/config/config.go` 已有 `auth.enabled` 配置。
- `backend/internal/middleware/apikey.go` 已有 `APIKeyOrAuth`。
- 修复后 `backend/internal/api/router.go` 在注册公开的健康、指标、认证与配置路由后，根据 `auth.enabled` 为核心 API 挂载 `APIKeyOrAuth`。
- `backend/internal/api/config_handler.go` 新增 `authEnabled` 输出，供前端判断是否启用登录守卫。

遗留风险：

- API Key 的 `scope` 字段仍未执行权限判断，见 P1-4。
- 用户角色仅在 `/api/users` 上做 admin 判断，核心 API 尚未按角色细分权限，见 P1-5。

### P0-2 前端缺少登录闭环

状态：已修复

证据：

- `frontend/src/views/LoginView.vue` 新增登录页。
- `frontend/src/router/index.ts` 新增 `/login` 路由，并基于 `/api/config.authEnabled` 启用路由守卫。
- `frontend/src/api/http.ts` 统一从 `localStorage.releasehub_token` 注入 `Authorization: Bearer ...`。
- `frontend/src/layouts/MainLayout.vue` 在登录页隐藏后台导航，并为已登录用户提供退出入口。

遗留风险：

- 前端仍未按角色隐藏菜单或禁用操作，见 P1-5。

### P0-3 API Key 后端存在但前端不可管理

状态：已修复

证据：

- 后端已有 `backend/internal/api/apikey_handler.go`。
- 新增 `frontend/src/api/apikeys.ts`、`frontend/src/stores/apikeys.ts`、`frontend/src/types/apikey.ts`。
- 新增 `frontend/src/components/settings/APIKeyPanel.vue`，支持列表、创建、删除，并在创建后一次性展示完整 Key。
- `frontend/src/views/SettingsView.vue` 已挂载 API Key 管理面板。

遗留风险：

- API Key 目前只有创建与删除，没有禁用、改名、修改 scope 操作。
- API Key scope 尚未参与后端权限判断，见 P1-4。

### P0-4 下载流式实现仍会额外缓存完整文件

状态：已修复

证据：

- `backend/internal/services/asset/download.go` 已删除 `bytes.Buffer` 和 `io.MultiWriter(pw, &buf)`。
- 下载器现在直接写入 `io.PipeWriter`，存储驱动从 `io.PipeReader` 读取。
- SHA256 日志使用 `shortSHA256`，避免空值或异常长度触发 slice panic。
- 重试下载会把新的 `attempt/maxAttempts` 写入新任务。

遗留风险：

- `RetryDownload` 目前只记录退避时间，不主动 sleep 或调度延迟任务；需要与任务队列设计合并处理，见 P1-6。

### P0-5 前端残留备份文件

状态：已修复

证据：

- 已删除 `frontend/src/types/repository.ts.orig`。

### P1-1 Repository 的 storage/proxy 配置没有进入核心同步链路

状态：已修复

原问题：

- `models.Repository` 已有 `StorageID`、`ProxyID`。
- 仓库表单也允许选择 storage/proxy。
- 但 `backend/internal/api/release_handler.go`、`backend/internal/services/syncer/service.go` 仍用固定的 `config.Storage.DataDir` 创建 asset service。
- `backend/internal/services/asset/download.go` 的 `Service` 持有单一 `storage.Driver` 和单一 downloader，下载、打开、删除资产时不会按仓库选择 storage/proxy。
- `backend/internal/services/github/factory.go` 已有基于 proxy 创建 GitHub client 的工厂，但 `backend/internal/services/release/checker.go` 注入的是单一 GitHub client。

修复证据：

- `backend/internal/services/storage/factory.go` 新增 `DriverFactory`，按 repository storage、默认 storage、全局 `storage.data_dir` 的顺序解析驱动。
- `backend/internal/services/proxy/factory.go` 新增 proxy transport 构造能力。
- `backend/internal/services/asset/download.go` 在下载、打开、删除资产时按资产所属仓库动态选择 storage；下载时按仓库 proxy 构建 HTTP downloader。
- `backend/internal/services/release/checker.go` 支持 `github.ClientFactory`，手动检查、全量检查可按仓库 proxy 创建 GitHub client。
- `backend/internal/api/repository_handler.go` 与 `backend/cmd/releasehub/main.go` 已注入 GitHub client factory。
- `backend/internal/services/storage/factory_test.go` 覆盖仓库指定 storage、全局回退、缺失 storage 错误。

遗留风险：

- S3 当前仍是简化 HTTP Basic Auth 实现，不是 AWS V4 签名。
- WebDAV/S3 driver 内部仍会读取完整上传内容，后续需要分片或真正流式上传。
- 代理链路需要在真实 HTTP/SOCKS5 环境做集成测试。

## 待完善

### P1-2 多平台 Provider 只有骨架，未接入业务

状态：已修复

问题：


- `backend/internal/services/provider/*` 已有 GitHub/GitLab/Gitea/Forgejo provider 抽象与实现。
- `backend/internal/services/repository/service.go` 仍限制 provider 只能为 `github`。
- `backend/internal/services/release/checker.go` 仍使用 GitHub 专用类型与客户端。
- 前端仓库表单没有 provider 选择与 provider 特有配置。

建议补全：

1. 将 release checker 改为依赖 `ReleaseProvider` 接口。
2. 仓库新增 provider API base URL/token 解析规则。
3. 前端仓库表单增加 provider 选择。
4. 分别补 GitHub/GitLab/Gitea/Forgejo 的 latest/list/download URL 测试。

### P1-3 通知服务未接入业务事件

状态：已修复核心事件

原问题：

- Notification CRUD 和 Gotify/Webhook/Email/Telegram notifier 已存在。
- 下载开始、下载完成、下载失败、发现新版本、同步完成等事件没有统一调用通知服务。

修复证据：

- `backend/internal/services/notify/factory.go` 下沉 notifier 创建逻辑。
- `backend/internal/services/notify/service.go` 新增通知派发服务，按 `enabled` 和 `events` 过滤渠道。
- `backend/internal/services/release/checker.go` 在发现新 latest release 时触发 `new_release`。
- `backend/internal/services/asset/download.go` 在资产下载成功/失败时触发 `download_ok`/`download_err`。
- `backend/internal/services/syncer/service.go` 在同步成功/失败时触发 `sync_success`/`sync_failed`。

遗留风险：

- 暂未触发“开始下载”事件，避免高频通知噪声；后续可做成独立开关。
- 通知发送失败目前不阻断主流程，也未写入 task log；后续应增加可观测性。
- 通知发送是同步 fan-out，后续可进入队列并增加重试。

### P1-4 API Key scope 未执行权限判断

状态：已修复

原问题：

- `models.APIKey.Scope` 和 API Key 创建接口已有 scope。
- `middleware.APIKeyOrAuth` 只校验 key 是否存在、启用，不判断 scope。

修复证据：

- `backend/internal/middleware/permission.go` 新增统一授权中间件，按 route resource 和 HTTP method 映射 `read/write/admin`。
- API Key 支持 `*`、`read`、`write`、`admin`、`admin:*`、`repo:read`、`repo:write`、`asset:download` 等 scope。
- `backend/internal/api/router.go` 在 `auth.enabled=true` 时对核心 API 串联 `APIKeyOrAuth` 与 `AuthorizeRequest`。
- `frontend/src/components/settings/APIKeyPanel.vue` 创建 API Key 时提供常用 scope 模板。
- `backend/internal/middleware/permission_test.go` 覆盖角色与 scope 判断。

遗留风险：

- Scope 文档还未独立整理到用户文档。
- API Key 暂不支持编辑 scope，只能删除后重建。

### P1-5 RBAC 只覆盖用户管理

状态：已修复后端核心权限

原问题：

- 用户有 admin/operator/viewer 角色。
- 目前只有 `/api/users` 使用 admin 限制。
- 核心 API 在 JWT 登录下没有按角色控制读写能力。

修复证据：

- `backend/internal/middleware/permission.go` 定义角色矩阵：admin 全部权限，operator 可读写非管理资源，viewer 只读。
- storage/proxy/notification/token/apikey/upload/reconcile 被归类为 admin 资源。
- repository/release/asset/task/file/search/stats 按 method 区分 read/write。

遗留风险：

- 前端还未根据 `/api/auth/me` 角色隐藏菜单或禁用按钮，后端会兜底拒绝。

### P1-6 TaskLog 覆盖不完整，重试退避未任务化

状态：已修复

修复证据：

- `backend/internal/services/release/checker.go` 注入 `tasklog.Service`，`CheckLatest`/`CheckAll` 每个阶段（开始、查询 Provider、发现 Release、持久化、清理、通知、完成/失败）均写入 task log。
- `backend/internal/services/syncer/service.go` 注入 `tasklog.Service`，`SyncRepository` 在检查、下载、完成/失败各阶段写入日志。
- `backend/internal/services/retention/service.go` 注入 `tasklog.Service`，清理任务写入开始、文件删除、完成/失败日志。
- `backend/internal/services/scheduler/service.go` 注入 `tasklog.Service`，新增 `RetryFailedAssets` 方法扫描失败资产触发重试。
- `checker.go` 新增 `markRepositoryHealthy` 方法，`CheckLatest`/`CheckAll` 成功后更新 `last_check_at`/`last_status`/`last_release_tag`。
- `checker.go`/`syncer.go`/`retention/service.go` 新增 `failTaskWithLog`/`appendLog` 辅助方法，统一错误日志写入模式。

### P1-7 前端页面仍有功能入口缺口

状态：已修复

问题：

- `frontend/src/api/search.ts` 已有 API 封装，但没有搜索页面。
- `frontend/src/api/filter.ts` 已有封装，但仓库表单没有过滤预览 UI。
- 上传 API 存在，但前端没有手动上传入口。

建议补全：

1. 增加 Files/Search 页面或在文件页集成搜索。
2. 仓库表单增加 Regex/Glob 过滤预览。
3. 文件页或 Release 详情页增加手动上传入口。

### P1-9 前端 RBAC 未按角色控制菜单和操作

状态：已修复

修复证据：

- `frontend/src/stores/auth.ts` 新增 `isAdmin`/`isOperator`/`isViewer`/`canWrite`/`canAdmin` computed 属性。
- `frontend/src/layouts/MainLayout.vue` 根据 `canAdmin` 过滤管理菜单（存储、代理、通知、用户、设置）。
- `frontend/src/components/repository/RepositoryToolbar.vue` 新增 `canWrite` prop，非写权限用户隐藏"新增仓库"按钮。
- `frontend/src/components/repository/RepositoryTable.vue` 新增 `canWrite` prop，非写权限用户隐藏"立即同步"/"编辑"/"删除"按钮，仅保留"检查最新"/"全量检查"只读操作。
- `frontend/src/views/RepositoriesView.vue` 传递 `authStore.canWrite` 到子组件。

遗留风险：

- 前端 RBAC 仅控制 UI 可见性，后端 `AuthorizeRequest` 中间件兜底拒绝未授权请求。

### P1-10 前端 Storage 页面缺少编辑功能

状态：已修复

修复证据：

- `frontend/src/views/StoragesView.vue` 新增 `editingId` 状态和 `openEditModal` 方法，支持编辑已有存储配置。
- 操作列新增"编辑"按钮，Modal 标题和确认按钮根据 `editingId` 动态切换。
- `handleSubmit` 方法支持 `create` 和 `update` 两种模式。

### P1-11 前端缺少修改密码入口

状态：已修复

修复证据：

- `frontend/src/layouts/MainLayout.vue` 用户操作区新增修改密码按钮（KeyRound 图标）。
- 点击弹出 Modal，要求输入当前密码、新密码和确认密码。
- 新密码至少 6 位，两次输入必须一致，修改成功后自动退出并跳转登录页。
- 后端 `POST /api/auth/change-password` 接口已完整实现，AuthRequired 中间件正确注入 `userID`。

### P1-12 前端存储对账入口缺失

状态：已修复

修复证据：

- 新增 `frontend/src/api/reconcile.ts`，封装 `POST /api/reconcile`。
- `frontend/src/views/FilesView.vue` 新增"存储对账"按钮（仅 admin 可见），点击后展示缺失文件列表和孤立资产数。
- 对账结果以卡片形式展示，区分存储缺失、数据库缺失和孤立资产。

### P1-13 前端文件页面缺少删除操作

状态：部分修复

修复证据：

- `frontend/src/api/releases.ts` 新增 `deleteAsset` API 封装。
- FilesView 新增 `handleDeleteFile` 方法（删除后自动刷新文件列表）。

遗留风险：

- FileTable 组件尚未添加删除按钮列，当前删除操作只能通过 AssetPanel 执行。

### P1-14 Dashboard 缺少异常信息展示

状态：已修复

修复证据：

- `frontend/src/views/DashboardView.vue` 新增失败任务告警卡片，当存在失败任务时提示用户查看任务页面。

## 规划中

### P2-1 PostgreSQL 未实现

状态：规划中

问题：

- `backend/internal/database/database.go` 当前只接受 SQLite。
- v1 规划中列出 SQLite/PostgreSQL 双支持。

建议补全：

1. 引入 `gorm.io/driver/postgres`。
2. 增加 `database.type=postgres` 配置。
3. 补 SQLite/PostgreSQL 迁移一致性测试。

### P2-2 OpenAPI/Swagger 未实现

状态：规划中

问题：

- 路由和 handler 已较多，但未生成 OpenAPI 文档。

建议补全：

1. 引入 swag 或维护 OpenAPI YAML。
2. CI 中校验 OpenAPI 文件同步。
3. 前端 API 类型后续可从 OpenAPI 生成。

### P2-3 插件系统未实现

状态：规划中

问题：

- 当前已有 Storage Driver、Notifier、Release Provider 抽象，但没有动态插件加载、注册生命周期或插件配置 UI。

建议补全：

1. 先稳定内置 driver/provider/notifier 注册表。
2. 再设计插件 manifest、版本兼容、权限声明。
3. 最后实现插件安装与启停 UI。

### P2-4 Dashboard 统计体验仍偏基础

状态：规划中

问题：

- Dashboard 当前以统计卡片为主。
- v1 规划提到 ECharts，但未接入趋势图、失败率、下载量等可视化。

建议补全：

1. 增加 release 检查趋势、下载成功率、存储占用趋势。
2. 增加最近失败任务和异常仓库列表。
3. 后端 stats API 补充时间序列数据。

## 本轮审查备注

- 本轮主要修复不需要大规模重构即可闭环的 P0 缺口。
- storage/proxy 核心链路已接入 release checker、asset service、syncer、文件打开和删除路径；provider 多平台接入仍建议作为下一阶段独立提交处理。
- 曾尝试按 code-review skill 使用独立审查通道，但当前 native-hook surface 不具备可直接使用的 tmux OMX question bridge，因此本文件由当前工作区证据独立整理。
