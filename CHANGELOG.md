# Changelog

所有项目的显著变更都将记录在此文件中。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)，
并且该项目遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [未发布]

### 新增

- 企业微信 / 钉钉内置 Markdown 通知、飞书内置状态卡片，按事件展示域名、Bucket、环境、证书到期时间及部署状态；自定义模板和旧 YAML 模板正文保持兼容。
- TOML 配置，提供最小与完整示例；旧 YAML 通过适配器转换到统一配置对象。
- Cobra 命令：配置生成、离线校验、迁移、只读检查、单轮运行和本地状态查询。
- ACME 账户持久化、生产 / staging 数据隔离、单写者锁和原子文件替换。
- 按域名选择管理范围，首次签发需显式授权，OSS / CDN 部署分别保存进度和重试。
- logrus TextFormatter 日志，支持等级过滤、业务上下文及 `--color=auto|always|never`。
- 默认离线测试、Linux / Windows 检查、漏洞扫描及升级回退文档。

### 调整

- 配置示例补充字段说明、默认值、多 Bucket 与通知用法，`config init` 生成带注释的常用配置，并说明 Compose 通知变量的传入方式。
- 配置格式由文件扩展名区分，TOML 不再包含 `version` 字段，生成和迁移配置同步简化。
- 开源协议由 Apache-2.0 改为 MIT，容器内包含许可证并设置 OCI 许可证标签。
- 生产 Compose 使用固定镜像、非 root 用户、只读根文件系统、资源限制和日志轮转，提供环境变量模板。
- 程序入口、Cobra 命令及 CLI 测试统一放入 `cmd/oss-auto-cert`，使用 `go build ./cmd/oss-auto-cert` 构建，保留旧启动参数和原环境变量兼容。
- 日志库由 charmbracelet/log 改为 `github.com/sirupsen/logrus`；应用日志写入 stderr，配置导出等命令正文保留 stdout。
- 重组内部包，移除旧 `pkg/utils`、`pkg/webhook` 等 Go 导入 API；源码调用方需要适配。
- Docker 按目标架构编译，统一配置和数据挂载路径；标签发布构建触发提交，并注入版本信息。

### 修复

- 手动发布 Release 时显式指定已有版本标签，校验、构建和发布统一使用该标签提交，避免缺失标签或将分支名误作版本号。
- 镜像发布关闭 provenance / SBOM 附加清单，使用 Docker 清单格式，兼容阿里云 ACR 并保留 amd64 / arm64 多架构镜像。
- CI、二进制和 Docker 构建统一使用 Go 1.26 系列最新补丁，满足 `govulncheck v1.9.0` 的最低版本要求并包含标准库安全修复。
- OSS 成功而 CDN 失败后无法恢复的问题，以及仅检查 OSS 有效期时遗漏 CDN 临期的问题。
- 通知网络错误处理、JSON 特殊字符转义和文件权限问题。
- 上传响应丢失后重复创建 CAS 记录的问题，恢复时核对名称和证书指纹；结果仍不明确时暂停并提示核对。

### 升级说明

- 旧 YAML 继续按 first 域名范围和最低提前 15 天策略运行；迁移导出不固化环境覆盖值。
- 新版 TOML 忽略旧 `DEBUG=true`；调试日志使用 `--log-level=debug`，测试环境显式选择 staging。
- 旧配置超出 TOML 约束时需先调整，迁移工具不会输出不合法的新配置。
- 当前继续使用 Let's Encrypt 和 OSS HTTP-01，暂不支持 ZeroSSL、DNS-01、通配符及 DCDN。

使用和回退步骤见 [使用说明](docs/usage.md)，日志配置见 [日志说明](docs/logging.md)。以上改动尚未发布到版本标签，现有仓库镜像可能仍为旧版本。

## [v0.0.9] - 2024-07-??

### 新增

- 优化日期过期检测逻辑

### 修复

- 修复未配置证书保存路径时默认存储路径为空的问题
- 修复 CDN 证书信息过期时间检查不一致问题

### 依赖更新

- 升级 `mikepenz/release-changelog-builder-action` 从 4 到 5
- 升级 `docker/build-push-action` 从 5 到 6

## [v0.0.8] - 2024-06-??

### 新增

- 添加调试模式环境变量开关 `DEBUG`
- 完善 Debug 日志输出

## [v0.0.7] - 2024-06-??

### 新增

- 启动时立即执行证书检测
- 添加 webhook 通知兼容性支持
- 添加 Dependabot 配置

## [v0.0.6] - 2024-06-??

### 修复

- 修复 lego 注册 ACME 次数限制问题

## [v0.0.5] - 2024-05-??

### 新增

- 证书续期时间支持可配置
- 添加 Docker Hub 镜像支持
- ACME 相关配置支持环境变量

### 修复

- 添加 CI 读取 PR 权限

## [v0.0.4] - 2024-05-??

### 新增

- 优化开发体验，添加开发证书申请环境变量支持
- 优化 Changelog 生成

### 文档

- 更新说明文档

## [v0.0.3] - 2024-05-??

### 新增

- 更新 CI 配置
- 添加 Changelog 构建配置

## [v0.0.2] - 2024-04-??

### 新增

- 添加 Dockerfile 支持容器化部署
- 添加 CI/CD 自动发布配置
- 添加 Webhook 通知功能（企业微信、钉钉等）
- 添加阿里云 DCDN 支持
- 实现证书申请续签和 OSS 证书更新功能
- 添加 OSS 自定义域名证书关联测试
- 添加文件操作工具函数
- 切换日志库为 charmbracelet/log
- 添加 UML 架构图

### 文档

- 完善项目说明文档
- 添加 LICENSE

## [v0.0.1] - 2024-04-??

### 新增

- 项目初始化
- 实现基础的阿里云 OSS 证书管理功能
- 集成 Let's Encrypt ACME 客户端 (lego)
- 实现配置加载功能
- 添加阿里云 OpenAPI 集成

[未发布]: https://github.com/nekoimi/oss-auto-cert/compare/v0.0.9...HEAD
[v0.0.9]: https://github.com/nekoimi/oss-auto-cert/compare/v0.0.8...v0.0.9
[v0.0.8]: https://github.com/nekoimi/oss-auto-cert/compare/v0.0.7...v0.0.8
[v0.0.7]: https://github.com/nekoimi/oss-auto-cert/compare/v0.0.6...v0.0.7
[v0.0.6]: https://github.com/nekoimi/oss-auto-cert/compare/v0.0.5...v0.0.6
[v0.0.5]: https://github.com/nekoimi/oss-auto-cert/compare/v0.0.4...v0.0.5
[v0.0.4]: https://github.com/nekoimi/oss-auto-cert/compare/v0.0.3...v0.0.4
[v0.0.3]: https://github.com/nekoimi/oss-auto-cert/compare/v0.0.2...v0.0.3
[v0.0.2]: https://github.com/nekoimi/oss-auto-cert/compare/v0.0.1...v0.0.2
[v0.0.1]: https://github.com/nekoimi/oss-auto-cert/releases/tag/v0.0.1
