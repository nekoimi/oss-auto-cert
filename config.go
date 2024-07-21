package main

import (
	"gopkg.in/yaml.v3"
	"io"
	"log"
	"os"
)

const (
	DefaultConfigPath = "/etc/oss-auto-cert/config.yaml"
	PathEnv           = "OSS_AUTO_CERT_CONFIG"
)

type Config struct {
	// 配置文件路径
	// 默认路径: DefaultConfigPath
	Path string
	// Acme配置
	Acme Acme `yaml:"acme,omitempty"`
	// Bucket配置
	Buckets []Bucket `yaml:"buckets,omitempty"`
}

type Acme struct {
	Email string `yaml:"email,omitempty"`
}

// Bucket OSS存储Bucket配置
type Bucket struct {
	// bucket名称
	Name string `yaml:"name,omitempty"`
	// 地域
	Region string `yaml:"region,omitempty"`
}

// LoadOptions 加载配置
func (conf *Config) LoadOptions() {
	//conf.loadAccessKey()

	if conf.Path == "" {
		// 检查环境变量配置
		if value, ok := os.LookupEnv(PathEnv); ok {
			conf.Path = value
		} else {
			conf.Path = DefaultConfigPath
		}
	}

	f, err := os.Open(conf.Path)
	if err != nil {
		log.Fatalf("读取配置文件 %s 出错: %s \n", conf.Path, err.Error())
	}
	defer f.Close()

	bts, err := io.ReadAll(f)
	if err != nil {
		log.Fatalf("读取配置文件 %s 出错: %s \n", conf.Path, err.Error())
	}

	err = yaml.Unmarshal(bts, &conf)
	if err != nil {
		log.Fatalf("读取配置文件 %s 出错: %s \n", conf.Path, err.Error())
	}

	log.Printf("Config: %s\n", conf)
}
