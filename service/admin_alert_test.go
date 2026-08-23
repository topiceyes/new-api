package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// alertChannelCalls 记录一次 SendAdminAlert 对各通道的调用情况。
type alertChannelCalls struct {
	dispatch int
	dingtalk int
	feishu   int
}

func resetAdminAlertSeams(t *testing.T) {
	t.Helper()
	originalLimitCount := constant.NotifyLimitCount
	originalLimitDuration := constant.NotificationLimitDurationMinute
	constant.NotifyLimitCount = 2
	constant.NotificationLimitDurationMinute = 10
	originals := map[string]any{
		"root":       getRootUserForAdminAlert,
		"dingtalk":   getDingTalkSettingsForAdminAlert,
		"feishu":     getFeishuSettingsForAdminAlert,
		"dispatch":   dispatchUserNotifyForAdminAlert,
		"dingSend":   sendDingTalkWorkNotice,
		"feishuSend": sendFeishuMessage,
	}
	t.Cleanup(func() {
		constant.NotifyLimitCount = originalLimitCount
		constant.NotificationLimitDurationMinute = originalLimitDuration
		getRootUserForAdminAlert = originals["root"].(func() *model.User)
		getDingTalkSettingsForAdminAlert = originals["dingtalk"].(func() *system_setting.DingTalkSettings)
		getFeishuSettingsForAdminAlert = originals["feishu"].(func() *system_setting.FeishuSettings)
		dispatchUserNotifyForAdminAlert = originals["dispatch"].(func(int, string, dto.UserSetting, dto.Notify) error)
		sendDingTalkWorkNotice = originals["dingSend"].(func(context.Context, string, string, string) error)
		sendFeishuMessage = originals["feishuSend"].(func(context.Context, string, string, string) error)
	})
}

func clearNotificationLimit(t *testing.T, userId int, notifyType string) {
	t.Helper()
	key := fmt.Sprintf("%d:%s:%s", userId, notifyType, time.Now().Format("2006010215"))
	notifyLimitStore.Delete(key)
}

func stubAdminAlertChannels(t *testing.T) *alertChannelCalls {
	t.Helper()
	calls := &alertChannelCalls{}
	dispatchUserNotifyForAdminAlert = func(userId int, userEmail string, userSetting dto.UserSetting, data dto.Notify) error {
		calls.dispatch++
		return nil
	}
	sendDingTalkWorkNotice = func(ctx context.Context, unionId, title, text string) error {
		calls.dingtalk++
		return nil
	}
	sendFeishuMessage = func(ctx context.Context, unionId, title, text string) error {
		calls.feishu++
		return nil
	}
	return calls
}

func rootUserWithBindings() *model.User {
	return &model.User{
		Id:         1,
		Username:   "root",
		Email:      "root@example.com",
		DingTalkId: "ding-root-union",
		FeishuId:   "feishu-root-union",
	}
}

func enabledDingTalkSettings() *system_setting.DingTalkSettings {
	return &system_setting.DingTalkSettings{
		NotifyEnabled: true,
		AppKey:        "test-app-key",
		AppSecret:     "test-app-secret",
		AgentId:       "123456789",
	}
}

func enabledFeishuSettings() *system_setting.FeishuSettings {
	return &system_setting.FeishuSettings{
		NotifyEnabled: true,
		AppId:         "test-app-id",
		AppSecret:     "test-app-secret",
	}
}

func TestSendAdminAlert_AllChannelsFiredWithinLimit(t *testing.T) {
	resetAdminAlertSeams(t)
	getRootUserForAdminAlert = rootUserWithBindings
	getDingTalkSettingsForAdminAlert = enabledDingTalkSettings
	getFeishuSettingsForAdminAlert = enabledFeishuSettings
	calls := stubAdminAlertChannels(t)

	notifyType := "admin_alert_all_channels"
	clearNotificationLimit(t, 1, notifyType)

	SendAdminAlert(notifyType, "subject", "content")

	assert.Equal(t, 1, calls.dispatch, "root 偏好通道应被调用")
	assert.Equal(t, 1, calls.dingtalk, "钉钉通道应被调用")
	assert.Equal(t, 1, calls.feishu, "飞书通道应被调用")
}

