package main

import (
	"flag"
	"github.com/charmbracelet/log"
	"github.com/nekoimi/oss-auto-cert/config"
	"github.com/nekoimi/oss-auto-cert/core"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var (
	logLevel string
	sig      = make(chan os.Signal)
	conf     = new(config.Config)
)

func init() {
	flag.StringVar(&logLevel, "log-level", "info", "日志等级")
	flag.StringVar(&conf.Path, "config", "", "配置文件路径")
	flag.Parse()

	log.SetReportCaller(true)
	if level, err := log.ParseLevel(logLevel); err != nil {
		log.Warnf("Invalid log level parameter: %s. Use default info level!", logLevel)
		log.SetLevel(log.InfoLevel)
	} else {
		log.SetLevel(level)
	}

	conf.LoadOptions()

	watchSig()
}

func watchSig() {
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
}

func main() {
	m := core.New(conf)

	tick := time.NewTicker(6 * time.Hour)
	for {
		select {
		case <-sig:
			tick.Stop()
			log.Infof("Exit.")
			os.Exit(0)
		case <-tick.C:
			go m.Run()
		}
	}
}
