package cdn

import (
	"fmt"
	cdn20180510 "github.com/alibabacloud-go/cdn-20180510/v5/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/charmbracelet/log"
	"github.com/nekoimi/oss-auto-cert/pkg/dto"
)

type Service struct {
	client *cdn20180510.Client
}

func New(access oss.Credentials) *Service {
	c := &openapi.Config{
		AccessKeyId:     tea.String(access.GetAccessKeyID()),
		AccessKeySecret: tea.String(access.GetAccessKeySecret()),

		// Endpoint 请参考 https://api.aliyun.com/product/cdn
		Endpoint: tea.String("cdn.aliyuncs.com"),
	}
	client, err := cdn20180510.NewClient(c)
	if err != nil {
		log.Fatalf(err.Error())
	}

	return &Service{
		client: client,
	}
}

// IsApplySSL 域名是否应用CDN加速SSL
// 域名CDN有效，SSL有效
func (d *Service) IsApplySSL(domain string) (bool, error) {
	// 查询加速域名信息
	req := new(cdn20180510.DescribeCdnDomainDetailRequest)
	req.DomainName = tea.String(domain)

	resp, err := d.client.DescribeCdnDomainDetail(req)
	if err != nil {
		return false, fmt.Errorf("获取CDN加速域名(%s)详情异常: %w", domain, err)
	}

	if *resp.StatusCode != 200 {
		return false, fmt.Errorf("获取CDN加速域名(%s)详情请求响应异常: 状态码 -> %d；响应: %s", domain, resp.StatusCode, resp)
	}

	log.Debugf("CDN加速域名(%s)详情响应: %s", domain, resp)

	detail := resp.Body.GetDomainDetailModel

	// 域名状态
	if *detail.DomainStatus != "online" {
		log.Infof("CDN加速域名(%s)状态异常: %s", domain, *detail.DomainStatus)
		return false, nil
	}

	// 是否开启 SSL 证书
	if *detail.ServerCertificateStatus != "on" {
		log.Infof("CDN加速域名(%s)未开启SSL", domain)
		return false, nil
	}

	return true, nil
}

// UpgradeCert 更新CDN加速域名证书
func (d *Service) UpgradeCert(domain string, info *dto.CertInfo) error {
	if b, err := d.IsApplySSL(domain); err != nil {
		return err
	} else if !b {
		log.Infof("CDN加速域名(%s)为应用SSL加速，忽略证书更换", domain)
		return nil
	}

	log.Infof("更新CDN加速域名(%s)SSL证书", domain)

	// 需要同步更新CDN加速域名的证书
	req := new(cdn20180510.SetCdnDomainSSLCertificateRequest)
	req.DomainName = tea.String(domain)
	req.CertId = tea.Int64(info.ID)
	req.CertName = tea.String(info.Name)
	req.CertType = tea.String("cas")
	req.CertRegion = tea.String(info.Region)
	// HTTPS 证书是否启用。
	// on：启用。
	// off：不启用。
	req.SSLProtocol = tea.String("on")

	resp, err := d.client.SetCdnDomainSSLCertificate(req)
	if err != nil {
		return fmt.Errorf("更新CDN加速域名(%s)证书异常: %w", domain, err)
	}

	if *resp.StatusCode != 200 {
		return fmt.Errorf("更新CDN加速域名(%s)证书请求响应异常: 状态码 -> %d；响应: %s", domain, resp.StatusCode, resp)
	}

	log.Debugf("更新CDN加速域名(%s)证书响应: %s", domain, resp)

	log.Infof("更新CDN加速域名(%s)证书成功!", domain)

	return nil
}
