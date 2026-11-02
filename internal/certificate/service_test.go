package certificate_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nekoimi/oss-auto-cert/internal/certificate"
	"github.com/nekoimi/oss-auto-cert/internal/config"
	"github.com/nekoimi/oss-auto-cert/internal/storage"
	"github.com/nekoimi/oss-auto-cert/internal/testutil"
)

type fakeCloud struct {
	remote                                            []certificate.Remote
	enabled, oss, cdn, failCDN, ambiguous, delayedCDN bool
	cdnExpiry                                         time.Time
	uploads, ossWrites, cdnWrites                     int
	uploaded                                          *certificate.Uploaded
}

func (f *fakeCloud) Discover(context.Context, config.Bucket) ([]certificate.Remote, error) {
	return f.remote, nil
}
func (f *fakeCloud) Upload(_ context.Context, _ certificate.Resource, name string) (certificate.Uploaded, error) {
	f.uploads++
	u := certificate.Uploaded{ID: 42, Name: name, Region: "cn-hangzhou"}
	f.uploaded = &u
	if f.ambiguous {
		return certificate.Uploaded{}, &certificate.TemporaryError{Err: fmt.Errorf("响应丢失")}
	}
	return u, nil
}
func (f *fakeCloud) FindUpload(context.Context, string, string) (*certificate.Uploaded, error) {
	return f.uploaded, nil
}
func (f *fakeCloud) DeployOSS(context.Context, config.Bucket, string, certificate.Uploaded) error {
	f.ossWrites++
	f.oss = true
	return nil
}
func (f *fakeCloud) VerifyOSS(context.Context, config.Bucket, string, certificate.Uploaded) (bool, error) {
	return f.oss, nil
}
func (f *fakeCloud) CDNEnabled(context.Context, string) (bool, error) { return f.enabled, nil }
func (f *fakeCloud) CDNNotAfter(context.Context, string) (time.Time, error) {
	if !f.cdnExpiry.IsZero() {
		return f.cdnExpiry, nil
	}
	return f.remote[0].NotAfter, nil
}
func (f *fakeCloud) DeployCDN(context.Context, string, certificate.Uploaded) error {
	f.cdnWrites++
	if f.failCDN {
		return &certificate.TemporaryError{Err: fmt.Errorf("模拟超时")}
	}
	f.cdn = true
	return nil
}
func (f *fakeCloud) VerifyCDN(context.Context, string, certificate.Uploaded) (bool, error) {
	return f.cdn && !f.delayedCDN, nil
}

type fakeIssuer struct {
	resource certificate.Resource
	calls    atomic.Int32
}

func (f *fakeIssuer) Issue(context.Context, config.Bucket, string) (certificate.Resource, error) {
	f.calls.Add(1)
	return f.resource, nil
}

func fixture(t *testing.T) (*certificate.Service, *fakeCloud, *fakeIssuer, *storage.Store, *time.Time) {
	t.Helper()
	now := time.Now().UTC()
	c := config.Defaults()
	c.Workers = 1
	c.DataDir = filepath.Join(t.TempDir(), "data")
	c.Buckets = []config.Bucket{{Name: "my-bucket", DomainMode: "listed", Domains: []string{"static.example.com"}, DeployOSS: true, CDN: "auto"}}
	cloud := &fakeCloud{remote: []certificate.Remote{{Domain: "static.example.com", HasCertificate: true, NotAfter: now.Add(time.Hour)}}, enabled: true}
	issuer := &fakeIssuer{resource: testutil.Certificate(t, "static.example.com", now.Add(90*24*time.Hour))}
	store := storage.New(c.DataDir, "production")
	service := &certificate.Service{Config: c, Cloud: cloud, Issuer: issuer, Store: store, Now: func() time.Time { return now }}
	return service, cloud, issuer, store, &now
}

func TestRestartRetriesOnlyFailedTarget(t *testing.T) {
	s, cloud, issuer, store, now := fixture(t)
	cloud.failCDN = true
	first := s.Run(context.Background(), false, false)
	if len(first) != 1 || !first[0].Failed {
		t.Fatalf("应报告 CDN 失败: %+v", first)
	}
	state, err := store.LoadState("my-bucket", "static.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if state.OSS.Status != "verified" || state.CDN.Status != "retrying" {
		t.Fatalf("未分别保存进度: %+v", state)
	}
	cloud.remote[0].NotAfter = now.Add(90 * 24 * time.Hour)
	cloud.failCDN = false
	*now = state.CDN.NextAttempt.Add(time.Second)
	restarted := &certificate.Service{Config: s.Config, Cloud: cloud, Issuer: issuer, Store: storage.New(s.Config.DataDir, "production"), Now: s.Now}
	second := restarted.Run(context.Background(), false, false)
	if second[0].Failed {
		t.Fatalf("恢复失败: %+v", second)
	}
	if issuer.calls.Load() != 1 || cloud.uploads != 1 || cloud.ossWrites != 1 || cloud.cdnWrites != 2 {
		t.Fatalf("重启重复了已完成步骤: issue=%d upload=%d oss=%d cdn=%d", issuer.calls.Load(), cloud.uploads, cloud.ossWrites, cloud.cdnWrites)
	}
}

