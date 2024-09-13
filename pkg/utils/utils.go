package utils

import (
	"bytes"
	"github.com/charmbracelet/log"
	"github.com/google/uuid"
	"os"
	"strings"
	"time"
)

var timeLayouts []string

func init() {
	timeLayouts = append(timeLayouts, time.RFC3339)
	timeLayouts = append(timeLayouts, time.DateOnly)
	timeLayouts = append(timeLayouts, time.DateTime)
}

func StrToTime(s string) (time.Time, error) {
	var (
		err    error
		target time.Time
	)

	for _, layout := range timeLayouts {
		target, err = time.Parse(layout, s)
		if err != nil {
			log.Debugf("format str to time err: layout %s for %s, %s", layout, s, err.Error())
		} else {
			break
		}
	}

	return target, err
}

// DateIsExpire 日期是否过期
// dateStr 日期字符串，格式：2006-01-02
// aheadHours 提前过期时间
func DateIsExpire(dateStr string, aheadHours time.Duration) bool {
	now := time.Now()

	// 字符串转成时间对象
	target, err := StrToTime(dateStr)
	if err != nil {
		log.Errorf(err.Error())
		return false
	}

	// fix: 上述转换格式可能不是 time.DateOnly 格式
	// 需要主动转换成 time.DateOnly 格式，避免因为时间导致比较不一致问题
	target, err = time.Parse(time.DateOnly, target.Format(time.DateOnly))
	if err != nil {
		log.Errorf(err.Error())
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

	debug := os.Getenv("DEBUG")
	if debug == "true" {
		// 测试直接让证书过期
		return true
	}

	return diff.Hours() <= aheadHours.Hours()
}

// TimeDiffDay 获取当前时间到目标时间相差的天数
func TimeDiffDay(s string) int {
	target, err := StrToTime(s)
	if err != nil {
		log.Errorf(err.Error())
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

func SplitGetN(s string, sep string, n int, returnIndex int) string {
	for i, item := range strings.SplitN(s, sep, n) {
		if i == returnIndex-1 {
			return item
		}
	}
	return s
}
