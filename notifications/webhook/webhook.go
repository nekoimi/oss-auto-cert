package webhook

import (
	"bytes"
	"github.com/charmbracelet/log"
	"html/template"
	"io"
	"net/http"
	"strings"
)

const DefaultWxWorkTpl = `
	{
		"msgtype": "text",
		"text": {
			"content": "{{ .Message }}"
		}
	}
`

type Notify struct {
	webhook string
	tpl     *template.Template
}

type TplData struct {
	Message string
}

func New(webhook string, webhookTpl string) *Notify {
	if webhookTpl == "" {
		webhookTpl = DefaultWxWorkTpl
	}
	tpl, err := template.New("webhook").Parse(webhookTpl)
	if err != nil {
		log.Fatalf("创建Webhook渲染模版异常: %s", err.Error())
	}
	return &Notify{
		webhook: webhook,
		tpl:     tpl,
	}
}

func (n *Notify) Notify(message string) {
	go func() {
		// 发送文本消息
		data := TplData{
			Message: message,
		}

		var buf bytes.Buffer
		err := n.tpl.Execute(&buf, data)
		if err != nil {
			log.Errorf("渲染Webhook消息异常: %s", err.Error())
			return
		}

		body := strings.NewReader(buf.String())
		req, _ := http.NewRequest("POST", n.webhook, body)
		req.Header.Add("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Errorf(err.Error())
		}
		if resp.StatusCode != 200 {
			raw, _ := io.ReadAll(resp.Body)
			log.Errorf(string(raw))
		}
	}()
}
