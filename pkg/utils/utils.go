package utils

import (
	"bytes"
	"github.com/charmbracelet/log"
	"github.com/google/uuid"
	"strings"
	"time"
)

// DateIsExpire 是否过期
// aheadHours 提前过期时间
func DateIsExpire(dateStr string, aheadHours time.Duration) bool {
	now := time.Now()
	// yyyy-MM-dd
	target, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		log.Errorf("日期解析异常: %s, %s", dateStr, err.Error())
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
	log.Debugf("diff: %f, ahead: %f", diff.Hours(), aheadHours.Hours())
	// 剩余小时数是否小于提前过期小时数
	// 如果小于 => 提前过期
	return diff.Hours() < aheadHours.Hours()
}

// TimeDiffDay 获取当前时间到目标时间相差的天数
func TimeDiffDay(dateStr string) int {
	target, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		log.Errorf("日期解析异常: %s, %s", dateStr, err.Error())
		return 0
	}

	now := time.Now()
	if target.Before(now) {
		return 0
	}

	diff := target.Sub(now)
	return int(diff.Hours()) / 24
}

// ShortDomain 简短化域名
// app.test.com => app-t-com
func ShortDomain(domain string) string {
	if domain == "" {
		return ""
	}

	buf := bytes.NewBufferString("")
	ss := strings.Split(strings.ReplaceAll(domain, "-", ""), ".")
	for i, s := range ss {
		if i == 0 {
			buf.WriteString(s)
		} else if i == len(ss)-1 {
			buf.WriteString("-")
			buf.WriteString(s)
		} else {
			buf.WriteString("-")
			buf.WriteString(s[:1])
		}
	}
	return buf.String()
}

func UUID() string {
	return uuid.New().String()
}

// SplitFirst 按指定符号分割 取第一个
func SplitFirst(s string, sep string) string {
	for _, item := range strings.SplitN(s, sep, 2) {
		return item
	}
	return s
}

func SplitGetN(s string, sep string, n int) string {
	for i, item := range strings.Split(s, sep) {
		if i == n-1 {
			return item
		}
	}
	return s
}
