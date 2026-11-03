package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nekoimi/oss-auto-cert/internal/certificate"
	"github.com/nekoimi/oss-auto-cert/internal/config"
)

func TestJSONEscapingAndTemplate(t *testing.T) {
	message := "Bucket<中文>\n含\"引号\"和\\反斜杠"
	for _, kind := range []string{"wecom", "dingtalk", "feishu", "template"} {
		raw, err := Render(config.Notification{Type: kind, Template: `{"text":{{ json .Message }}}`}, certificate.Event{Type: "failed", Message: message})
		if err != nil || !json.Valid(raw) {
			t.Fatalf("%s: %v %s", kind, err, raw)
		}
	}
	if _, err := Render(config.Notification{Type: "legacy-template", Template: `{"text":"{{ .Message }}"}`}, certificate.Event{Type: "failed", Message: message}); err == nil {
		t.Fatal("旧模板无效 JSON 应明确失败")
	}
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("transport failure")
}
func TestTransportFailureAndBodyClose(t *testing.T) {
	sender := &Sender{Channels: []config.Notification{{Type: "wecom", URL: "http://local.invalid"}}, Client: &http.Client{Transport: failingTransport{}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sender.Send(ctx, certificate.Event{Type: "failed", Message: "message"}); err == nil {
		t.Fatal("请求失败应返回错误，不访问空响应")
	}
	var captured string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if err := r.Body.Close(); err != nil {
			t.Error(err)
		}
		captured = string(raw)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"errcode":0}`)
	}))
	defer server.Close()
	sender = &Sender{Channels: []config.Notification{{Type: "wecom", URL: server.URL}}}
	if err := sender.Send(context.Background(), certificate.Event{Type: "updated", Message: "hello\nworld"}); err != nil {
		t.Fatal(err)
	}
	if !json.Valid([]byte(captured)) || !strings.Contains(captured, `\n`) {
		t.Fatalf("消息转义错误: %s", captured)
	}
}
