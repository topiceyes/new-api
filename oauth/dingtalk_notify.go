package oauth

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// GetUserIdByUnionId 通过 unionId 查询企业内部 userid。
// 用户存在时返回 (userid, true, nil); 已离职/不存在时返回 ("", false, nil);
// 其他错误返回 ("", false, err)。
func (p *DingTalkProvider) GetUserIdByUnionId(ctx context.Context, unionId string) (string, bool, error) {
	if unionId == "" {
		return "", false, fmt.Errorf("empty unionId")
	}
	appToken, err := p.GetAppAccessToken(ctx)
	if err != nil {
		return "", false, err
	}

	bodyBytes, err := common.Marshal(map[string]string{"unionid": unionId})
	if err != nil {
		return "", false, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://oapi.dingtalk.com/topapi/user/getbyunionid?access_token="+url.QueryEscape(appToken),
		bytes.NewReader(bodyBytes))
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", false, err
	}
	defer res.Body.Close()

	var resp struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		Result  struct {
			UserID string `json:"userid"`
		} `json:"result"`
	}
	if err = common.DecodeJson(res.Body, &resp); err != nil {
		return "", false, err
	}
	switch resp.ErrCode {
	case 0:
		return resp.Result.UserID, true, nil
	case dingTalkErrUserNotFound:
		return "", false, nil
	default:
		return "", false, fmt.Errorf("dingtalk getbyunionid errcode=%d errmsg=%s", resp.ErrCode, resp.ErrMsg)
	}
}

// SendWorkNotice 给指定 unionId 的企业内部用户发送钉钉工作通知(asyncsend_v2,text 类型)。
func (p *DingTalkProvider) SendWorkNotice(ctx context.Context, unionId string, title string, text string) error {
	if unionId == "" {
		return fmt.Errorf("empty unionId")
	}
	settings := system_setting.GetDingTalkSettings()
	if !settings.NotifyEnabled {
		return fmt.Errorf("dingtalk notify is not enabled")
	}
	if settings.AgentId == "" {
		return fmt.Errorf("dingtalk agent_id is not configured")
	}
	agentId, err := strconv.ParseInt(settings.AgentId, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid dingtalk agent_id: %w", err)
	}

	userId, active, err := p.GetUserIdByUnionId(ctx, unionId)
	if err != nil {
		return fmt.Errorf("failed to resolve dingtalk userid: %w", err)
	}
	if !active {
		return fmt.Errorf("dingtalk user not in organization: %s", unionId)
	}
	if userId == "" {
		return fmt.Errorf("dingtalk returned empty userid for unionId: %s", unionId)
	}

	appToken, err := p.GetAppAccessToken(ctx)
	if err != nil {
		return err
	}

	payload := map[string]interface{}{
		"agent_id":    agentId,
		"userid_list": userId,
		"msg": map[string]interface{}{
			"msgtype": "text",
			"text": map[string]string{
				"content": title + "\n" + text,
			},
		},
	}
	bodyBytes, err := common.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://oapi.dingtalk.com/topapi/message/corpconversation/asyncsend_v2?access_token="+url.QueryEscape(appToken),
		bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	var resp struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		TaskId  int64  `json:"task_id"`
	}
	if err = common.DecodeJson(res.Body, &resp); err != nil {
		return err
	}
	if resp.ErrCode != 0 {
		return fmt.Errorf("dingtalk asyncsend_v2 errcode=%d errmsg=%s", resp.ErrCode, resp.ErrMsg)
	}
	common.SysLog(fmt.Sprintf("dingtalk work notice sent: task_id=%d", resp.TaskId))
	return nil
}
