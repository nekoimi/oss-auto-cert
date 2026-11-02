package certificate

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/nekoimi/oss-auto-cert/internal/config"
)

// Service 按域名执行计划，所有异步任务在 Run 返回前收尾。
type Service struct {
	Config   config.Config
	Cloud    Cloud
	Issuer   Issuer
	Store    Store
	Notifier Notifier
	Logger   *logrus.Logger
	Now      func() time.Time
}

// Run 完成一轮；dryRun 只读取资源与存储。
func (s *Service) Run(ctx context.Context, dryRun, force bool) []Result {
	type task struct {
		bucket config.Bucket
		remote Remote
	}
	var tasks []task
	var results []Result
	seen := map[string]bool{}
	for _, b := range s.Config.Buckets {
		s.log(logrus.DebugLevel, b.Name, "", "discover", "读取 Bucket 域名与证书状态")
		items, err := s.Cloud.Discover(ctx, b)
		if err != nil {
			results = append(results, Result{Bucket: b.Name, Action: "检查失败", Message: err.Error(), Failed: true})
			continue
		}
		selected, err := Select(b, items)
		if err != nil {
			results = append(results, Result{Bucket: b.Name, Action: "范围错误", Message: err.Error(), Failed: true})
			continue
		}
		for _, item := range selected {
			if seen[item.Domain] {
				results = append(results, Result{Bucket: b.Name, Domain: item.Domain, Action: "范围冲突", Message: "域名在多个 Bucket 重复出现，本轮跳过冲突域名", Failed: true})
				continue
			}
			seen[item.Domain] = true
			tasks = append(tasks, task{b, item})
		}
	}
	// 运行前排除所有重复域名，不能先写第一个再报告第二个冲突。
	conflicts := map[string]bool{}
	for _, r := range results {
		if r.Action == "范围冲突" {
			conflicts[r.Domain] = true
		}
	}
	var mu sync.Mutex
	limit := make(chan struct{}, s.Config.Workers)
	var wg sync.WaitGroup
launch:
	for _, t := range tasks {
		if conflicts[t.remote.Domain] {
			continue
		}
		select {
		case limit <- struct{}{}:
		case <-ctx.Done():
			break launch
		}
		wg.Add(1)
		go func(t task) {
			defer wg.Done()
			defer func() { <-limit }()
			result := s.process(ctx, t.bucket, t.remote, dryRun, force)
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(t)
	}
	wg.Wait()
	if ctx.Err() != nil {
		results = append(results, Result{Action: "已取消", Message: "已停止新任务，在途任务已收尾", Failed: true})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Bucket == results[j].Bucket {
			return results[i].Domain < results[j].Domain
		}
		return results[i].Bucket < results[j].Bucket
	})
	return results
}

