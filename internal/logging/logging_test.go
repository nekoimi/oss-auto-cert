package logging

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestTextFormatterColorsAndContext(t *testing.T) {
	var out bytes.Buffer
	logger := New(&out)
	if err := Configure(logger, "debug", "always"); err != nil {
		t.Fatal(err)
	}
	colors := map[string]bool{}
	pattern := regexp.MustCompile("\x1b\\[([0-9;]+)m")
	for _, level := range []logrus.Level{logrus.InfoLevel, logrus.WarnLevel, logrus.ErrorLevel} {
		out.Reset()
		logger.WithTime(time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)).WithFields(logrus.Fields{"bucket": "my-bucket", "domain": "static.example.com"}).Log(level, "证书状态")
		line := out.String()
		color := pattern.FindStringSubmatch(line)
		if len(color) != 2 {
			t.Fatalf("未输出 ANSI 颜色: %q", line)
		}
		colors[color[1]] = true
		for _, want := range []string{"2026-11-02 12:00:00", "my-bucket", "static.example.com", "证书状态"} {
			if !strings.Contains(line, want) {
				t.Fatalf("缺少时间或业务上下文 %q: %q", want, line)
			}
		}
	}
	if len(colors) != 3 {
		t.Fatal("INFO / WARN / ERROR 没有使用不同颜色")
	}
}

func TestLevelsColorPolicyAndIndependentLoggers(t *testing.T) {
	var first, second bytes.Buffer
	a, b := New(&first), New(&second)
	if err := Configure(a, "error", "never"); err != nil {
		t.Fatal(err)
	}
	if err := Configure(b, "debug", "always"); err != nil {
		t.Fatal(err)
	}
	a.Info("应隐藏")
	a.Error("失败")
	b.Debug("调试")
	if strings.Contains(first.String(), "应隐藏") || strings.Contains(first.String(), "\x1b[") || !strings.Contains(first.String(), "失败") {
		t.Fatalf("等级过滤或禁色失败: %q", first.String())
	}
	if !strings.Contains(second.String(), "调试") || !strings.Contains(second.String(), "\x1b[") {
		t.Fatalf("日志实例设置相互影响: %q", second.String())
	}
	for _, options := range [][2]string{{"bad-level", "never"}, {"info", "bad-color"}, {"panic", "auto"}} {
		if err := Configure(a, options[0], options[1]); err == nil {
			t.Fatal("无效日志选项未拒绝")
		}
	}
	if a.GetLevel() != logrus.ErrorLevel {
		t.Fatal("无效配置改变了日志等级")
	}
	first.Reset()
	t.Setenv("NO_COLOR", "1")
	if err := Configure(a, "info", "auto"); err != nil {
		t.Fatal(err)
	}
	a.Info("自动关闭颜色")
	if strings.Contains(first.String(), "\x1b[") {
		t.Fatal("未遵循 NO_COLOR")
	}
}
