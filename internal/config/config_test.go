package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"
)

func writeConfig(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func environment(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) { v, ok := values[key]; return v, ok }
}

const legacyExample = `acme:
  email: admin@example.com
  expired-early: 3
buckets:
  - name: my-bucket
    endpoint: oss-cn-hangzhou.aliyuncs.com
`
const tomlExample = `[acme]
email = "admin@example.com"
[schedule]
run-on-start = false
[[buckets]]
name = "my-bucket"
endpoint = "oss-cn-hangzhou.aliyuncs.com"
domains = ["static.example.com"]
[buckets.deploy]
oss = false
cdn = "auto"
`

func TestLegacyMigrationAndEnvironment(t *testing.T) {
	path := writeConfig(t, "config.yaml", legacyExample)
	r, err := Load(LoadOptions{Path: path, IgnoreEnv: true})
	if err != nil {
		t.Fatal(err)
	}
	if r.Config.RenewBefore != 15*24*time.Hour || r.Config.Buckets[0].DomainMode != "first" {
		t.Fatalf("旧默认值未保持: %+v", r.Config)
	}
	raw, err := Encode(r.Config)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := toml.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if _, exists := document["version"]; exists {
		t.Fatal("迁移配置不应包含版本字段")
	}
	migrated, err := Load(LoadOptions{Path: writeConfig(t, "config.toml", string(raw)), IgnoreEnv: true})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Config, migrated.Config) {
		t.Fatalf("迁移不等价\n%+v\n%+v", r.Config, migrated.Config)
	}
	r, err = Load(LoadOptions{Path: path, LookupEnv: environment(map[string]string{"ACME_EXPIRED_EARLY": "30", "ACME_EMAIL": "other@example.com", "DEBUG": "true"})})
	if err != nil {
		t.Fatal(err)
	}
	if r.Config.ACME.Email != "other@example.com" || r.Config.RenewBefore != 30*24*time.Hour || !r.Config.Force || r.Config.ACME.Environment != "staging" {
		t.Fatalf("环境覆盖错误: %+v", r.Config)
	}
	r, err = Load(LoadOptions{Path: path, LookupEnv: environment(map[string]string{"ACME_EXPIRED_EARLY": "oops"})})
	if err != nil || len(r.Warnings) == 0 {
		t.Fatalf("旧无效环境变量应警告: %v", err)
	}
}

func TestExplicitFalseAndStrictFields(t *testing.T) {
	r, err := Load(LoadOptions{Path: writeConfig(t, "config.toml", tomlExample), IgnoreEnv: true})
	if err != nil {
		t.Fatal(err)
	}
	if r.Config.RunOnStart || r.Config.Buckets[0].DeployOSS {
		t.Fatal("显式 false 被默认值覆盖")
	}
	for name, content := range map[string]string{"unknown": "unknown = true\n" + tomlExample, "duration": strings.Replace(tomlExample, "run-on-start = false", "run-on-start = false\ninterval = \"0s\"", 1), "domain": strings.ReplaceAll(tomlExample, "static.example.com", "../escape"), "duplicate": strings.Replace(tomlExample, `["static.example.com"]`, `["static.example.com", "STATIC.EXAMPLE.COM"]`, 1), "empty": strings.Replace(tomlExample, `["static.example.com"]`, `[]`, 1)} {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(LoadOptions{Path: writeConfig(t, "config.toml", content), IgnoreEnv: true}); err == nil {
				t.Fatal("无效配置未拒绝")
			}
		})
	}
	r, err = Load(LoadOptions{Path: writeConfig(t, "config.toml", tomlExample), LookupEnv: environment(map[string]string{"DEBUG": "true"})})
	if err != nil {
		t.Fatal(err)
	}
	if r.Config.Force || r.Config.ACME.Environment != "production" {
		t.Fatal("TOML 不应解释 DEBUG")
	}
}

func TestDiscoverAndLegacyBoundaries(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(legacyExample), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover("", []string{dir}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(tomlExample), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover("", []string{dir}); err == nil {
		t.Fatal("双文件应拒绝")
	}
	if _, err := Load(LoadOptions{Path: filepath.Join(dir, "missing.yaml"), SearchDirs: []string{dir}, IgnoreEnv: true}); err == nil {
		t.Fatal("显式路径不能回退")
	}
	for _, days := range []string{"0", "-5", "3", "90"} {
		r, err := Load(LoadOptions{Path: writeConfig(t, "old.yml", strings.Replace(legacyExample, "expired-early: 3", "expired-early: "+days, 1)), IgnoreEnv: true})
		if err != nil {
			t.Fatal(err)
		}
		if r.Config.RenewBefore < 15*24*time.Hour {
			t.Fatal("旧配置下限未保持")
		}
	}
	r, err := Load(LoadOptions{Path: writeConfig(t, "old.yml", legacyExample+"unknown: ignored\n"), IgnoreEnv: true})
	if err != nil || len(r.Warnings) == 0 {
		t.Fatalf("旧未知字段应警告: %v", err)
	}
}

func TestConfigurationFormatByExtension(t *testing.T) {
	for _, tc := range []struct {
		name, content, mode string
		valid               bool
	}{
		{"config.toml", tomlExample, "listed", true},
		{"config.TOML", tomlExample, "listed", true},
		{"config.yaml", legacyExample, "first", true},
		{"config.yml", legacyExample, "first", true},
		{"renamed.toml", legacyExample, "", false},
		{"renamed.yaml", tomlExample, "", false},
		{"config.json", tomlExample, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loaded, err := Load(LoadOptions{Path: writeConfig(t, tc.name, tc.content), IgnoreEnv: true})
			if !tc.valid {
				if err == nil {
					t.Fatal("配置格式不匹配时不应尝试其他解析器")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if loaded.Config.Buckets[0].DomainMode != tc.mode {
				t.Fatal("未按扩展名应用配置策略")
			}
		})
	}
}
