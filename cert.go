package main

import (
	"crypto"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/go-acme/lego/v4/registration"
	"log"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
)

type AutoCert struct {
	signalChan  chan os.Signal
	certRunning *atomic.Bool
	conf        *Config
	acmeUser    registration.User
}

func New(conf *Config) *AutoCert {
	b := new(atomic.Bool)
	b.Store(false)

	return &AutoCert{
		signalChan:  make(chan os.Signal),
		certRunning: b,
		conf:        conf,
	}
}

func Run(conf *Config) {
	conf.LoadOptions()

	ac := New(conf)
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
		ac.bucketCart(bucket)
	}
}

func (ac *AutoCert) bucketCart(bucket Bucket) {

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

func newOssClient() {
	credentialsProvider, err := oss.NewEnvironmentVariableCredentialsProvider()
	if err != nil {
		log.Fatalf("缺少OSS访问AccessKey环境变量配置: %s\n", err.Error())
	}

	credentials := credentialsProvider.GetCredentials()

	client, err := oss.New("", credentials.GetAccessKeyID(), credentials.GetAccessKeySecret())

	client.ListBucketCname("")
}
