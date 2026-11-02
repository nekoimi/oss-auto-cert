// Package aliyun 将阿里云 SDK 的响应转换为业务模型。
package aliyun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	cas "github.com/alibabacloud-go/cas-20200407/v2/client"
	cdn "github.com/alibabacloud-go/cdn-20180510/v5/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	"github.com/alibabacloud-go/tea/dara"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"

	"github.com/nekoimi/oss-auto-cert/internal/certificate"
	"github.com/nekoimi/oss-auto-cert/internal/config"
)

// Cloud 创建每次调用独立的 SDK 客户端，避免共享可变请求上下文。
type Cloud struct {
	KeyID, KeySecret string
	Timeout          time.Duration
	client           *http.Client
}

// New 构造云服务，不发送网络请求。
func New(id, secret string, timeout time.Duration) (*Cloud, error) {
	if id == "" || secret == "" {
		return nil, fmt.Errorf("请设置 OSS_ACCESS_KEY_ID 和 OSS_ACCESS_KEY_SECRET")
	}
	return &Cloud{KeyID: id, KeySecret: secret, Timeout: timeout}, nil
}

type contextualClient struct {
	ctx     context.Context
	timeout time.Duration
	client  *http.Client
}

func (c contextualClient) Call(req *http.Request, transport *http.Transport) (*http.Response, error) {
	if c.client != nil {
		return c.client.Do(req.Clone(c.ctx))
	}
	return (&http.Client{Transport: transport, Timeout: c.timeout}).Do(req.Clone(c.ctx))
}

func (c *Cloud) api(ctx context.Context, endpoint string) *openapi.Config {
	return &openapi.Config{AccessKeyId: tea.String(c.KeyID), AccessKeySecret: tea.String(c.KeySecret), Endpoint: tea.String(endpoint), ReadTimeout: tea.Int(int(c.Timeout.Milliseconds())), ConnectTimeout: tea.Int(int(c.Timeout.Milliseconds())), HttpClient: contextualClient{ctx: ctx, timeout: c.Timeout, client: c.client}}
}
func (c *Cloud) cas(ctx context.Context) (*cas.Client, error) {
	return cas.NewClient(c.api(ctx, "cas.aliyuncs.com"))
}
func (c *Cloud) cdn(ctx context.Context) (*cdn.Client, error) {
	return cdn.NewClient(c.api(ctx, "cdn.aliyuncs.com"))
}

// OSSClient 创建有请求超时的 OSS 客户端。
func (c *Cloud) OSSClient(b config.Bucket) (*oss.Client, error) {
	if c.client != nil {
		return oss.New(b.Endpoint, c.KeyID, c.KeySecret, oss.HTTPClient(c.client))
	}
	return oss.New(b.Endpoint, c.KeyID, c.KeySecret, oss.HTTPClient(&http.Client{Timeout: c.Timeout}))
}

// Classify 将错误转换为不包含请求秘密的用户错误。
func Classify(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &certificate.TemporaryError{Err: fmt.Errorf("请求超时")}
	}
	var network net.Error
	if errors.As(err, &network) {
		return &certificate.TemporaryError{Err: fmt.Errorf("网络请求失败，请检查连接")}
	}
	var sdk *tea.SDKError
	if errors.As(err, &sdk) {
		code := tea.StringValue(sdk.Code)
		message := fmt.Errorf("阿里云请求失败，错误码 %s；请检查权限与资源配置", code)
		status := tea.IntValue(sdk.StatusCode)
		if status == 429 || status >= 500 || strings.Contains(code, "Throttl") {
			return &certificate.TemporaryError{Err: message}
		}
		return message
	}
	var daraErr *dara.SDKError
	if errors.As(err, &daraErr) {
		code := tea.StringValue(daraErr.Code)
		message := fmt.Errorf("阿里云请求失败，错误码 %s；请检查权限与资源配置", code)
		status := tea.IntValue(daraErr.StatusCode)
		if status == 429 || status >= 500 || strings.Contains(code, "Throttl") {
			return &certificate.TemporaryError{Err: message}
		}
		return message
	}
	var ossErr oss.ServiceError
	if errors.As(err, &ossErr) {
		message := fmt.Errorf("OSS 请求失败，错误码 %s，请求 ID %s", ossErr.Code, ossErr.RequestID)
		if ossErr.StatusCode == 429 || ossErr.StatusCode >= 500 {
			return &certificate.TemporaryError{Err: message}
		}
		return message
	}
	return fmt.Errorf("云端请求失败，请检查网络、访问权限和资源状态")
}

