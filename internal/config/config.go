// Package config 根据文件扩展名将旧 YAML 和 TOML 转换为统一运行配置。
package config

import (
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Config 是业务使用的配置，不包含配置格式分支。
type Config struct {
	DataDir         string
	Interval        time.Duration
	RunOnStart      bool
	RenewBefore     time.Duration
	ACME            ACME
	Workers         int
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
	Force           bool
	Buckets         []Bucket
	Notifications   []Notification
}

// ACME 保存证书签发参数。
type ACME struct {
	Email, Environment string
	Timeout            time.Duration
}

// Bucket 描述管理范围及部署策略。
type Bucket struct {
	Name, Endpoint, DomainMode string
	Domains                    []string
	AllowInitialIssuance       bool
	DeployOSS                  bool
	CDN                        string
}

// Notification 描述通知渠道，URL 不得写入日志。
type Notification struct {
	Type     string   `toml:"type"`
	URL      string   `toml:"url,omitempty"`
	URLEnv   string   `toml:"url-env,omitempty"`
	Template string   `toml:"template,omitempty"`
	Events   []string `toml:"events,omitempty"`
}

// Result 包含有效配置及来源诊断。
type Result struct {
	Config   Config
	Path     string
	Warnings []string
}

// Defaults 返回独立的默认配置。
func Defaults() Config {
	return Config{DataDir: "/var/lib/oss-auto-cert", Interval: 6 * time.Hour, RunOnStart: true,
		RenewBefore: 15 * 24 * time.Hour, ACME: ACME{Environment: "production", Timeout: 2 * time.Minute},
		Workers: 2, RequestTimeout: 30 * time.Second, ShutdownTimeout: 30 * time.Second}
}

var labelPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var envPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Domain 校验并归一化本轮支持的 ASCII 域名。
func Domain(value string) (string, error) {
	d := strings.ToLower(strings.TrimSuffix(value, "."))
	if len(d) > 253 || !strings.Contains(d, ".") {
		return "", fmt.Errorf("域名 %q 无效，请使用完整 ASCII 域名", value)
	}
	for _, label := range strings.Split(d, ".") {
		if !labelPattern.MatchString(label) {
			return "", fmt.Errorf("域名 %q 无效；本轮不支持通配符和非 ASCII 域名", value)
		}
	}
	return d, nil
}

func validate(c *Config, legacy bool) error {
	if strings.TrimSpace(c.DataDir) == "" {
		return fmt.Errorf("storage.data-dir 不能为空")
	}
	if c.Interval <= 0 || c.RequestTimeout <= 0 || c.ShutdownTimeout <= 0 || c.ACME.Timeout <= 0 {
		return fmt.Errorf("巡检、请求、签发及退出时长必须为正数")
	}
	if c.Workers < 1 || c.Workers > 16 {
		return fmt.Errorf("runtime.workers 必须在 1 至 16 之间")
	}
	if !legacy && (c.RenewBefore < 24*time.Hour || c.RenewBefore > 60*24*time.Hour) {
		return fmt.Errorf("renewal.before-days 必须在 1 至 60 之间")
	}
	if c.ACME.Environment != "production" && c.ACME.Environment != "staging" {
		return fmt.Errorf("acme.environment 只能为 production 或 staging")
	}
	if c.ACME.Email != "" {
		a, err := mail.ParseAddress(c.ACME.Email)
		if err != nil || a.Address != c.ACME.Email {
			return fmt.Errorf("acme.email 必须是有效邮箱地址")
		}
	}
	names, domains := map[string]bool{}, map[string]bool{}
	for i := range c.Buckets {
		b := &c.Buckets[i]
		if len(b.Name) < 3 || len(b.Name) > 63 || !labelPattern.MatchString(b.Name) {
			return fmt.Errorf("buckets[%d].name 无效", i)
		}
		if names[b.Name] {
			return fmt.Errorf("Bucket %q 重复配置", b.Name)
		}
		names[b.Name] = true
		u, err := url.Parse(b.Endpoint)
		if !strings.Contains(b.Endpoint, "://") {
			u, err = url.Parse("https://" + b.Endpoint)
		}
		if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("Bucket %s 的 endpoint 无效", b.Name)
		}
		b.Endpoint = strings.TrimSuffix(u.String(), "/")
		switch b.DomainMode {
		case "listed":
			if len(b.Domains) == 0 {
				return fmt.Errorf("Bucket %s 的 listed 模式要求 domains 非空", b.Name)
			}
		case "first", "all":
			if len(b.Domains) != 0 {
				return fmt.Errorf("Bucket %s 的 %s 模式不能同时填写 domains", b.Name, b.DomainMode)
			}
		default:
			return fmt.Errorf("Bucket %s 的 domain-mode 无效", b.Name)
		}
		for j, d := range b.Domains {
			normalized, err := Domain(d)
			if err != nil {
				return err
			}
			if domains[normalized] {
				return fmt.Errorf("域名 %s 重复配置", normalized)
			}
			domains[normalized] = true
			b.Domains[j] = normalized
		}
		if b.CDN != "auto" && b.CDN != "off" {
			return fmt.Errorf("Bucket %s 的 deploy.cdn 只能为 auto 或 off", b.Name)
		}
		if !b.DeployOSS && b.CDN == "off" {
			return fmt.Errorf("Bucket %s 至少需要一个部署目标", b.Name)
		}
	}
	for i, n := range c.Notifications {
		switch n.Type {
		case "wecom", "dingtalk", "feishu", "template", "legacy-template":
		default:
			return fmt.Errorf("notifications[%d].type 无效", i)
		}
		if (n.URL == "") == (n.URLEnv == "") {
			return fmt.Errorf("notifications[%d] 必须且只能指定 url 或 url-env", i)
		}
		if n.URL != "" {
			if err := ValidateURL(n.URL); err != nil {
				return fmt.Errorf("notifications[%d].url 无效", i)
			}
		}
		if n.URLEnv != "" && !envPattern.MatchString(n.URLEnv) {
			return fmt.Errorf("notifications[%d].url-env 无效", i)
		}
		if (n.Type == "template" || n.Type == "legacy-template") && n.Template == "" {
			return fmt.Errorf("notifications[%d].template 不能为空", i)
		}
		for _, event := range n.Events {
			if event != "updated" && event != "failed" && event != "recovered" {
				return fmt.Errorf("notifications[%d].events 含未知事件", i)
			}
		}
	}
	return nil
}

// ValidateURL 校验通知地址，不在错误中回显令牌。
func ValidateURL(value string) error {
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("需要有效 HTTP(S) 地址，不能包含用户信息或片段")
	}
	return nil
}
