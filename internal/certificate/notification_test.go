package certificate_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nekoimi/oss-auto-cert/internal/certificate"
)

// fixture 使用单域名单 worker；Run 返回前通知已发送完毕。
type recordingNotifier struct{ events []certificate.Event }

func (n *recordingNotifier) Send(_ context.Context, event certificate.Event) error {
	n.events = append(n.events, event)
	return nil
}

func TestNotificationFailureDeduplicationAndRecovery(t *testing.T) {
	s, cloud, issuer, store, now := fixture(t)
	notifier := &recordingNotifier{}
	s.Notifier = notifier
	cloud.failCDN = true
	if results := s.Run(context.Background(), true, false); results[0].Failed || len(notifier.events) != 0 {
		t.Fatalf("预演不应发送通知: %+v", results)
	}
	if results := s.Run(context.Background(), false, false); !results[0].Failed || len(notifier.events) != 1 {
		t.Fatalf("失败事件未发送: %+v %+v", results, notifier.events)
	}
	failed := notifier.events[0]
	_, expiry, err := certificate.Inspect(issuer.resource, "static.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if failed.Type != "failed" || failed.Bucket != "my-bucket" || failed.Domain != "static.example.com" || failed.Environment != "production" || !failed.NotAfter.Equal(expiry) || !failed.OccurredAt.Equal(*now) {
		t.Fatalf("失败通知缺少事件数据: %+v", failed)
	}
	if failed.OSSStatus != "verified" || failed.CDNStatus != "retrying" || !strings.Contains(failed.Message, "模拟超时") {
		t.Fatalf("部分部署失败状态错误: %+v", failed)
	}
	s.Run(context.Background(), false, false)
	if len(notifier.events) != 1 {
		t.Fatal("连续失败重复通知")
	}
	state, err := store.LoadState("my-bucket", "static.example.com")
	if err != nil {
		t.Fatal(err)
	}
	cloud.failCDN = false
	cloud.remote[0].NotAfter = expiry
	*now = state.CDN.NextAttempt.Add(time.Second)
	if results := s.Run(context.Background(), false, false); results[0].Failed || len(notifier.events) != 2 {
		t.Fatalf("恢复事件未发送: %+v %+v", results, notifier.events)
	}
	recovered := notifier.events[1]
	if recovered.Type != "recovered" || recovered.OSSStatus != "verified" || recovered.CDNStatus != "verified" {
		t.Fatalf("恢复后的目标状态错误: %+v", recovered)
	}
	s.Run(context.Background(), false, false)
	if len(notifier.events) != 2 {
		t.Fatal("正常巡检不应重复发送更新通知")
	}
}

func TestNotificationUpdatedAndRecoveryWithoutIssuance(t *testing.T) {
	t.Run("更新成功", func(t *testing.T) {
		s, _, _, _, _ := fixture(t)
		s.Config.Buckets[0].CDN = "off"
		notifier := &recordingNotifier{}
		s.Notifier = notifier
		if results := s.Run(context.Background(), false, false); results[0].Failed || len(notifier.events) != 1 {
			t.Fatalf("更新失败: %+v", results)
		}
		event := notifier.events[0]
		if event.Type != "updated" || event.OSSStatus != "verified" || event.CDNStatus != "skipped" || event.NotAfter.IsZero() {
			t.Fatalf("更新事件数据错误: %+v", event)
		}
	})
	t.Run("检查恢复", func(t *testing.T) {
		s, cloud, issuer, _, now := fixture(t)
		notifier := &recordingNotifier{}
		s.Notifier = notifier
		cloud.remote[0].HasCertificate = false
		s.Run(context.Background(), false, false)
		cloud.remote[0].HasCertificate = true
		cloud.remote[0].NotAfter = now.Add(90 * 24 * time.Hour)
		if results := s.Run(context.Background(), false, false); results[0].Failed || len(notifier.events) != 2 {
			t.Fatalf("检查恢复失败: %+v %+v", results, notifier.events)
		}
		event := notifier.events[1]
		if event.Type != "recovered" || event.OSSStatus != "unchanged" || !event.NotAfter.Equal(cloud.remote[0].NotAfter) || issuer.calls.Load() != 0 {
			t.Fatalf("未签发时恢复事件数据错误: %+v", event)
		}
		if event.Message != "static.example.com：检查已恢复正常" {
			t.Fatalf("旧模板正文被改变: %q", event.Message)
		}
	})
}
