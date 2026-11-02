# 使用、升级与恢复说明

## 1. 配置和运行

```bash
go build -o oss-auto-cert ./cmd/oss-auto-cert
./oss-auto-cert config init --output=config.toml
./oss-auto-cert config validate --config=config.toml
```

根目录兼容入口仍支持 `go build -o oss-auto-cert main.go` 和 `go build .`。配置工具和帮助不需要阿里云凭证；运行、检查和预演需要通过环境变量提供凭证。

Linux / macOS：

```bash
export OSS_ACCESS_KEY_ID="your-access-key-id"
export OSS_ACCESS_KEY_SECRET="your-access-key-secret"
```

PowerShell：

```powershell
$env:OSS_ACCESS_KEY_ID = "your-access-key-id"
$env:OSS_ACCESS_KEY_SECRET = "your-access-key-secret"
```

程序不自动读取 `.env`。最低 Go 版本为 1.25，正式发布应使用对应系列最新补丁版本构建。

先运行 `run --dry-run` 查看范围和计划，再执行 `run --once`；常驻使用 `run` 或不带子命令的原启动方式。预演不会创建账户、验证文件、运行锁、证书和状态，也不能证明写权限及完整 HTTP-01 验证可用。

## 2. 配置选择与覆盖

- 指定 `--config` 时只读取该文件，不存在则报错。
- 未指定时，先找 `/etc/oss-auto-cert/`，其次找当前目录，每处识别 `config.toml`、`config.yaml`、`config.yml`。
- 同一位置存在多份标准配置时报错，要求显式指定。
- `.toml` 要求 `version = 2`；`.yaml` / `.yml` 读取旧版，允许省略版本或 `version: 1`。
- 新版未知字段、错误类型及无效环境覆盖直接报错，不换解析器兜底。
- 环境变量覆盖文件值；命令行只覆盖明确传入的对应选项。

| 环境变量 | 用途 |
| --- | --- |
| `OSS_ACCESS_KEY_ID` / `OSS_ACCESS_KEY_SECRET` | 阿里云访问凭证 |
| `ACME_EMAIL` | 覆盖联系邮箱 |
| `ACME_DATA_DIR` | 覆盖整个持久化数据目录 |
| `ACME_EXPIRED_EARLY` | 覆盖提前续期天数 |
| `DEBUG=true` | 仅旧 YAML 保留 staging + 强制续期行为；TOML 忽略并提示 |

日志使用 `github.com/sirupsen/logrus` 的 `TextFormatter`，包含完整时间戳、日志级别及 Bucket / 域名 / 阶段等上下文。运行日志与警告写入 stderr；帮助、版本、状态和配置导出正文写入 stdout。支持 `trace`、`debug`、`info`、`warn`、`error` 等级，默认 `info`。

`--color=auto` 默认在终端使用不同颜色，在文件或管道中关闭颜色；设置 `NO_COLOR` 也会在 auto 模式禁用颜色。可用 `--color=always` 强制彩色输出（例如 Docker 日志），或 `--color=never` 禁用。示例：

```bash
./oss-auto-cert run --config=config.toml --log-level=debug --color=always
```

日志调试使用 `--log-level=debug`。生产与 staging 账户、证书和状态分开保存，但显式 staging 运行仍可能把测试证书部署到所选目标，应使用专用测试资源。

## 3. TOML 参数

最小示例见 [simple-config.toml](../simple-config.toml)，完整示例见 [config.full.toml](../examples/config.full.toml)。

| 配置 | 默认值及说明 |
| --- | --- |
| `storage.data-dir` | `/var/lib/oss-auto-cert`，写入时必须有权限并持久化 |
| `schedule.interval` | `6h`，正的 Go 时长字符串 |
| `schedule.run-on-start` | `true`，常驻启动时立即执行 |
| `renewal.before-days` | `15`，新版接受 1 至 60 天；依据真实到期时刻 |
| `acme.email` | 联系邮箱，可由环境变量提供 |
| `acme.environment` | `production`，测试使用 `staging` |
| `acme.provider` / `acme.key-type` | 本轮仅 `letsencrypt` / `rsa2048` |
| `acme.timeout` | `2m`，每次签发期限 |
| `runtime.workers` | `2`，1 至 16 个并发域名任务，签发按账户串行 |
| `runtime.request-timeout` | `30s`，单次请求期限 |
| `runtime.shutdown-timeout` | `30s`，收到信号后等待任务收尾的期限 |

