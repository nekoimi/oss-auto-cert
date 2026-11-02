package certificate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"

	"github.com/nekoimi/oss-auto-cert/internal/config"
)

// prepareCertificate 只在没有待恢复部署时签发，并先持久化证书和进度。
func (s *Service) prepareCertificate(ctx context.Context, b config.Bucket, domain string, state *State, force bool) (Resource, error) {
	var resource Resource
	var err error
	if !force && !s.Config.Force {
		imported, importErr := s.Store.ImportLegacy(domain)
		if importErr == nil {
			_, expiry, checkErr := Inspect(imported, domain)
			if checkErr == nil && expiry.After(s.now().Add(s.Config.RenewBefore)) {
				s.log(logrus.InfoLevel, b.Name, domain, "import", "复用旧版证书")
				resource = imported
			}
		} else if !errors.Is(importErr, os.ErrNotExist) {
			return Resource{}, fmt.Errorf("旧证书导入失败，请核对文件或使用 --once --force: %w", importErr)
		}
	}
	if len(resource.Certificate) == 0 {
		s.log(logrus.InfoLevel, b.Name, domain, "issue", "开始申请证书")
		resource, err = s.Issuer.Issue(ctx, b, domain)
		if err != nil {
			return Resource{}, err
		}
	}
	fp, expiry, err := Inspect(resource, domain)
	if err != nil {
		return Resource{}, err
	}
	if !expiry.After(s.now()) {
		return Resource{}, fmt.Errorf("签发结果已经过期")
	}
	path, err := s.Store.SaveCertificate(b.Name, domain, resource)
	if err != nil {
		return Resource{}, err
	}
	*state = State{Schema: 1, Bucket: b.Name, Domain: domain, Environment: s.Config.ACME.Environment, Fingerprint: fp, NotAfter: expiry, CertificatePath: path}
	if err := s.Store.SaveState(*state); err != nil {
		return Resource{}, err
	}
	return resource, nil
}

// ensureUpload 恢复响应丢失的上传，未知结果不会盲目重建。
func (s *Service) ensureUpload(ctx context.Context, b config.Bucket, domain string, state *State, resource Resource) error {
	if state.Uploaded == nil {
		if state.UploadIntent != "" {
			s.log(logrus.InfoLevel, b.Name, domain, "upload", "核对 CAS 上传记录与证书指纹")
			found, err := s.Cloud.FindUpload(ctx, state.UploadIntent, state.Fingerprint)
			if err != nil {
				return err
			}
			if found == nil {
				return fmt.Errorf("CAS 上传结果不明确，请核对名称 %s 后恢复；为避免重复创建暂停上传", state.UploadIntent)
			}
			state.Uploaded = found
		} else {
			sum := sha256.Sum256([]byte(b.Name + "\x00" + domain + "\x00" + state.Fingerprint))
			state.UploadIntent = "oac-" + hex.EncodeToString(sum[:16])
			if err := s.Store.SaveState(*state); err != nil {
				return err
			}
			s.log(logrus.InfoLevel, b.Name, domain, "upload", "上传证书到 CAS")
			uploaded, err := s.Cloud.Upload(ctx, resource, state.UploadIntent)
			if err != nil {
				return err
			}
			state.Uploaded = &uploaded
		}
		if err := s.Store.SaveState(*state); err != nil {
			return err
		}
	}
	return nil
}

// deployTargets 分别保存并核验每个目标，一个目标失败不阻止另一个目标。
func (s *Service) deployTargets(ctx context.Context, b config.Bucket, domain string, state *State, enabled bool) error {
	var err error
	var failures []error
	if b.DeployOSS {
		err = s.target(ctx, &state.OSS, func() (bool, error) { return s.Cloud.VerifyOSS(ctx, b, domain, *state.Uploaded) }, func() error {
			s.log(logrus.InfoLevel, b.Name, domain, "deploy_oss", "更新 OSS 证书绑定")
			return s.Cloud.DeployOSS(ctx, b, domain, *state.Uploaded)
		}, func() error { return s.Store.SaveState(*state) })
		if err != nil {
			failures = append(failures, fmt.Errorf("OSS: %w", err))
		}
	} else {
		state.OSS = Target{Status: "skipped", Message: "配置禁用"}
	}
	if enabled {
		err = s.target(ctx, &state.CDN, func() (bool, error) { return s.Cloud.VerifyCDN(ctx, domain, *state.Uploaded) }, func() error {
			s.log(logrus.InfoLevel, b.Name, domain, "deploy_cdn", "更新 CDN 证书绑定")
			return s.Cloud.DeployCDN(ctx, domain, *state.Uploaded)
		}, func() error { return s.Store.SaveState(*state) })
		if err != nil {
			failures = append(failures, fmt.Errorf("CDN: %w", err))
		}
	} else {
		state.CDN = Target{Status: "skipped", Message: "未使用 CDN 或 HTTPS 未开启"}
	}
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	return nil
}
