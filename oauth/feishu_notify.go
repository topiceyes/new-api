package oauth

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// SendMessage 给指定 union_id 的飞书用户发送一条文本消息。
func (p *FeishuProvider) SendMessage(ctx context.Context, unionId string, title string, text string) error {
	if unionId == "" {
		return fmt.Errorf("empty unionId")
	}
	settings := system_setting.GetFeishuSettings()
	if !settings.NotifyEnabled {
		return fmt.Errorf("feishu notify is not enabled")
	}

	tenantToken, err := p.GetTenantAccessToken(ctx)
	if err != nil {
		return err
	}

	contentBytes, err := common.Marshal(map[string]string{"text": title + "\n" + text})
	if err != nil {
		return err
	}

	payload := map[string]string{
		"receive_id": unionId,
		"msg_type":   "text",
		"content":    string(contentBytes),
	}
	bodyBytes, err := common.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=union_id",
		bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tenantToken)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	client := http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			MessageId string `json:"message_id"`
		} `json:"data"`
	}
	if err = common.DecodeJson(res.Body, &resp); err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("feishu send message code=%d msg=%s", resp.Code, resp.Msg)
	}
	common.SysLog(fmt.Sprintf("feishu message sent: message_id=%s", resp.Data.MessageId))
	return nil
}
