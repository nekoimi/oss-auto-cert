package wxwork

import (
	"fmt"
	"github.com/charmbracelet/log"
	"io"
	"net/http"
	"strings"
)

type Notify struct {
	webhook string
}

func New(webhook string) *Notify {
	return &Notify{
		webhook: webhook,
	}
}

func (w *Notify) Notify(message string) {
	go func() {
		// 发送文本消息
		txt := `{
	"msgtype": "text",
	"text": {
		"content": "%s"
	}
}`
		data := strings.NewReader(fmt.Sprintf(txt, message))
		req, _ := http.NewRequest("POST", w.webhook, data)
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