Bucket 的选择与部署：

| 配置 | 行为 |
| --- | --- |
| `name` / `endpoint` | 已有 Bucket 名称与 endpoint，推荐 HTTPS |
| `domain-mode = "listed"` | 默认模式，只管理 `domains` 列出的已绑定域名 |
| `domain-mode = "all"` | 管理 Bucket 全部已绑定域名，不同时填写 `domains` |
| `domain-mode = "first"` | 旧版范围，只管理 API 返回的第一个域名 |
| `allow-initial-issuance` | 默认 `false`，首次绑定证书需显式开启 |
| `challenge.type` | 仅支持 `oss-http-01`，域名验证路径需公网可访问 |
| `deploy.oss` | 默认 `true`，设置 `false` 不更换 OSS 绑定证书 |
| `deploy.cdn` | `auto` 或 `off`；auto 只更新已开启 HTTPS 的在线 CDN |

`listed` 支持的域名采用 ASCII 格式；本轮不支持通配符，每个域名单独签发。不能在不同 Bucket 重复管理同一域名。首次开启 `allow-initial-issuance` 不会创建新的 OSS 域名绑定，也不会开启新的 CDN HTTPS。

`deploy.cdn = "off"` 可以完全避免 CDN 访问权限要求；auto 模式中的权限错误不会被伪装成“未使用 CDN”。

## 4. 命令与退出码

```bash
./oss-auto-cert --help
./oss-auto-cert version
./oss-auto-cert config validate -config=config.yaml
./oss-auto-cert run --config=config.toml --dry-run
./oss-auto-cert run --config=config.toml --once
./oss-auto-cert status --config=config.toml
```

`--config` 和 `--log-level` 可以放在子命令前后；旧 `-config`、`-log-level`、`-help` 和迁移命令的 `-output` 也支持。重复参数报错，未知子命令不会启动服务。

强制签发只能使用 `run --once --force` 或预演 `run --dry-run --force`。已有未完成部署时，优先恢复已签发证书，不通过强制选项清除未完成任务。

| 退出码 | 含义 |
| --- | --- |
| `0` | 命令成功，或常驻收到信号后正常收尾 |
| `1` | 执行失败、部分任务待重试，或退出等待超时 |
| `2` | 参数、配置、凭证或本地存储前提错误 |

`status` 读取本地快照，显示检查时间和错误，不向云端刷新。常驻模式中单个域名失败不影响其他域名；不可恢复的本地存储错误会停止运行。

## 5. 通知

```toml
[[notifications]]
type = "wecom"
url-env = "CERT_WEBHOOK_URL"
events = ["updated", "failed", "recovered"]
```

在环境中设置 `CERT_WEBHOOK_URL`。内置 `wecom`、`dingtalk`、`feishu` 格式通过 JSON 编码生成消息。没有通知配置时，处理结果仍输出至本地。

自定义模板：

```toml
[[notifications]]
type = "template"
url-env = "CUSTOM_WEBHOOK_URL"
template = '''{"text": {{ json .Message }}}'''
```

`json` 返回包含引号的完整 JSON 字符串，不要再次包裹引号。通知有独立超时和有限重试，网络错误不会导致 panic；内置渠道检查 HTTP 状态及平台业务状态。失败事件按状态去重，恢复后单独通知。

旧 `webhook-tpl` 转为 `legacy-template`，保留 `.Message` 和原 HTML 模板渲染语义。旧模板遇到换行等内容可能生成无效 JSON，`config validate` 会指出问题；运行时该渠道报错，不发送无效请求，不中断证书部署。迁移不会擅自重写自定义模板。

## 6. 恢复与持久化

账户、证书和逐目标状态存放在同一数据目录，不再退出注销 ACME 账户。一个数据目录只允许一个写进程；运行锁由操作系统释放，不能删除正在使用的锁文件来启动第二个实例。

证书使用指纹标识完整版本，私钥在 Unix 下为 `0600`，存储目录为 `0700`；Windows 对新写文件设置当前用户、SYSTEM 和管理员可访问的 ACL。

