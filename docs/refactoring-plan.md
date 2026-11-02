# oss-auto-cert 全面重构方案

日期：2026-11-02。

状态：已按本方案实现首版重构，使用方法见 [使用文档](usage.md)，已执行的检查见 [验证记录](refactoring-validation.md)。本文保留设计合同；本地验证不等同于真实云端签发与部署验收。日志参数及输出规则见 [日志说明](logging.md)。

首版实现包括 Cobra 命令、TOML / YAML 统一配置、旧参数兼容、账户及证书存储、独立部署恢复、通知和发布流程。HTTP-01 验证实现放在 `internal/provider/acme`，未单独拆出 `challenge` 包。CAS 上传结果无法确定时暂停任务并提示核对，暂未提供人工状态修复命令。超出新版约束的旧配置在迁移时须明确报错，原 YAML 仍按兼容规则运行。

## 1. 已确定的方向

重新设计配置、执行流程、持久化和命令行，替换现有业务实现。继续复用 lego、阿里云 SDK 等基础依赖。

允许重新组织项目目录、包边界和程序入口，不要求沿用当前结构。命令行框架确定采用 `github.com/spf13/cobra`；配置读取与覆盖规则仍由独立配置模块管理。

新版主配置采用 `config.toml`，旧版 `config.yaml` / `config.yml` 通过专用解析器和转换适配器接入。核心业务只消费一个统一的内部配置对象，不读取旧字段，不根据配置版本分支执行。

本轮继续使用 Let's Encrypt，通过 ACME 协议申请证书，使用 lego 作为客户端。不接入 ZeroSSL，不增加 EAB 凭证要求，也不实现多 CA 自动切换。

重构完成后的主要体验：

- 原用户保留配置和启动命令即可升级，主动迁移时可以导出 TOML。
- 新用户可以生成配置、离线校验、只读检查，再执行一轮或常驻运行。
- 用户能够看清哪些域名受管理、哪些目标已更新、哪些任务等待重试。
- 签发、CAS 上传、OSS 部署和 CDN 部署可以分别恢复，单个目标失败不导致重复申请证书。

## 2. 范围与边界

| 本轮交付 | 后续扩展 |
| --- | --- |
| TOML v2 与旧 YAML 兼容适配 | ZeroSSL、Google Public CA、自定义 CA |
| Let's Encrypt 生产与测试环境分离 | 多 CA 自动切换及 EAB 管理 |
| OSS HTTP-01 验证 | 阿里云 DNS-01、通配符证书 |
| 每个域名单独签发、多个域名分别管理 | 多域名合并为一张 SAN 证书 |
| CAS 上传、OSS / CDN 独立部署与重试 | 其他云平台、DCDN 和插件系统 |
| 配置生成、校验、迁移、只读检查、状态查询 | Web 控制台、配置热加载 |
| 文件持久化、同一数据目录的单写者锁 | 数据库、跨主机任务协调 |
| Linux 容器和现有发布平台的构建验证 | 分布式运行 |

程序仍以已绑定 OSS 的自定义域名为管理入口。首次绑定证书需要显式开启；不会自动绑定新的 OSS 自定义域名，也不会自动开启 CDN HTTPS。

## 3. 当前实现需要解决的问题

以下是基于当前仓库审查确认的改造输入，不是待保留的行为。

| 当前问题 | 重构要求 |
| --- | --- |
| `main.go` 在 `init()` 内解析参数、加载配置，构造 ACME 客户端时立即注册账户 | 帮助、版本、配置工具和只读操作均不触发注册、签发或部署 |
| Webhook 请求失败后继续读取空响应，可能导致整个进程退出 | 通知错误隔离，显式返回错误，设置超时并关闭响应体 |
| 默认通知模板把实际换行插入 JSON 字符串 | 内置通知使用 JSON 序列化，覆盖换行、引号、反斜杠和中文 |
| 证书目录使用 `os.ModeDir`，私钥使用 `os.ModePerm` | Unix 目录 `0700`、私钥 `0600`；Windows 使用适配的 ACL 策略 |
| 判断 OSS 证书未过期后，不再重试失败的 CDN 更新 | 签发判断与部署判断分开，逐目标持久化和重试 |
| 只处理 Bucket 返回的第一个域名 | 保留旧范围，提供显式域名列表和全部发现模式 |
| ACME 账户每次启动新建，退出时注销 | 账户持久化复用，正常退出不注销账户 |
| 后台保存和部署任务没有统一等待及取消机制 | 明确任务生命周期、超时、并发限制和退出期限 |
| Docker 固定编译 amd64，工作流声明 arm64 | 使用目标平台参数，校验镜像内二进制架构 |
| Compose 数据目录与挂载目录不一致 | 配置、环境变量、挂载位置和权限保持一致 |
| 标签发布工作流固定检出 master | 发布内容对应触发版本的提交 |
| 测试主要打印结果，部分测试直接修改真实云资源 | 默认离线测试含有效断言，云端测试显式启用 |

