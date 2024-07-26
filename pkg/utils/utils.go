package utils

import (
	"fmt"
	"log"
	"strings"
	"time"
)

func StrSplitFirst(s string, sep string) string {
	for _, item := range strings.SplitN(s, sep, 2) {
		return item
	}
	return s
}

// DateIsExpire 是否过期
// aheadHours 提前过期小时数
func DateIsExpire(dateStr string, aheadHours time.Duration) bool {
	now := time.Now()
	// yyyy-MM-dd
	target, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		log.Printf("日期解析异常: %s, %s\n", dateStr, err.Error())
		return false
	}

	if target.Before(now) {
		// 当前时间比目标时间晚 => 过期
		// 当前时间：2024-07-26 目标时间：2024-07-25
		return true
	}

	// 目标时间比当前时间晚
	// 获取当前时间到目标时间的剩余小时数
	diff := target.Sub(now)
	fmt.Printf("diff: %f, ahead: %f \n", diff.Hours(), aheadHours.Hours())
	// 剩余小时数是否小于提前过期小时数
	// 如果小于 => 提前过期
	return diff.Hours() < aheadHours.Hours()
}

func DateExpireDay(dateStr string) int {
	target, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		log.Printf("日期解析异常: %s, %s\n", dateStr, err.Error())
		return 0
	}

	diff := target.Sub(time.Now())
	return int(diff.Hours()) / 24
}
