# 用户指南

本文面向 ReleaseHub 的最终使用者，介绍日常使用流程与各功能模块的操作方式。

## 一、登录与账号

### 默认账号

启用认证后（在「设置 → 全局配置」中开启），使用默认账号登录：

- 用户名：`admin`（可通过 `RELEASEHUB_AUTH_DEFAULT_ADMIN` 配置）
- 密码：`admin`（可通过 `RELEASEHUB_AUTH_DEFAULT_PASSWORD` 配置）

**首次登录后请立即在「设置」页修改密码。**

### 角色与权限

| 角色 | 权限范围 |
| --- | --- |
| admin | 全部权限，含用户管理、存储/代理/通知配置 |
| operator | 可读写仓库/Release/资产，不可管理基础设施 |
| viewer | 只读访问所有资源 |

菜单与操作按钮会按当前用户角色自动过滤/隐藏。

### 修改密码

登录后进入「设置」页，点击「修改密码」，填入当前密码与新密码（6–128 位）。

## 二、GitHub Token 管理

1. 进入「设置」页的 **GitHub Token** 面板。
2. 点击「添加」，填写名称和 Token 值。
3. 保存后 Token 值不再展示，仅显示 hint（如 `ghp_****abcd`）。
4. 可点击「健康检查」查看 Token 当前状态和剩余 Rate Limit。

Token 用于调用 GitHub API，私有仓库或避免 API 限流时必需。

## 三、仓库管理

### 添加仓库

进入「仓库」页，点击「新增仓库」：

| 字段 | 说明 |
| --- | --- |
| Provider | GitHub / GitLab / Gitea / Forgejo |
| Owner | 仓库所有者 |
| Repo | 仓库名 |
| GitHub Token | 关联的 Token（可选） |
| 存储目标 | 选择一个或多个存储（可选，默认使用本地存储） |
| 代理 | API 与下载请求使用的代理（可选） |
| Provider API Base URL | 自托管实例地址（可选） |
| 同步间隔 | 该仓库的检查间隔（秒，默认 1800） |
| 过滤模式 | `glob` 或 `regex` |
| 包含/排除规则 | 匹配规则，逗号或换行分隔 |
| 保留最近 N 版 | 保留策略（默认 5） |

### 同步操作

| 操作 | 说明 |
| --- | --- |
| 检查最新 | 仅查询远程最新 Release，不下载 |
| 全量检查 | 查询远程所有 Release，不下载 |
| 立即同步 | 检查并下载最新 Release 的资产 |
| 同步指定 Tag | 弹窗选择远程 Tag，下载指定版本 |
| 历史 | 查看该仓库所有 Release 及其资产 |

Scheduler 默认每 60 秒扫描所有启用仓库，自动执行检查 + 下载。

### 过滤规则示例

- Glob：`*.tar.gz`、`linux-amd64`
- Regex：`linux-amd64\.tar\.gz$`
- 排除：`*.sha256`、`*.sig`

可在「设置」页使用「过滤预览」验证规则匹配结果后再应用到仓库。

## 四、Release 与 Asset

- **置顶（Pin）**：固定某个版本，使其不受保留策略清理
- **取消置顶**：解除固定
- **删除 Release**：硬删除该 Release 及其所有资产
- **资产状态**：pending / skipped / downloading / downloaded / verified / failed / deleted
- **资产操作**：下载、重新下载（失败后重试）、删除

资产下载失败会自动重试（指数退避），可在「任务」页查看错误信息。

## 五、文件浏览

- **文件页**：以树形结构浏览所有已同步文件，支持搜索和下载
- 文件树采用后端分层懒加载 + 前端 NTree 组件，适合大量文件场景
- 可在文件树节点上直接下载或删除文件

## 六、存储管理

进入「存储」页管理存储目标：

| 类型 | 必填字段 |
| --- | --- |
| local | BasePath（默认 `data/releases`） |
| s3 | Endpoint、Bucket、Region、AccessKey、SecretKey、BasePath |
| webdav | RemoteURL、Username、Password、BasePath |

- 创建后可点击「测试」验证连通性
- 可标记某个存储为「默认」，未指定存储的仓库会使用它
- 一个仓库可关联多个存储，资产会分发写入到每个存储

## 七、代理管理

进入「代理」页：

1. 选择类型：HTTP / HTTPS / SOCKS5
2. 填写地址、端口和认证信息
3. 点击「测试」查看连通性和延迟
4. 在仓库表单中关联代理

代理同时作用于 GitHub API 请求和资产下载请求。

## 八、通知配置

进入「通知」页：

1. 选择类型：Gotify / Webhook / Email / Telegram
2. 填写对应的服务器地址和 Token
3. 勾选要订阅的事件：
   - `new_release`：发现新版本
   - `download_ok`：下载成功
   - `download_err`：下载失败
   - `sync_success`：同步完成
   - `sync_failed`：同步失败
4. 保存并启用
5. 点击「发送测试」验证通知是否可达

通知发送失败会记录到推送历史（`NotificationLog` 表）和任务日志。同一渠道+事件 5 分钟内自动去重。当前为同步 fan-out，后续规划进入队列并增加重试。

## 九、存储对账（Reconcile）

进入「文件」页触发对账（管理员功能）：

- **默认 dry-run 安全预检**：仅报告不一致项，不修改任何数据
- **安全修复模式**（`dryRun=false`）：
  - 存储中存在但数据库无记录的文件 → 自动补建 DB 记录
  - 数据库有记录但存储中缺失的文件 → 重置状态为 `pending` 以便重新下载

对账按存储维度扫描，正确区分不同存储上的同名文件。

## 十、Dashboard

Dashboard 展示概览数据：

- 仓库总数 / 启用数
- 已同步文件数
- 失败任务数与告警

趋势图展示近 30 天的 Release 新增和资产下载趋势（基于 Chart.js Bar 组件）。

## 十一、API Key

适合脚本/CI 调用 API 时使用：

1. 进入「设置」页的 **API Key** 面板。
2. 点击「添加」，填写名称和 scope。
3. 保存时一次性返回完整 Key 值，请妥善保存（后续不再展示）。

Scope 取值：`*` / `read` / `write` / `admin` / 细粒度如 `repo:read,asset:download`。

调用时在请求头携带 `X-API-Key: <key>`。

## 下一步

- [API 参考](../api-reference.md)
- [完整配置参考](../configuration.md)
- [部署指南](../deployment/deployment.md)
