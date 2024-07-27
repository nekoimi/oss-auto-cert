package core

import (
	"fmt"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/nekoimi/oss-auto-cert/config"
	"github.com/nekoimi/oss-auto-cert/pkg/acme"
	"github.com/nekoimi/oss-auto-cert/pkg/alioss"
	"github.com/nekoimi/oss-auto-cert/pkg/cas"
	"log"
)

type Manager struct {
	running bool
	buckets []config.Bucket
	access  oss.Credentials
	cas     *cas.Service
	lego    *acme.Lego
}

func New(conf *config.Config) *Manager {
	credentialsProvider, err := oss.NewEnvironmentVariableCredentialsProvider()
	if err != nil {
		log.Fatalf("缺少OSS访问AccessKey环境变量配置: %s\n", err.Error())
	}

	access := credentialsProvider.GetCredentials()
	return &Manager{
		running: false,
		buckets: conf.Buckets,
		access:  access,
		cas:     cas.New(access),
		lego:    acme.NewLego(conf.Acme),
	}
}

func (m *Manager) Run() {
	if m.running {
		return
	}

	m.running = true
	defer func() {
		m.running = false
	}()

	for _, bucket := range m.buckets {
		b, err := alioss.New(bucket, m.access)
		if err != nil {
			log.Printf(err.Error())
			continue
		}

		info, err := b.GetCert()
		if err != nil {
			log.Printf(err.Error())
			continue
		}

		expired, err := m.cas.IsExpired(info.ID)
		if err != nil {
			log.Printf(err.Error())
			continue
		}

		if expired {
			// 过期，申请新证书
			cert, err := m.lego.Obtain(bucket.Name, info.Domain, b.Client)
			if err != nil {
				log.Printf(err.Error())
				continue
			}

			log.Printf("新证书信息: %s\n", cert)
			// 上传证书文件到阿里云数字证书管理服务
			certID, err := m.cas.Upload(cert)
			if err != nil {
				log.Printf(err.Error())
				continue
			}

			// 更新OSS域名关联的证书
			go func() {
				err := b.UpgradeCert(info.Domain, fmt.Sprintf("%d-%s", certID, info.Region))
				if err != nil {
					log.Printf(err.Error())
				}
			}()

			go func() {
				// 更新CDN关联的域名证书
			}()
		}
	}
}
