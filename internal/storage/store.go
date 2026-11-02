// Package storage 提供带版本、原子替换和运行锁的本地文件存储。
package storage

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"github.com/nekoimi/oss-auto-cert/internal/certificate"
	"github.com/nekoimi/oss-auto-cert/internal/config"
)

// Store 为每个 CA 环境隔离数据。
type Store struct{ Root, Environment string }

// New 构造存储对象，不创建文件或目录。
func New(root, environment string) *Store { return &Store{Root: root, Environment: environment} }

func id(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Lock 获取本机数据目录的单写者锁，操作系统在进程退出时释放锁。
func (s *Store) Lock() (func() error, error) {
	dir := filepath.Join(s.Root, "locks")
	if err := secureDir(dir); err != nil {
		return nil, err
	}
	lock := flock.New(filepath.Join(dir, "run.lock"))
	ok, err := lock.TryLock()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("数据目录已有运行实例，请停止后重试")
	}
	return lock.Unlock, nil
}

func (s *Store) statePath(bucket, domain string) string {
	return filepath.Join(s.Root, "state", s.Environment, id(bucket, domain)+".json")
}

// LoadState 读取状态；未知版本、损坏文件和读取错误不会被当作空状态。
func (s *Store) LoadState(bucket, domain string) (certificate.State, error) {
	var state certificate.State
	raw, err := os.ReadFile(s.statePath(bucket, domain))
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return state, fmt.Errorf("任务状态损坏: %w", err)
	}
	if state.Schema != 1 || state.Environment != s.Environment || state.Domain != domain || state.Bucket != bucket {
		return state, fmt.Errorf("任务状态版本或资源身份不匹配")
	}
	return state, nil
}

// SaveState 原子保存恢复进度。
func (s *Store) SaveState(state certificate.State) error {
	state.Schema = 1
	state.UpdatedAt = time.Now().UTC()
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return WriteFile(s.statePath(state.Bucket, state.Domain), raw)
}

// States 读取本地快照，不要求运行锁，不修改文件。
func (s *Store) States() ([]certificate.State, error) {
	files, err := filepath.Glob(filepath.Join(s.Root, "state", s.Environment, "*.json"))
	if err != nil {
		return nil, err
	}
	var states []certificate.State
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		var state certificate.State
		if err := json.Unmarshal(raw, &state); err != nil {
			return nil, fmt.Errorf("状态文件损坏: %w", err)
		}
		if state.Schema != 1 {
			return nil, fmt.Errorf("不支持的状态 schema %d", state.Schema)
		}
		states = append(states, state)
	}
	return states, nil
}