当前 `go.mod` 声明 Go 1.25.0，开发文档仍写 Go 1.23，实施时应统一。依赖漏洞修复应作为发布检查的一部分，不能仅以编译通过作为交付标准。

## 4. 配置加载设计

### 4.1 单一内部模型

```text
旧 YAML → LegacyConfig → LegacyAdapter ──┐
                                        ├→ 配置草稿 → 环境与参数覆盖
新 TOML → TOMLConfigV2 → TOMLAdapter ─────┘
        → 补全默认值 → 校验与归一化 → Config → 核心业务
```

“合并到新配置对象”指将选中文件的显式配置覆盖到统一模型的默认值上，不是同时读取并混合两个配置文件。

- `LegacyConfig` 只描述旧 YAML 的字段和读取行为。
- `TOMLConfigV2` 只描述新版文件格式。
- 解析用结构需要保留字段是否出现的信息，例如指针或显式 optional 类型。
- 不能通过“值是否为零”判断是否填写，尤其是 `false`、`0` 和空数组。
- `Config` 保存已解析的时长、续期策略、域名选择策略和部署目标等运行参数。
- 字段来源和兼容警告保存在加载结果中，用于解释有效配置，不作为业务版本标志。
- 格式识别、兼容转换及历史默认值处理均在配置边界完成，核心业务不出现 `if legacy`。
- 移除 `expiredEarlyDay` 等全局可变配置，加载过程不调用 `os.Exit` / `log.Fatal`。

### 4.2 格式、版本与文件发现

1. 显式传入 `-config` / `--config` 时，只读取指定文件；文件不存在直接报错，不回退搜索。
2. `.toml` 使用新版解析器，要求 `version = 2`。
3. `.yaml` / `.yml` 使用旧版解析器，接受省略版本或 `version: 1`。
4. 暂不接受 v2 YAML、v1 TOML、未知版本和未知扩展名，给出明确错误。
5. TOML 解析失败不得尝试 YAML，版本错误不得按旧格式兜底。
6. 未指定路径时，先检查默认配置目录 `/etc/oss-auto-cert/`，其次检查当前工作目录；每个位置识别三个标准文件名。
7. 每个位置中若发现多个标准配置文件，停止并提示显式指定路径；高优先级位置有唯一配置时，不再读取低优先级位置。
8. 非 Linux 平台可以显式指定任意配置路径；本轮不新增另一套隐式系统配置路径。

该规则保留现有 `/etc/oss-auto-cert/config.yaml` 的部署入口，也能发现同位置的 `config.toml`。迁移文件与旧文件并存时，迁移命令必须提示后续启动要指定新路径。

### 4.3 覆盖优先级与校验

优先级从高到低：命令行显式参数、环境变量、选中的配置文件、对应配置来源的默认值。命令行参数的默认值不得覆盖文件中的值。

旧环境变量继续可用：

| 环境变量 | 统一模型位置 |
| --- | --- |
| `OSS_ACCESS_KEY_ID`、`OSS_ACCESS_KEY_SECRET` | 阿里云凭证解析器 |
| `ACME_EMAIL` | `acme.email` |
| `ACME_DATA_DIR` | `storage.data-dir` |
| `ACME_EXPIRED_EARLY` | `renewal.before-days` |
| `DEBUG` | 旧 YAML 的历史兼容转换，规则见下节 |

本轮不增加重复的凭证别名，不自动加载 `.env` 文件。Compose 的 `.env` 插值属于 Compose 行为。

新版 TOML 对未知字段、无效环境覆盖值、非法 URL、重复 Bucket、重复域名、未知模式及非正时长直接报错。旧 YAML 对历史上被忽略的未知字段和无效 `ACME_EXPIRED_EARLY` 给出警告，后者继续使用文件值和旧默认规则。无法安全执行的输入仍须报错，例如非法域名或会逃逸数据目录的路径。

本地校验不要求凭证值已提供；执行云端读取或写入的命令才解析并检查所需凭证。配置中引用的环境变量名称可以离线校验，实际值缺失在运行前报告。

### 4.4 旧配置兼容合同

