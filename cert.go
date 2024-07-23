package main

import (
	"crypto"
	cas20200407 "github.com/alibabacloud-go/cas-20200407/v2/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/go-acme/lego/v4/registration"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"
)

type AutoCert struct {
	signalChan  chan os.Signal
	certRunning *atomic.Bool
	conf        *Config

	// credentials
	credentials oss.Credentials

	casClient *cas20200407.Client

	// acme
	acmeUser registration.User
}

func New(conf *Config) *AutoCert {
	b := new(atomic.Bool)
	b.Store(false)

	// 从环境变量读取
	credentialsProvider, err := oss.NewEnvironmentVariableCredentialsProvider()
	if err != nil {
		log.Fatalf("缺少OSS访问AccessKey环境变量配置: %s\n", err.Error())
	}

	return &AutoCert{
		signalChan:  make(chan os.Signal),
		certRunning: b,
		conf:        conf,

		credentials: credentialsProvider.GetCredentials(),
	}
}

func (ac *AutoCert) Init() {
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

func Run(conf *Config) {
	conf.LoadOptions()

	ac := New(conf)
	ac.Init()

	signal.Notify(ac.signalChan, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	tick := time.NewTicker(3 * time.Second)
	for {
		select {
		case <-ac.signalChan:
			tick.Stop()
			log.Println("Exit.")
			os.Exit(0)
		case <-tick.C:
			go ac.CertRun()
		}
	}
}

func (ac *AutoCert) CertRun() {
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
func (ac *AutoCert) handleBucket(bucket Bucket) {
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
func (ac *AutoCert) handleBucketCname(bucket Bucket, cname oss.Cname) {
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
	certID := certificate.CertId
	log.Printf("证书ID: %s\n", certID)
	request := new(cas20200407.GetUserCertificateDetailRequest)
	certIDInt64, _ := strconv.ParseInt(certID, 10, 64)
	request.SetCertId(certIDInt64)
	details, err := ac.casClient.GetUserCertificateDetail(request)
	if err != nil {
		log.Printf("获取证书详情异常: CertId-%s, %s\n", certID, err.Error())
		return
	}

	log.Printf("证书详情: %s\n", details)
}

type RegistrationUser struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *RegistrationUser) GetEmail() string {
	return u.Email
}

func (u RegistrationUser) GetRegistration() *registration.Resource {
	return u.Registration
}

func (u *RegistrationUser) GetPrivateKey() crypto.PrivateKey {
	return u.key
}
