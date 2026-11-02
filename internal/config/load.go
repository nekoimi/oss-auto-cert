package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

// Document 是 TOML 文件模型，指针区分省略与零值。
type Document struct {
	Version int `toml:"version"`
	Storage struct {
		DataDir *string `toml:"data-dir,omitempty"`
	} `toml:"storage"`
	Schedule struct {
		Interval   *string `toml:"interval,omitempty"`
		RunOnStart *bool   `toml:"run-on-start,omitempty"`
	} `toml:"schedule"`
	Renewal struct {
		BeforeDays *int `toml:"before-days,omitempty"`
	} `toml:"renewal"`
	ACME struct {
		Provider    string  `toml:"provider,omitempty"`
		Email       string  `toml:"email,omitempty"`
		Environment string  `toml:"environment,omitempty"`
		KeyType     string  `toml:"key-type,omitempty"`
		Timeout     *string `toml:"timeout,omitempty"`
	} `toml:"acme"`
	Runtime struct {
		Workers         *int    `toml:"workers,omitempty"`
		RequestTimeout  *string `toml:"request-timeout,omitempty"`
		ShutdownTimeout *string `toml:"shutdown-timeout,omitempty"`
	} `toml:"runtime"`
	Buckets       []BucketDocument `toml:"buckets"`
	Notifications []Notification   `toml:"notifications,omitempty"`
}

// BucketDocument 是新版 Bucket 文件模型。
type BucketDocument struct {
	Name                 string   `toml:"name"`
	Endpoint             string   `toml:"endpoint"`
	DomainMode           string   `toml:"domain-mode,omitempty"`
	Domains              []string `toml:"domains,omitempty"`
	AllowInitialIssuance bool     `toml:"allow-initial-issuance"`
	Challenge            struct {
		Type string `toml:"type,omitempty"`
	} `toml:"challenge"`
	Deploy struct {
		OSS *bool  `toml:"oss,omitempty"`
		CDN string `toml:"cdn,omitempty"`
	} `toml:"deploy"`
}

// LegacyConfig 仅用于旧 YAML 解析。
type LegacyConfig struct {
	Version    int    `yaml:"version"`
	Webhook    string `yaml:"webhook"`
	WebhookTpl string `yaml:"webhook-tpl"`
	ACME       struct {
		Email        string `yaml:"email"`
		DataDir      string `yaml:"data-dir"`
		ExpiredEarly int    `yaml:"expired-early"`
	} `yaml:"acme"`
	Buckets []struct {
		Name     string `yaml:"name"`
		Endpoint string `yaml:"endpoint"`
	} `yaml:"buckets"`
}

// LoadOptions 控制环境注入与文件搜索。
type LoadOptions struct {
	Path       string
	LookupEnv  func(string) (string, bool)
	SearchDirs []string
	IgnoreEnv  bool
}

