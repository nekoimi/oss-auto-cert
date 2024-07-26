package core

import (
	"flag"
	"github.com/nekoimi/oss-auto-cert/config"
	"testing"
)

var conf = new(config.Config)

func init() {
	flag.StringVar(&conf.Path, "config-path", "", "配置文件路径")
	flag.Parse()
	conf.LoadOptions()
}

func TestRun(t *testing.T) {
	manager := New(conf)

	manager.CertRun()
}
