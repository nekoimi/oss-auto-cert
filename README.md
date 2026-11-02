# 阿里云 OSS 证书自动化工具

[![GitHub Release](https://img.shields.io/github/release/nekoimi/oss-auto-cert.svg)](https://github.com/nekoimi/oss-auto-cert/releases)
[![License](https://img.shields.io/github/license/nekoimi/oss-auto-cert.svg)](LICENSE)

使用 Let's Encrypt 自动签发证书，上传到阿里云 CAS，并分别更新 OSS 与已开启 HTTPS 的 CDN 域名。

新版使用 TOML 和 Cobra 命令，保留旧 YAML、原环境变量、单横线启动参数及证书文件的读取。支持单轮执行、只读预演、状态查询和部署失败恢复。本轮暂不支持 ZeroSSL、DNS-01、通配符和 DCDN。

## 快速开始

源码构建要求 Go 1.25 或更新版本：

```bash
go build -o oss-auto-cert ./cmd/oss-auto-cert
./oss-auto-cert config init
```

修改生成的 `config.toml`，填写邮箱、已有 Bucket 与其已绑定的域名：

```toml
version = 2

[acme]
email = "admin@example.com"

[[buckets]]
name = "my-bucket"
endpoint = "oss-cn-hangzhou.aliyuncs.com"
domains = ["static.example.com"]
```

准备环境变量，校验并查看计划：

```bash
export OSS_ACCESS_KEY_ID="your-access-key-id"
export OSS_ACCESS_KEY_SECRET="your-access-key-secret"

./oss-auto-cert config validate --config=config.toml
./oss-auto-cert run --config=config.toml --dry-run
./oss-auto-cert run --config=config.toml --once
# 常驻：启动执行一次，之后默认每 6 小时巡检
./oss-auto-cert run --config=config.toml
```

HTTP-01 要求公网能访问 `http://域名/.well-known/acme-challenge/令牌`，并将路径路由到配置 Bucket。首次部署证书需要显式设置 `allow-initial-issuance = true`，程序不会创建新的域名绑定或开启新的 CDN HTTPS。

## 原用户升级

原命令仍然可用：

```bash
./oss-auto-cert -config=config.yaml -log-level=info
```

旧 YAML 保持原来的第一个域名范围及最低提前 15 天续期策略；新版 TOML 默认要求列出需要管理的域名。可离线导出配置：

```bash
./oss-auto-cert config migrate -config=config.yaml -output=config.toml
./oss-auto-cert config validate --config=config.toml
```

迁移不会改写原文件或固化环境覆盖值。新旧文件并存时请显式传入 `--config`。升级前备份原二进制、配置及整个证书数据目录，回退需恢复整组备份。旧 Go 包 API 已被内部模块替代，源码导入方需要调整。

## Docker

```bash
docker build -t oss-auto-cert:local .
docker run --rm \
  -v "$PWD/config.toml:/etc/oss-auto-cert/config.toml:ro" \
  -v "$PWD/certs:/var/lib/oss-auto-cert" \
  -e OSS_ACCESS_KEY_ID -e OSS_ACCESS_KEY_SECRET \
  oss-auto-cert:local run --config=/etc/oss-auto-cert/config.toml
```

仓库附带 [Compose 示例](docker-compose.yaml)。数据目录必须持久化，并与 `storage.data-dir` / `ACME_DATA_DIR` 一致。非 root 运行前需要为挂载的数据目录设置对应属主，见使用文档。

## 命令

| 命令 | 用途 |
| --- | --- |
| `config init` | 生成最小配置，不覆盖已有文件 |
| `config validate` | 离线检查字段和通知模板 |
| `config migrate` | 将旧 YAML 导出为 TOML |
| `check` / `run --dry-run` | 只读检查和计划，不申请或部署证书 |
| `run --once` | 执行一轮后退出 |
| `run` | 常驻巡检并恢复待重试部署 |
| `status` | 查看带检查时间的本地快照 |
| `version` / `--help` | 离线查看版本和帮助 |

运行日志使用 logrus `TextFormatter`，包含时间、级别和域名上下文，写入 stderr。终端默认彩色输出；`--color=always` 强制开启、`--color=never` 关闭，`--log-level=debug` 查看详细过程。配置导出等命令的 stdout 不混入日志。

默认离线测试不需要凭证，也不会改动真实云资源：

```bash
go test ./...
go test -race ./...
go vet ./...
```

## 文档

- [完整使用、升级及故障恢复说明](docs/usage.md)
- [完整 TOML 示例](examples/config.full.toml)
- [旧 YAML 示例](simple-config.yaml)
- [重构设计与实现状态](docs/refactoring-plan.md)
- [首版重构验证记录](docs/refactoring-validation.md)
- [开发规范](AGENTS.md)

许可证见 [LICENSE](LICENSE)。基础依赖：[lego](https://github.com/go-acme/lego)、[Cobra](https://github.com/spf13/cobra)、阿里云 SDK。
