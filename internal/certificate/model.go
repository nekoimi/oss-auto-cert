// Package certificate 实现独立于 CLI 和云 SDK 的证书管理流程。
package certificate

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/nekoimi/oss-auto-cert/internal/config"
)

// Resource 是一份已签发的证书和对应私钥。
type Resource struct{ Certificate, PrivateKey, Issuer []byte }

// Inspect 检查证书与私钥匹配并覆盖域名，返回证书身份和到期时间。
func Inspect(r Resource, domain string) (string, time.Time, error) {
	pair, err := tls.X509KeyPair(r.Certificate, r.PrivateKey)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("证书或私钥不匹配: %w", err)
	}
	if len(pair.Certificate) == 0 {
		return "", time.Time{}, fmt.Errorf("证书为空")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return "", time.Time{}, err
	}
	if err := leaf.VerifyHostname(domain); err != nil {
		return "", time.Time{}, fmt.Errorf("证书不覆盖域名: %w", err)
	}
	if leaf.IsCA {
		return "", time.Time{}, fmt.Errorf("不能将 CA 证书用作域名证书")
	}
	hash := sha256.Sum256(pair.Certificate[0])
	return hex.EncodeToString(hash[:]), leaf.NotAfter, nil
}

// Remote 描述 OSS 当前证书，不依赖 CAS ID 是否存在。
type Remote struct {
	Domain, CertID, Fingerprint string
	NotAfter                    time.Time
	HasCertificate              bool
}

// Uploaded 描述已上传 CAS 的证书。
type Uploaded struct {
	ID           int64
	Name, Region string
}

// Target 保存某一部署目标的恢复进度。
type Target struct {
	Status      string
	Attempts    int
	NextAttempt time.Time
	Message     string
	Submitted   bool
}

// State 是可持久化的逐域名任务，不保存私钥或秘密。
type State struct {
	Schema                                   int
	Bucket, Domain, Environment, Fingerprint string
	NotAfter                                 time.Time
	CertificatePath                          string
	Uploaded                                 *Uploaded
	UploadIntent                             string
	OSS, CDN                                 Target
	UpdatedAt                                time.Time
	Error                                    string
	Event                                    string
}

// Cloud 负责发现和部署，所有实现必须提供请求取消和超时。
type Cloud interface {
	Discover(context.Context, config.Bucket) ([]Remote, error)
	Upload(context.Context, Resource, string) (Uploaded, error)
	FindUpload(context.Context, string, string) (*Uploaded, error)
	DeployOSS(context.Context, config.Bucket, string, Uploaded) error
	VerifyOSS(context.Context, config.Bucket, string, Uploaded) (bool, error)
	CDNEnabled(context.Context, string) (bool, error)
	CDNNotAfter(context.Context, string) (time.Time, error)
	DeployCDN(context.Context, string, Uploaded) error
	VerifyCDN(context.Context, string, Uploaded) (bool, error)
}

// Issuer 只在明确执行签发计划时调用。
type Issuer interface {
	Issue(context.Context, config.Bucket, string) (Resource, error)
}

// Store 负责持久化与旧文件导入。
type Store interface {
	LoadState(string, string) (State, error)
	SaveState(State) error
	LoadCertificate(State) (Resource, error)
	SaveCertificate(string, string, Resource) (string, error)
	ImportLegacy(string) (Resource, error)
}

// Notifier 发送聚合后的域名事件。
type Notifier interface {
	Send(context.Context, string, string) error
}

// Result 描述一次域名任务的用户可见结果。
type Result struct {
	Bucket, Domain, Action, Message string
	Failed                          bool
	Fatal                           bool
}

// TemporaryError 表示允许自动重试的错误。
type TemporaryError struct{ Err error }

func (e *TemporaryError) Error() string { return e.Err.Error() }
func (e *TemporaryError) Unwrap() error { return e.Err }
