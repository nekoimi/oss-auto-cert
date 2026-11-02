package app

import (
	"io"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"

	"github.com/nekoimi/oss-auto-cert/internal/certificate"
	"github.com/nekoimi/oss-auto-cert/internal/logging"
)

func TestResultLoggingLevelsAndFields(t *testing.T) {
	logger := logging.New(io.Discard)
	hook := test.NewLocal(logger)
	results := []certificate.Result{
		{Bucket: "my-bucket", Domain: "static.example.com", Action: "更新已核验"},
		{Bucket: "my-bucket", Domain: "failed.example.com", Action: "处理失败", Message: "权限不足", Failed: true},
	}
	failed, err := reportResults(logger, results, false)
	if failed != 1 || err != nil {
		t.Fatalf("日志改变了执行结果: %d %v", failed, err)
	}
	entries := hook.AllEntries()
	if len(entries) != 2 || entries[0].Level != logrus.InfoLevel || entries[1].Level != logrus.ErrorLevel {
		t.Fatalf("结果日志等级错误: %+v", entries)
	}
	if entries[1].Data["domain"] != "failed.example.com" || entries[1].Data["bucket"] != "my-bucket" || entries[1].Data["detail"] != "权限不足" {
		t.Fatal("错误日志缺少资源上下文")
	}
}