| 项目 | 兼容行为 |
| --- | --- |
| `-config`、`-log-level` | 保留单横线形式，无子命令等价于 `run` |
| `webhook` | 转成一个通知渠道，未配置时仍输出日志 |
| `webhook-tpl` 与 `.Message` | 保留自定义模板入口和变量合同，具体错误处理见第 9 节 |
| `acme.email` | 转成 Let's Encrypt 默认账户邮箱 |
| `acme.data-dir` | 沿用原路径，缺省仍为 `/var/lib/oss-auto-cert` |
| `acme.expired-early` | 保留原来的 `max(15, 配置值)`，在环境覆盖后归一化 |
| `buckets[].name` / `endpoint` | 转成统一 Bucket 配置；不因升级新增管理范围 |
| 域名范围 | 转成通用的 `domain-mode = "first"`，保持只处理 API 返回的第一个域名 |
| 调度 | 启动立即巡检，此后每 6 小时；续期天数使用上述旧规则 |
| CDN | 保留“已开启 HTTPS 时尝试更新”的意图；未使用、跳过和成功分别报告 |
| 旧配置无 Bucket | 常驻命令警告并保持运行；新版 TOML 执行命令要求至少一个 Bucket |
| 旧证书文件 | 可以从原目录读取并导入新存储，不删除或改写原文件 |

兼容输入与管理范围，不保留 panic、无效 JSON、错误权限、重复注册和失败不重试等缺陷。日期计算改为按证书真实 `NotAfter` 判断，属于修复；迁移报告需要说明可能出现的巡检时点差异。

`DEBUG=true` 在旧 YAML 中仍映射为 staging 和强制续期，启动时显著提示，测试数据写入独立命名空间。TOML v2 不再解释 `DEBUG`；发现其值为 `true` 时仅提示明确替代项：`acme.environment = "staging"` 和 `run --once --force`。新版调试日志使用 `--log-level=debug`。

### 4.5 配置迁移

```bash
oss-auto-cert config migrate -config=config.yaml
oss-auto-cert config migrate -config=config.yaml -output=config.toml
```

- 不带 `-output` 时输出 TOML 到标准输出；警告输出到标准错误。
- 带 `-output` 时拒绝覆盖已有文件，成功后打印新启动命令。
- 默认只迁移文件的含义，不固化当前进程环境变量中的覆盖值或秘密。
- 为保持原行为，显式导出 `domain-mode = "first"`、6 小时周期及实际生效的续期默认值。
- 自定义模板原文保留为兼容模板，不根据字符串猜测通知服务或擅自重写模板。
- 使用非敏感的配置归一化结果验证迁移前后策略等价。
- 不注册 ACME 账户、不读取云端资源、不改变证书或运行状态。

## 5. TOML v2 配置合同

### 5.1 最小配置

```toml
version = 2

[acme]
email = "admin@example.com"

[[buckets]]
name = "my-bucket"
endpoint = "oss-cn-hangzhou.aliyuncs.com"
domains = ["static.example.com"]
```

省略字段使用下表中的默认值。凭证继续从 `OSS_ACCESS_KEY_ID` 与 `OSS_ACCESS_KEY_SECRET` 读取，不新增明文凭证配置。

### 5.2 完整配置示例

```toml
version = 2

[storage]
data-dir = "/var/lib/oss-auto-cert"

[schedule]
interval = "6h"
run-on-start = true

[renewal]
before-days = 30

[acme]
provider = "letsencrypt"
email = "admin@example.com"
environment = "production"
key-type = "rsa2048"
timeout = "2m"

[runtime]
workers = 2
request-timeout = "30s"
shutdown-timeout = "30s"

[[buckets]]
name = "my-bucket"
endpoint = "oss-cn-hangzhou.aliyuncs.com"
domain-mode = "listed"
domains = ["static.example.com", "images.example.com"]
allow-initial-issuance = false

[buckets.challenge]
type = "oss-http-01"

[buckets.deploy]
oss = true
cdn = "auto"

[[notifications]]
type = "wecom"
url-env = "CERT_WEBHOOK_URL"
events = ["updated", "failed", "recovered"]
```

### 5.3 字段语义和默认值

