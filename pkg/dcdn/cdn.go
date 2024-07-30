package dcdn

import (
	"fmt"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	dcdn20180115 "github.com/alibabacloud-go/dcdn-20180115/v3/client"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"log"
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
		log.Fatalln(err)
	}

	return &Service{
		client: client,
	}
}

// UpgradeCert 更新CDN加速域名证书
func (d *Service) UpgradeCert(domain string, certID int64) error {
	// 查询加速域名信息
	request := new(dcdn20180115.DescribeDcdnDomainDetailRequest)
	request.DomainName = tea.String(domain)

	resp, err := d.client.DescribeDcdnDomainDetail(request)
	if err != nil {
		return fmt.Errorf("获取CDN加速域名(%s)详情异常: %w", domain, err)
	}

	if *resp.StatusCode != 200 {
		return fmt.Errorf("获取CDN加速域名(%s)详情请求响应异常: 状态码 -> %d；响应: %s", domain, resp.StatusCode, resp)
	}

	detail := resp.Body.DomainDetail
	// 域名ssl关闭 or
	if *detail.SSLProtocol == "off" {
		return fmt.Errorf("CDN加速域名(%s)未开启SSL", domain)
	}

	return nil
}
