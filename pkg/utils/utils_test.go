package utils

import (
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
	t.Log(DateIsExpire("2024-07-27", time.Hour*3))
}

func TestShortDomain(t *testing.T) {
	t.Log(ShortDomain("tieba.baidu.com"))
}

func TestUUID(t *testing.T) {
	t.Log(UUID())
}