| 字段 | 默认值 / 校验规则 |
| --- | --- |
| `version` | 必须显式为 `2` |
| `storage.data-dir` | `/var/lib/oss-auto-cert`，执行写操作时须可写 |
| `schedule.interval` | `6h`，正的 Go duration 字符串；不使用 cron 表达式 |
| `schedule.run-on-start` | `true`，显式 `false` 必须生效；只影响常驻运行 |
| `renewal.before-days` | `15`，新版允许 1 至 60 的整数，按实际到期时刻计算 |
| `acme.provider` | `letsencrypt`，本轮仅允许此值；不接受 ZeroSSL 或自定义端点 |
| `acme.email` | 云端执行时必填、校验邮箱格式；本地校验可由 `ACME_EMAIL` 提供 |
| `acme.environment` | `production`，可选 `staging` |
| `acme.key-type` | `rsa2048`，本轮固定支持，保持云端兼容性 |
| `acme.timeout` | `2m`，ACME 签发阶段的总期限 |
| `runtime.workers` | `2`，允许 1 至 16；按域名任务限并发 |
| `runtime.request-timeout` | `30s`，单次云 API / HTTP 请求期限 |
| `runtime.shutdown-timeout` | `30s`，退出等待期限 |
| `buckets[].name` / `endpoint` | 必填；同一 Bucket 只能配置一次 |
| `buckets[].domain-mode` | 缺省为 `listed`；支持 `listed`、`all`、`first` |
| `buckets[].domains` | `listed` 模式要求非空，其他模式不得同时填写 |
| `buckets[].allow-initial-issuance` | `false`，无已有证书时先报告，显式开启才签发部署 |
| `buckets[].challenge.type` | `oss-http-01`，本轮唯一验证方式 |
| `buckets[].deploy.oss` | `true`，`false` 时不写 OSS 证书绑定 |
| `buckets[].deploy.cdn` | `auto` 或 `off`；`auto` 只更新已启用 CDN HTTPS 的域名 |
| `notifications[].type` | `wecom`、`dingtalk`、`feishu`、`template`、`legacy-template` |
| `notifications[].url-env` | 新建渠道必填；兼容迁移允许保留原来的 `url`，并按秘密处理 |
| `notifications[].events` | 缺省为 `updated`、`failed`、`recovered` |

生产环境不自动使用 staging 证书兜底。`--force` 只改变是否需要签发的判断，不改变 CA 环境、域名范围或部署目标。

`listed` 只接受该 Bucket 已绑定的域名，未发现的域名报错；`all` 仅覆盖已绑定域名，不创建绑定。多个域名默认分别签发。相同域名在不同 Bucket 重复出现时先报配置冲突，本轮不做跨 Bucket 的隐式证书共享。

`allow-initial-issuance` 只授权首次为已绑定域名配置证书。`cdn = "auto"` 不授权开启新的 CDN HTTPS。显式 `oss = false` 时仍可使用 Bucket 放置验证文件，CAS 上传是否需要由实际部署计划决定。

## 6. 代码结构与依赖

以下为首版实现布局，省略测试文件。CLI 命令集中在 root.go，配置文件模型和适配集中在 load.go，周期与重试调度由 app.go 负责；HTTP-01 实现位于 provider/acme。后续可按职责继续拆分，目录变化不能破坏已约定的配置、运行参数、模块路径及兼容构建入口。

```text
main.go                         兼容旧构建命令的薄入口
cmd/oss-auto-cert/main.go       推荐构建入口、进程退出码
internal/
  cli/root.go                   Cobra 命令、全局参数及旧参数适配
  app/app.go                    依赖组装、巡检调度、生命周期与退出码
  config/
    config.go                   统一配置、默认值、校验及归一化
    load.go                     TOML / YAML 模型、适配、加载及迁移
  certificate/
    service.go                  单轮执行、计划、逐目标核验及重试
    deployment.go               证书准备、上传恢复及目标部署
    model.go                    证书、状态与基础设施接口
    logging.go                  业务阶段日志
  provider/
    acme/issuer.go              lego、账户复用与 OSS HTTP-01
    aliyun/cloud.go             OSS、CDN、CAS 适配
  storage/                      账户、证书、状态、锁和旧文件导入
  notification/                 内置通知及自定义模板
  logging/                      logrus TextFormatter、等级和颜色策略
  testutil/                     离线测试证书
examples/config.full.toml       完整 TOML 配置示例
docs/
  refactoring-plan.md           本方案与实现边界
  refactoring-validation.md     已执行的验证记录
  usage.md                      使用、迁移及恢复说明
  logging.md                    日志等级、颜色与日志保存
```

`cmd/oss-auto-cert/main.go` 与根目录 `main.go` 均只委托同一个 CLI 执行入口，处理最终退出码，不复制命令注册或业务实现。推荐使用 `go build -o oss-auto-cert ./cmd/oss-auto-cert`，同时保留 `go build -o oss-auto-cert main.go` 和 `go build .`。

依赖方向为 CLI 调用应用层用例，应用层组装配置、业务服务和适配器，业务层通过小接口调用签发、发现、上传、部署、存储和通知。`certificate` 不依赖 Cobra、具体 SDK 或存储实现。接口放在使用它的包内，避免扩张一个承担所有职责的 `types` 包。SDK 响应在适配层转换为业务模型，不穿透到调度和计划层。