OSS 成功而 CDN 失败时，下次巡检或重启后只恢复 CDN，复用证书和 CAS ID。网络 / 限流等暂时错误采用带抖动的指数退避；单轮执行不等待未来所有重试，常驻进程负责按计划恢复。权限错误和确定性配置错误在后续巡检重新评估，不立即循环写入。

CAS 上传响应丢失时，先按稳定名称查询已上传记录，并核对证书内容的 SHA-256 指纹。查不到结果时暂停上传并报告名称，避免盲目重复创建；核对远端后可以再次运行以查找已生效记录。当前不提供自动清除不明确上传意图的命令。确需恢复该任务时，先停止服务、备份整个目录并核对 CAS，联系维护者处理对应状态；不要直接删除全部数据重试。

外部 API 与本地文件之间没有事务：不能承诺恰好执行一次，尤其是 CA 已签发而本地尚未保存时进程退出。需要结合日志、状态与云端结果核对。CDN 部署核验基于云 API 返回的证书绑定及状态，不代表所有边缘节点已完成传播。

## 7. 旧版升级与回退

1. 停止旧进程，备份旧二进制、配置文件及整个数据目录。
2. 用新程序显式加载旧 YAML，先执行 `config validate` 和 `run --dry-run`。
3. 检查域名范围仍为 first、续期提前天数仍为旧规则，再执行一轮。
4. 可选运行迁移，修改服务启动参数明确选择新 TOML，移除旧 `DEBUG=true`。
5. 观察 `status` 和云端证书绑定，再启用常驻运行。

旧 `<data-dir>/<domain>/cert.crt`、`cert.key`、`cert-issuer.crt`、`cert.csr` 不会被删除。需要续期时，新程序可校验并导入仍可复用的旧证书；不复用旧 CSR 驱动签发。来源无法验证或文件损坏时给出错误，核对后可显式强制签发。新程序使用的新账户和状态不会回写为旧版格式。

迁移命令不读取当前环境覆盖、不连接云端，也不覆盖已有输出文件。原来配置超过新版 60 天范围或没有 Bucket 时，迁移工具需提示调整，不能将不合法配置作为成功结果交付。

回退时先停止新进程，再恢复备份的旧二进制、旧配置及完整旧数据目录。旧程序不能理解新存储。新程序如果已改变云端证书，回退本地文件不会恢复云端旧证书，应单独核对 OSS / CDN；不要自动撤销仍在使用的新证书。

旧的 Go 包导入 API 已删除，应用配置和命令行兼容不等于源码 API 兼容。

## 8. Docker、权限与 Systemd

Compose 默认配置文件只读挂载到 `/etc/oss-auto-cert/config.toml`，数据挂载到 `/var/lib/oss-auto-cert`，停止宽限为 45 秒。

保持原镜像默认用户，避免升级时静默改变旧卷权限。新装环境可准备归属于专用 UID/GID 的可写数据目录，再使用 Docker 的 `--user UID:GID`；应用不会递归修改宿主目录。不要把容器中的路径与宿主目录混淆。

Systemd 示例：

```ini
[Unit]
Description=OSS certificate manager
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=/etc/oss-auto-cert/credentials.env
ExecStart=/usr/bin/oss-auto-cert run --config=/etc/oss-auto-cert/config.toml
Restart=on-failure
RestartSec=10
TimeoutStopSec=45

[Install]
WantedBy=multi-user.target
```

`credentials.env` 保存阿里云环境变量，应只允许服务用户读取。若配置专用 `User`，需提前准备数据目录及属主。

RAM 权限按实际操作授予：读取 Bucket CNAME、上传 / 删除本程序验证文件、更新 OSS 证书、上传 / 查询 CAS（含证书详情），auto 模式还需读取 CDN 域名与证书、更新 CDN HTTPS 证书。完整 HTTP-01 写权限不能通过纯只读预检证明。

## 9. 开发检查

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
```

漏洞工具可能自动下载更新的 Go 工具链。默认测试使用模拟云服务、本地 HTTP 服务器和自签测试证书。真实云资源集成测试必须显式启用 `integration` build tag 并使用专用测试 Bucket，发布前仍需在真实测试环境核验云 API、HTTP-01 和边缘生效情况。
