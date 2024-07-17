package config

import (
	"gopkg.in/yaml.v3"
	"io"
	"log"
	"os"
)

type Config struct {
	Path    string
	Options Options
}

type Options struct {
}

func (conf *Config) LoadOptions() {
	yamlF, err := os.Open(conf.Path)
	if err != nil {
		log.Fatalln(err)
	}
	defer yamlF.Close()

	yamlBytes, err := io.ReadAll(yamlF)
	if err != nil {
		log.Fatalln(err)
	}

	err = yaml.Unmarshal(yamlBytes, &conf.Options)
	if err != nil {
		log.Fatalln(err)
	}
}
