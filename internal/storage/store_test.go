package storage

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/nekoimi/oss-auto-cert/internal/certificate"
	"github.com/nekoimi/oss-auto-cert/internal/testutil"
)

func TestLockAndAtomicState(t *testing.T) {
	s := New(t.TempDir(), "production")
	release, err := s.Lock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(s.Root, "staging").Lock(); err == nil {
		t.Fatal("同一数据目录必须单写")
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	state := certificate.State{Schema: 1, Bucket: "my-bucket", Domain: "static.example.com", Environment: "production"}
	if err := s.SaveState(state); err != nil {
		t.Fatal(err)
	}
	state.Error = "second write"
	if err := s.SaveState(state); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadState(state.Bucket, state.Domain)
	if err != nil || got.Error != state.Error {
		t.Fatalf("状态替换失败: %+v %v", got, err)
	}
	if _, err := New(s.Root, "staging").LoadState(state.Bucket, state.Domain); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("环境数据未隔离")
	}
	if err := os.WriteFile(s.statePath(state.Bucket, state.Domain), []byte(`{"Schema":99}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadState(state.Bucket, state.Domain); err == nil {
		t.Fatal("未知状态版本不能当作空状态")
	}
	if err := os.WriteFile(s.statePath(state.Bucket, state.Domain), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadState(state.Bucket, state.Domain); err == nil {
		t.Fatal("缺少版本和资源身份的文件不能当作新任务")
	}
}

func TestCertificatePermissionsAndTampering(t *testing.T) {
	s := New(t.TempDir(), "production")
	domain := "static.example.com"
	r := testutil.Certificate(t, domain, time.Now().Add(24*time.Hour))
	fp, expiry, err := certificate.Inspect(r, domain)
	if err != nil {
		t.Fatal(err)
	}
	path, err := s.SaveCertificate("my-bucket", domain, r)
	if err != nil {
		t.Fatal(err)
	}
	state := certificate.State{Domain: domain, Fingerprint: fp, CertificatePath: path, NotAfter: expiry}
	if _, err := s.LoadCertificate(state); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(s.Root, path, "cert.key"))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("私钥权限: %o", info.Mode().Perm())
		}
		dir, err := os.Stat(filepath.Join(s.Root, path))
		if err != nil {
			t.Fatal(err)
		}
		if dir.Mode().Perm() != 0700 {
			t.Fatalf("目录权限: %o", dir.Mode().Perm())
		}
	}
	state.CertificatePath = "../escape"
	if _, err := s.LoadCertificate(state); err == nil {
		t.Fatal("越界路径未拒绝")
	}
	state.CertificatePath = path
	state.Fingerprint = "wrong"
	if _, err := s.LoadCertificate(state); err == nil {
		t.Fatal("指纹变化未拒绝")
	}
	if _, err := s.ImportLegacy("../escape"); err == nil {
		t.Fatal("旧目录越界未拒绝")
	}
}
