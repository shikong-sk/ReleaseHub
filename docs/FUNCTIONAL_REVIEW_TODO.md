# ReleaseHub 功能审查与待完善清单

更新时间：2026-06-27

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

状态：待完善

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

状态：待完善

问题：

- Notification CRUD 和 Gotify/Webhook/Email/Telegram notifier 已存在。
- 下载开始、下载完成、下载失败、发现新版本、同步完成等事件没有统一调用通知服务。

建议补全：

1. 增加事件模型，例如 `release.discovered`、`asset.download.succeeded`、`asset.download.failed`。
2. 在 checker、asset service、syncer 中发布事件。
3. notification service 根据启用项 fan-out 发送。
4. 失败通知要包含 repo、tag、asset、错误信息、任务 ID。

### P1-4 API Key scope 未执行权限判断

状态：待完善

问题：

- `models.APIKey.Scope` 和 API Key 创建接口已有 scope。
- `middleware.APIKeyOrAuth` 只校验 key 是否存在、启用，不判断 scope。

建议补全：

1. 定义 scope 规范，例如 `repo:read`、`repo:write`、`asset:download`、`admin:*`。
2. 为路由注册权限元数据。
3. middleware 中按 route scope 执行匹配。
4. 前端 API Key 面板增加 scope 模板选择。

### P1-5 RBAC 只覆盖用户管理

状态：待完善

问题：

- 用户有 admin/operator/viewer 角色。
- 目前只有 `/api/users` 使用 admin 限制。
- 核心 API 在 JWT 登录下没有按角色控制读写能力。

建议补全：

1. 定义角色权限矩阵。
2. viewer 只能读 dashboard/files/releases/tasks。
3. operator 可触发检查、同步、下载、重试。
4. admin 可管理用户、存储、代理、通知、API Key。
5. 前端根据 `/api/auth/me` 结果隐藏或禁用不可用操作。

### P1-6 TaskLog 覆盖不完整，重试退避未任务化

状态：待完善

问题：

- 下载流程已有部分 task log。
- 检查 release、批量 sync、scheduler 触发等路径缺少系统性日志。
- `RetryDownload` 目前记录退避时间，但没有延迟执行机制。

建议补全：

1. 为 check/sync/scheduler 每个任务写入开始、阶段、完成、失败日志。
2. 为任务详情页展示 task log。
3. 将 retry 改为任务队列或 scheduler 延迟执行，避免同步请求长时间 sleep。

### P1-7 前端页面仍有功能入口缺口

状态：待完善

问题：

- `frontend/src/api/search.ts` 已有 API 封装，但没有搜索页面。
- `frontend/src/api/taskLogs.ts` 已有 API 封装，但任务页面没有日志详情。
- `frontend/src/api/filter.ts` 已有封装，但仓库表单没有过滤预览 UI。
- 上传 API 存在，但前端没有手动上传入口。

建议补全：

1. 增加 Files/Search 页面或在文件页集成搜索。
2. 任务页增加日志抽屉。
3. 仓库表单增加 Regex/Glob 过滤预览。
4. 文件页或 Release 详情页增加手动上传入口。

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