// Load 读取选中文件，应用环境覆盖并校验。
func Load(o LoadOptions) (Result, error) {
	var result Result
	path, err := Discover(o.Path, o.SearchDirs)
	if err != nil {
		return result, err
	}
	result.Path = path
	raw, err := os.ReadFile(path)
	if err != nil {
		return result, fmt.Errorf("读取配置文件失败: %w", err)
	}
	c := Defaults()
	legacy := false
	switch strings.ToLower(filepath.Ext(path)) {
	case ".toml":
		var d Document
		if err := toml.NewDecoder(bytes.NewReader(raw)).DisallowUnknownFields().Decode(&d); err != nil {
			return result, fmt.Errorf("TOML 配置解析失败: %w", err)
		}
		if d.Version != 2 {
			return result, fmt.Errorf("TOML 配置要求 version = 2")
		}
		if err := applyDocument(&c, d); err != nil {
			return result, err
		}
	case ".yaml", ".yml":
		legacy = true
		var d LegacyConfig
		dec := yaml.NewDecoder(bytes.NewReader(raw))
		if err := dec.Decode(&d); err != nil {
			return result, fmt.Errorf("旧 YAML 解析失败: %w", err)
		}
		var extra any
		if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
			return result, fmt.Errorf("配置只允许一个 YAML 文档")
		}
		strict := yaml.NewDecoder(bytes.NewReader(raw))
		strict.KnownFields(true)
		if err := strict.Decode(&LegacyConfig{}); err != nil {
			result.Warnings = append(result.Warnings, "旧 YAML 含未知字段，已兼容忽略，请检查迁移结果")
		}
		if d.Version != 0 && d.Version != 1 {
			return result, fmt.Errorf("YAML 只支持旧版 version: 1 或省略版本")
		}
		c.ACME.Email = d.ACME.Email
		if d.ACME.DataDir != "" {
			c.DataDir = d.ACME.DataDir
		}
		days := max(15, d.ACME.ExpiredEarly)
		if days > int(time.Duration(1<<63-1)/(24*time.Hour)) {
			return result, fmt.Errorf("旧续期提前天数超出可表示范围")
		}
		c.RenewBefore = time.Duration(days) * 24 * time.Hour
		for _, b := range d.Buckets {
			c.Buckets = append(c.Buckets, Bucket{Name: b.Name, Endpoint: b.Endpoint, DomainMode: "first", DeployOSS: true, CDN: "auto"})
		}
		if d.Webhook != "" {
			n := Notification{Type: "wecom", URL: d.Webhook}
			if d.WebhookTpl != "" {
				n.Type = "legacy-template"
				n.Template = d.WebhookTpl
			}
			c.Notifications = append(c.Notifications, n)
		}
		if len(c.Buckets) == 0 {
			result.Warnings = append(result.Warnings, "未配置任何 Bucket，常驻模式将保持空闲")
		}
	default:
		return result, fmt.Errorf("配置扩展名必须为 .toml、.yaml 或 .yml")
	}
	if !o.IgnoreEnv {
		lookup := o.LookupEnv
		if lookup == nil {
			lookup = os.LookupEnv
		}
		for _, binding := range []struct {
			name string
			dst  *string
		}{{"ACME_EMAIL", &c.ACME.Email}, {"ACME_DATA_DIR", &c.DataDir}} {
			if v, ok := lookup(binding.name); ok && v != "" {
				*binding.dst = v
			}
		}
		if v, ok := lookup("ACME_EXPIRED_EARLY"); ok && v != "" {
			days, err := strconv.Atoi(v)
			if err != nil {
				if !legacy {
					return result, fmt.Errorf("ACME_EXPIRED_EARLY 必须为整数")
				}
				result.Warnings = append(result.Warnings, "ACME_EXPIRED_EARLY 无效，保留文件值")
			} else {
				if legacy {
					days = max(15, days)
				}
				if days < 1 || (!legacy && days > 60) || days > int(time.Duration(1<<63-1)/(24*time.Hour)) {
					return result, fmt.Errorf("ACME_EXPIRED_EARLY 必须在 1 至 60 之间")
				}
				c.RenewBefore = time.Duration(days) * 24 * time.Hour
			}
		}
		if v, _ := lookup("DEBUG"); v == "true" {
			if legacy {
				c.ACME.Environment = "staging"
				c.Force = true
				result.Warnings = append(result.Warnings, "旧 DEBUG=true：使用 staging 并强制续期，请勿用于生产")
			} else {
				result.Warnings = append(result.Warnings, "TOML v2 忽略 DEBUG，请使用 --log-level=debug；测试需显式选择 staging")
			}
		}
	}
	if err := validate(&c, legacy); err != nil {
		return result, err
	}
	if !legacy && len(c.Buckets) == 0 {
		return result, fmt.Errorf("新版配置至少需要一个 Bucket")
	}
	result.Config = c
	return result, nil
}

// Discover 查找唯一配置，显式路径不回退。
func Discover(path string, dirs []string) (string, error) {
	if path != "" {
		return path, nil
	}
	if dirs == nil {
		dirs = []string{"/etc/oss-auto-cert", "."}
	}
	for _, dir := range dirs {
		var found []string
		for _, name := range []string{"config.toml", "config.yaml", "config.yml"} {
			p := filepath.Join(dir, name)
			info, err := os.Stat(p)
			if err == nil {
				if info.IsDir() {
					return "", fmt.Errorf("配置路径 %s 是目录", p)
				}
				found = append(found, p)
			} else if !errors.Is(err, os.ErrNotExist) {
				return "", err
			}
		}
		if len(found) > 1 {
			return "", fmt.Errorf("目录 %s 存在多个配置，请用 --config 明确选择", dir)
		}
		if len(found) == 1 {
			return found[0], nil
		}
	}
	return "", fmt.Errorf("未找到配置，请先执行 config init 或用 --config 指定")
}