func parseDate(s string) time.Time {
	for _, layout := range []string{time.RFC3339, time.DateTime, time.DateOnly, "Jan _2 15:04:05 2006 MST", "2006-01-02 15:04:05 -0700 MST"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// Discover 读取全部已绑定域名，不在适配器内扩大选择范围。
func (c *Cloud) Discover(ctx context.Context, b config.Bucket) ([]certificate.Remote, error) {
	client, err := c.OSSClient(b)
	if err != nil {
		return nil, err
	}
	response, err := client.ListBucketCname(b.Name, oss.WithContext(ctx))
	if err != nil {
		return nil, Classify(err)
	}
	var result []certificate.Remote
	for _, item := range response.Cname {
		domain, err := config.Domain(item.Domain)
		if err != nil {
			return nil, err
		}
		cert := item.Certificate
		result = append(result, certificate.Remote{Domain: domain, CertID: cert.CertId, Fingerprint: cert.Fingerprint, NotAfter: parseDate(cert.ValidEndDate), HasCertificate: cert.CertId != "" || cert.Fingerprint != "" || cert.ValidEndDate != ""})
	}
	return result, nil
}

// Upload 上传新证书，名称在调用前持久化以便响应丢失时查找。
func (c *Cloud) Upload(ctx context.Context, r certificate.Resource, name string) (certificate.Uploaded, error) {
	client, err := c.cas(ctx)
	if err != nil {
		return certificate.Uploaded{}, err
	}
	resp, err := client.UploadUserCertificate(&cas.UploadUserCertificateRequest{Name: tea.String(name), Cert: tea.String(string(r.Certificate)), Key: tea.String(string(r.PrivateKey))})
	if err != nil {
		return certificate.Uploaded{}, Classify(err)
	}
	if resp == nil || resp.Body == nil || resp.Body.CertId == nil || *resp.Body.CertId <= 0 {
		return certificate.Uploaded{}, fmt.Errorf("CAS 上传响应缺少证书 ID")
	}
	return certificate.Uploaded{ID: *resp.Body.CertId, Name: name, Region: "cn-hangzhou"}, nil
}

// FindUpload 查找名称完全匹配的上传结果，不对未知状态重复上传。
func (c *Cloud) FindUpload(ctx context.Context, name, fingerprint string) (*certificate.Uploaded, error) {
	client, err := c.cas(ctx)
	if err != nil {
		return nil, err
	}
	var found *certificate.Uploaded
	for page := int64(1); ; page++ {
		// Keyword 只保证按域名或订单 ID 搜索，不能用上传名称过滤，否则可能漏掉响应丢失的记录。
		resp, err := client.ListUserCertificateOrder(&cas.ListUserCertificateOrderRequest{CurrentPage: tea.Int64(page), ShowSize: tea.Int64(100), OrderType: tea.String("UPLOAD")})
		if err != nil {
			return nil, Classify(err)
		}
		if resp == nil || resp.Body == nil {
			return nil, fmt.Errorf("CAS 查询响应为空")
		}
		for _, item := range resp.Body.CertificateOrderList {
			if tea.StringValue(item.Name) == name {
				if found != nil {
					return nil, fmt.Errorf("发现重复 CAS 上传记录，需要人工核对")
				}
				found = &certificate.Uploaded{ID: tea.Int64Value(item.CertificateId), Name: name, Region: "cn-hangzhou"}
			}
		}
		if int64(page*100) >= tea.Int64Value(resp.Body.TotalCount) {
			break
		}
	}
	if found != nil {
		if found.ID <= 0 {
			return nil, fmt.Errorf("CAS 查询响应缺少有效证书 ID")
		}
		detail, err := client.GetUserCertificateDetail(&cas.GetUserCertificateDetailRequest{CertId: tea.Int64(found.ID)})
		if err != nil {
			return nil, Classify(err)
		}
		if detail == nil || detail.Body == nil {
			return nil, fmt.Errorf("CAS 证书详情为空")
		}
		block, _ := pem.Decode([]byte(tea.StringValue(detail.Body.Cert)))
		if block == nil || block.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("CAS 证书内容无效，不能恢复上传")
		}
		digest := sha256.Sum256(block.Bytes)
		if hex.EncodeToString(digest[:]) != fingerprint {
			return nil, fmt.Errorf("CAS 同名证书与本地指纹不一致，请人工核对")
		}
	}
	return found, nil
}

// DeployOSS 更新证书绑定，不新增域名绑定。
func (c *Cloud) DeployOSS(ctx context.Context, b config.Bucket, domain string, u certificate.Uploaded) error {
	client, err := c.OSSClient(b)
	if err != nil {
		return err
	}
	err = client.PutBucketCnameWithCertificate(b.Name, oss.PutBucketCname{Cname: domain, CertificateConfiguration: &oss.CertificateConfiguration{CertId: fmt.Sprintf("%d-%s", u.ID, u.Region), Force: true}}, oss.WithContext(ctx))
	return Classify(err)
}

// VerifyOSS 读取实际绑定确认 CAS ID。
func (c *Cloud) VerifyOSS(ctx context.Context, b config.Bucket, domain string, u certificate.Uploaded) (bool, error) {
	items, err := c.Discover(ctx, b)
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if item.Domain == domain {
			return strings.SplitN(item.CertID, "-", 2)[0] == strconv.FormatInt(u.ID, 10), nil
		}
	}
	return false, fmt.Errorf("OSS 域名已不在 Bucket 绑定范围内")
}