// SaveCertificate 发布一份完整证书，不改写已有版本。
func (s *Store) SaveCertificate(bucket, domain string, r certificate.Resource) (string, error) {
	fingerprint, _, err := certificate.Inspect(r, domain)
	if err != nil {
		return "", err
	}
	relative := filepath.Join("certificates", s.Environment, id(bucket, domain), fingerprint)
	dir := filepath.Join(s.Root, relative)
	if existing, err := readCertificate(dir); err == nil {
		fp, _, err := certificate.Inspect(existing, domain)
		if err != nil {
			return "", err
		}
		if fp != fingerprint {
			return "", fmt.Errorf("已有证书版本损坏")
		}
		return filepath.ToSlash(relative), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := secureDir(filepath.Dir(dir)); err != nil {
		return "", err
	}
	temporary, err := os.MkdirTemp(filepath.Dir(dir), ".certificate-*")
	if err != nil {
		return "", err
	}
	defer func() {
		if err := os.RemoveAll(temporary); err != nil { /* 未发布临时目录将在下次维护时清理。 */
		}
	}()
	if err := WriteFile(filepath.Join(temporary, "cert.key"), r.PrivateKey); err != nil {
		return "", err
	}
	if err := WriteFile(filepath.Join(temporary, "cert.crt"), r.Certificate); err != nil {
		return "", err
	}
	if err := WriteFile(filepath.Join(temporary, "issuer.crt"), r.Issuer); err != nil {
		return "", err
	}
	if err := os.Rename(temporary, dir); err != nil {
		return "", err
	}
	return filepath.ToSlash(relative), nil
}

// LoadCertificate 读取状态引用的证书，拒绝越界路径和身份变化。
func (s *Store) LoadCertificate(state certificate.State) (certificate.Resource, error) {
	local := filepath.FromSlash(state.CertificatePath)
	if local == "" || !filepath.IsLocal(local) || !strings.HasPrefix(local, filepath.Join("certificates", s.Environment)+string(filepath.Separator)) {
		return certificate.Resource{}, fmt.Errorf("证书引用无效")
	}
	r, err := readCertificate(filepath.Join(s.Root, local))
	if err != nil {
		return r, err
	}
	fp, _, err := certificate.Inspect(r, state.Domain)
	if err != nil {
		return r, err
	}
	if fp != state.Fingerprint {
		return r, fmt.Errorf("证书指纹与任务状态不一致")
	}
	return r, nil
}

func readCertificate(dir string) (certificate.Resource, error) {
	var r certificate.Resource
	var err error
	r.Certificate, err = os.ReadFile(filepath.Join(dir, "cert.crt"))
	if err != nil {
		return r, err
	}
	r.PrivateKey, err = os.ReadFile(filepath.Join(dir, "cert.key"))
	return r, err
}

// ImportLegacy 只读导入旧文件，生产环境必须通过系统信任链校验。
func (s *Store) ImportLegacy(domain string) (certificate.Resource, error) {
	if _, err := config.Domain(domain); err != nil {
		return certificate.Resource{}, err
	}
	r, err := readCertificate(filepath.Join(s.Root, domain))
	if err != nil {
		return r, err
	}
	if _, _, err := certificate.Inspect(r, domain); err != nil {
		return r, err
	}
	if s.Environment != "production" {
		return r, fmt.Errorf("旧证书没有可靠测试环境来源，请显式重新签发")
	}
	var certs []*x509.Certificate
	rest := r.Certificate
	for {
		block, next := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = next
		if block.Type == "CERTIFICATE" {
			parsed, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return r, err
			}
			certs = append(certs, parsed)
		}
	}
	if len(certs) == 0 {
		return r, fmt.Errorf("旧证书为空")
	}
	intermediates := x509.NewCertPool()
	for _, c := range certs[1:] {
		intermediates.AddCert(c)
	}
	verifyTime := time.Now()
	if !certs[0].NotAfter.After(verifyTime) {
		verifyTime = certs[0].NotBefore.Add(time.Minute)
	}
	if _, err := certs[0].Verify(x509.VerifyOptions{DNSName: domain, Intermediates: intermediates, CurrentTime: verifyTime}); err != nil {
		return r, fmt.Errorf("旧证书信任校验失败: %w", err)
	}
	return r, nil
}

// ReadJSON 读取账户元数据。
func (s *Store) ReadJSON(name string, target any) error {
	if !filepath.IsLocal(name) {
		return fmt.Errorf("非法存储路径")
	}
	raw, err := os.ReadFile(filepath.Join(s.Root, name))
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

// WriteJSON 安全保存账户元数据。
func (s *Store) WriteJSON(name string, value any) error {
	if !filepath.IsLocal(name) {
		return fmt.Errorf("非法存储路径")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return WriteFile(filepath.Join(s.Root, name), raw)
}

// WriteFile 在同一目录写临时文件并原子替换，不跟随目标文件的符号链接。
func WriteFile(path string, raw []byte) error {
	if err := secureDir(filepath.Dir(path)); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".write-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer func() {
		if err := os.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) { /* 已发布文件无需清理。 */
		}
	}()
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if err := restrictFile(name); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(raw); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return replace(name, path)
}
