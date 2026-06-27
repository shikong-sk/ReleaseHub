# ReleaseHub 功能审查与待完善清单

更新时间：2026-06-27

本文跟踪当前仍需完善的功能缺口。已修复的条目已归档至 git 历史。

## 待完善

### P1-13 前端文件页面删除按钮

状态：部分修复

说明：`FilesView` 已有 `handleDeleteFile` 方法和 `deleteAsset` API 封装，但 FileTable 组件尚未添加删除按钮列，当前删除操作只能通过 AssetPanel 执行。

建议：在 FileTable 操作列增加删除按钮。

### P1-2 多平台 Provider 业务链路

状态：待完善

说明：
- `backend/internal/services/provider/*` 已有 GitHub/GitLab/Gitea/Forgejo provider 抽象与实现。
- Release checker 仍使用 GitHub 专用类型与客户端，未完全切换到 `ReleaseProvider` 接口。
- 前端仓库表单已有 provider 选择和 API Base URL 输入。
- 需要验证 GitLab/Gitea/Forgejo 的完整同步流程。

建议：
1. 将 release checker 改为依赖 `ReleaseProvider` 接口。
2. 补充各 Provider 的 latest/list/download URL 集成测试。

### P1-3 通知发送可观测性

状态：核心已完成

遗留：
- 通知发送失败日志已记录（`zap.Warn`）。
- 通知发送是同步 fan-out，后续可进入队列并增加重试。
- 暂未触发"开始下载"事件，避免高频通知噪声。

## 规划中

### P2-1 PostgreSQL 未实现

说明：
- `backend/internal/database/database.go` 当前只接受 SQLite。
- v1.0 规划中列出 SQLite/PostgreSQL 双支持。

建议：
1. 引入 `gorm.io/driver/postgres`。
2. 增加 `database.type=postgres` 配置。
3. 补 SQLite/PostgreSQL 迁移一致性测试。

### P2-2 OpenAPI/Swagger 未实现

说明：
- 路由和 handler 已较多，但未生成 OpenAPI 文档。

建议：
1. 引入 swag 或维护 OpenAPI YAML。
2. CI 中校验 OpenAPI 文件同步。
3. 前端 API 类型后续可从 OpenAPI 生成。

### P2-3 插件系统未实现

说明：
- 当前已有 Storage Driver、Notifier、Release Provider 抽象，但没有动态插件加载、注册生命周期或插件配置 UI。

建议：
1. 先稳定内置 driver/provider/notifier 注册表。
2. 再设计插件 manifest、版本兼容、权限声明。
3. 最后实现插件安装与启停 UI。

### P2-4 Dashboard 统计可视化

说明：
- Dashboard 当前以统计卡片为主。
- 后端已有 `GET /api/stats/trend` 返回时间序列数据。
- 前端未调用此接口，未展示趋势图。

建议：
1. 前端调用 `getTrendStats` API。
2. 引入 ECharts 或轻量图表库。
3. 展示 Release 检查趋势和下载量趋势。
4. 增加最近失败任务和异常仓库列表。

### P2-5 S3 签名实现

说明：
- S3 当前为简化 HTTP Basic Auth 实现，非 AWS V4 签名。
- 可能导致部分 S3 兼容存储（如 AWS S3 本身）无法正常使用。

建议：
1. 引入 `aws-sdk-go-v2` 或实现 V4 签名。
2. 补充 AWS S3 真实环境测试。

### P2-6 WebDAV/S3 流式上传

说明：
- WebDAV/S3 driver 内部仍会读取完整上传内容后再上传。
- 大文件可能占用较多内存。

建议：
1. 实现分片或真正流式上传。
2. 设置上传大小限制和内存水位。
