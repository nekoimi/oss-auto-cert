package acme

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
	oss_provider "github.com/nekoimi/oss-auto-cert/providers/oss"
	"log"
)

type Lego struct {
	user registration.User

	client *lego.Client
}

func NewLego(email string) *Lego {
	user := newUser(email)
	config := lego.NewConfig(user)

	// 此处配置密钥的类型和密钥申请的地址，记得上线后替换成 lego.LEDirectoryProduction
	// 测试环境下就用 lego.LEDirectoryStaging
	config.CADirURL = lego.LEDirectoryStaging
	config.Certificate.KeyType = certcrypto.RSA2048

	// 创建与CA服务器交互的客户端
	client, err := lego.NewClient(config)
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

	return &Lego{
		user:   user,
		client: client,
	}
}

// Obtain 申请证书
func (lg *Lego) Obtain(domain string, bucket string, ossClient *oss.Client) (*certificate.Resource, error) {
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

	// 发起申请请求
	req := certificate.ObtainRequest{
		Domains: []string{domain},
		// 这里如果是true，将把颁发者证书一起返回，也就是返回里面certificates.IssuerCertificate
		Bundle: true,
	}

	cert, err := lg.client.Certificate.Obtain(req)
	if err != nil {
		return nil, err
	}

	log.Printf("域名(%s)申请证书成功: %s\n", domain, cert)

	return cert, nil
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
