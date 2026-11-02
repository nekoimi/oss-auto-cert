// Package acme 封装 Let's Encrypt，并按环境持久化账户。
package acme

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/go-acme/lego/v4/certcrypto"
	legocert "github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/http01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"

	"github.com/nekoimi/oss-auto-cert/internal/certificate"
	"github.com/nekoimi/oss-auto-cert/internal/config"
	"github.com/nekoimi/oss-auto-cert/internal/provider/aliyun"
	"github.com/nekoimi/oss-auto-cert/internal/storage"
)

// Issuer 按账户串行签发，避免共享 lego challenge provider 的竞态。
type Issuer struct {
	Config config.Config
	Cloud  *aliyun.Cloud
	Store  *storage.Store
	mu     sync.Mutex
}

type account struct {
	Schema       int
	Email        string
	PrivateKey   []byte
	Registration *registration.Resource
	key          crypto.PrivateKey
}

func (a *account) GetEmail() string                        { return a.Email }
func (a *account) GetRegistration() *registration.Resource { return a.Registration }
func (a *account) GetPrivateKey() crypto.PrivateKey        { return a.key }

type transport struct {
	ctx  context.Context
	base http.RoundTripper
}

func (t transport) RoundTrip(r *http.Request) (*http.Response, error) {
	// 同时保留 HTTPClient 的单请求超时和整轮签发取消，不能覆盖原请求上下文。
	ctx, cancel := context.WithCancel(r.Context())
	stop := context.AfterFunc(t.ctx, cancel)
	if t.ctx.Err() != nil {
		cancel()
	}
	cleanup := func() { stop(); cancel() }
	response, err := t.base.RoundTrip(r.Clone(ctx))
	if err != nil {
		cleanup()
		return response, err
	}
	response.Body = &scopedBody{ReadCloser: response.Body, cleanup: cleanup}
	return response, nil
}

type scopedBody struct {
	io.ReadCloser
	cleanup func()
}

func (b *scopedBody) Close() error {
	defer b.cleanup()
	return b.ReadCloser.Close()
}

// Issue 懒加载并复用账户，不调用注销接口；每次明确签发新证书。
func (i *Issuer) Issue(parent context.Context, b config.Bucket, domain string) (certificate.Resource, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	ctx, cancel := context.WithTimeout(parent, i.Config.ACME.Timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return certificate.Resource{}, err
	}
	a, err := i.loadAccount()
	if err != nil {
		return certificate.Resource{}, err
	}
	cfg := lego.NewConfig(&a)
	cfg.Certificate.KeyType = certcrypto.RSA2048
	cfg.Certificate.Timeout = i.Config.ACME.Timeout
	if i.Config.ACME.Environment == "staging" {
		cfg.CADirURL = lego.LEDirectoryStaging
	} else {
		cfg.CADirURL = lego.LEDirectoryProduction
	}
	cfg.HTTPClient = &http.Client{Timeout: i.Config.RequestTimeout, Transport: transport{ctx, http.DefaultTransport}}
	client, err := lego.NewClient(cfg)
	if err != nil {
		return certificate.Resource{}, fmt.Errorf("ACME 服务初始化失败，请检查网络和 CA 可用性")
	}
	if a.Registration == nil {
		// 同一持久化账户密钥重复注册会定位原账户，覆盖注册响应丢失场景。
		a.Registration, err = client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			return certificate.Resource{}, fmt.Errorf("ACME 账户注册失败，请检查网络、邮箱和 CA 限流")
		}
		if err := i.Store.WriteJSON(i.accountPath(), a); err != nil {
			return certificate.Resource{}, err
		}
	}
	ossClient, err := i.Cloud.OSSClient(b)
	if err != nil {
		return certificate.Resource{}, err
	}
	provider := &provider{ctx: ctx, client: ossClient, bucket: b.Name, store: i.Store, environment: i.Config.ACME.Environment}
	if err := provider.recover(); err != nil {
		return certificate.Resource{}, err
	}
	if err := client.Challenge.SetHTTP01Provider(provider); err != nil {
		return certificate.Resource{}, err
	}
	r, err := client.Certificate.Obtain(legocert.ObtainRequest{Domains: []string{domain}, Bundle: true})
	if err != nil {
		if ctx.Err() != nil {
			return certificate.Resource{}, ctx.Err()
		}
		return certificate.Resource{}, fmt.Errorf("域名 %s 的 ACME 签发失败，请检查 HTTP-01 访问、CAA 和 CA 限流", domain)
	}
	return certificate.Resource{Certificate: r.Certificate, PrivateKey: r.PrivateKey, Issuer: r.IssuerCertificate}, nil
}