// Select 根据明确策略选择域名，不新增绑定。
func Select(b config.Bucket, items []Remote) ([]Remote, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("Bucket 未绑定自定义域名")
	}
	switch b.DomainMode {
	case "first":
		return items[:1], nil
	case "all":
		return items, nil
	case "listed":
		var result []Remote
		for _, domain := range b.Domains {
			found := false
			for _, item := range items {
				if item.Domain == domain {
					result = append(result, item)
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("配置域名 %s 尚未绑定到 Bucket", domain)
			}
		}
		return result, nil
	}
	return nil, fmt.Errorf("未知域名选择策略")
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) process(ctx context.Context, b config.Bucket, remote Remote, dry, force bool) (result Result) {
	result = Result{Bucket: b.Name, Domain: remote.Domain}
	state, err := s.Store.LoadState(b.Name, remote.Domain)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		result.Action = "状态错误"
		result.Message = err.Error()
		result.Failed = true
		result.Fatal = true
		return
	}
	if errors.Is(err, os.ErrNotExist) {
		state = State{Schema: 1, Bucket: b.Name, Domain: remote.Domain, Environment: s.Config.ACME.Environment}
	}
	s.log(logrus.DebugLevel, b.Name, remote.Domain, "plan", "评估证书续期与部署恢复计划")
	oldEvent := state.Event
	fail := func(err error) Result {
		result.Action = "处理失败"
		result.Message = err.Error()
		result.Failed = true
		if !dry {
			state.Error = result.Message
			state.Event = "failed"
			if saveErr := s.Store.SaveState(state); saveErr != nil {
				result.Message = "持久化状态失败: " + saveErr.Error()
				result.Fatal = true
			}
			if oldEvent != "failed" && s.Notifier != nil {
				if err := s.Notifier.Send(ctx, "failed", fmt.Sprintf("%s：%s", remote.Domain, result.Message)); err != nil {
					result.Message += "；通知失败: " + err.Error()
				}
			}
		}
		return result
	}
	enabled := false
	var cdnExpiry time.Time
	if b.CDN == "auto" {
		enabled, err = s.Cloud.CDNEnabled(ctx, remote.Domain)
		if err != nil {
			return fail(err)
		}
		if enabled {
			cdnExpiry, err = s.Cloud.CDNNotAfter(ctx, remote.Domain)
			if err != nil {
				return fail(err)
			}
		}
	}
	if !b.DeployOSS && !enabled {
		return fail(fmt.Errorf("未找到可更新的部署目标，请检查 CDN HTTPS 是否启用"))
	}
	pending := state.CertificatePath != "" && ((b.DeployOSS && state.OSS.Status != "verified") || (enabled && state.CDN.Status != "verified"))
	// 有本地证书时独立核对目标，检测外部修改及 CDN-only 的续期状态。
	if state.Uploaded != nil && state.CertificatePath != "" && state.NotAfter.After(s.now()) {
		if b.DeployOSS && state.OSS.Status == "verified" {
			ok, err := s.Cloud.VerifyOSS(ctx, b, remote.Domain, *state.Uploaded)
			if err != nil {
				return fail(err)
			}
			if !ok {
				state.OSS = Target{}
				pending = true
			}
		}
		if enabled && state.CDN.Status == "verified" {
			ok, err := s.Cloud.VerifyCDN(ctx, remote.Domain, *state.Uploaded)
			if err != nil {
				return fail(err)
			}
			if !ok {
				state.CDN = Target{}
				pending = true
			}
		}
	}
	if !remote.HasCertificate && !(enabled && !b.DeployOSS) && !b.AllowInitialIssuance && !pending {
		return fail(fmt.Errorf("域名未绑定证书，首次部署需显式开启 allow-initial-issuance"))
	}
	if remote.HasCertificate && remote.NotAfter.IsZero() && !pending {
		return fail(fmt.Errorf("无法读取证书到期时间，不能判定是否需要续期"))
	}
	needIssue := force || s.Config.Force || !remote.HasCertificate || !remote.NotAfter.After(s.now().Add(s.Config.RenewBefore))
	if enabled && !cdnExpiry.After(s.now().Add(s.Config.RenewBefore)) {
		needIssue = true
	}
	if !b.DeployOSS && enabled {
		needIssue = force || s.Config.Force || !cdnExpiry.After(s.now().Add(s.Config.RenewBefore))
	}
	if !b.DeployOSS && state.CertificatePath != "" {
		needIssue = force || s.Config.Force || !state.NotAfter.After(s.now().Add(s.Config.RenewBefore))
	}
	if state.CertificatePath != "" && !state.NotAfter.After(s.now()) {
		pending = false
		needIssue = true
	}
	if dry {
		switch {
		case pending:
			result.Action = "计划恢复部署"
		case needIssue:
			result.Action = "计划签发并部署"
		default:
			result.Action = "无需处理"
		}
		result.Message = fmt.Sprintf("OSS=%t，CDN=%t；写权限及 HTTP-01 实际验证未验证", b.DeployOSS, enabled)
		return
	}
	var resource Resource
	if state.CertificatePath != "" {
		resource, err = s.Store.LoadCertificate(state)
		if err != nil {
			return fail(err)
		}
	}
	// 未部署完成时优先复用证书，避免因云端旧证书临期而重复签发。
	if !pending && needIssue {
		resource, err = s.prepareCertificate(ctx, b, remote.Domain, &state, force)
		if err != nil {
			return fail(err)
		}
		pending = true
	}
	if !pending {
		result.Action = "证书正常"
		if state.CertificatePath == "" {
			state.NotAfter = remote.NotAfter
			state.Fingerprint = remote.Fingerprint
			if b.DeployOSS {
				state.OSS.Status = "unchanged"
			} else {
				state.OSS.Status = "skipped"
			}
			if enabled {
				state.CDN.Status = "unchanged"
				if state.NotAfter.IsZero() || cdnExpiry.Before(state.NotAfter) {
					state.NotAfter = cdnExpiry
				}
			} else {
				state.CDN.Status = "skipped"
			}
		}
		state.Error = ""
		state.Event = "normal"
		if oldEvent == "failed" && s.Notifier != nil {
			if err := s.Notifier.Send(ctx, "recovered", remote.Domain+"：检查已恢复正常"); err != nil {
				result.Message = "通知失败: " + err.Error()
			}
		}
		if err := s.Store.SaveState(state); err != nil {
			return fail(err)
		}
		return
	}
	if err := s.ensureUpload(ctx, b, remote.Domain, &state, resource); err != nil {
		return fail(err)
	}
	if err := s.deployTargets(ctx, b, remote.Domain, &state, enabled); err != nil {
		return fail(err)
	}
	state.Error = ""
	event := "updated"
	if oldEvent == "failed" {
		event = "recovered"
	}
	state.Event = event
	if err := s.Store.SaveState(state); err != nil {
		return fail(err)
	}
	result.Action = "更新已核验"
	result.Message = fmt.Sprintf("OSS=%s，CDN=%s", state.OSS.Status, state.CDN.Status)
	if s.Notifier != nil {
		if err := s.Notifier.Send(ctx, event, remote.Domain+"："+result.Message); err != nil {
			result.Message += "；通知失败: " + err.Error()
		}
	}
	return
}