旧 `pkg/webhook` 和 `pkg/utils` 已按职责迁入内部包并删除，通用逻辑优先放在使用它的包内，不创建新的万能工具包。旧的 Go 导入 API 不属于配置与命令行兼容合同；源码 API 变化已列入 [变更记录](../CHANGELOG.md)。

构造函数只验证参数、创建本地对象，返回错误；网络读取和写入由明确的方法执行。使用 github.com/sirupsen/logrus 的 TextFormatter，日志按等级着色并写入 stderr，通过独立实例注入；日志与用户消息使用中文，错误通过 `%w` 保留根因。

TOML 解析采用 `github.com/pelletier/go-toml/v2`，旧 YAML 继续使用现有解析库。CLI 使用 `github.com/spf13/cobra` 及其 pflag 参数能力；本轮不引入 Viper，避免配置优先级和旧版转换规则分散在多个框架中。

## 7. 巡检、签发与部署

### 7.1 执行顺序

```text
发现范围内域名 → 读取证书和各目标实际状态 → 读取待恢复任务
              → 生成计划 → 按需签发 → 可靠保存证书
              → 按需上传 CAS → 独立部署 OSS / CDN
              → 核验目标证书 → 保存结果 → 汇总通知
```

计划需要明确区分：无需处理、首次签发未授权、需要续期、需要上传、需要部署、等待重试、目标跳过和永久错误。

判断续期使用实际证书的 `NotAfter`，不把“某个 CAS ID 存在”作为唯一前提。OSS 上已有非 CAS 证书时，在能够可靠读取有效期和证书身份的前提下处理；无法判断时报告“未知”，不能当作正常。

### 7.2 独立部署与恢复

同一证书版本的每个目标分别记录 `pending`、`applied`、`verified`、`retrying`、`failed` 或 `skipped`。

当 OSS 成功、CDN 失败时，下一轮和进程重启后均复用已保存证书、CAS ID，仅恢复 CDN；不会因为 OSS 已换成新证书而忽略 CDN。

- 新的证书版本按域名、CA 环境和指纹标识；同一目标结果属于明确的证书版本。
- 调用部署 API 成功只表示已提交，读取目标信息核验后才记录 `verified`。
- CDN 延迟生效进入核验重试，不立即重复签发或重复上传。
- 调用前记录意图，成功后记录外部 ID 和结果，允许进程在任意步骤中断。
- CAS 上传使用稳定的任务标识和名称；上传响应丢失时先核对远端证书身份再恢复。
- 外部 API 与本地文件无法形成事务，不承诺恰好执行一次。无法可靠核对的上传结果标记为待核对，不盲目重复创建。
- 任务从配置中移除后停止写入，保留历史状态用于诊断，不继续重试旧目标。

### 7.3 重试与并发

网络超时、限流和暂时服务错误可以重试；权限不足、非法配置和确定性验证失败停止本轮尝试，报告处理建议。CA 返回的重试时间和限流要求优先于本地退避。

默认从 1 分钟开始指数退避，加入抖动，单次间隔不超过 1 小时；每轮最多 5 次尝试，随后标记失败，常驻模式在后续巡检重新评估。永久错误在配置变更或下一轮检查时重新读取状态，不循环调用写接口。

重试计划持久化，常驻调度同时考虑常规巡检与待重试时间。`run --once` 不等待未来所有退避任务，完成本轮可执行动作后退出并保留待重试状态。

同一域名同一时间仅允许一个执行任务。调度防重入使用可靠互斥或原子比较交换，不采用分开的 `Load` / `Store`。并发任务都由服务持有和等待，不启动无人跟踪的保存或部署 goroutine。

### 7.4 取消与退出

通过 `signal.NotifyContext` 管理取消，任务入口和 channel 操作均响应 context。停止后不再接收新任务，等待当前操作在期限内结束，并保存可恢复的状态。

SDK 适配层设置连接和读写超时；支持 context 的调用必须传递 context。不支持时适配请求传输层或更换兼容调用方式，不通过遗留后台 goroutine 假装完成取消。

退出时不注销 ACME 账户。被中断的 challenge 清理由明确任务记录跟踪；恢复时仅处理本程序拥有的验证对象，不扫描删除用户对象。

## 8. 持久化和升级

本轮使用文件存储。建议布局：

```text
<data-dir>/
  accounts/letsencrypt/production/     生产账户元数据和账户私钥
  accounts/letsencrypt/staging/        测试账户元数据和账户私钥
  certificates/<environment>/<id>/    不同版本的证书和私钥
  state/                              运行和逐目标任务状态
  locks/                              单写者运行锁
  <domain>/cert.crt                    旧文件，升级导入时只读
  <domain>/cert.key
  <domain>/cert-issuer.crt
  <domain>/cert.csr
```