func TestDryRunAndInitialIssuance(t *testing.T) {
	s, cloud, issuer, _, _ := fixture(t)
	result := s.Run(context.Background(), true, false)
	if result[0].Failed {
		t.Fatal(result)
	}
	if issuer.calls.Load() != 0 || cloud.uploads != 0 {
		t.Fatal("预演产生写操作")
	}
	if _, err := os.Stat(s.Config.DataDir); !os.IsNotExist(err) {
		t.Fatalf("预演创建了存储目录: %v", err)
	}
	cloud.remote[0].HasCertificate = false
	result = s.Run(context.Background(), false, false)
	if !result[0].Failed || issuer.calls.Load() != 0 {
		t.Fatal("首次部署未授权仍签发")
	}
	s.Config.Buckets[0].AllowInitialIssuance = true
	result = s.Run(context.Background(), false, false)
	if result[0].Failed || issuer.calls.Load() != 1 {
		t.Fatalf("显式首次签发失败: %+v", result)
	}
}

func TestUploadResponseLostAndRemoteDrift(t *testing.T) {
	s, cloud, issuer, _, _ := fixture(t)
	cloud.ambiguous = true
	if !s.Run(context.Background(), false, false)[0].Failed {
		t.Fatal("上传响应丢失应失败")
	}
	cloud.ambiguous = false
	if result := s.Run(context.Background(), false, false); result[0].Failed {
		t.Fatal(result)
	}
	if cloud.uploads != 1 || issuer.calls.Load() != 1 {
		t.Fatal("响应丢失后重复签发或上传")
	}
	cloud.remote[0].NotAfter = time.Now().Add(80 * 24 * time.Hour)
	cloud.cdn = false
	if result := s.Run(context.Background(), false, false); result[0].Failed {
		t.Fatal(result)
	}
	if cloud.cdnWrites != 2 || issuer.calls.Load() != 1 {
		t.Fatal("CDN 外部修改没有通过已有证书修复")
	}
}

func TestCancelledRunWaitsForTasks(t *testing.T) {
	s, _, _, _, _ := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := s.Run(ctx, false, false)
	if len(result) == 0 || !result[len(result)-1].Failed {
		t.Fatal("取消没有被报告")
	}
}

func TestDomainSelection(t *testing.T) {
	items := []certificate.Remote{{Domain: "a.example.com"}, {Domain: "b.example.com"}}
	for mode, count := range map[string]int{"first": 1, "all": 2, "listed": 1} {
		b := config.Bucket{DomainMode: mode}
		if mode == "listed" {
			b.Domains = []string{"b.example.com"}
		}
		selected, err := certificate.Select(b, items)
		if err != nil || len(selected) != count {
			t.Fatalf("范围错误 %s: %v", mode, err)
		}
	}
	if _, err := certificate.Select(config.Bucket{DomainMode: "listed", Domains: []string{"missing.example.com"}}, items); err == nil {
		t.Fatal("未绑定域名应失败")
	}
}

func TestCDNExpiryTriggersRenewalWhenOSSIsHealthy(t *testing.T) {
	s, cloud, issuer, _, now := fixture(t)
	cloud.remote[0].NotAfter = now.Add(80 * 24 * time.Hour)
	cloud.cdnExpiry = now.Add(time.Hour)
	result := s.Run(context.Background(), false, false)
	if result[0].Failed || issuer.calls.Load() != 1 || cloud.cdnWrites != 1 {
		t.Fatalf("遗漏 CDN 临期: %+v", result)
	}
}

func TestPropagationWaitDoesNotRepeatSubmission(t *testing.T) {
	s, cloud, issuer, store, now := fixture(t)
	cloud.delayedCDN = true
	if result := s.Run(context.Background(), false, false); !result[0].Failed {
		t.Fatal("传播中应报告等待")
	}
	state, err := store.LoadState("my-bucket", "static.example.com")
	if err != nil {
		t.Fatal(err)
	}
	*now = state.CDN.NextAttempt.Add(time.Second)
	if result := s.Run(context.Background(), false, false); !result[0].Failed {
		t.Fatal("传播中应继续等待")
	}
	if cloud.cdnWrites != 1 || cloud.uploads != 1 || issuer.calls.Load() != 1 {
		t.Fatal("等待生效重复了写操作")
	}
	state, err = store.LoadState("my-bucket", "static.example.com")
	if err != nil {
		t.Fatal(err)
	}
	*now = state.CDN.NextAttempt.Add(time.Second)
	cloud.delayedCDN = false
	if result := s.Run(context.Background(), false, false); result[0].Failed {
		t.Fatal(result)
	}
	if cloud.cdnWrites != 1 {
		t.Fatal("生效后仍重复提交")
	}
}

func TestFailedDomainDoesNotBlockOtherSelectedDomains(t *testing.T) {
	s, cloud, issuer, store, now := fixture(t)
	cloud.enabled = false
	cloud.remote = []certificate.Remote{
		{Domain: "a.example.com", HasCertificate: true, NotAfter: now.Add(time.Hour)},
		{Domain: "static.example.com", HasCertificate: true, NotAfter: now.Add(time.Hour)},
		{Domain: "unselected.example.com", HasCertificate: true, NotAfter: now.Add(time.Hour)},
	}
	s.Config.Buckets[0].Domains = []string{"a.example.com", "static.example.com"}
	// 签发器只返回 static.example.com 的证书，另一个域名应独立报告身份校验失败。
	results := s.Run(context.Background(), false, false)
	if len(results) != 2 || !results[0].Failed || results[1].Failed {
		t.Fatalf("域名错误未隔离: %+v", results)
	}
	if issuer.calls.Load() != 2 || cloud.uploads != 1 {
		t.Fatal("未选域名被处理，或失败域名仍进行了上传")
	}
	state, err := store.LoadState("my-bucket", "static.example.com")
	if err != nil || state.OSS.Status != "verified" {
		t.Fatalf("其他域名未完成部署: %+v %v", state, err)
	}
}
