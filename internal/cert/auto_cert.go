package cert

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/charmbracelet/log"
	"github.com/nekoimi/oss-auto-cert/internal/acme"
	"github.com/nekoimi/oss-auto-cert/internal/alioss"
	"github.com/nekoimi/oss-auto-cert/internal/config"
	"github.com/nekoimi/oss-auto-cert/pkg/webhook"
)

// ScheduleInterval 证书巡检定时周期，与 ScheduleRun 中 Ticker 间隔一致。
const ScheduleInterval = 6 * time.Hour

const maxBucketNamesInInfoLog = 5

type AutoCert struct {
	ctx           context.Context
	running       atomic.Bool
	buckets       []config.Bucket
	access        oss.Credentials
	cas           *alioss.CasService
	cdn           *alioss.CDNService
	acme          *acme.LegoAcme
	messageCh     chan string
	messageHandle func(message string)
}

func NewAutoCert(ctx context.Context, conf *config.Config) (*AutoCert, error) {
	credentialsProvider, err := oss.NewEnvironmentVariableCredentialsProvider()
	if err != nil {
		log.Errorf("缺少OSS访问AccessKey环境变量配置: %s", err.Error())
		return nil, err
	}

	access := credentialsProvider.GetCredentials()
	c := &AutoCert{
		ctx:       ctx,
		buckets:   conf.Buckets,
		access:    access,
		cas:       alioss.NewCasService(access),
		cdn:       alioss.NewCDNService(access),
		acme:      acme.NewLegoAcme(conf.Acme),
		messageCh: make(chan string),
	}
	c.running.Store(false)
	if len(c.buckets) <= 0 {
		log.Warnf("OSS存储Bucket配置为空!")
	} else {
		names := make([]string, 0, len(c.buckets))
		for _, b := range c.buckets {
			names = append(names, b.Name)
			log.Debugf("Bucket开启监测: %s => %s", b.Name, b.Endpoint)
		}
		summary := strings.Join(names, ", ")
		if len(c.buckets) > maxBucketNamesInInfoLog {
			summary = strings.Join(names[:maxBucketNamesInInfoLog], ", ") +
				fmt.Sprintf(" 等共 %d 个", len(c.buckets))
		}
		log.Infof("已加载 %d 个待监测 Bucket: %s", len(c.buckets), summary)
	}

	if conf.Webhook != "" {
		tplHook := webhook.NewTplWebHook(conf.Webhook, conf.WebhookTpl)
		c.withMessageHandle(func(message string) {
			tplHook.SendHook(message)
		})
	} else {
		c.withMessageHandle(func(message string) {
			// 打印日志
			log.Info(message)
		})
	}

	return c, nil
}

func (c *AutoCert) withMessageHandle(messageHandle func(message string)) {
	c.messageHandle = messageHandle
}

func (c *AutoCert) ScheduleRun() {
	log.Infof(
		"定时调度已启动：证书检查间隔为 %s；启动后将立即执行一次巡检，此后按间隔重复执行",
		ScheduleInterval,
	)

	go func() {
		for {
			select {
			case <-c.ctx.Done():
				return
			case message := <-c.messageCh:
				if c.messageHandle != nil {
					c.messageHandle(message)
				}
			}
		}
	}()

	go func() {
		go c.run()
		tick := time.NewTicker(ScheduleInterval)
		defer tick.Stop()
		for {
			select {
			case <-c.ctx.Done():
				return
			case <-tick.C:
				go c.run()
			}
		}
	}()
}

func (c *AutoCert) Stop() {
	c.acme.Stop()
}

func (c *AutoCert) run() {
	if c.running.Load() {
		return
	}

	c.running.Store(true)
	defer func() {
		c.running.Store(false)
	}()

	if len(c.buckets) > 0 {
		log.Infof("开始执行证书巡检，共 %d 个 Bucket", len(c.buckets))
	}

	for _, bucket := range c.buckets {
		log.Debugf("开始检测Bucket: %s ...", bucket.Name)

		b, err := alioss.NewAliYunOss(bucket, c.access)
		if err != nil {
			log.Errorf(err.Error())
			continue
		}

		info, err := b.GetCert()
		if err != nil {
			log.Errorf(err.Error())
			continue
		}

		expired, err := c.cas.IsExpired(info.ID)
		if err != nil {
			log.Errorf(err.Error())
			continue
		}

		if expired {
			messagePrefix := fmt.Sprintf("【oss-auto-cert】Bucket<%s> 域名: %s\n", bucket.Name, info.Domain)

			c.messageCh <- fmt.Sprintf("%s 证书过期，需要更换新证书", messagePrefix)

			// 过期，申请新证书
			cert, err := c.acme.Obtain(bucket.Name, info.Domain, b.Client)
			if err != nil {
				log.Errorf(err.Error())
				continue
			}

			// 上传证书文件到阿里云数字证书管理服务
			certInfo, err := c.cas.Upload(cert)
			if err != nil {
				log.Errorf(err.Error())
				c.messageCh <- fmt.Sprintf("%s 上传证书到数字证书管理异常: %s", messagePrefix, err.Error())
				continue
			}

			certInfo.Region = info.Region

			log.Infof("证书上传信息: %s", certInfo)

			go func() {
				// 更新OSS域名关联的证书
				err := b.UpgradeCert(info.Domain, fmt.Sprintf("%d-%s", certInfo.ID, info.Region))
				if err != nil {
					log.Errorf(err.Error())
					c.messageCh <- fmt.Sprintf("%s 更新OSS域名证书失败: %s", messagePrefix, err.Error())
				} else {
					c.messageCh <- fmt.Sprintf("%s 更新OSS域名证书成功，请及时检查证书生效", messagePrefix)
				}
			}()

			go func() {
				// 更新CDN关联的域名证书
				err := c.cdn.UpgradeCert(info.Domain, certInfo)
				if err != nil {
					log.Errorf(err.Error())
					c.messageCh <- fmt.Sprintf("%s 更新CDN加速域名证书失败: %s", messagePrefix, err.Error())
				} else {
					c.messageCh <- fmt.Sprintf("%s 更新CDN加速域名证书成功，请及时检查证书生效", messagePrefix)
				}
			}()
		}
	}
}
