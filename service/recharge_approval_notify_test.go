package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFanOutRechargeNotice 通知 fan-out:按 provider 选通道、用快照 unionId、
// designated 审批人按本地绑定兜底、未绑定者跳过、开关关闭不发。
func TestFanOutRechargeNotice(t *testing.T) {
	originalProvider := rechargeActiveOrgProvider
	originalGetUser := getUserByIdForRechargeNotify
	originalDing := getDingTalkSettingsForAdminAlert
	originalFeishu := getFeishuSettingsForAdminAlert
	originalDingSend := sendDingTalkWorkNotice
	originalFeishuSend := sendFeishuMessage
	t.Cleanup(func() {
		rechargeActiveOrgProvider = originalProvider
		getUserByIdForRechargeNotify = originalGetUser
		getDingTalkSettingsForAdminAlert = originalDing
		getFeishuSettingsForAdminAlert = originalFeishu
		sendDingTalkWorkNotice = originalDingSend
		sendFeishuMessage = originalFeishuSend
	})

	var dingSent, feishuSent []string
	sendDingTalkWorkNotice = func(ctx context.Context, unionId string, title string, text string) error {
		dingSent = append(dingSent, unionId)
		return nil
	}
	sendFeishuMessage = func(ctx context.Context, unionId string, title string, text string) error {
		feishuSent = append(feishuSent, unionId)
		return nil
	}
	getDingTalkSettingsForAdminAlert = func() *system_setting.DingTalkSettings {
		return &system_setting.DingTalkSettings{NotifyEnabled: true}
	}
	getFeishuSettingsForAdminAlert = func() *system_setting.FeishuSettings {
		return &system_setting.FeishuSettings{NotifyEnabled: true}
	}
	getUserByIdForRechargeNotify = func(id int) (*model.User, error) {
		if id == 301 {
			return &model.User{Id: 301, DingTalkId: "u-designated"}, nil
		}
		return &model.User{Id: id}, nil // 302 无绑定
	}

	steps := []model.RechargeApprovalStep{
		{RequestId: 1, Level: 1, UserId: 201, UnionId: "u-l1", Decision: model.RechargeStepPending},
		{RequestId: 1, Level: 1, UserId: 301, UnionId: "", Decision: model.RechargeStepPending}, // designated,兜底查绑定
		{RequestId: 1, Level: 1, UserId: 302, UnionId: "", Decision: model.RechargeStepPending}, // 无绑定,跳过
	}

	fanOutRechargeNotice(model.OrgProviderDingTalk, steps, "t", "x")
	assert.Equal(t, []string{"u-l1", "u-designated"}, dingSent)
	assert.Empty(t, feishuSent)

	dingSent = nil
	fanOutRechargeNotice(model.OrgProviderFeishu, steps, "t", "x")
	// 飞书通道:u-l1 快照 unionId 仍发送(快照可能是钉钉 id,但 provider 一致时即飞书 id);
	// designated 兜底取 FeishuId(空)被跳过
	assert.Equal(t, []string{"u-l1"}, feishuSent)

	// 开关关闭:一条都不发
	getDingTalkSettingsForAdminAlert = func() *system_setting.DingTalkSettings {
		return &system_setting.DingTalkSettings{NotifyEnabled: false}
	}
	dingSent = nil
	fanOutRechargeNotice(model.OrgProviderDingTalk, steps, "t", "x")
	assert.Empty(t, dingSent)
}

// rechargeNotifyProvider:无 org sync 时按已启用通知的 IM 登录兜底。
func TestRechargeNotifyProviderFallback(t *testing.T) {
	originalProvider := rechargeActiveOrgProvider
	originalDing := getDingTalkSettingsForAdminAlert
	originalFeishu := getFeishuSettingsForAdminAlert
	t.Cleanup(func() {
		rechargeActiveOrgProvider = originalProvider
		getDingTalkSettingsForAdminAlert = originalDing
		getFeishuSettingsForAdminAlert = originalFeishu
	})

	rechargeActiveOrgProvider = func() string { return "" }
	getDingTalkSettingsForAdminAlert = func() *system_setting.DingTalkSettings {
		return &system_setting.DingTalkSettings{NotifyEnabled: false}
	}
	getFeishuSettingsForAdminAlert = func() *system_setting.FeishuSettings {
		return &system_setting.FeishuSettings{NotifyEnabled: true}
	}
	require.Equal(t, model.OrgProviderFeishu, rechargeNotifyProvider())

	getFeishuSettingsForAdminAlert = func() *system_setting.FeishuSettings {
		return &system_setting.FeishuSettings{NotifyEnabled: false}
	}
	assert.Equal(t, "", rechargeNotifyProvider())

	rechargeActiveOrgProvider = func() string { return model.OrgProviderDingTalk }
	assert.Equal(t, model.OrgProviderDingTalk, rechargeNotifyProvider())
}

// 通知正文里的申请人称呼:优先 display_name 真名,查不到回退 username 快照。
func TestRechargeApplicantName(t *testing.T) {
	originalGetUser := getUserByIdForRechargeNotify
	t.Cleanup(func() {
		getUserByIdForRechargeNotify = originalGetUser
	})

	getUserByIdForRechargeNotify = func(id int) (*model.User, error) {
		if id == 1 {
			return &model.User{Id: 1, DisplayName: "张三", Username: "zhangsan"}, nil
		}
		if id == 2 {
			return &model.User{Id: 2, DisplayName: "", Username: "lisi"}, nil
		}
		return nil, assert.AnError
	}

	req := &model.RechargeRequest{UserId: 1, Username: "zhangsan"}
	assert.Equal(t, "张三", rechargeApplicantName(req))

	req = &model.RechargeRequest{UserId: 2, Username: "lisi"}
	assert.Equal(t, "lisi", rechargeApplicantName(req))

	req = &model.RechargeRequest{UserId: 3, Username: "wangwu"}
	assert.Equal(t, "wangwu", rechargeApplicantName(req))
}