// loadAccount 先保存账户密钥；注册响应丢失后仍复用原身份。
func (i *Issuer) loadAccount() (account, error) {
	path := i.accountPath()
	a := account{Schema: 1, Email: i.Config.ACME.Email}
	err := i.Store.ReadJSON(path, &a)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return account{}, fmt.Errorf("读取 ACME 账户失败: %w", err)
	}
	if errors.Is(err, os.ErrNotExist) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return account{}, err
		}
		raw, err := x509.MarshalECPrivateKey(key)
		if err != nil {
			return account{}, err
		}
		a.PrivateKey = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: raw})
		a.key = key
		if err := i.Store.WriteJSON(path, a); err != nil {
			return account{}, err
		}
	}
	if a.Schema != 1 {
		return account{}, fmt.Errorf("不支持的 ACME 账户 schema")
	}
	a.key, err = certcrypto.ParsePEMPrivateKey(a.PrivateKey)
	if err != nil {
		return account{}, fmt.Errorf("账户私钥损坏: %w", err)
	}
	if a.Email != i.Config.ACME.Email {
		return account{}, fmt.Errorf("持久化 ACME 账户邮箱与配置不一致，请先核对账户配置")
	}
	return a, nil
}

func (i *Issuer) accountPath() string {
	return filepath.Join("accounts", "letsencrypt", i.Config.ACME.Environment, "account.json")
}

type challengeRecord struct {
	Schema      int
	Bucket, Key string
}
type provider struct {
	ctx         context.Context
	client      *oss.Client
	bucket      string
	store       *storage.Store
	environment string
}

func (p *provider) recordPath() string {
	return filepath.Join("accounts", "letsencrypt", p.environment, "challenge-"+p.bucket+".json")
}
func (p *provider) recover() error {
	var r challengeRecord
	err := p.store.ReadJSON(p.recordPath(), &r)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if r.Key == "" {
		return nil
	}
	if r.Schema != 1 || r.Bucket != p.bucket || !strings.HasPrefix(r.Key, ".well-known/acme-challenge/") {
		return fmt.Errorf("发现未清理的 challenge，请核对 Bucket %s 的验证对象", r.Bucket)
	}
	bucket, err := p.client.Bucket(p.bucket)
	if err != nil {
		return err
	}
	if err := bucket.DeleteObject(r.Key, oss.WithContext(p.ctx)); err != nil {
		return aliyun.Classify(err)
	}
	return p.store.WriteJSON(p.recordPath(), challengeRecord{Schema: 1})
}
func (p *provider) Present(domain, token, keyAuth string) error {
	bucket, err := p.client.Bucket(p.bucket)
	if err != nil {
		return err
	}
	key := strings.TrimPrefix(http01.ChallengePath(token), "/")
	if err := p.store.WriteJSON(p.recordPath(), challengeRecord{Schema: 1, Bucket: p.bucket, Key: key}); err != nil {
		return err
	}
	return aliyun.Classify(bucket.PutObject(key, bytes.NewBufferString(keyAuth), oss.ObjectACL(oss.ACLPublicRead), oss.WithContext(p.ctx)))
}
func (p *provider) CleanUp(domain, token, keyAuth string) error {
	bucket, err := p.client.Bucket(p.bucket)
	if err != nil {
		return err
	}
	key := strings.TrimPrefix(http01.ChallengePath(token), "/")
	if err := bucket.DeleteObject(key, oss.WithContext(p.ctx)); err != nil {
		return aliyun.Classify(err)
	}
	return p.store.WriteJSON(p.recordPath(), challengeRecord{Schema: 1})
}
