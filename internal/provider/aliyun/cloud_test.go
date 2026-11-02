package aliyun

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alibabacloud-go/tea/dara"
	"github.com/alibabacloud-go/tea/tea"

	"github.com/nekoimi/oss-auto-cert/internal/certificate"
	"github.com/nekoimi/oss-auto-cert/internal/config"
	"github.com/nekoimi/oss-auto-cert/internal/testutil"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(raw string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(raw))}
}

func TestSDKContextAndClassification(t *testing.T) {
	cloud, err := New("fake-id", "fake-secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cloud.client = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Context().Err() == nil {
			t.Error("SDK 请求未携带取消上下文")
		}
		return nil, context.Canceled
	})}
	if _, err := cloud.CDNEnabled(ctx, "static.example.com"); err == nil {
		t.Fatal("取消应返回错误")
	}
	var temporary *certificate.TemporaryError
	if !errors.As(Classify(&dara.SDKError{Code: tea.String("Throttling"), StatusCode: tea.Int(429)}), &temporary) {
		t.Fatal("新版 SDK 限流未分类")
	}
	if strings.Contains(Classify(&tea.SDKError{Code: tea.String("Forbidden"), Message: tea.String("fake-secret")}).Error(), "fake-secret") {
		t.Fatal("SDK 秘密泄漏")
	}
}

func TestOSSDiscoveryAndCDNVerification(t *testing.T) {
	cloud, err := New("fake-id", "fake-secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	cloud.client = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Host, "oss-") {
			return response(`<ListCnameResult><Cname><Domain>static.example.com</Domain><Certificate><CertId>42-cn-hangzhou</CertId><ValidEndDate>2030-01-02T03:04:05Z</ValidEndDate></Certificate></Cname></ListCnameResult>`), nil
		}
		return response(`{"CertInfos":{"CertInfo":[{"CertId":"42","CertName":"same-name","ServerCertificateStatus":"on","Status":"checking"}]}}`), nil
	})}
	items, err := cloud.Discover(context.Background(), config.Bucket{Name: "my-bucket", Endpoint: "https://oss-cn-hangzhou.aliyuncs.com"})
	if err != nil || len(items) != 1 || items[0].NotAfter.IsZero() {
		t.Fatalf("OSS 解析失败: %+v %v", items, err)
	}
	ok, err := cloud.VerifyCDN(context.Background(), "static.example.com", certificate.Uploaded{ID: 42, Name: "same-name"})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("checking 状态不能视为生效")
	}
}

func TestUploadRecoveryChecksCertificateIdentity(t *testing.T) {
	r := testutil.Certificate(t, "static.example.com", time.Now().Add(24*time.Hour))
	fingerprint, _, err := certificate.Inspect(r, "static.example.com")
	if err != nil {
		t.Fatal(err)
	}
	cloud, err := New("fake-id", "fake-secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	details := 0
	cloud.client = &http.Client{Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
		action := ""
		for key, values := range req.Header {
			if strings.EqualFold(key, "x-acs-action") && len(values) > 0 {
				action = values[0]
			}
		}
		if action == "" {
			action = req.URL.Query().Get("Action")
		}
		switch action {
		case "ListUserCertificateOrder":
			return response(`{"TotalCount":1,"CertificateOrderList":[{"Name":"oac-test","CertificateId":42}]}`), nil
		case "GetUserCertificateDetail":
			details++
			raw, err := json.Marshal(map[string]string{"Cert": string(r.Certificate)})
			if err != nil {
				t.Fatal(err)
			}
			return response(string(raw)), nil
		default:
			t.Fatalf("意外 API 调用: %s", action)
			return nil, nil
		}
	})}
	found, err := cloud.FindUpload(context.Background(), "oac-test", fingerprint)
	if err != nil || found == nil || found.ID != 42 {
		t.Fatalf("未恢复同一证书: %+v %v", found, err)
	}
	if _, err := cloud.FindUpload(context.Background(), "oac-test", "wrong-fingerprint"); err == nil {
		t.Fatal("复用了同名但不同内容的证书")
	}
	if details != 2 {
		t.Fatal("未查询证书内容进行核对")
	}
}

func TestCertificateDates(t *testing.T) {
	for _, raw := range []string{"2030-01-02T03:04:05Z", "2030-01-02 03:04:05", "Jan  2 03:04:05 2030 GMT", "2030-01-02 03:04:05 +0000 UTC"} {
		if got := parseDate(raw); !got.Equal(time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)) {
			t.Errorf("日期解析失败 %q: %v", raw, got)
		}
	}
}
