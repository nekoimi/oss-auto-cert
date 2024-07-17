package cert

import (
	"fmt"
	"github.com/nekoimi/oss-auto-cert/internal/config"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func Run(conf *config.Config) {
	conf.LoadOptions()

	signalChan := make(chan os.Signal)

	signal.Notify(signalChan, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	for {
		select {
		case <-signalChan:
			break
		case <-time.Tick(3 * time.Second):
			fmt.Println(time.Now())
		}
	}
}
