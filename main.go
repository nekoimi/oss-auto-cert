package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/charmbracelet/log"
	"github.com/nekoimi/oss-auto-cert/internal/cert"
	"github.com/nekoimi/oss-auto-cert/internal/config"
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
	conf.LoadOptionsFromEnv()
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	m, err := cert.NewAutoCert(ctx, conf)
	if err != nil {
		log.Fatal(err)
	}

	m.ScheduleRun()

	log.Infof("oss-auto-cert 服务已成功启动，进程常驻运行中")
	log.Infof("配置文件路径: %s", conf.Path)
	switch n := len(conf.Buckets); {
	case n == 0:
		log.Warnf("当前未配置任何 Bucket，请检查配置文件中的 buckets 段")
	case n <= 5:
		names := make([]string, 0, n)
		for _, b := range conf.Buckets {
			names = append(names, b.Name)
		}
		log.Infof("监控 Bucket（%d）: %s", n, strings.Join(names, ", "))
	default:
		names := make([]string, 0, 5)
		for i := 0; i < 5; i++ {
			names = append(names, conf.Buckets[i].Name)
		}
		log.Infof("监控 Bucket 共 %d 个: %s 等", n, strings.Join(names, ", "))
	}
	log.Infof("调度说明: 启动时已触发一次巡检；之后每隔 %s 自动巡检一次", cert.ScheduleInterval)
	log.Infof("退出方式: 发送 SIGINT、SIGTERM 或 SIGQUIT 可优雅退出")

	// wait
	select {
	case <-sig:
		cancel()
		m.Stop()
		log.Infof("Exit.")
		os.Exit(0)
	}
}
