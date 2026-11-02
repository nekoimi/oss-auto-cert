package acme

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/go-acme/lego/v4/registration"

	"github.com/nekoimi/oss-auto-cert/internal/config"
	"github.com/nekoimi/oss-auto-cert/internal/storage"
)

func TestAccountReuseAndEnvironmentIsolation(t *testing.T) {
	c := config.Defaults()
	c.ACME.Email = "admin@example.com"
	c.DataDir = filepath.Join(t.TempDir(), "data")
	i := &Issuer{Config: c, Store: storage.New(c.DataDir, "production")}
	first, err := i.loadAccount()
	if err != nil {
		t.Fatal(err)
	}
	first.Registration = &registration.Resource{URI: "https://example.invalid/acct/42"}
	if err := i.Store.WriteJSON(i.accountPath(), first); err != nil {
		t.Fatal(err)
	}
	restarted := &Issuer{Config: c, Store: storage.New(c.DataDir, "production")}
	second, err := restarted.loadAccount()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.PrivateKey, second.PrivateKey) || second.Registration == nil || second.Registration.URI != first.Registration.URI {
		t.Fatal("重启没有复用同一账户身份和注册记录")
	}
	restarted.Config.ACME.Environment = "staging"
	staging, err := restarted.loadAccount()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(staging.PrivateKey, first.PrivateKey) || staging.Registration != nil {
		t.Fatal("staging 复用了 production 账户")
	}
	i.Config.ACME.Email = "changed@example.com"
	if _, err := i.loadAccount(); err == nil {
		t.Fatal("账户邮箱冲突未报告")
	}
}

func TestCancelledIssueDoesNotCreateAccount(t *testing.T) {
	c := config.Defaults()
	c.DataDir = filepath.Join(t.TempDir(), "data")
	i := &Issuer{Config: c, Store: storage.New(c.DataDir, "production")}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := i.Issue(ctx, config.Bucket{}, "static.example.com"); !errors.Is(err, context.Canceled) {
		t.Fatalf("未响应取消: %v", err)
	}
	if _, err := os.Stat(c.DataDir); !os.IsNotExist(err) {
		t.Fatalf("已取消任务创建了数据目录: %v", err)
	}
}

func TestChallengeRecoveryDeletesOnlyTrackedObject(t *testing.T) {
	var puts, deletes int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/my-bucket/.well-known/acme-challenge/test-token" {
			t.Errorf("操作了非预期对象: %s", r.URL.Path)
		}
		switch r.Method {
		case http.MethodPut:
			puts++
			raw, err := io.ReadAll(r.Body)
			if err != nil || string(raw) != "key-auth" || r.Header.Get("x-oss-object-acl") != "public-read" {
				t.Error("验证对象内容或权限错误")
			}
		case http.MethodDelete:
			deletes++
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("意外云端操作: %s", r.Method)
		}
	}))
	defer server.Close()
	client, err := oss.New(server.URL, "fake-id", "fake-secret", oss.ForcePathStyle(true), oss.HTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	p := &provider{ctx: context.Background(), client: client, bucket: "my-bucket", store: storage.New(t.TempDir(), "production"), environment: "production"}
	if err := p.Present("static.example.com", "test-token", "key-auth"); err != nil {
		t.Fatal(err)
	}
	// 模拟 Present 后退出，下次只删除已记录的对象，不枚举 Bucket。
	if err := p.recover(); err != nil {
		t.Fatal(err)
	}
	if err := p.recover(); err != nil {
		t.Fatal(err)
	}
	if puts != 1 || deletes != 1 {
		t.Fatalf("恢复重复操作: PUT=%d DELETE=%d", puts, deletes)
	}
	if err := p.store.WriteJSON(p.recordPath(), challengeRecord{Schema: 1, Bucket: "other-bucket", Key: ".well-known/acme-challenge/test-token"}); err != nil {
		t.Fatal(err)
	}
	if err := p.recover(); err == nil || deletes != 1 {
		t.Fatal("恢复清理越过资源边界")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTransportPreservesRequestTimeoutAndParentCancellation(t *testing.T) {
	for _, cancelParent := range []bool{false, true} {
		parent, cancel := context.WithTimeout(context.Background(), time.Second)
		if cancelParent {
			cancel()
		}
		client := &http.Client{Timeout: 20 * time.Millisecond, Transport: transport{ctx: parent, base: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			<-r.Context().Done()
			return nil, r.Context().Err()
		})}}
		start := time.Now()
		response, err := client.Get("https://example.invalid/")
		cancel()
		if response != nil {
			response.Body.Close()
		}
		if err == nil || time.Since(start) >= 500*time.Millisecond {
			t.Fatalf("未及时响应请求超时或父级取消: %v", err)
		}
	}
}
