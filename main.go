package main

import (
	"flag"
)

var conf = new(Config)

func init() {
	flag.StringVar(&conf.Path, "config-path", "", "配置文件路径")
}

func main() {
	flag.Parse()

	Run(conf)
}
