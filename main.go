package main

import (
	"flag"
	"github.com/nekoimi/oss-auto-cert/config"
	"github.com/nekoimi/oss-auto-cert/core"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var conf = new(config.Config)

func init() {
	flag.StringVar(&conf.Path, "config-path", "", "配置文件路径")
	flag.Parse()
	conf.LoadOptions()
}

func main() {
	sig := make(chan os.Signal)
	manager := core.New(conf)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	tick := time.NewTicker(6 * time.Hour)
	for {
		select {
		case <-sig:
			tick.Stop()
			log.Println("Exit.")
			os.Exit(0)
		case <-tick.C:
			go manager.CertRun()
		}
	}
}