// CDNEnabled 只将确定未使用 CDN 的域名视为跳过，权限和网络错误必须报告。
func (c *Cloud) CDNEnabled(ctx context.Context, domain string) (bool, error) {
	client, err := c.cdn(ctx)
	if err != nil {
		return false, err
	}
	resp, err := client.DescribeCdnDomainDetail(&cdn.DescribeCdnDomainDetailRequest{DomainName: tea.String(domain)})
	if err != nil {
		var sdk *tea.SDKError
		if errors.As(err, &sdk) && tea.StringValue(sdk.Code) == "InvalidDomain.NotFound" {
			return false, nil
		}
		var daraErr *dara.SDKError
		if errors.As(err, &daraErr) && tea.StringValue(daraErr.Code) == "InvalidDomain.NotFound" {
			return false, nil
		}
		return false, Classify(err)
	}
	if resp == nil || resp.Body == nil || resp.Body.GetDomainDetailModel == nil {
		return false, fmt.Errorf("CDN 域名详情为空")
	}
	detail := resp.Body.GetDomainDetailModel
	return tea.StringValue(detail.DomainStatus) == "online" && tea.StringValue(detail.ServerCertificateStatus) == "on", nil
}

// DeployCDN 更新已经启用 HTTPS 的 CDN 域名证书。
func (c *Cloud) DeployCDN(ctx context.Context, domain string, u certificate.Uploaded) error {
	client, err := c.cdn(ctx)
	if err != nil {
		return err
	}
	_, err = client.SetCdnDomainSSLCertificate(&cdn.SetCdnDomainSSLCertificateRequest{DomainName: tea.String(domain), CertId: tea.Int64(u.ID), CertName: tea.String(u.Name), CertRegion: tea.String(u.Region), CertType: tea.String("cas"), SSLProtocol: tea.String("on")})
	return Classify(err)
}

// CDNNotAfter 独立读取 CDN 证书到期时间，避免仅按 OSS 状态决定是否续期。
func (c *Cloud) CDNNotAfter(ctx context.Context, domain string) (time.Time, error) {
	client, err := c.cdn(ctx)
	if err != nil {
		return time.Time{}, err
	}
	resp, err := client.DescribeDomainCertificateInfo(&cdn.DescribeDomainCertificateInfoRequest{DomainName: tea.String(domain)})
	if err != nil {
		return time.Time{}, Classify(err)
	}
	if resp != nil && resp.Body != nil && resp.Body.CertInfos != nil {
		for _, info := range resp.Body.CertInfos.CertInfo {
			if tea.StringValue(info.ServerCertificateStatus) == "on" {
				if expiry := parseDate(tea.StringValue(info.CertExpireTime)); !expiry.IsZero() {
					return expiry, nil
				}
			}
		}
	}
	return time.Time{}, fmt.Errorf("CDN HTTPS 已开启但无法确定证书有效期，请核对域名证书")
}

// VerifyCDN 用域名证书查询核验上传记录。
func (c *Cloud) VerifyCDN(ctx context.Context, domain string, u certificate.Uploaded) (bool, error) {
	client, err := c.cdn(ctx)
	if err != nil {
		return false, err
	}
	resp, err := client.DescribeDomainCertificateInfo(&cdn.DescribeDomainCertificateInfoRequest{DomainName: tea.String(domain)})
	if err != nil {
		return false, Classify(err)
	}
	if resp == nil || resp.Body == nil || resp.Body.CertInfos == nil {
		return false, nil
	}
	for _, info := range resp.Body.CertInfos.CertInfo {
		if tea.StringValue(info.CertId) == strconv.FormatInt(u.ID, 10) && tea.StringValue(info.ServerCertificateStatus) == "on" && tea.StringValue(info.Status) == "success" {
			return true, nil
		}
	}
	return false, nil
}
