package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// sendDingTalkWorkNotice 与 sendFeishuMessage 是测试 seam,生产代码会注入真实实现。
// 依赖查询函数也抽成 seam,方便单元测试不依赖真实数据库/root 用户。
var (
	sendDingTalkWorkNotice           = defaultSendDingTalkWorkNotice
	sendFeishuMessage                = defaultSendFeishuMessage
	getRootUserForAdminAlert         = model.GetRootUser
	getDingTalkSettingsForAdminAlert = system_setting.GetDingTalkSettings
	getFeishuSettingsForAdminAlert   = system_setting.GetFeishuSettings
	dispatchUserNotifyForAdminAlert  = dispatchUserNotify
)

func defaultSendDingTalkWorkNotice(ctx context.Context, unionId string, title string, text string) error {
	provider, ok := oauth.GetProvider("dingtalk").(*oauth.DingTalkProvider)
	if !ok {
		return fmt.Errorf("dingtalk provider not registered")
	}
	return provider.SendWorkNotice(ctx, unionId, title, text)
}

func defaultSendFeishuMessage(ctx context.Context, unionId string, title string, text string) error {
	provider, ok := oauth.GetProvider("feishu").(*oauth.FeishuProvider)
	if !ok {
		return fmt.Errorf("feishu provider not registered")
	}
	return provider.SendMessage(ctx, unionId, title, text)
}

// SendAdminAlert 管理员告警统一入口: root 偏好通道 + 钉钉工作通知 + 飞书 IM 并发发送。
// 限流只检查一次(CheckNotificationLimit 检查即计数),IM 通道共享同一配额,不重复计数。
func SendAdminAlert(notifyType string, subject string, content string) {
	user := getRootUserForAdminAlert()
	// model.GetRootUser 找不到 root 时返回零值结构体而不是 nil,两种形态都要挡
	if user == nil || user.Id == 0 {
		common.SysLog("admin alert failed: root user not found")
		return
	}

	canSend, err := CheckNotificationLimit(user.Id, notifyType)
	if err != nil {
		common.SysLog(fmt.Sprintf("admin alert: failed to check notification limit: %s", err.Error()))
		return
	}
	if !canSend {
		common.SysLog(fmt.Sprintf("admin alert skipped: notification limit exceeded for type %s", notifyType))
		return
	}

	data := dto.NewNotify(notifyType, subject, content, nil)
	if err := dispatchUserNotifyForAdminAlert(user.Id, user.Email, user.GetSetting(), data); err != nil {
		common.SysLog(fmt.Sprintf("admin alert: failed to dispatch user notify: %s", err.Error()))
	}

	sendDingTalkAdminAlert(user, subject, content)
	sendFeishuAdminAlert(user, subject, content)
}

func sendDingTalkAdminAlert(user *model.User, subject string, content string) {
	settings := getDingTalkSettingsForAdminAlert()
	if !settings.NotifyEnabled {
		return
	}
	if user.DingTalkId == "" {
		common.SysLog("admin alert: skip dingtalk, root user has no dingtalk binding")
		return
	}
	if settings.AgentId == "" {
		common.SysLog("admin alert: skip dingtalk, agent_id is empty")
		return
	}
	if _, err := strconv.ParseInt(settings.AgentId, 10, 64); err != nil {
		common.SysLog(fmt.Sprintf("admin alert: skip dingtalk, invalid agent_id: %s", err.Error()))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := sendDingTalkWorkNotice(ctx, user.DingTalkId, subject, content); err != nil {
		common.SysLog(fmt.Sprintf("admin alert: failed to send dingtalk work notice: %s", err.Error()))
	}
}

func sendFeishuAdminAlert(user *model.User, subject string, content string) {
	settings := getFeishuSettingsForAdminAlert()
	if !settings.NotifyEnabled {
		return
	}
	if user.FeishuId == "" {
		common.SysLog("admin alert: skip feishu, root user has no feishu binding")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := sendFeishuMessage(ctx, user.FeishuId, subject, content); err != nil {
		common.SysLog(fmt.Sprintf("admin alert: failed to send feishu message: %s", err.Error()))
	}
}

// SendDingTalkTestMessage 向 root 用户发送一条钉钉测试消息,用于验证应用权限/AgentId。
func SendDingTalkTestMessage(ctx context.Context) error {
	settings := getDingTalkSettingsForAdminAlert()
	if !settings.NotifyEnabled {
		return fmt.Errorf("钉钉消息通知未启用")
	}
	if settings.AppKey == "" || settings.AppSecret == "" {
		return fmt.Errorf("请先填写钉钉 AppKey 与 AppSecret")
	}
	if settings.AgentId == "" {
		return fmt.Errorf("请先填写钉钉 AgentId")
	}
	if _, err := strconv.ParseInt(settings.AgentId, 10, 64); err != nil {
		return fmt.Errorf("钉钉 AgentId 必须是纯数字")
	}
	user := getRootUserForAdminAlert()
	if user == nil {
		return fmt.Errorf("未找到 root 用户")
	}
	if user.DingTalkId == "" {
		return fmt.Errorf("root 用户未绑定钉钉账号")
	}
	return sendDingTalkWorkNotice(ctx, user.DingTalkId, "测试消息", "这是一条来自 new-api 套餐监控的测试消息。收到说明钉钉工作通知配置正确。")
}

// SendFeishuTestMessage 向 root 用户发送一条飞书测试消息。
func SendFeishuTestMessage(ctx context.Context) error {
	settings := getFeishuSettingsForAdminAlert()
	if !settings.NotifyEnabled {
		return fmt.Errorf("飞书消息通知未启用")
	}
	if settings.AppId == "" || settings.AppSecret == "" {
		return fmt.Errorf("请先填写飞书 AppId 与 AppSecret")
	}
	user := getRootUserForAdminAlert()
	if user == nil {
		return fmt.Errorf("未找到 root 用户")
	}
	if user.FeishuId == "" {
		return fmt.Errorf("root 用户未绑定飞书账号")
	}
	return sendFeishuMessage(ctx, user.FeishuId, "测试消息", "这是一条来自 new-api 套餐监控的测试消息。收到说明飞书消息通知配置正确。")
}
