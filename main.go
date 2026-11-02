// 保留根目录入口，兼容 go build main.go。
package main

import (
	"os"

	"github.com/nekoimi/oss-auto-cert/internal/cli"
)

func main() { os.Exit(cli.Main()) }