func applyDocument(c *Config, d Document) error {
	if d.ACME.Provider != "" && d.ACME.Provider != "letsencrypt" {
		return fmt.Errorf("acme.provider 本轮只支持 letsencrypt")
	}
	if d.ACME.KeyType != "" && d.ACME.KeyType != "rsa2048" {
		return fmt.Errorf("acme.key-type 本轮只支持 rsa2048")
	}
	if d.Storage.DataDir != nil {
		c.DataDir = *d.Storage.DataDir
	}
	if d.Schedule.RunOnStart != nil {
		c.RunOnStart = *d.Schedule.RunOnStart
	}
	if d.Renewal.BeforeDays != nil {
		days := *d.Renewal.BeforeDays
		if days < 1 || days > 60 {
			return fmt.Errorf("renewal.before-days 必须在 1 至 60 之间")
		}
		c.RenewBefore = time.Duration(days) * 24 * time.Hour
	}
	c.ACME.Email = d.ACME.Email
	if d.ACME.Environment != "" {
		c.ACME.Environment = d.ACME.Environment
	}
	if d.Runtime.Workers != nil {
		c.Workers = *d.Runtime.Workers
	}
	for _, v := range []struct {
		name string
		src  *string
		dst  *time.Duration
	}{{"schedule.interval", d.Schedule.Interval, &c.Interval}, {"acme.timeout", d.ACME.Timeout, &c.ACME.Timeout}, {"runtime.request-timeout", d.Runtime.RequestTimeout, &c.RequestTimeout}, {"runtime.shutdown-timeout", d.Runtime.ShutdownTimeout, &c.ShutdownTimeout}} {
		if v.src != nil {
			duration, err := time.ParseDuration(*v.src)
			if err != nil {
				return fmt.Errorf("%s 必须为有效时长", v.name)
			}
			*v.dst = duration
		}
	}
	for _, b := range d.Buckets {
		if b.Challenge.Type != "" && b.Challenge.Type != "oss-http-01" {
			return fmt.Errorf("challenge.type 本轮只支持 oss-http-01")
		}
		item := Bucket{Name: b.Name, Endpoint: b.Endpoint, DomainMode: b.DomainMode, Domains: b.Domains, AllowInitialIssuance: b.AllowInitialIssuance, DeployOSS: true, CDN: b.Deploy.CDN}
		if item.DomainMode == "" {
			item.DomainMode = "listed"
		}
		if item.CDN == "" {
			item.CDN = "auto"
		}
		if b.Deploy.OSS != nil {
			item.DeployOSS = *b.Deploy.OSS
		}
		c.Buckets = append(c.Buckets, item)
	}
	c.Notifications = d.Notifications
	return nil
}

// Encode 导出显式策略，不读取环境变量。
func Encode(c Config) ([]byte, error) {
	if err := validate(&c, false); err != nil {
		return nil, fmt.Errorf("无法转换为 TOML v2，请先调整旧配置: %w", err)
	}
	if len(c.Buckets) == 0 {
		return nil, fmt.Errorf("TOML v2 要求至少一个 Bucket，请补充后迁移")
	}
	d := Document{Version: 2}
	d.Storage.DataDir = &c.DataDir
	interval := c.Interval.String()
	d.Schedule.Interval = &interval
	d.Schedule.RunOnStart = &c.RunOnStart
	days := int(c.RenewBefore / (24 * time.Hour))
	d.Renewal.BeforeDays = &days
	d.ACME.Provider = "letsencrypt"
	d.ACME.Email = c.ACME.Email
	d.ACME.Environment = c.ACME.Environment
	d.ACME.KeyType = "rsa2048"
	timeout := c.ACME.Timeout.String()
	d.ACME.Timeout = &timeout
	d.Runtime.Workers = &c.Workers
	request, shutdown := c.RequestTimeout.String(), c.ShutdownTimeout.String()
	d.Runtime.RequestTimeout = &request
	d.Runtime.ShutdownTimeout = &shutdown
	for _, b := range c.Buckets {
		item := BucketDocument{Name: b.Name, Endpoint: b.Endpoint, DomainMode: b.DomainMode, Domains: b.Domains, AllowInitialIssuance: b.AllowInitialIssuance}
		enabled := b.DeployOSS
		item.Deploy.OSS = &enabled
		item.Deploy.CDN = b.CDN
		item.Challenge.Type = "oss-http-01"
		d.Buckets = append(d.Buckets, item)
	}
	d.Notifications = c.Notifications
	return toml.Marshal(d)
}
