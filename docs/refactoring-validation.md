# 首版重构验证记录

日期：2026-11-02。范围：本地构建、离线模拟与容器验证。

已完成以下检查：

- Windows 上 `go test -race ./...`、`go vet ./...`、`go build ./...` 通过。
- 命令入口 `./cmd/oss-auto-cert` 成功构建，帮助、版本与配置工具可离线执行。
- Linux、Windows、macOS 的 amd64 / arm64 发布目标均通过构建检查；跨平台构建不代表已在每个操作系统上实际运行。
- 最小 / 完整 TOML 示例及旧 YAML 示例通过配置校验；迁移测试检查有效策略等价且不固化环境覆盖。
- Linux amd64 测试二进制在断网、UID/GID 为 `65534:65534` 的 Alpine 容器中通过，包含存储权限、锁、恢复流程、SDK 模拟及通知测试。
- amd64 和 arm64 Docker 镜像构建成功；arm64 镜像内二进制已检查 ELF 架构为 AArch64，并在模拟运行环境成功执行 `version`。
- `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...` 未发现可达漏洞；工具自动使用较新的 Go 工具链，此结果不能替代部署时的 Go 运行时补丁检查。
- `integration` 标签测试仅编译验证，工作流 / Compose YAML、TOML 文件和文档本地链接通过检查。
- 日志切换为 logrus `TextFormatter` 后，全量竞态测试、vet、构建和漏洞扫描再次通过；额外验证 INFO / WARN / ERROR 不同颜色、等级过滤、日志实例隔离，以及实际命令强制彩色 / 禁色时迁移输出仍为纯 TOML。Windows 与 Linux amd64 入口构建通过。
- MIT 与生产部署调整后，镜像构建通过，镜像内许可证和 OCI 许可证标签均为 MIT。Compose 的固定镜像及凭证必填校验、非 root 用户、挂载、停止期限、资源限制和日志轮转配置通过解析检查。断网容器中以 UID/GID `10001:10001`、只读根文件系统、受限权限和资源运行，配置校验与存储测试通过；该项使用预设属主的临时 Docker 数据卷，不代替生产宿主机绑定目录的权限检查。

配置版本字段移除后，全量竞态测试、vet 和构建通过；最小 / 完整 TOML 示例与 CLI 生成、迁移结果均已验证无需版本字段，迁移前后策略等价。

CLI 命令及测试移入 `cmd/oss-auto-cert` 后，全量竞态测试、vet 和构建通过；实际构建验证 `-X main.Version` 注入后，`version`、`--version`、旧参数 `-v` 均返回指定版本，帮助命令可离线执行。

CI 工具链修复验证：Go 1.25.14 配合兼容的扫描工具可启动扫描，但报告 12 项可达的标准库漏洞。因此测试、扫描、二进制和 Docker 构建统一升级到 Go 1.26 系列最新补丁，保留 `govulncheck v1.9.0` 并禁止扫描任务自动切换工具链。Windows 上使用 Go 1.26.10、`GOTOOLCHAIN=local` 执行竞态测试、vet 和构建均通过；扫描 Windows 与 Linux amd64 目标均未发现可达漏洞。Linux 目标扫描通过 `GOOS=linux GOARCH=amd64` 在 Windows 上完成，不代表已实际运行 Linux 测试或远端 CI。

核心回归覆盖：OSS 成功而 CDN 失败后重启恢复、上传响应丢失后按证书指纹恢复、等待 CDN 生效时不重复提交、CDN 单独临期、域名失败隔离、未选择域名不处理、只读模式不创建数据目录、账户复用与环境隔离、取消和 HTTP-01 遗留对象清理。

没有运行真实 CA 注册 / 签发、OSS 写入、CAS 上传或 CDN 更新。云 API 权限、真实 HTTP-01 路由和 CDN 边缘传播仍需使用专用测试资源验收。当前 CAS 上传结果无法确定且查询不到记录时会暂停并提示人工核对，尚无人工状态修复命令。

操作与恢复方式见 [使用文档](usage.md)，设计边界见 [重构方案](refactoring-plan.md)。

阿里云 ACR 清单兼容修复（2026-11-03）：发布工作流关闭 provenance / SBOM，并设置 `oci-mediatypes=false`。本地使用包含占位文件的最小镜像，以相同兼容选项导出多架构归档，检查结果为 Docker manifest list、amd64 / arm64 两份 Docker image manifest，以及 Docker image config；没有 OCI attestation 或 empty config。工作流 YAML 和差异格式检查通过；未向远端镜像仓库推送，实际 ACR 接收结果需重跑发布工作流验证。

Release 标签修复（2026-11-03）：手动发布增加必填 `tag`，准备任务解析已有标签的提交，checks、构建和 Release 显式使用相同提交。使用临时 Git 仓库运行工作流中的实际 Bash 脚本，验证标签推送、从较新分支手动发布旧标签、附注预发布标签均解析正确；缺失标签、分支名、空输入、分支触发及带命令或换行的输入均被拒绝。修改的两个工作流通过 actionlint v1.7.12 检查（未启用 shellcheck / pyflakes）。未创建远端标签或 Release，远端运行结果待发布工作流验证。