func (s *Service) target(ctx context.Context, target *Target, verify func() (bool, error), apply func() error, save func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if target.NextAttempt.After(s.now()) {
		return fmt.Errorf("等待重试至 %s", target.NextAttempt.Format(time.RFC3339))
	}
	if target.Status != "verified" {
		target.Attempts++
	}
	// 即使本地记录成功也核对远端，响应丢失时不重复更新。
	ok, err := verify()
	if err != nil {
		return s.retry(target, err)
	}
	if ok {
		*target = Target{Status: "verified"}
		return save()
	}
	if target.Status == "verified" {
		*target = Target{}
	}
	if target.Status == "failed" && target.Attempts >= 5 {
		target.Attempts = 1
	}
	target.Status = "pending"
	if err := save(); err != nil {
		return err
	}
	if !target.Submitted {
		if err := apply(); err != nil {
			return s.retry(target, err)
		}
		target.Submitted = true
	}
	target.Status = "applied"
	if err := save(); err != nil {
		return err
	}
	ok, err = verify()
	if err != nil {
		return s.retry(target, err)
	}
	if !ok {
		return s.retry(target, &TemporaryError{Err: fmt.Errorf("已提交，等待云端证书生效")})
	}
	*target = Target{Status: "verified"}
	return save()
}

func (s *Service) retry(target *Target, err error) error {
	target.Message = err.Error()
	var temporary *TemporaryError
	if errors.As(err, &temporary) && target.Attempts < 5 {
		target.Status = "retrying"
		delay := min(time.Hour, time.Minute*time.Duration(1<<min(max(0, target.Attempts-1), 6)))
		target.NextAttempt = s.now().Add(delay + time.Duration(rand.Int64N(int64(delay/4))))
	} else {
		target.Status = "failed"
		target.NextAttempt = s.now().Add(s.Config.Interval)
	}
	return err
}
