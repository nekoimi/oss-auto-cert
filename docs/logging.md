# 日志使用说明

应用日志使用 `github.com/sirupsen/logrus` 的 `TextFormatter`，默认等级为 `info`，时间戳格式为 `2006-01-02 15:04:05`。每个命令创建独立的日志实例，通过依赖注入传给执行流程。

## 等级与颜色

```bash
./oss-auto-cert run --config=config.toml --log-level=debug
```

| 等级 | 内容 |
| --- | --- |
| `trace` | 包含 debug 及以上等级的日志 |
| `debug` | 资源发现、续期与部署恢复计划等详细过程 |
| `info` | 签发、上传、部署、成功结果、巡检统计及服务启停 |
| `warn` | 配置兼容提示、巡检未全部完成、正常取消等情况 |
| `error` | 域名处理失败及命令执行失败 |

选择一个等级后会输出该等级及更严重的日志，例如 `warn` 会保留 WARN 和 ERROR。旧 `-log-level=debug` 形式仍然支持。`DEBUG=true` 属于旧 YAML 的测试环境兼容开关，不用于设置日志等级。

| 参数 | 行为 |
| --- | --- |
| `--color=auto` | 默认值，识别 stderr 是否为终端；重定向至文件或管道时不输出颜色 |
| `--color=always` | 强制输出 ANSI 颜色，适合支持颜色的容器日志查看器 |
| `--color=never` | 禁用颜色，适合日志文件和采集器 |

判断颜色的是 stderr；仅将 stdout 接入管道不会关闭仍连接终端的 stderr 颜色。

颜色由 TextFormatter 按等级提供，INFO、WARN、ERROR 使用不同颜色。终端或日志查看器需要支持 ANSI 转义序列才能显示颜色。设置 `NO_COLOR` 环境变量会在 auto 模式禁色，即使其值为空；显式 `--color=always` 仍会开启颜色。

`--log-level` 和 `--color` 均为全局参数，可以放在子命令前后。日志等级和颜色通过命令行设置，不在 TOML 中增加对应字段。

## 日志字段

不同阶段使用不同字段，并非每条日志都包含全部字段。

| 字段 | 说明 |
| --- | --- |
| `bucket` / `domain` | 当前资源和域名 |
| `stage` | `discover`、`plan`、`import`、`issue`、`upload`、`deploy_oss` 或 `deploy_cdn` |
| `detail` / `error` | 结果补充信息或错误原因 |
| `environment` | `production` 或 `staging` |
| `dry_run` / `force` | 本轮是否预演、是否请求强制签发 |
| `buckets` | 配置中的 Bucket 数量 |
| `total` / `failed` / `duration` | 本轮任务总数、失败数和耗时 |
| `interval` | 常驻巡检间隔 |

应用不会记录 Access Key、私钥、完整配置或通知地址中的令牌。排查问题优先使用 `--log-level=debug`；只读检查使用 `check` 或 `run --dry-run`，其结果也写入 stderr。

## 保存日志和导出配置

运行日志、兼容警告和错误写入 stderr。帮助、版本、状态快照、配置校验结果和迁移导出的 TOML 正文写入 stdout。

Linux / macOS 保存单轮运行日志：

```bash
./oss-auto-cert run --once --config=config.toml --color=never 2>run.log
```

PowerShell 保存日志：

```powershell
.\oss-auto-cert.exe run --once --config=config.toml --color=never 2>run.log
```

迁移时推荐使用 `--output`，由应用直接写入文件，并拒绝覆盖已有文件：

```bash
./oss-auto-cert config migrate --config=config.yaml --output=config.toml --color=never 2>migration.log
```

也可在 Bash 中分别重定向 stdout 和 stderr：

```bash
./oss-auto-cert config migrate --config=config.yaml --color=never >config.toml 2>migration.log
```

Shell 的 `>` 会创建或截断输出文件，不具备 `--output` 的拒绝覆盖保护。不要使用 `2>&1` 把迁移警告合入 TOML。Windows PowerShell 5.1 的文本重定向可能改变编码，配置迁移应优先使用 `--output`。

## 容器与 Systemd

已构建本地镜像后，显式指定颜色：

```bash
docker run --rm \
  -v "$PWD/config.toml:/etc/oss-auto-cert/config.toml:ro" \
  -v "$PWD/certs:/var/lib/oss-auto-cert" \
  -e OSS_ACCESS_KEY_ID -e OSS_ACCESS_KEY_SECRET \
  oss-auto-cert:local run --config=/etc/oss-auto-cert/config.toml --color=always
```

容器通常没有分配终端，auto 模式下无颜色属于正常行为。面向日志采集器时选择 `--color=never`。Compose 用户可在服务的 `command` 列表中添加 `--color=always` 或 `--color=never`。

Systemd 可在 `ExecStart` 后添加 `--color=never`，并通过以下命令查看日志：

```bash
journalctl -u oss-auto-cert -f
```

日志等级只影响显示，不改变退出码或是否重试。查看日志时请同时检查命令退出码；需要了解部署恢复策略时参阅 [使用说明](usage.md)。
