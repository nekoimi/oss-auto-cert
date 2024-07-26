package core

import (
	cas20200407 "github.com/alibabacloud-go/cas-20200407/v2/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/nekoimi/oss-auto-cert/config"
	"github.com/nekoimi/oss-auto-cert/pkg/acme"
	"github.com/nekoimi/oss-auto-cert/pkg/utils"
	"log"
	"strconv"
	"sync/atomic"
	"time"
)

type CertManager struct {
	certRunning *atomic.Bool
	conf        *config.Config

	// credentials
	credentials oss.Credentials

	casClient *cas20200407.Client

	lego *acme.Lego
}

func New(conf *config.Config) *CertManager {
	b := new(atomic.Bool)
	b.Store(false)

	credentialsProvider, err := oss.NewEnvironmentVariableCredentialsProvider()
	if err != nil {
		log.Fatalf("缺少OSS访问AccessKey环境变量配置: %s\n", err.Error())
	}

	return &CertManager{
		certRunning: b,
		conf:        conf,

		credentials: credentialsProvider.GetCredentials(),
		lego:        acme.NewLego(conf.Acme),
	}
}

func (ac *CertManager) Init() {
	casClient, err := cas20200407.NewClient(&openapi.Config{
		AccessKeyId:     tea.String(ac.credentials.GetAccessKeyID()),
		AccessKeySecret: tea.String(ac.credentials.GetAccessKeySecret()),

		// endpoint参考：https://api.aliyun.com/product/cas
		Endpoint: tea.String("cas.aliyuncs.com"),
	})
	if err != nil {
		log.Fatalln(err)
	}

	ac.casClient = casClient
}

func Run(conf *config.Config) {
	conf.LoadOptions()

	ac := New(conf)
	ac.Init()

	//tick := time.NewTicker(3 * time.Second)
	//for {
	//	select {
	//	case <-ac.signalChan:
	//		tick.Stop()
	//		log.Println("Exit.")
	//		os.Exit(0)
	//	case <-tick.C:
	//		go ac.CertRun()
	//	}
	//}
	ac.CertRun()
}

func (ac *CertManager) CertRun() {
	if ac.certRunning.Load() {
		return
	}
	ac.certRunning.Store(true)
	defer ac.certRunning.Store(false)

	for _, bucket := range ac.conf.Buckets {
		ac.handleBucket(bucket)
	}
}

// 更新bucket的自定义域名证书
func (ac *CertManager) handleBucket(bucket config.Bucket) {
	// 新建oss客户端实例
	client, err := oss.New(bucket.Endpoint, ac.credentials.GetAccessKeyID(), ac.credentials.GetAccessKeySecret())
	if err != nil {
		log.Printf("创建oss-client实例异常: %s\n", err.Error())
		return
	}

	// 获取bucket全部自定义域名列表
	cnameResult, err := client.ListBucketCname(bucket.Name)
	if err != nil {
		log.Printf("获取bucket@%s下自定义域名列表异常: %s\n", bucket.Name, err.Error())
		return
	}

	cnames := cnameResult.Cname
	for _, cname := range cnames {
		ac.handleBucketCname(bucket, cname)
	}
}

// 处理自定义域名
func (ac *CertManager) handleBucketCname(bucket config.Bucket, cname oss.Cname) {
	log.Printf("处理bucket@%s自定义域名: %s \n", bucket.Name, cname)
	log.Printf("Cname信息: %s\n", cname)
	log.Printf("Cname信息: Status-%s\n", cname.Status)
	log.Printf("Cname信息: Domain-%s\n", cname.Domain)
	log.Printf("Cname信息: LastModified-%s \n", cname.LastModified)

	certificate := cname.Certificate
	// 域名证书信息
	log.Printf("证书信息: %s\n", certificate)
	log.Printf("证书信息: Type-%s\n", certificate.Type)
	log.Printf("证书信息: CertId-%s\n", certificate.CertId)
	log.Printf("证书信息: Status-%s\n", certificate.Status)
	log.Printf("证书信息: CreationDate-%s\n", certificate.CreationDate)
	log.Printf("证书信息: Fingerprint-%s\n", certificate.Fingerprint)
	log.Printf("证书信息: ValidStartDate-%s\n", certificate.ValidStartDate)
	log.Printf("证书信息: ValidEndDate-%s\n", certificate.ValidEndDate)

	// 根据证书ID查询证书信息
	casID := certificate.CertId
	// 123456789-cn-hangzhou
	log.Printf("证书ID: %s\n", casID)
	certID := utils.StrSplitFirst(casID, "-")
	request := new(cas20200407.GetUserCertificateDetailRequest)
	certIDInt64, _ := strconv.ParseInt(certID, 10, 64)
	request.SetCertId(certIDInt64)
	resp, err := ac.casClient.GetUserCertificateDetail(request)
	if err != nil {
		log.Printf("获取证书详情异常: CertId-%s, %s\n", certID, err.Error())
		return
	}

	if *resp.StatusCode != 200 {
		log.Printf("获取证书详情异常: StatusCode-%d, %s\n", resp.StatusCode, resp)
		return
	}

	details := resp.Body
	log.Printf("证书详情: %s\n", details)

	// 证书已经过期或者还有3天过期
	if *details.Expired || utils.DateIsExpire(*details.EndDate, time.Hour*24*3) {
		// 申请证书 替换新证书
		log.Printf("域名(%s)证书过期，更换新证书\n", cname.Domain)
		go ac.renewCert(cname.Domain, bucket)
	} else {
		log.Printf("域名(%s)证书未过期，过期日期: %s, 还剩%d天\n", cname.Domain, *details.EndDate, utils.DateExpireDay(*details.EndDate))
	}
}

func (ac *CertManager) renewCert(domain string, bucket config.Bucket) {
	// 新建oss客户端实例
	client, err := oss.New(bucket.Endpoint, ac.credentials.GetAccessKeyID(), ac.credentials.GetAccessKeySecret())
	if err != nil {
		log.Printf("创建oss-client实例异常: %s\n", err.Error())
		return
	}

	_, err = ac.lego.Obtain(domain, bucket.Name, client)
	if err != nil {
		log.Printf("域名(%s)申请证书失败: %s \n", domain, err.Error())
		return
	}

	// 保存证书文件到本地存储

	// 上传证书文件到阿里云数字证书管理服务

	// 更新OSS域名关联的证书
	// 更新CDN关联的域名证书
}
