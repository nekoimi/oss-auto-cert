// Package app 组装用例及其基础设施依赖。
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/nekoimi/oss-auto-cert/internal/certificate"
	"github.com/nekoimi/oss-auto-cert/internal/config"
	"github.com/nekoimi/oss-auto-cert/internal/logging"
	"github.com/nekoimi/oss-auto-cert/internal/notification"
	"github.com/nekoimi/oss-auto-cert/internal/provider/acme"
	"github.com/nekoimi/oss-auto-cert/internal/provider/aliyun"
	"github.com/nekoimi/oss-auto-cert/internal/storage"
)

// Error 包含面向 CLI 的退出码。
type Error struct {
	Code int
	Err  error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// Invalid 将参数和本地运行前提错误标记为退出码 2。
func Invalid(err error) error {
	if err == nil {
		return nil
	}
	return &Error{Code: 2, Err: err}
}

// Options 指定单次执行方式。
type Options struct{ Once, DryRun, Force bool }

// Run 执行巡检或常驻运行；预演不会创建任何状态文件。
func Run(ctx context.Context, c config.Config, o Options, logger *logrus.Logger) (returnErr error) {
	if logger == nil {
		logger = logging.New(os.Stderr)
	}
	if c.ACME.Email == "" {
		return Invalid(fmt.Errorf("请设置 acme.email 或 ACME_EMAIL"))
	}
	cloud, err := aliyun.New(os.Getenv("OSS_ACCESS_KEY_ID"), os.Getenv("OSS_ACCESS_KEY_SECRET"), c.RequestTimeout)
	if err != nil {
		return Invalid(err)
	}
	store := storage.New(c.DataDir, c.ACME.Environment)
	if !o.DryRun {
		unlock, err := store.Lock()
		if err != nil {
			return Invalid(err)
		}
		defer func() {
			if err := unlock(); err != nil {
				returnErr = errors.Join(returnErr, err)
			}
		}()
	}
	service := &certificate.Service{Config: c, Cloud: cloud, Issuer: &acme.Issuer{Config: c, Cloud: cloud, Store: store}, Store: store, Notifier: &notification.Sender{Channels: c.Notifications}, Logger: logger}
	run := func() error {
		started := time.Now()
		logger.WithFields(logrus.Fields{"dry_run": o.DryRun, "force": o.Force || c.Force, "environment": c.ACME.Environment, "buckets": len(c.Buckets)}).Info("开始巡检")
		completed := make(chan []certificate.Result, 1)
		go func() { completed <- service.Run(ctx, o.DryRun, o.Force) }()
		var results []certificate.Result
		select {
		case results = <-completed:
		case <-ctx.Done():
			timer := time.NewTimer(c.ShutdownTimeout)
			defer timer.Stop()
			select {
			case results = <-completed:
			case <-timer.C:
				return &Error{Code: 1, Err: fmt.Errorf("退出等待超时，未完成任务将在下次运行恢复")}
			}
		}
		failed, fatalErr := reportResults(logger, results, ctx.Err() != nil)
		summary := logger.WithFields(logrus.Fields{"total": len(results), "failed": failed, "duration": time.Since(started).Round(time.Millisecond)})
		if failed > 0 {
			summary.Warn("巡检完成，存在失败或待重试任务")
		} else {
			summary.Info("巡检完成")
		}
		if fatalErr != nil {
			return fatalErr
		}
		if failed > 0 && ctx.Err() == nil {
			return &Error{Code: 1, Err: fmt.Errorf("部分任务失败或等待重试，请查看上方处理建议")}
		}
		return nil
	}
	if o.Once || o.DryRun {
		return run()
	}
	next := time.Now().Add(c.Interval)
	if c.RunOnStart {
		if err := run(); err != nil {
			var coded *Error
			if errors.As(err, &coded) && coded.Code == 2 {
				return err
			}
			logger.WithError(err).Warn("本轮任务未全部完成，将继续巡检")
		}
		next = time.Now().Add(c.Interval)
	}
	logger.WithFields(logrus.Fields{"interval": c.Interval, "environment": c.ACME.Environment}).Info("服务已启动")
	lastRun := time.Now()
	for {
		delay := time.Until(next)
		states, err := store.States()
		if err != nil {
			return err
		}
		for _, state := range states {
			if !managed(c, state) {
				continue
			}
			for _, target := range []certificate.Target{state.OSS, state.CDN} {
				if target.Status == "retrying" && target.NextAttempt.After(lastRun) {
					until := time.Until(target.NextAttempt)
					if until < delay {
						delay = until
					}
				}
			}
		}
		if delay < 0 {
			delay = 0
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			logger.Info("收到退出信号，服务已停止")
			return nil
		case <-timer.C:
			if err := run(); err != nil {
				var coded *Error
				if errors.As(err, &coded) && coded.Code == 2 {
					return err
				}
				logger.WithError(err).Warn("本轮任务未全部完成，将继续巡检")
			}
			lastRun = time.Now()
			if !next.After(lastRun) {
				next = lastRun.Add(c.Interval)
			}
		}
	}
}

func reportResults(logger *logrus.Logger, results []certificate.Result, cancelled bool) (int, error) {
	failed := 0
	var fatalErr error
	for _, result := range results {
		entry := logger.WithFields(logrus.Fields{"bucket": result.Bucket, "domain": result.Domain})
		if result.Message != "" {
			entry = entry.WithField("detail", result.Message)
		}
		switch {
		case cancelled && result.Failed && !result.Fatal:
			entry.Warn(result.Action)
		case result.Failed:
			entry.Error(result.Action)
		default:
			entry.Info(result.Action)
		}
		if result.Failed {
			failed++
		}
		if result.Fatal {
			fatalErr = Invalid(fmt.Errorf("无法继续安全写入：%s", result.Message))
		}
	}
	return failed, fatalErr
}

func managed(c config.Config, state certificate.State) bool {
	for _, b := range c.Buckets {
		if b.Name == state.Bucket {
			if b.DomainMode != "listed" {
				return true
			}
			for _, d := range b.Domains {
				if d == state.Domain {
					return true
				}
			}
		}
	}
	return false
}

// Status 输出带时间的本地状态，明确不代表实时云端查询。
func Status(c config.Config, out io.Writer) error {
	states, err := storage.New(c.DataDir, c.ACME.Environment).States()
	if err != nil {
		return Invalid(err)
	}
	count := 0
	for _, state := range states {
		if !managed(c, state) {
			continue
		}
		count++
		fmt.Fprintf(out, "%s / %s：OSS=%s CDN=%s，检查时间=%s，到期=%s，错误=%s\n", state.Bucket, state.Domain, state.OSS.Status, state.CDN.Status, state.UpdatedAt.Format(time.RFC3339), state.NotAfter.Format(time.RFC3339), state.Error)
	}
	if count == 0 {
		fmt.Fprintln(out, "尚无运行快照，请执行 run --once；status 不查询实时云端状态")
	}
	return nil
}
