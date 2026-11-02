// Package notification 隔离通知错误，使用安全的 JSON 内置消息格式。
package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	htmltemplate "html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"text/template"
	"time"

	"github.com/nekoimi/oss-auto-cert/internal/config"
)

// Sender 聚合多个独立通知渠道。
type Sender struct {
	Channels []config.Notification
	Client   *http.Client
}

// Render 渲染消息；旧模板保留 HTML 模板语义，新模板提供 json 函数。
func Render(n config.Notification, message string) ([]byte, error) {
	data := struct{ Message string }{message}
	var payload []byte
	switch n.Type {
	case "wecom", "dingtalk":
		payload, _ = json.Marshal(map[string]any{"msgtype": "text", "text": map[string]string{"content": message}})
	case "feishu":
		payload, _ = json.Marshal(map[string]any{"msg_type": "text", "content": map[string]string{"text": message}})
	case "template", "legacy-template":
		var b bytes.Buffer
		if n.Type == "legacy-template" {
			t, err := htmltemplate.New("notification").Parse(n.Template)
			if err != nil {
				return nil, fmt.Errorf("旧通知模板语法错误")
			}
			if err := t.Execute(&b, data); err != nil {
				return nil, fmt.Errorf("旧通知模板执行失败")
			}
		} else {
			t, err := template.New("notification").Funcs(template.FuncMap{"json": func(v any) (string, error) { raw, err := json.Marshal(v); return string(raw), err }}).Option("missingkey=error").Parse(n.Template)
			if err != nil {
				return nil, fmt.Errorf("通知模板语法错误")
			}
			if err := t.Execute(&b, data); err != nil {
				return nil, fmt.Errorf("通知模板执行失败")
			}
		}
		payload = b.Bytes()
	default:
		return nil, fmt.Errorf("未知通知类型")
	}
	if !json.Valid(payload) {
		return nil, fmt.Errorf("通知模板生成无效 JSON；请使用 {{ json .Message }} 并去掉其外围引号")
	}
	return payload, nil
}

// ValidateTemplates 在不访问网络的情况下检查模板与消息边界。
func ValidateTemplates(channels []config.Notification) []error {
	var result []error
	for index, n := range channels {
		if _, err := Render(n, "测试消息\n包含\"引号\"、反斜杠\\和<中文>"); err != nil {
			result = append(result, fmt.Errorf("通知渠道 %d: %w", index+1, err))
		}
	}
	return result
}

// Send 发送事件，错误不会造成 panic，也不记录包含令牌的 URL。
func (s *Sender) Send(ctx context.Context, event, message string) error {
	var failures []error
	for index, n := range s.Channels {
		if len(n.Events) > 0 && !slices.Contains(n.Events, event) {
			continue
		}
		raw, err := Render(n, message)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		address := n.URL
		if n.URLEnv != "" {
			address = os.Getenv(n.URLEnv)
		}
		if err := config.ValidateURL(address); err != nil {
			failures = append(failures, fmt.Errorf("通知渠道 %d 地址缺失或无效", index+1))
			continue
		}
		var sendErr error
		for attempt := 0; attempt < 3; attempt++ {
			sendErr = s.send(ctx, n, address, raw)
			if sendErr == nil {
				break
			}
			if ctx.Err() != nil {
				break
			}
			if attempt < 2 {
				timer := time.NewTimer(time.Duration(attempt+1) * time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
				case <-timer.C:
				}
			}
		}
		if sendErr != nil {
			failures = append(failures, fmt.Errorf("通知渠道 %d: %w", index+1, sendErr))
		}
	}
	return errors.Join(failures...)
}
func (s *Sender) send(ctx context.Context, n config.Notification, address string, raw []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("创建通知请求失败")
	}
	req.Header.Set("Content-Type", "application/json")
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	response, err := client.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) && ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("通知网络请求失败")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("读取通知响应失败")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("通知 HTTP 状态 %d", response.StatusCode)
	}
	if n.Type == "wecom" || n.Type == "dingtalk" || n.Type == "feishu" {
		var result struct {
			ErrCode    *int `json:"errcode"`
			Code       *int `json:"code"`
			StatusCode *int `json:"StatusCode"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return fmt.Errorf("通知平台返回非 JSON 响应")
		}
		code := result.ErrCode
		if n.Type == "feishu" {
			code = result.Code
			if code == nil {
				code = result.StatusCode
			}
		}
		if code == nil {
			return fmt.Errorf("通知响应缺少业务状态")
		}
		if *code != 0 {
			return fmt.Errorf("通知平台返回业务错误 %d", *code)
		}
	}
	return nil
}
