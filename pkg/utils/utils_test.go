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

	t.Log(SplitGetN(id, "-", 2, 2))
}

func TestExpire(t *testing.T) {
	t.Log(DateIsExpire("2024-07-27", time.Hour*3))
	t.Log(DateIsExpire("2024-10-13T23:59:59Z", time.Hour*24*time.Duration(30)))
	t.Log(DateIsExpire("2024-10-13", time.Hour*24*time.Duration(30)))
}

func TestShortDomain(t *testing.T) {
	t.Log(ShortDomain("tieba.baidu.com"))
}

func TestUUID(t *testing.T) {
	t.Log(UUID())
}
