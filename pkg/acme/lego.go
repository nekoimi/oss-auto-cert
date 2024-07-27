package acme

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
	"github.com/nekoimi/oss-auto-cert/config"
	"github.com/nekoimi/oss-auto-cert/pkg/files"
	oss_provider "github.com/nekoimi/oss-auto-cert/providers/oss"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// DefaultSaveDir 默认证书保存目录
// 保存路径格式: {SaveDir}/{domain}/
const DefaultSaveDir = "/var/lib/oss-auto-cert"

type Lego struct {
	cmux    *sync.Mutex
	saveDir string
	user    registration.User
	client  *lego.Client
}

func NewLego(acme config.Acme) *Lego {
	user := newUser(acme.Email)
	c := lego.NewConfig(user)

	// 此处配置密钥的类型和密钥申请的地址，记得上线后替换成 lego.LEDirectoryProduction
	// 测试环境下就用 lego.LEDirectoryStaging
	c.CADirURL = lego.LEDirectoryStaging
	c.Certificate.KeyType = certcrypto.RSA2048

	// 创建与CA服务器交互的客户端
	client, err := lego.NewClient(c)
	if err != nil {
		log.Fatal(err)
	}

	reg, err := client.Registration.Register(registration.RegisterOptions{
		TermsOfServiceAgreed: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	user.Registration = reg

	saveDir := acme.DataDir
	if saveDir == "" {
		saveDir = DefaultSaveDir
	}

	return &Lego{
		cmux:    new(sync.Mutex),
		saveDir: acme.DataDir,
		user:    user,
		client:  client,
	}
}

// Obtain 申请证书
func (lg *Lego) Obtain(bucket string, domain string, ossClient *oss.Client) (*certificate.Resource, error) {
	provider, err := oss_provider.NewHTTPProvider(bucket, ossClient)
	if err != nil {
		return nil, err
	}

	if err = lg.client.Challenge.SetHTTP01Provider(provider); err != nil {
		return nil, err
	}

	if err = lg.client.Challenge.SetTLSALPN01Provider(provider); err != nil {
		return nil, err
	}

	var cert *certificate.Resource

	// 检查本地是否存在证书
	localCert := filepath.Join(lg.saveDir, domain, "cert.key")
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

	log.Printf("域名(%s)申请证书成功: %s\n", domain, cert)

	// 保存证书到本地
	go lg.save(cert)

	return cert, nil
}

func (lg *Lego) save(cert *certificate.Resource) {
	lg.cmux.Lock()
	defer lg.cmux.Unlock()

	baseDir := filepath.Join(lg.saveDir, cert.Domain)
	if exists, err := files.Exists(baseDir); err != nil {
		log.Printf(err.Error())
		return
	} else if !exists {
		err = os.MkdirAll(baseDir, os.ModeDir)
		if err != nil {
			log.Printf(err.Error())
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
			log.Printf(err.Error())
		} else {
			err = files.Write(filepath.Join(baseDir, name), raw)
			if err != nil {
				log.Printf(err.Error())
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
		log.Fatal(err)
	}

	return &User{
		Email:        email,
		Registration: nil,
		key:          privateKey,
	}
}
