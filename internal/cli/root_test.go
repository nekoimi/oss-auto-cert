package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestNormalizeLegacyArguments(t *testing.T) {
	for _, tc := range []struct{ input, want []string }{{[]string{"-config=a b.yaml", "-log-level", "debug"}, []string{"--config=a b.yaml", "--log-level", "debug"}}, {[]string{"config", "migrate", "-output=新配置.toml"}, []string{"config", "migrate", "--output=新配置.toml"}}, {[]string{"--config", "-log-level=x", "--", "-config=y"}, []string{"--config", "-log-level=x", "--", "-config=y"}}} {
		got, err := Normalize(tc.input)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("转换错误: %v %v", got, err)
		}
	}
	if _, err := Normalize([]string{"-config=a.yaml", "--config", "b.toml"}); err == nil {
		t.Fatal("重复别名未拒绝")
	}
}

func TestOfflineCommandsDoNotLoadConfig(t *testing.T) {
	t.Setenv("OSS_ACCESS_KEY_ID", "")
	t.Setenv("OSS_ACCESS_KEY_SECRET", "")
	for _, args := range [][]string{{"--help"}, {"-h"}, {"help", "config"}, {"-v"}, {"-help"}, {"version"}, {"--version"}, {"--config=missing.toml", "version"}} {
		var out, stderr bytes.Buffer
		if code := Execute(context.Background(), args, &out, &stderr); code != 0 {
			t.Fatalf("离线命令失败 %v: %s", args, stderr.String())
		}
		if out.Len() == 0 {
			t.Fatal("离线输出为空")
		}
	}
	var out, stderr bytes.Buffer
	if code := Execute(context.Background(), []string{"unknown"}, &out, &stderr); code != 2 {
		t.Fatalf("未知命令不应启动服务: %d", code)
	}
}

func TestInitValidateAndMigrate(t *testing.T) {
	t.Setenv("ACME_EMAIL", "")
	t.Setenv("ACME_DATA_DIR", "")
	t.Setenv("ACME_EXPIRED_EARLY", "")
	t.Setenv("DEBUG", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "配置 文件.toml")
	execute := func(args ...string) (int, string, string) {
		var out, stderr bytes.Buffer
		code := Execute(context.Background(), args, &out, &stderr)
		return code, out.String(), stderr.String()
	}
	if code, _, err := execute("config", "init", "--output", path); code != 0 {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var generated map[string]any
	if err := toml.Unmarshal(raw, &generated); err != nil {
		t.Fatal(err)
	}
	if _, exists := generated["version"]; exists {
		t.Fatal("生成配置不应包含版本字段")
	}
	if code, _, _ := execute("config", "init", "--output", path); code != 2 {
		t.Fatal("不应覆盖已有配置")
	}
	if code, _, err := execute("config", "validate", "-config", path); code != 0 {
		t.Fatal(err)
	}
	old := filepath.Join(dir, "old.yaml")
	if err := os.WriteFile(old, []byte("acme:\n  email: admin@example.com\nbuckets:\n  - name: my-bucket\n    endpoint: oss-cn-hangzhou.aliyuncs.com\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ACME_EMAIL", "environment@example.com")
	code, body, stderr := execute("config", "migrate", "-config", old)
	if code != 0 {
		t.Fatal(stderr)
	}
	if strings.Contains(body, "version =") || strings.Contains(body, "environment@example.com") || !strings.Contains(body, `domain-mode = 'first'`) && !strings.Contains(body, `domain-mode = "first"`) {
		t.Fatalf("迁移内容不正确: %s", body)
	}
	if code, _, err := execute("config", "validate", "--config", path); code != 0 {
		t.Fatalf("新命令树状态泄漏: %s", err)
	}
	if code, _, _ := execute("run", "--force", "--config", path); code != 2 {
		t.Fatal("常驻不能强制每轮签发")
	}
}

func TestColoredLogsDoNotPolluteMigrationOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.yaml")
	if err := os.WriteFile(path, []byte("acme:\n  email: admin@example.com\nbuckets:\n  - name: my-bucket\n    endpoint: oss-cn-hangzhou.aliyuncs.com\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"config", "migrate", "--config", path, "--color=always"}, &out, &stderr)
	if code != 0 {
		t.Fatal(stderr.String())
	}
	if strings.Contains(out.String(), "\x1b[") || !strings.Contains(stderr.String(), "\x1b[") {
		t.Fatal("彩色日志与配置正文未正确分流")
	}
	var document map[string]any
	if err := toml.Unmarshal(out.Bytes(), &document); err != nil {
		t.Fatalf("迁移输出不是有效 TOML: %v", err)
	}
}

func TestLoggingOptionsAndErrors(t *testing.T) {
	for _, args := range [][]string{{"version", "--log-level=invalid"}, {"version", "--color=invalid"}, {"config", "validate", "--config=missing.toml", "--color=always"}} {
		var out, stderr bytes.Buffer
		if code := Execute(context.Background(), args, &out, &stderr); code != 2 {
			t.Fatalf("无效命令未返回退出码 2: %v", args)
		}
		if out.Len() != 0 || !strings.Contains(stderr.String(), "命令执行失败") {
			t.Fatalf("错误输出未使用日志: %q %q", out.String(), stderr.String())
		}
	}
}