func TestSendAdminAlert_LimitSilencesAll(t *testing.T) {
	resetAdminAlertSeams(t)
	getRootUserForAdminAlert = rootUserWithBindings
	getDingTalkSettingsForAdminAlert = enabledDingTalkSettings
	getFeishuSettingsForAdminAlert = enabledFeishuSettings
	calls := stubAdminAlertChannels(t)

	notifyType := "admin_alert_limit"
	clearNotificationLimit(t, 1, notifyType)

	SendAdminAlert(notifyType, "subject", "content")
	SendAdminAlert(notifyType, "subject", "content")
	SendAdminAlert(notifyType, "subject", "content")

	// 默认限额 2 条/窗口,前两次应放行,第三次应被限流。
	assert.Equal(t, 2*1, calls.dispatch, "限额内只发两次")
	assert.Equal(t, 2*1, calls.dingtalk)
	assert.Equal(t, 2*1, calls.feishu)
}

func TestSendAdminAlert_SkipsIMWhenNotBound(t *testing.T) {
	resetAdminAlertSeams(t)
	getRootUserForAdminAlert = func() *model.User {
		u := rootUserWithBindings()
		u.DingTalkId = ""
		u.FeishuId = ""
		return u
	}
	getDingTalkSettingsForAdminAlert = enabledDingTalkSettings
	getFeishuSettingsForAdminAlert = enabledFeishuSettings
	calls := stubAdminAlertChannels(t)

	notifyType := "admin_alert_no_binding"
	clearNotificationLimit(t, 1, notifyType)

	SendAdminAlert(notifyType, "subject", "content")

	assert.Equal(t, 1, calls.dispatch, "root 偏好通道仍应被调用")
	assert.Equal(t, 0, calls.dingtalk, "未绑定钉钉时不应调用")
	assert.Equal(t, 0, calls.feishu, "未绑定飞书时不应调用")
}

func TestSendAdminAlert_SkipsIMWhenDisabled(t *testing.T) {
	resetAdminAlertSeams(t)
	getRootUserForAdminAlert = rootUserWithBindings
	getDingTalkSettingsForAdminAlert = func() *system_setting.DingTalkSettings {
		s := enabledDingTalkSettings()
		s.NotifyEnabled = false
		return s
	}
	getFeishuSettingsForAdminAlert = func() *system_setting.FeishuSettings {
		s := enabledFeishuSettings()
		s.NotifyEnabled = false
		return s
	}
	calls := stubAdminAlertChannels(t)

	notifyType := "admin_alert_disabled"
	clearNotificationLimit(t, 1, notifyType)

	SendAdminAlert(notifyType, "subject", "content")

	assert.Equal(t, 1, calls.dispatch)
	assert.Equal(t, 0, calls.dingtalk)
	assert.Equal(t, 0, calls.feishu)
}

func TestSendAdminAlert_SkipsDingTalkWhenAgentIdInvalid(t *testing.T) {
	resetAdminAlertSeams(t)
	getRootUserForAdminAlert = rootUserWithBindings
	getDingTalkSettingsForAdminAlert = func() *system_setting.DingTalkSettings {
		s := enabledDingTalkSettings()
		s.AgentId = "not-a-number"
		return s
	}
	getFeishuSettingsForAdminAlert = enabledFeishuSettings
	calls := stubAdminAlertChannels(t)

	notifyType := "admin_alert_invalid_agent"
	clearNotificationLimit(t, 1, notifyType)

	SendAdminAlert(notifyType, "subject", "content")

	assert.Equal(t, 1, calls.dispatch)
	assert.Equal(t, 0, calls.dingtalk, "AgentId 非法时不应调用钉钉")
	assert.Equal(t, 1, calls.feishu, "飞书应正常调用")
}

func TestSendAdminAlert_NoRootUser(t *testing.T) {
	resetAdminAlertSeams(t)
	// 生产上 model.GetRootUser 找不到 root 时返回零值结构体(Id=0),不是 nil
	getRootUserForAdminAlert = func() *model.User { return &model.User{} }
	calls := stubAdminAlertChannels(t)

	SendAdminAlert("admin_alert_no_root", "subject", "content")

	assert.Equal(t, 0, calls.dispatch)
	assert.Equal(t, 0, calls.dingtalk)
	assert.Equal(t, 0, calls.feishu)
}

func TestSendDingTalkTestMessage_ValidatesBinding(t *testing.T) {
	resetAdminAlertSeams(t)
	getRootUserForAdminAlert = func() *model.User {
		u := rootUserWithBindings()
		u.DingTalkId = ""
		return u
	}
	getDingTalkSettingsForAdminAlert = enabledDingTalkSettings

	err := SendDingTalkTestMessage(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "未绑定")
}

func TestSendFeishuTestMessage_ValidatesBinding(t *testing.T) {
	resetAdminAlertSeams(t)
	getRootUserForAdminAlert = func() *model.User {
		u := rootUserWithBindings()
		u.FeishuId = ""
		return u
	}
	getFeishuSettingsForAdminAlert = enabledFeishuSettings

	err := SendFeishuTestMessage(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "未绑定")
}
