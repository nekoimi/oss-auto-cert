// Package logging 统一日志格式、等级和终端颜色。
package logging

import (
	"fmt"
	"io"
	"os"

	"github.com/sirupsen/logrus"
)

// New 创建独立日志实例，避免不同命令共享全局输出和等级。
func New(out io.Writer) *logrus.Logger {
	logger := logrus.New()
	logger.SetOutput(out)
	logger.SetFormatter(formatter("auto"))
	return logger
}

// Configure 设置日志等级和颜色策略；无效输入不改变当前实例。
func Configure(logger *logrus.Logger, level, color string) error {
	parsed, err := logrus.ParseLevel(level)
	if err != nil || parsed < logrus.ErrorLevel {
		return fmt.Errorf("log-level 必须为 trace、debug、info、warn 或 error")
	}
	switch color {
	case "auto", "always", "never":
	default:
		return fmt.Errorf("color 必须为 auto、always 或 never")
	}
	logger.SetLevel(parsed)
	logger.SetFormatter(formatter(color))
	return nil
}

func formatter(color string) *logrus.TextFormatter {
	_, noColor := os.LookupEnv("NO_COLOR")
	return &logrus.TextFormatter{
		FullTimestamp:          true,
		TimestampFormat:        "2006-01-02 15:04:05",
		PadLevelText:           true,
		DisableLevelTruncation: true,
		ForceColors:            color == "always",
		DisableColors:          color == "never" || (color == "auto" && noColor),
	}
}
