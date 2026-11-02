# 阿里云 OSS 证书自动化工具

[![GitHub Release](https://img.shields.io/github/release/nekoimi/oss-auto-cert.svg)](https://github.com/nekoimi/oss-auto-cert/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

使用 Let's Encrypt 自动申请和续期 SSL 证书，上传到阿里云 CAS，并更新 OSS 与已开启 HTTPS 的 CDN 域名证书。

- 支持多个 Bucket 和域名，可指定管理范围。
- 提供配置校验、只读预演、单轮执行和常驻巡检。
- 持久化账户、证书和部署进度，分别恢复 OSS / CDN 的失败任务。
- 支持企业微信、钉钉、飞书和自定义 Webhook 通知。
- 支持 Docker Compose 部署，提供日志等级、颜色及本地状态查询。

## 快速开始

源码构建要求 Go 1.25 或更新版本，建议使用对应系列最新补丁版本：

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

设置凭证，校验配置并查看计划：

```bash
export OSS_ACCESS_KEY_ID="your-access-key-id"
export OSS_ACCESS_KEY_SECRET="your-access-key-secret"

./oss-auto-cert config validate --config=config.toml
./oss-auto-cert run --config=config.toml --dry-run
./oss-auto-cert run --config=config.toml --once
# 常驻：启动执行一次，之后默认每 6 小时巡检
./oss-auto-cert run --config=config.toml
```

HTTP-01 要求公网能访问 `http://域名/.well-known/acme-challenge/令牌`，并将路径路由到配置 Bucket。首次部署证书需要在对应 Bucket 设置 `allow-initial-issuance = true`。数据目录默认为 `/var/lib/oss-auto-cert`，可通过 `ACME_DATA_DIR` 指定可写路径。

## Docker Compose 部署

使用 [docker-compose.yaml](docker-compose.yaml) 和 [.env.example](.env.example)：

```bash
cp simple-config.toml config.toml
cp .env.example .env
```

修改配置中的邮箱、Bucket 和域名，在 `.env` 中填写阿里云凭证，并将 `OSS_AUTO_CERT_IMAGE` 设置为经过验证的固定版本镜像或镜像摘要。Linux 主机使用默认 UID/GID `10001:10001` 时，准备权限：

```bash
sudo install -d -m 0700 -o 10001 -g 10001 certs
sudo chown 10001:10001 config.toml
sudo chmod 0400 config.toml
chmod 0600 .env

docker compose config --quiet
docker compose pull
docker compose run --rm --no-deps oss-auto-cert config validate --config=/etc/oss-auto-cert/config.toml
docker compose up -d
docker compose logs -f oss-auto-cert
```

部署配置包含非 root 用户、只读根文件系统、只读配置挂载、数据持久化、CPU / 内存 / 进程数限制及日志轮转。数据卷固定使用 `/var/lib/oss-auto-cert`，停止宽限为 60 秒。已有数据目录的权限调整和备份方式见 [部署说明](docs/usage.md#8-docker权限与-systemd)。

从源码构建镜像时，可以执行 `docker build --build-arg VERSION=local -t oss-auto-cert:local .`，并设置 `OSS_AUTO_CERT_IMAGE=oss-auto-cert:local`；使用本地镜像时跳过 `docker compose pull`。

## 配置迁移

已有 YAML 配置可以继续运行，也可导出为 TOML：

```bash
./oss-auto-cert config migrate --config=config.yaml --output=config.toml
./oss-auto-cert config validate --config=config.toml
```

迁移保留配置中的域名选择与续期策略，不改写原文件、不固化环境覆盖值。新旧文件并存时请使用 `--config` 指定路径。升级前备份二进制、配置和完整数据目录，操作步骤见 [升级与回退说明](docs/usage.md#7-旧版升级与回退)。

## 命令

| 命令 | 用途 |
| --- | --- |
| `config init` | 生成最小配置，不覆盖已有文件 |
| `config validate` | 离线检查字段和通知模板 |
| `config migrate` | 将 YAML 配置导出为 TOML |
| `check` / `run --dry-run` | 只读检查资源并展示计划 |
| `run --once` | 执行一轮后退出 |
| `run` | 常驻巡检并恢复待重试部署 |
| `status` | 查看带检查时间的本地快照 |
| `version` / `--help` | 查看版本和帮助 |

## 日志

日志使用 logrus `TextFormatter`，包含时间、级别和域名上下文，写入 stderr。终端自动彩色输出；`--color=always` 强制开启、`--color=never` 关闭。配置导出等命令正文保留在 stdout。

```bash
./oss-auto-cert run --config=config.toml --log-level=debug --color=always
```

保存日志、容器颜色和配置导出重定向示例见 [日志说明](docs/logging.md)。

## 开发

默认测试无需凭证，使用模拟服务，不修改真实云资源：

```bash
go test ./...
go test -race ./...
go vet ./...
```

## 文档

- [完整使用、部署及故障恢复说明](docs/usage.md)
- [日志等级、颜色与日志保存](docs/logging.md)
- [完整 TOML 示例](examples/config.full.toml)
- [YAML 配置示例](simple-config.yaml)
- [开发规范](AGENTS.md)
- [变更记录](CHANGELOG.md)

## 许可证

本项目采用 [MIT License](LICENSE)。

基础依赖：[lego](https://github.com/go-acme/lego)、[Cobra](https://github.com/spf13/cobra)、[logrus](https://github.com/sirupsen/logrus)、阿里云 SDK。
