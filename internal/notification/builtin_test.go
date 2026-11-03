package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nekoimi/oss-auto-cert/internal/certificate"
	"github.com/nekoimi/oss-auto-cert/internal/config"
)

func TestBuiltinEventMessages(t *testing.T) {
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	for _, platform := range []string{"wecom", "dingtalk", "feishu"} {
		for _, event := range []struct{ kind, title, color string }{
			{"updated", "证书更新成功", "green"},
			{"failed", "证书处理失败", "red"},
			{"recovered", "证书任务已恢复", "turquoise"},
		} {
			t.Run(platform+"/"+event.kind, func(t *testing.T) {
				message := "模拟失败：\"引号\"\\反斜杠\n[链接](https://example.com)<at id=all>所有人</at>"
				raw, err := Render(config.Notification{Type: platform}, certificate.Event{
					Type: event.kind, Bucket: "my-bucket", Domain: "static.example.com", Environment: "staging",
					Message: message, OSSStatus: "verified", CDNStatus: "retrying", OccurredAt: now, NotAfter: now.Add(90 * 24 * time.Hour),
				})
				if err != nil {
					t.Fatal(err)
				}
				var payload map[string]any
				if err := json.Unmarshal(raw, &payload); err != nil {
					t.Fatal(err)
				}
				var body string
				if platform == "feishu" {
					if payload["msg_type"] != "interactive" {
						t.Fatalf("飞书未使用卡片: %s", raw)
					}
					card := payload["card"].(map[string]any)
					header := card["header"].(map[string]any)
					if header["template"] != event.color || !strings.Contains(header["title"].(map[string]any)["content"].(string), event.title) {
						t.Fatalf("卡片状态标题或颜色错误: %s", raw)
					}
					elements := card["elements"].([]any)
					for _, item := range elements[0].(map[string]any)["fields"].([]any) {
						text := item.(map[string]any)["text"].(map[string]any)
						if text["tag"] != "plain_text" {
							t.Fatal("动态字段必须使用纯文本")
						}
						body += text["content"].(string) + "\n"
					}
					detail := elements[2].(map[string]any)["text"].(map[string]any)
					if detail["tag"] != "plain_text" || detail["content"] != "处理说明\n"+message {
						t.Fatal("飞书处理说明未保留纯文本")
					}
				} else {
					if payload["msgtype"] != "markdown" {
						t.Fatalf("未使用 Markdown: %s", raw)
					}
					markdown := payload["markdown"].(map[string]any)
					key := "content"
					if platform == "dingtalk" {
						key = "text"
						if !strings.Contains(markdown["title"].(string), event.title) {
							t.Fatal("钉钉缺少事件标题")
						}
					}
					body = markdown[key].(string)
					if !strings.Contains(body, event.title) || strings.Contains(body, "<at") || strings.Contains(body, "[链接](") {
						t.Fatalf("Markdown 标题或动态内容转义错误: %s", body)
					}
				}
				for _, value := range []string{"my-bucket", "static.example.com", "测试环境", "已核验", "等待重试", "2026-11-03 10:00:00 UTC", "2027-02-01 10:00:00 UTC"} {
					if !strings.Contains(body, value) {
						t.Errorf("缺少通知字段 %q: %s", value, body)
					}
				}
			})
		}
	}
}

func TestCustomTemplateMessageCompatibility(t *testing.T) {
	message := "static.example.com：检查已恢复正常"
	for _, kind := range []string{"template", "legacy-template"} {
		raw, err := Render(config.Notification{Type: kind, Template: `{"text":"{{ .Message }}"}`}, certificate.Event{
			Type: "recovered", Bucket: "my-bucket", Domain: "static.example.com", Message: message,
		})
		if err != nil {
			t.Fatal(err)
		}
		var data struct{ Text string }
		if err := json.Unmarshal(raw, &data); err != nil {
			t.Fatal(err)
		}
		if data.Text != message {
			t.Fatalf("%s 改变了旧消息正文: %q", kind, data.Text)
		}
	}
}

func TestBuiltinMissingDataAndEventFiltering(t *testing.T) {
	for _, kind := range []string{"wecom", "dingtalk", "feishu"} {
		if _, err := Render(config.Notification{Type: kind}, certificate.Event{Type: "invalid"}); err == nil {
			t.Fatalf("%s 未拒绝未知事件", kind)
		}
	}
	raw, err := Render(config.Notification{Type: "wecom"}, certificate.Event{Type: "failed"})
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Markdown struct{ Content string }
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(data.Markdown.Content, "未知") || !strings.Contains(data.Markdown.Content, "未检查") || strings.Contains(data.Markdown.Content, "0001-") {
		t.Fatalf("缺失数据被误报: %s", raw)
	}
	var received []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct{ Text string }
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		received = append(received, payload.Text)
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	sender := Sender{Channels: []config.Notification{
		{Type: "wecom", URL: "invalid-url", Events: []string{"failed"}},
		{Type: "template", URL: server.URL, Template: `{"text":{{ json .Message }}}`, Events: []string{"updated"}},
	}}
	if err := sender.Send(context.Background(), certificate.Event{Type: "updated", Message: "原始正文"}); err != nil {
		t.Fatal(err)
	}
	if len(received) != 1 || received[0] != "原始正文" {
		t.Fatalf("事件筛选或模板正文错误: %v", received)
	}
}
