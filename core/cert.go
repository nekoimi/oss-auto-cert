package core

import (
	"fmt"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/charmbracelet/log"
	"github.com/nekoimi/oss-auto-cert/config"
	"github.com/nekoimi/oss-auto-cert/notifications"
	"github.com/nekoimi/oss-auto-cert/notifications/webhook"
	"github.com/nekoimi/oss-auto-cert/pkg/acme"
	"github.com/nekoimi/oss-auto-cert/pkg/alioss"
	"github.com/nekoimi/oss-auto-cert/pkg/cas"
	"github.com/nekoimi/oss-auto-cert/pkg/cdn"
)

type Manager struct {
	running    bool
	buckets    []config.Bucket
	access     oss.Credentials
	cas        *cas.Service
	cdn        *cdn.Service
	lego       *acme.LegoService
	notifiable notifications.Notifiable
}

func New(conf *config.Config) *Manager {
	credentialsProvider, err := oss.NewEnvironmentVariableCredentialsProvider()
	if err != nil {
		log.Fatalf("缺少OSS访问AccessKey环境变量配置: %s", err.Error())
	}

	access := credentialsProvider.GetCredentials()
	m := &Manager{
		running:    false,
		buckets:    conf.Buckets,
		access:     access,
		cas:        cas.New(access),
		cdn:        cdn.New(access),
		lego:       acme.NewLego(conf.Acme),
		notifiable: webhook.New(conf.Webhook, conf.WebhookTpl),
	}

	if len(m.buckets) <= 0 {
		log.Warnf("OSS存储Bucket配置为空!")
	} else {
		for _, b := range m.buckets {
			log.Debugf("Bucket开启监测: %s => %s", b.Name, b.Endpoint)
		}
	}

	return m
}

func (m *Manager) Stop() {
	m.lego.Stop()
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
		log.Debugf("开始检测Bucket: %s ...", bucket.Name)

		b, err := alioss.New(bucket, m.access)
		if err != nil {
			log.Errorf(err.Error())
			continue
		}

		info, err := b.GetCert()
		if err != nil {
			log.Errorf(err.Error())
			continue
		}

		expired, err := m.cas.IsExpired(info.ID)
		if err != nil {
			log.Errorf(err.Error())
			continue
		}

		if expired {
			messagePrefix := fmt.Sprintf("【oss-auto-cert】Bucket<%s> 域名: %s\n", bucket.Name, info.Domain)

			m.notifiable.Notify(fmt.Sprintf("%s 证书过期，需要更换新证书", messagePrefix))

			// 过期，申请新证书
			cert, err := m.lego.Obtain(bucket.Name, info.Domain, b.Client)
			if err != nil {
				log.Errorf(err.Error())
				continue
			}

			// 上传证书文件到阿里云数字证书管理服务
			certInfo, err := m.cas.Upload(cert)
			if err != nil {
				log.Errorf(err.Error())
				m.notifiable.Notify(fmt.Sprintf("%s 上传证书到数字证书管理异常: %s", messagePrefix, err.Error()))
				continue
			}

			certInfo.Region = info.Region

			log.Infof("证书上传信息: %s", certInfo)

			go func() {
				// 更新OSS域名关联的证书
				err := b.UpgradeCert(info.Domain, fmt.Sprintf("%d-%s", certInfo.ID, info.Region))
				if err != nil {
					log.Errorf(err.Error())
					m.notifiable.Notify(fmt.Sprintf("%s 更新OSS域名证书失败: %s", messagePrefix, err.Error()))
				} else {
					m.notifiable.Notify(fmt.Sprintf("%s 更新OSS域名证书成功，请及时检查证书生效", messagePrefix))
				}
			}()

			go func() {
				// 更新CDN关联的域名证书
				err := m.cdn.UpgradeCert(info.Domain, certInfo)
				if err != nil {
					log.Errorf(err.Error())
					m.notifiable.Notify(fmt.Sprintf("%s 更新CDN加速域名证书失败: %s", messagePrefix, err.Error()))
				} else {
					m.notifiable.Notify(fmt.Sprintf("%s 更新CDN加速域名证书成功，请及时检查证书生效", messagePrefix))
				}
			}()
		}
	}
}

func (m *Manager) Send() {
	m.notifiable.Notify("测试")
}
