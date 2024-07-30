package dcdn

import (
	"fmt"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	dcdn20180115 "github.com/alibabacloud-go/dcdn-20180115/v3/client"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/charmbracelet/log"
)

type Service struct {
	client *dcdn20180115.Client
}

func New(access oss.Credentials) *Service {
	config := &openapi.Config{
		AccessKeyId:     tea.String(access.GetAccessKeyID()),
		AccessKeySecret: tea.String(access.GetAccessKeySecret()),

		// Endpoint 请参考 https://api.aliyun.com/product/dcdn
		Endpoint: tea.String("dcdn.aliyuncs.com"),
	}
	client, err := dcdn20180115.NewClient(config)
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
	req := new(dcdn20180115.DescribeDcdnDomainDetailRequest)
	req.DomainName = tea.String(domain)

	resp, err := d.client.DescribeDcdnDomainDetail(req)
	if err != nil {
		return false, fmt.Errorf("获取CDN加速域名(%s)详情异常: %w", domain, err)
	}

	if *resp.StatusCode != 200 {
		return false, fmt.Errorf("获取CDN加速域名(%s)详情请求响应异常: 状态码 -> %d；响应: %s", domain, resp.StatusCode, resp)
	}

	log.Debugf("CDN加速域名(%s)详情响应: %s", domain, resp)

	detail := resp.Body.DomainDetail

	// 域名状态
	if *detail.DomainStatus != "online" {
		log.Infof("CDN加速域名(%s)状态异常: %s", domain, *detail.DomainStatus)
		return false, nil
	}

	// 域名ssl关闭
	if *detail.SSLProtocol != "on" {
		log.Infof("CDN加速域名(%s)未开启SSL", domain)
		return false, nil
	}

	return true, nil
}

// UpgradeCert 更新CDN加速域名证书
func (d *Service) UpgradeCert(domain string, certID int64) error {
	if b, err := d.IsApplySSL(domain); err != nil {
		return err
	} else if !b {
		log.Infof("CDN加速域名(%s)为应用SSL加速，忽略证书更换", domain)
		return nil
	}

	// 查询加速域名证书信息
	req := new(dcdn20180115.DescribeDcdnDomainCertificateInfoRequest)
	req.DomainName = tea.String(domain)

	resp, err := d.client.DescribeDcdnDomainCertificateInfo(req)
	if err != nil {
		return fmt.Errorf("获取CDN加速域名(%s)证书信息异常: %w", domain, err)
	}

	if *resp.StatusCode != 200 {
		return fmt.Errorf("获取CDN加速域名(%s)证书信息请求响应异常: 状态码 -> %d；响应: %s", domain, resp.StatusCode, resp)
	}

	log.Debugf("CDN加速域名(%s)证书信息响应: %s", domain, resp)

	//certInfos := resp.Body.CertInfos
	//if certInfos.CertInfo {
	//
	//}

	return nil
}