- 证书 ID 使用经过归一化的资源标识生成，不直接把任意外部字符串拼接成路径。
- 证书、私钥和元数据先写入临时版本目录，校验密钥匹配及域名覆盖后发布版本引用。
- 状态通过同目录临时文件和适配平台的原子替换写入；测试 Windows 替换语义，不能假设所有平台的 `os.Rename` 都等价。
- 状态文件有独立 schema 版本，未知较新版本停止写入，禁止当作空状态重建。
- 状态记录证书身份、CA 环境、外部 ID、目标结果、尝试次数和时间，不保存私钥或含秘密的完整 URL。
- 旧文件首次使用时校验后导入，不删除旧文件；旧 CSR 不直接决定新的续签路径，避免返回没有对应私钥的证书。
- 无法证明来源的旧证书不直接认定为生产可信证书；校验信任链、域名、有效期及私钥匹配。
- 写命令使用数据目录的单写者锁；`status` 读取已发布快照，不与写进程竞争更新状态。
- 锁定目标范围为本机、普通本地文件系统，不承诺 NFS 和多主机协调；不凭 PID 文件随意删除仍可能有效的锁。
- Unix 文件权限和 Windows ACL 分别实现。通知、日志和状态均不输出秘密。

新程序支持读取旧数据，不承诺旧二进制能读取新存储格式。升级前备份旧二进制、配置和数据目录；回退通过停止新进程并恢复整组备份完成。迁移和回退步骤必须在发布说明中给出。

## 9. 通知与用户反馈

内置企业微信、钉钉、飞书格式用结构体和 JSON 序列化，避免手工拼 JSON。通知设置独立超时和有界重试，关闭响应体，校验 HTTP 状态及内置平台的业务结果。

`template` 使用 `text/template`，提供 JSON 编码函数；表达式 `{{ json .Message }}` 输出包含引号的完整 JSON 字符串。发送前检查生成内容是否为合法 JSON。

`legacy-template` 保留旧 `.Message` 变量及原模板渲染语义，用于读取或迁移旧 `webhook-tpl`。旧配置未指定模板时直接使用安全的企业微信内置格式。历史自定义模板如果生成无效 JSON，渠道报告错误并提示改为 JSON 编码表达式，不擅自修改模板，不发送无效请求，也不阻断证书管理。

模板兼容保证旧模板能够加载、旧变量可以使用，不保证继续发送本来不合法的 JSON。配置迁移需用包含换行和引号的测试消息渲染预览，报告存在的问题，但不向真实 Webhook 发送测试消息。

日志默认输出资源名、域名、阶段、结果和下一步建议，debug 日志补充资源发现和计划阶段，不输出 SDK 原始响应或凭证。区分“未使用 CDN”“未开启 HTTPS”“更新已提交”“更新已核验”和“更新失败”，不能将跳过显示为更新成功。

禁用颜色时的 TextFormatter 输出示意（时间、资源和耗时仅作示例）：

```text
time="2026-11-02 12:00:00" level=info msg="开始巡检" buckets=1 dry_run=false environment=production force=false
time="2026-11-02 12:00:02" level=error msg="处理失败" bucket=my-bucket detail="CDN: 请求超时" domain=static.example.com
time="2026-11-02 12:00:02" level=warning msg="巡检完成，存在失败或待重试任务" duration=2s failed=1 total=1
```

通知按域名和执行结果聚合，避免每次重试重复发送相同故障；故障恢复单独通知。无通知配置时仍保留完整本地运行结果。

## 10. 命令行合同

### 10.1 Cobra 命令树

```text
oss-auto-cert                   无子命令时执行常驻运行
├── run                         --once / --dry-run / --force
├── check
├── status
├── version
└── config
    ├── init
    ├── validate
    └── migrate
```

`--config`、`--log-level` 和 `--color` 注册为根命令的持久参数。运行选项注册到 `run` 命令，配置生成、迁移等选项注册到各自子命令，避免不相关命令接受运行写入选项。`version` 子命令及 `--version` 均离线可用。

### 10.2 命令行为

