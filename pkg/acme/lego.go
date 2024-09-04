package acme

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/charmbracelet/log"
	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
	"github.com/nekoimi/oss-auto-cert/config"
	"github.com/nekoimi/oss-auto-cert/pkg/files"
	oss_provider "github.com/nekoimi/oss-auto-cert/providers/oss"
	"os"
	"path/filepath"
	"sync"
)

// DefaultSaveDir 默认证书保存目录
// 保存路径格式: {SaveDir}/{domain}/
const DefaultSaveDir = "/var/lib/oss-auto-cert"

type LegoService struct {
	cmux    *sync.Mutex
	saveDir string
	user    registration.User
	client  *lego.Client
}

func NewLego(acme config.Acme) *LegoService {
	user := newUser(acme.Email)
	c := lego.NewConfig(user)

	// 此处配置密钥的类型和密钥申请的地址，记得上线后替换成 lego.LEDirectoryProduction
	debug := os.Getenv("DEBUG")
	if debug == "true" {
		// 演示
		// 测试环境下就用 lego.LEDirectoryStaging
		log.Warnf("Lego CA-Dir use staging")
		c.CADirURL = lego.LEDirectoryStaging
	} else {
		log.Infof("Lego CA-Dir use production")
		c.CADirURL = lego.LEDirectoryProduction
	}
	c.Certificate.KeyType = certcrypto.RSA2048

	// 创建与CA服务器交互的客户端
	client, err := lego.NewClient(c)
	if err != nil {
		log.Fatalf(err.Error())
	}

	reg, err := client.Registration.Register(registration.RegisterOptions{
		TermsOfServiceAgreed: true,
	})
	if err != nil {
		log.Fatalf(err.Error())
	}

	user.Registration = reg

	saveDir := acme.DataDir
	if saveDir == "" {
		saveDir = DefaultSaveDir
	}

	return &LegoService{
		cmux:    new(sync.Mutex),
		saveDir: acme.DataDir,
		user:    user,
		client:  client,
	}
}

// Obtain 申请证书
func (lg *LegoService) Obtain(bucket string, domain string, ossClient *oss.Client) (*certificate.Resource, error) {
	provider, err := oss_provider.NewHTTPProvider(bucket, ossClient)
	if err != nil {
		return nil, err
	}

	if err = lg.client.Challenge.SetHTTP01Provider(provider); err != nil {
		return nil, err
	}

	var cert *certificate.Resource

	// 检查本地是否存在证书
	localCert := filepath.Join(lg.saveDir, domain, "cert.crt")
	ok, b := files.ReadIfExists(localCert)
	if ok {
		// 续签证书
		renew := certificate.Resource{
			Domain:      domain,
			Certificate: b,
		}

		// 尝试读取证书签名CSR
		localCsr := filepath.Join(lg.saveDir, domain, "cert.csr")
		ok, b = files.ReadIfExists(localCsr)
		if ok {
			renew.CSR = b
		}

		// 尝试读取颁发者证书
		localIssuerCert := filepath.Join(lg.saveDir, domain, "cert-issuer.crt")
		ok, b = files.ReadIfExists(localIssuerCert)
		if ok {
			renew.IssuerCertificate = b
		}

		// 发起续签请求
		cert, err = lg.client.Certificate.RenewWithOptions(renew, &certificate.RenewOptions{
			Bundle: true,
		})
		if err != nil {
			return nil, fmt.Errorf("域名(%s)续签证书失败：%w", domain, err)
		}
	} else {
		// 发起申请请求
		req := certificate.ObtainRequest{
			Domains: []string{domain},
			// 这里如果是true，将把颁发者证书一起返回，也就是返回里面certificates.IssuerCertificate
			Bundle: true,
		}
		// 申请证书
		cert, err = lg.client.Certificate.Obtain(req)
		if err != nil {
			return nil, fmt.Errorf("域名(%s)申请证书失败：%w", domain, err)
		}
	}

	log.Infof("域名(%s)申请证书成功!", domain)

	// 保存证书到本地
	go lg.save(cert)

	return cert, nil
}

func (lg *LegoService) Stop() {
	err := lg.client.Registration.DeleteRegistration()
	if err != nil {
		log.Errorf(err.Error())
	}
}

func (lg *LegoService) save(cert *certificate.Resource) {
	lg.cmux.Lock()
	defer lg.cmux.Unlock()

	baseDir := filepath.Join(lg.saveDir, cert.Domain)
	if exists, err := files.Exists(baseDir); err != nil {
		log.Errorf(err.Error())
		return
	} else if !exists {
		err = os.MkdirAll(baseDir, os.ModeDir)
		if err != nil {
			log.Errorf(err.Error())
			return
		}
	}

	// 分别保存证书文件和私钥文件
	data := make(map[string][]byte)
	data["cert.key"] = cert.PrivateKey
	data["cert.crt"] = cert.Certificate
	data["cert-issuer.crt"] = cert.IssuerCertificate
	data["cert.csr"] = cert.CSR

	for name, raw := range data {
		if err := files.BackupIfExists(filepath.Join(baseDir, name)); err != nil {
			log.Errorf(err.Error())
		} else {
			err = os.WriteFile(filepath.Join(baseDir, name), raw, os.ModePerm)
			if err != nil {
				log.Errorf(err.Error())
			}
		}
	}
}

type User struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *User) GetEmail() string {
	return u.Email
}

func (u User) GetRegistration() *registration.Resource {
	return u.Registration
}

func (u *User) GetPrivateKey() crypto.PrivateKey {
	return u.key
}

func newUser(email string) *User {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatalf(err.Error())
	}

	return &User{
		Email:        email,
		Registration: nil,
		key:          privateKey,
	}
}
