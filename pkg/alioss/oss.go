package alioss

import (
	"fmt"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/charmbracelet/log"
	"github.com/nekoimi/oss-auto-cert/config"
	"github.com/nekoimi/oss-auto-cert/pkg/utils"
	"strconv"
)

type BucketService struct {
	name   string
	Client *oss.Client
}

func New(bucket config.Bucket, access oss.Credentials) (*BucketService, error) {
	client, err := oss.New(bucket.Endpoint, access.GetAccessKeyID(), access.GetAccessKeySecret())
	if err != nil {
		return nil, fmt.Errorf("创建oss client异常: %w", err)
	}

	return &BucketService{
		name:   bucket.Name,
		Client: client,
	}, nil
}

// GetCert 获取bucket下自定义域名证书ID信息
func (b *BucketService) GetCert() (*CertInfo, error) {
	// 获取bucket全部自定义域名列表
	result, err := b.Client.ListBucketCname(b.name)
	if err != nil {
		return nil, fmt.Errorf("获取bucket(%s)下自定义域名列表异常: %w", b.name, err)
	}

	// TODO bucket下自定义域名列表，有多个
	cnameArr := result.Cname
	if len(cnameArr) <= 0 {
		return nil, fmt.Errorf("bucket(%s)自定义域名为空，请检查bucket配置", b.name)
	}

	// 这里先只取第一个
	cname := cnameArr[0]
	log.Debugf("处理bucket(%s)自定义域名: %s", b.name, cname.Domain)
	log.Debugf("Status: %s", cname.Status)
	log.Debugf("Domain: %s", cname.Domain)
	log.Debugf("LastModified: %s", cname.LastModified)

	// 检查证书信息
	cert := cname.Certificate
	// 域名证书信息
	log.Debugf("证书信息: %s", cert)
	log.Debugf("Type: %s", cert.Type)
	log.Debugf("CertId: %s", cert.CertId)
	log.Debugf("Status: %s", cert.Status)
	log.Debugf("CreationDate: %s", cert.CreationDate)
	log.Debugf("Fingerprint: %s", cert.Fingerprint)
	log.Debugf("ValidStartDate: %s", cert.ValidStartDate)
	log.Debugf("ValidEndDate: %s", cert.ValidEndDate)

	certID := cert.CertId
	if certID == "" {
		return nil, fmt.Errorf("bucket(%s)域名(%s)证书信息ID为空", b.name, cname.Domain)
	}

	int64Str := utils.SplitFirst(certID, "-")
	int64ID, err := strconv.ParseInt(int64Str, 10, 64)
	if err != nil {
		return nil, err
	}

	return &CertInfo{
		ID:     int64ID,
		Region: utils.SplitGetN(certID, "-", 2),
		Domain: cname.Domain,
	}, nil
}

// UpgradeCert 更新域名绑定的证书
func (b *BucketService) UpgradeCert(domain string, certID string) error {
	log.Debugf("更新域名(%s)证书：%s", domain, certID)

	putCname := oss.PutBucketCname{
		Cname: domain,
		CertificateConfiguration: &oss.CertificateConfiguration{
			CertId:            certID,
			Force:             true,
			DeleteCertificate: false,
		},
	}
	err := b.Client.PutBucketCnameWithCertificate(b.name, putCname)
	if err != nil {
		return fmt.Errorf("bucket(%s)更新证书失败：%w", b.name, err)
	}

	return nil
}