| 命令 | 作用及副作用 |
| --- | --- |
| `--help` / `version` | 不读取配置、不要求凭证、不访问网络 |
| `config init` | 生成最小 TOML，默认不覆盖已有文件；不启动交互式问答 |
| `config validate` | 只做本地解析、覆盖和校验，不注册账户、不写运行状态 |
| `config migrate` | 离线转换 YAML；仅在显式指定输出时创建文件 |
| `check` | 只读查询云资源及验证条件，不放置 challenge、不发送 Webhook |
| `run` | 常驻管理；无子命令的旧启动方式映射至此 |
| `run --once` | 执行一轮，适合手动操作、Cron 和 CI |
| `run --dry-run` | 只读生成计划，执行一轮即退出；不创建账户、证书、锁或状态 |
| `run --once --force` | 显式强制签发，只覆盖已选中的管理范围 |
| `status` | 读取本地快照，显示检查时间、配置资源范围及状态是否过期 |

支持 `-config` / `--config`、`-log-level` / `--log-level`；全局参数可放在子命令前后，重复同一参数时报告错误。`--force` 要求与 `--once` 配合，避免常驻进程每轮强制签发。`--dry-run --force` 可以展示强制签发计划，但仍然不得写入。

### 10.3 参数兼容与执行约束

Cobra / pflag 不原生兼容旧标准 flag 包的多字母单横线参数，必须通过 `compat.go` 显式适配，而不是假设更换框架后自然兼容。

- 仅将已知旧参数 `-config`、`-config=...`、`-log-level`、`-log-level=...` 转为对应双横线形式，兼容旧 `-h` / `-help` 帮助入口。
- 参数扫描必须识别需要取值的选项，不修改作为参数值出现的文本；遇到独立的 `--` 后停止改写，不重写未知参数。
- 转换后对单 / 双横线别名进行统一重复检测，不能仅依赖 pflag 默认的后值覆盖行为。
- 无子命令且参数合法时，根命令 `RunE` 与 `run` 复用同一应用层用例；未知子命令或多余位置参数返回错误，不能误启动常驻服务。
- 通过 `Flags().Changed` 或等价的显式出现记录生成覆盖值，不能将 Cobra 参数默认值当成用户覆盖。
- 命令使用 `RunE` 返回错误、`ExecuteContext` 传入上下文；配置加载及云端操作按命令需要执行，不放进全局 `init()` 或对所有命令生效的根预执行钩子。
- 通过 `SetArgs`、`SetIn`、`SetOut`、`SetErr` 注入参数与输入输出，每次构造新的命令树及选项实例，避免测试间共享参数状态。
- 设置 `SilenceUsage`、`SilenceErrors`，由统一入口输出一次中文错误和必要的帮助指引，并映射退出码。
- 固定本轮命令树，禁用 Cobra 自动添加的 completion 命令；Shell 补全可以后续单独设计。

`check` 无法通过纯读取证明某个写权限存在时，明确显示“未验证”，不能宣称预检已证明全部权限。`status` 不假装是实时云端状态，没有快照时明确提示未运行过。

### 10.4 退出码

退出码约定：

| 退出码 | 含义 |
| --- | --- |
| `0` | 命令成功；单轮任务全部成功、正常或按策略跳过 |
| `1` | 部分或全部执行失败、仍有待重试任务，或检查发现需用户处理的问题 |
| `2` | 参数、配置、必要凭证或本地运行前提错误 |

常驻模式的某个域名失败不退出整个服务；初始化错误和不可恢复的存储错误需要明确失败。退出信号收到后正常完成收尾返回成功，超过退出期限仍有未完成写任务时返回失败并说明恢复方式。

## 11. 部署和发布

- 推荐构建入口改为 `go build -o oss-auto-cert ./cmd/oss-auto-cert`，Docker、发布工作流和新文档统一采用该入口；通过根目录薄入口保留 `go build -o oss-auto-cert main.go` 和 `go build .`，保持现有模块路径。
- 新目录布局、Cobra 使用约定和两种构建入口同步写入 AGENTS.md，统一 Go 最低版本与开发文档。
- 新镜像通过 `TARGETOS` / `TARGETARCH` 构建，测试 amd64 和 arm64，标签发布检出触发提交。
- 显式安装并验证容器内信任根，保持 ACME 和云 API 的 TLS 校验。
- 保留旧配置挂载路径；新版示例显式传入 TOML 路径，避免双配置发现歧义。
- Compose 中持久化目录、`storage.data-dir` 和 `ACME_DATA_DIR` 必须一致，不保留示例中的 `/data` 冲突。
- 新装容器提供非 root 运行方式；已有卷属主不匹配时给出修复说明，不静默递归改写宿主机文件权限，不无提示改变旧镜像默认用户。
- Systemd 和容器设置足够的停止期限，覆盖应用的 `shutdown-timeout`。
- CI 增加离线测试、vet、构建、依赖漏洞扫描与双平台存储测试，发布工作流依赖检查通过。

## 12. 实施阶段

