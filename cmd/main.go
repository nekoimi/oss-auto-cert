package main

import (
	"flag"
	"github.com/nekoimi/oss-auto-cert/internal/cert"
	"github.com/nekoimi/oss-auto-cert/internal/config"
)

var autoCertConf = new(config.Config)

func init() {
	flag.StringVar(&autoCertConf.Path, "config-path", "/etc/oss-auto-cert/config.yaml", "配置文件路径")
}

func main() {
	flag.Parse()

	cert.Run(autoCertConf)
}
