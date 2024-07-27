package alioss

import (
	"fmt"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/nekoimi/oss-auto-cert/config"
	"github.com/nekoimi/oss-auto-cert/pkg/utils"
	"log"
	"strconv"
)

type Bucket struct {
	name   string
	Client *oss.Client
}

func New(bucket config.Bucket, access oss.Credentials) (*Bucket, error) {
	client, err := oss.New(bucket.Endpoint, access.GetAccessKeyID(), access.GetAccessKeySecret())
	if err != nil {
		return nil, fmt.Errorf("创建oss client异常: %w", err)
	}

	return &Bucket{
		name:   bucket.Name,
		Client: client,
	}, nil
}

// GetCert 获取bucket下自定义域名证书ID信息
func (b *Bucket) GetCert() (*CertInfo, error) {
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
	log.Printf("处理bucket(%s)自定义域名: %s \n", b.name, cname.Domain)
	log.Printf("Status: %s\n", cname.Status)
	log.Printf("Domain: %s\n", cname.Domain)
	log.Printf("LastModified: %s \n", cname.LastModified)

	// 检查证书信息
	cert := cname.Certificate
	// 域名证书信息
	log.Printf("证书信息: %s\n", cert)
	log.Printf("Type: %s\n", cert.Type)
	log.Printf("CertId: %s\n", cert.CertId)
	log.Printf("Status: %s\n", cert.Status)
	log.Printf("CreationDate: %s\n", cert.CreationDate)
	log.Printf("Fingerprint: %s\n", cert.Fingerprint)
	log.Printf("ValidStartDate: %s\n", cert.ValidStartDate)
	log.Printf("ValidEndDate: %s\n", cert.ValidEndDate)

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
func (b *Bucket) UpgradeCert(domain string, certID string) error {
	log.Printf("更新域名(%s)证书：%s\n", domain, certID)

	putCname := oss.PutBucketCname{
		Cname: domain,
		CertificateConfiguration: &oss.CertificateConfiguration{
			CertId: certID,
		},
	}
	err := b.Client.PutBucketCnameWithCertificate(b.name, putCname)
	if err != nil {
		return fmt.Errorf("bucket(%s)更新证书失败：%w", b.name, err)
	}

	return nil
}