采用分批替换的方式实现全面重构，每个阶段保持可构建、可审查。以下阶段均属于本轮改造，不以只完成基础框架作为整体交付。

| 阶段 | 交付内容 | 验收门槛 |
| --- | --- | --- |
| P1：兼容基线 | 旧 YAML、环境覆盖、启动参数、文件样例；隔离真实云测试 | 能精确说明旧配置的实际行为，测试不接触生产资源 |
| P2：配置与 CLI | 新目录布局、Cobra 命令树、双构建入口、统一模型、TOML v2、旧适配器及配置工具 | YAML 与迁移 TOML 的有效策略等价，旧参数正确转换，帮助及离线命令无网络副作用 |
| P3：存储与适配 | 账户复用、证书存储、旧文件导入、锁、云服务接口 | 非 root 可保存，秘密不泄漏，写入中断可恢复，staging 与 production 隔离 |
| P4：执行核心 | 发现、计划、签发、CAS 上传、独立部署、重试和取消 | OSS 成功 / CDN 失败后重启，仅恢复 CDN，不重复签发 |
| P5：体验与交付 | 通知、只读检查、状态、Docker、CI、迁移和回退文档 | 全部验收项通过，标签产物、镜像架构和挂载路径正确 |

依赖漏洞修复、日志格式和信号处理错误在相应阶段一并解决。现有实现待替代功能通过测试后删除，不长期保留两套运行引擎；最终保留的旧版代码职责仅为输入适配与数据导入。

## 13. 测试与最终验收

### 13.1 配置及兼容

- 现有 `simple-config.yaml` 的字段结构、README 与使用文档中的有效示例均有固定样例。
- 覆盖字段省略、显式 `false`、空列表、无效覆盖值、未知字段、未知版本和格式错误。
- 覆盖 YAML 中 0、负数、小于 15 的续期配置，以及对应环境覆盖后的旧规则。
- 验证旧启动命令、TOML 新命令、单 / 双横线、参数位置及重复参数。
- Cobra 命令测试覆盖无子命令默认运行、未知子命令拒绝、`--help` / `-help` / `version` / `--version` 离线执行，以及连续构造命令时参数状态不泄漏。
- 参数兼容测试覆盖等号写法、空格写法、带空格和中文的路径、看起来像选项的参数值、独立 `--` 分隔及单 / 双横线混用的重复检测。
- 同一位置双配置时报错；显式路径不存在时不寻找其他文件。
- 迁移不覆盖原文件、不把环境凭证写入 TOML，迁移前后范围与策略一致。
- 旧 `DEBUG=true` 的历史映射与新版忽略加提示行为分别测试。

### 13.2 核心流程及故障

- 多域名中一个失败不阻止其他域名，未选择的域名不发生写操作。
- 无证书、证书损坏、临近过期、已过期、私钥不匹配、未知状态均有明确结果。
- 默认不为无证书域名签发；显式授权后按所选目标执行首次部署。
- CAS 上传前后、每个目标部署前后中断，重启后正确核对和恢复。
- OSS 已更新但 CDN 更新失败、CDN 已提交但尚未生效、CDN 未启用分别验证。
- 网络抖动、限流、权限错误和 CA 验证失败采用不同处理策略。
- 模拟日期边界，使用可注入时钟，不依赖已经过去的固定日期。

### 13.3 存储、通知及运行

- Unix 目录与私钥权限、Windows 文件替换和 ACL、跨进程写锁均有针对性检查。
- 旧证书可导入，账户重启复用，退出不注销，测试证书不会用于生产任务。
- 通知覆盖多行、引号、反斜杠、HTML 字符、请求构造失败、网络错误及业务错误。
- 旧自定义模板能加载；生成无效 JSON 时报告渠道错误，不导致进程退出。
- 退出取消、并发上限、防重入和状态快照读取不发生死锁或泄漏。
- 本地 HTTP 测试服务和假 SDK 实现完成默认测试；云集成测试使用 `integration` build tag 显式启用，并要求专用测试资源。

### 13.4 交付检查

```bash
go test ./...
go vet ./...
go build ./...
go build -o oss-auto-cert ./cmd/oss-auto-cert
go build -o oss-auto-cert main.go
govulncheck ./...
```

在支持竞态检测的平台补充 `go test -race ./...`；在 Linux 非 root 和 Windows 环境运行存储检查。不得以关闭 vet 或跳过核心断言作为最终通过标准。

最终交付包含新实现、兼容样例、最小及完整 TOML 示例、配置迁移工具、升级回退说明、更新后的 README / usage / AGENTS 开发信息、修正的容器及发布流程。任何未完成能力必须明确列出，不能写成已经可用。
