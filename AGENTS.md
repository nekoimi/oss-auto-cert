# AGENTS.md - oss-auto-cert 编码规范

本文档为 AI 编码助手提供 oss-auto-cert 项目的编码规范指南。

## 项目概述

- **语言**: Go 1.25.0（发布使用最新补丁版本）
- **模块**: `github.com/nekoimi/oss-auto-cert`
- **用途**: 基于 Let's Encrypt 实现阿里云 OSS/CDN SSL 证书自动管理

## 构建命令

```bash
# 构建项目
go build -o oss-auto-cert ./cmd/oss-auto-cert

# 生产环境构建（静态链接）
go build --ldflags "-extldflags -static -s -w" -o oss-auto-cert ./cmd/oss-auto-cert

# 安装依赖
go mod tidy && go mod download

# 运行应用
./oss-auto-cert run --config=/path/to/config.toml --log-level=info
```

## 测试命令

```bash
# 运行所有测试
go test ./...

# 详细模式运行测试
go test -v ./...

# 运行特定包的测试
go test -v ./internal/config/
go test -v ./internal/certificate/

# 运行单个测试函数
go test -v -run TestLegacyMigrationAndEnvironment ./internal/config/
go test -v -run TestRestartRetriesOnlyFailedTarget ./internal/certificate/

# 测试覆盖率
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## 代码风格规范

### 导入规范

- 导入分组：标准库 → 第三方库 → 项目内部包
- 使用 goimports 格式化
- 示例：
```go
import (
    "context"
    "os"
    
    log "github.com/sirupsen/logrus"
    "github.com/aliyun/aliyun-oss-go-sdk/oss"
    
    "github.com/nekoimi/oss-auto-cert/internal/config"
    "github.com/nekoimi/oss-auto-cert/internal/certificate"
)
```

### 格式化规范

- 使用 `gofmt` 格式化所有 Go 文件
- 使用 tab 缩进（Go 标准）
- 运行 `go vet` 检查问题

### 命名规范

- **包名**: 小写，单字，无下划线
- **导出标识符**: PascalCase（如 `NewAutoCert`, `Config`）
- **非导出标识符**: camelCase（如 `loadOptions`, `expiredEarly`）
- **常量**: MixedCase（如 `DefaultExpiredEarly`）
- **接口**: PascalCase，-er 后缀（如 `Provider`）
- **缩写**: 全大写（如 `OSS`, `CDN`）

### 类型和结构体

- 使用 struct tags 进行 YAML/JSON 序列化
- 导出类型需添加文档注释
- 示例：
```go
// Config 保存应用配置
type Config struct {
    Path    string   `yaml:"-"`        // 配置文件路径
    Webhook string   `yaml:"webhook"`  // 通知 Webhook URL
    Buckets []Bucket `yaml:"buckets"`  // OSS Bucket 配置
}
```

### 错误处理

- 返回错误而不是记录后继续执行
- 使用 `fmt.Errorf()` 添加上下文
- 使用 logrus 记录错误
- 显式处理错误，避免使用 `_` 忽略
- 示例：
```go
if err != nil {
    log.WithError(err).Error("加载配置失败")
    return nil, err
}
```

### 日志规范

- 使用 `github.com/sirupsen/logrus` 记录日志，通过 `internal/logging` 创建独立实例
- 日志级别: Trace, Debug, Info, Warn, Error；通过返回错误退出，避免调用 Fatal / Panic
- 使用 `TextFormatter`，完整时间戳，按级别着色；通过 `WithFields` / `WithError` 记录 Bucket、域名、阶段和错误
- 日志写入 stderr；帮助、版本、配置导出和状态正文保留 stdout
- `--color=auto|always|never` 控制颜色，`--log-level` 控制等级，禁止记录配置全文、私钥或凭证
- 日志和注释可使用中文（项目约定）

### 上下文使用

- 将 `context.Context` 作为函数第一个参数
- 使用 context 处理取消和超时
- 示例：
```go
func (c *AutoCert) Run(ctx context.Context) error {
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
        // 处理逻辑
    }
}
```

### 并发规范

- 简单标志使用 `atomic.Bool`
- goroutine 间使用 channel 通信
- 始终处理 context 取消信号
- 示例：
```go
type Service struct {
    running atomic.Bool
    msgCh   chan string
}
```

### 包组织结构

- `cmd/oss-auto-cert/main.go`: 推荐入口；根目录 `main.go` 保留旧构建方式
- `internal/`: 内部实现
  - `cli/`: Cobra 命令、旧参数适配和输出；不使用全局 init 加载配置
  - `app/`: 用例组装、生命周期及退出码
  - `config/`: TOML v2、旧 YAML 适配、覆盖和迁移；核心只消费统一 Config
  - `certificate/`: 证书计划、签发及逐目标部署恢复；不依赖具体 SDK 或 Cobra
  - `provider/aliyun/`: OSS/CDN/CAS 适配
  - `provider/acme/`: Let's Encrypt、账户和 HTTP-01 验证
  - `storage/`: 账户、证书、状态、运行锁及旧文件只读导入
  - `notification/`: 内置 JSON 消息及兼容模板
  - `testutil/`: 离线测试证书

CLI 使用 `github.com/spf13/cobra`；通过显式参数记录区分默认值与用户覆盖，旧多字母单横线参数在进入 pflag 前归一化。不引入 Viper。离线测试不调用真实 CA 或云资源；真实集成测试使用 `integration` build tag。

## 测试规范

- 测试文件: `*_test.go`
- 使用标准 `testing` 包
- 测试函数命名: `TestFunctionName`
- 使用 `t.Log()` 记录测试日志
- 显式检查错误
- 示例：
```go
func TestDateIsExpire(t *testing.T) {
    result := DateIsExpire("2024-10-13", time.Hour*24*30)
    t.Logf("result: %v", result)
}
```

## Docker 命令

```bash
# 构建镜像
docker build -t oss-auto-cert:latest .

# 运行容器
docker run -d --rm \
  -v $PWD/config.yaml:/etc/oss-auto-cert/config.yaml \
  -e OSS_ACCESS_KEY_ID=xxx \
  -e OSS_ACCESS_KEY_SECRET=xxx \
  oss-auto-cert:latest

# Docker Compose
docker-compose up -d
```

## 环境变量

- `OSS_ACCESS_KEY_ID`: 阿里云 Access Key ID（必填）
- `OSS_ACCESS_KEY_SECRET`: 阿里云 Access Key Secret（必填）
- `ACME_EMAIL`: 证书注册邮箱
- `ACME_DATA_DIR`: 证书存储目录
- `ACME_EXPIRED_EARLY`: 提前续期天数
- `DEBUG`: 旧 YAML 保留 staging + 强制续期；TOML 忽略并提示。调试日志使用 `--log-level=debug`

## 安全规范

- 切勿记录或暴露凭证
- 敏感数据使用环境变量
- 验证所有外部输入
- 优雅处理错误

## CI/CD

GitHub Actions 工作流位于 `.github/workflows/release-docker.yml`：
- 版本标签（v*）触发
- 构建多架构镜像（linux/amd64, linux/arm64）
- 发布到 Docker Hub、GHCR 和阿里云镜像仓库

## 其他说明

- 项目对用户可见的消息和注释使用中文
- 遵循代码库现有模式
- 保持函数专注且精简
- 导出函数需添加文档注释
- 使用依赖注入提高可测试性
