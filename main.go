package main

import (
	"flag"
	"github.com/charmbracelet/log"
	"github.com/nekoimi/oss-auto-cert/config"
	"github.com/nekoimi/oss-auto-cert/core"
	"os"
	"os/signal"
	"syscall"
)

var (
	sig  = make(chan os.Signal)
	conf = new(config.Config)
)

func init() {
	log.SetLevel(log.DebugLevel)

	flag.StringVar(&conf.Path, "config", "", "配置文件路径")
	flag.Parse()

	conf.LoadOptions()

	watchSig()
}

func watchSig() {
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
}

func main() {
	m := core.New(conf)

	m.Run()
	//tick := time.NewTicker(6 * time.Hour)
	//for {
	//	select {
	//	case <-sig:
	//		tick.Stop()
	//		log.Println("Exit.")
	//		os.Exit(0)
	//	case <-tick.C:
	//		go cm.Run()
	//	}
	//}
}
