package main

import (
	"log"
	"strings"
	"testing"
	"time"
)

func TestCasID(t *testing.T) {
	id := "123456789-cn-hangzhou"
	for _, item := range strings.SplitN(id, "-", 2) {
		t.Log(item)
	}
}

func TestExpire(t *testing.T) {
	log.Println(dateIsExpire("2024-07-27", time.Hour*3))
}
