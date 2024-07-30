package config

import (
	"github.com/charmbracelet/log"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"time"
)

const (
	DefaultConfigPath = "/etc/oss-auto-cert/config.yaml"
	// ExpiredEarly 提前过期时间点
	ExpiredEarly = time.Hour * 24 * 5
)

type Config struct {
	// 配置文件路径
	// 默认路径: DefaultConfigPath
	Path string
	// 通知地址
	Webhook string `yaml:"webhook"`
	// Acme配置
	Acme Acme `yaml:"acme"`
	// Bucket配置
	Buckets []Bucket `yaml:"buckets"`
}

type Acme struct {
	// 证书申请邮箱
	Email string `yaml:"email"`

	// 证书保存位置
	DataDir string `yaml:"data-dir"`
}

// Bucket OSS存储Bucket配置
type Bucket struct {
	// bucket名称
	Name string `yaml:"name"`
	// Endpoint
	Endpoint string `yaml:"endpoint"`
}

// LoadOptions 加载配置
func (conf *Config) LoadOptions() {
	if conf.Path == "" {
		conf.Path = DefaultConfigPath
	}

	f, err := os.Open(conf.Path)
	if err != nil {
		log.Fatalf("读取配置文件 %s 出错: %s", conf.Path, err.Error())
	}
	defer f.Close()

	bts, err := io.ReadAll(f)
	if err != nil {
		log.Fatalf("读取配置文件 %s 出错: %s", conf.Path, err.Error())
	}

	err = yaml.Unmarshal(bts, &conf)
	if err != nil {
		log.Fatalf("读取配置文件 %s 出错: %s", conf.Path, err.Error())
	}

	log.Debugf("配置文件: %s", conf)
}