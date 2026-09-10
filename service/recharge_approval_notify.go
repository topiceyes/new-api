package service

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

// 充值审批的 IM 通知:按当前组织同步 provider 走钉钉工作通知或飞书 IM。
// 全部 fire-and-forget:发送失败只记日志,不影响申请/审批主流程。
// 发送函数复用 admin_alert.go 的 seam(defaultSendDingTalkWorkNotice 等),
// 测试替换同一组 seam 即可同时覆盖两处。

var getUserByIdForRechargeNotify = func(id int) (*model.User, error) {
	return model.GetUserById(id, false)
}

// rechargeNotifyUnionId 取通知目标的 IM unionId:优先用审批步骤里快照的
// unionId(dept_leader 级),为空时(designated 级)按本地用户当前绑定兜底。
func rechargeNotifyUnionId(provider string, userId int, snapshotUnionId string) string {
	if snapshotUnionId != "" {
		return snapshotUnionId
	}
	user, err := getUserByIdForRechargeNotify(userId)
	if err != nil || user == nil {
		return ""
	}
	if provider == model.OrgProviderFeishu {
		return user.FeishuId
	}
	return user.DingTalkId
}

// rechargeNotifyEnabled 对应 provider 的 IM 通知开关是否打开。
func rechargeNotifyEnabled(provider string) bool {
	if provider == model.OrgProviderFeishu {
		return getFeishuSettingsForAdminAlert().NotifyEnabled
	}
	return getDingTalkSettingsForAdminAlert().NotifyEnabled
}

// sendRechargeNotice 向单个目标发送,带 15s 超时,失败仅记日志。
func sendRechargeNotice(provider string, unionId string, title string, text string) {
	if unionId == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var err error
	if provider == model.OrgProviderFeishu {
		err = sendFeishuMessage(ctx, unionId, title, text)
	} else {
		err = sendDingTalkWorkNotice(ctx, unionId, title, text)
	}
	if err != nil {
		common.SysLog(fmt.Sprintf("recharge approval notify failed: %s", err.Error()))
	}
}

// fanOutRechargeNotice 向一批审批人(申请步骤)发送同一通知。
func fanOutRechargeNotice(provider string, steps []model.RechargeApprovalStep, title string, text string) {
	if !rechargeNotifyEnabled(provider) {
		return
	}
	for _, step := range steps {
		unionId := rechargeNotifyUnionId(provider, step.UserId, step.UnionId)
		if unionId == "" {
			common.SysLog(fmt.Sprintf("recharge approval notify skipped: approver %d has no IM binding", step.UserId))
			continue
		}
		sendRechargeNotice(provider, unionId, title, text)
	}
}

func rechargeCategoryText(category string) string {
	if category == model.RechargeCategoryProjectDelivery {
		return "项目交付"
	}
	return "技术研究"
}

// rechargeRequestDigest 通知正文公共部分:申请人、事由、金额、说明。
func rechargeRequestDigest(req *model.RechargeRequest) string {
	return fmt.Sprintf("申请人: %s\n事由: %s\n充值金额: $%.2f(额度 %s)\n具体信息: %s",
		req.Username, rechargeCategoryText(req.Category), req.AmountUsd, logger.FormatQuota(req.Quota), req.Detail)
}

// rechargeNotifyProvider 当前有效的 IM provider;无 org sync 时按已启用的
// IM 登录设置兜底(designated-only 流程不依赖组织架构)。
func rechargeNotifyProvider() string {
	if provider := rechargeActiveOrgProvider(); provider != "" {
		return provider
	}
	if getDingTalkSettingsForAdminAlert().NotifyEnabled {
		return model.OrgProviderDingTalk
	}
	if getFeishuSettingsForAdminAlert().NotifyEnabled {
		return model.OrgProviderFeishu
	}
	return ""
}

// NotifyRechargeSubmitted 提交后通知第一级审批人。commit 后异步调用。
func NotifyRechargeSubmitted(requestId int) {
	provider := rechargeNotifyProvider()
	if provider == "" {
		return
	}
	req, err := model.GetRechargeRequestById(requestId)
	if err != nil {
		return
	}
	steps, err := model.PendingRechargeSteps(requestId, req.CurrentLevel)
	if err != nil {
		return
	}
	fanOutRechargeNotice(provider, steps,
		"充值申请待审批",
		rechargeRequestDigest(req)+"\n请登录控制台「充值申请-我的审批」处理。")
}

// NotifyRechargeLevelApproved 某级通过后通知下一级审批人。
func NotifyRechargeLevelApproved(req *model.RechargeRequest) {
	provider := rechargeNotifyProvider()
	if provider == "" {
		return
	}
	steps, err := model.PendingRechargeSteps(req.Id, req.CurrentLevel)
	if err != nil {
		return
	}
	fanOutRechargeNotice(provider, steps,
		fmt.Sprintf("充值申请待审批(第 %d/%d 级)", req.CurrentLevel, req.TotalLevels),
		rechargeRequestDigest(req)+"\n请登录控制台「充值申请-我的审批」处理。")
}

// notifyRechargeApplicant 终审通过/拒绝后通知申请人本人。
func notifyRechargeApplicant(req *model.RechargeRequest, title string, text string) {
	provider := rechargeNotifyProvider()
	if provider == "" {
		return
	}
	if !rechargeNotifyEnabled(provider) {
		return
	}
	unionId := rechargeNotifyUnionId(provider, req.UserId, "")
	if unionId == "" {
		return
	}
	sendRechargeNotice(provider, unionId, title, text)
}

// NotifyRechargeFinalApproved 终审通过且额度已到账。
func NotifyRechargeFinalApproved(req *model.RechargeRequest) {
	notifyRechargeApplicant(req, "充值申请已通过",
		fmt.Sprintf("你的充值申请已审批通过,额度 %s($%.2f)已到账。\n事由: %s",
			logger.FormatQuota(req.Quota), req.AmountUsd, rechargeCategoryText(req.Category)))
}

// NotifyRechargeRejected 申请被拒绝,附拒绝原因。
func NotifyRechargeRejected(req *model.RechargeRequest) {
	notifyRechargeApplicant(req, "充值申请被拒绝",
		fmt.Sprintf("你的充值申请未通过审批。\n事由: %s\n拒绝原因: %s",
			rechargeCategoryText(req.Category), req.RejectReason))
}
