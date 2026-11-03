package notification

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/nekoimi/oss-auto-cert/internal/certificate"
)

type field struct{ name, value string }

// renderBuiltin 按平台生成固定格式，动态内容只作为文本填入。
func renderBuiltin(kind string, event certificate.Event) ([]byte, error) {
	var title, color string
	switch event.Type {
	case "updated":
		title, color = "证书更新成功", "green"
	case "failed":
		title, color = "证书处理失败", "red"
	case "recovered":
		title, color = "证书任务已恢复", "turquoise"
	default:
		return nil, fmt.Errorf("未知通知事件")
	}
	title = "oss-auto-cert · " + title
	fields := []field{
		{"域名", known(event.Domain)},
		{"Bucket", known(event.Bucket)},
		{"环境", environment(event.Environment)},
		{"证书到期", timestamp(event.NotAfter)},
		{"OSS 部署", targetStatus(event.OSSStatus)},
		{"CDN 部署", targetStatus(event.CDNStatus)},
		{"通知时间", timestamp(event.OccurredAt)},
	}
	if kind == "feishu" {
		// plain_text 防止错误内容中的 Markdown 或 <at> 被解释成卡片指令。
		items := make([]map[string]any, 0, len(fields))
		for _, item := range fields {
			items = append(items, map[string]any{
				"is_short": true,
				"text":     map[string]string{"tag": "plain_text", "content": item.name + "\n" + item.value},
			})
		}
		return json.Marshal(map[string]any{
			"msg_type": "interactive",
			"card": map[string]any{
				"config": map[string]bool{"wide_screen_mode": true},
				"header": map[string]any{"template": color, "title": map[string]string{"tag": "plain_text", "content": title}},
				"elements": []map[string]any{
					{"tag": "div", "fields": items},
					{"tag": "hr"},
					{"tag": "div", "text": map[string]string{"tag": "plain_text", "content": "处理说明\n" + event.Message}},
				},
			},
		})
	}
	var text strings.Builder
	fmt.Fprintf(&text, "### %s\n", title)
	for _, item := range fields {
		fmt.Fprintf(&text, "\n**%s：** %s\n", item.name, escapeMarkdown(item.value))
	}
	text.WriteString("\n**处理说明：**\n> ")
	text.WriteString(strings.ReplaceAll(escapeMarkdown(event.Message), "\n", "\n> "))
	if kind == "dingtalk" {
		return json.Marshal(map[string]any{"msgtype": "markdown", "markdown": map[string]string{"title": title, "text": text.String()}})
	}
	return json.Marshal(map[string]any{"msgtype": "markdown", "markdown": map[string]string{"content": text.String()}})
}

func escapeMarkdown(value string) string {
	value = strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n")
	value = html.EscapeString(value)
	return strings.NewReplacer(
		"\\", "\\\\", "`", "\\`", "*", "\\*", "_", "\\_",
		"[", "\\[", "]", "\\]", "(", "\\(", ")", "\\)", "#", "\\#",
	).Replace(value)
}

func known(value string) string {
	if value == "" {
		return "未知"
	}
	return value
}

func timestamp(value time.Time) string {
	if value.IsZero() {
		return "未知"
	}
	return value.UTC().Format("2006-01-02 15:04:05 MST")
}

func environment(value string) string {
	switch value {
	case "production":
		return "生产环境"
	case "staging":
		return "测试环境（staging）"
	default:
		return "未知"
	}
}

func targetStatus(value string) string {
	switch value {
	case "verified":
		return "已核验"
	case "unchanged":
		return "无需更新"
	case "skipped":
		return "已跳过"
	case "pending":
		return "待部署"
	case "applied":
		return "已提交，等待生效"
	case "retrying":
		return "等待重试"
	case "failed":
		return "失败"
	default:
		return "未检查"
	}
}
