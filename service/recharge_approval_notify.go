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
// 每个跳过/失败原因都写日志:业务侧"没收到消息"时可直接从后端日志定位。
func fanOutRechargeNotice(provider string, steps []model.RechargeApprovalStep, title string, text string) {
	if !rechargeNotifyEnabled(provider) {
		common.SysLog(fmt.Sprintf("recharge approval notify skipped: %s notify_enabled is off", provider))
		return
	}
	if len(steps) == 0 {
		common.SysLog("recharge approval notify skipped: no pending approvers at current level")
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

// rechargeApplicantName 通知正文里的申请人称呼:优先 users.display_name
// 真名,查不到(已删/未写)回退申请行上的 username 快照。
func rechargeApplicantName(req *model.RechargeRequest) string {
	user, err := getUserByIdForRechargeNotify(req.UserId)
	if err == nil && user != nil && user.DisplayName != "" {
		return user.DisplayName
	}
	return req.Username
}

// rechargeRequestDigest 通知正文公共部分:申请人、事由、金额、说明。
func rechargeRequestDigest(req *model.RechargeRequest) string {
	return fmt.Sprintf("申请人: %s\n事由: %s\n充值金额: $%.2f(额度 %s)\n具体信息: %s",
		rechargeApplicantName(req), rechargeCategoryText(req.Category), req.AmountUsd, logger.FormatQuota(req.Quota), req.Detail)
}

// rechargeNotifyProvider 当前有效的 IM provider;无 org sync 时按已启用的
// IM 登录设置兜底(designated-only 流程不依赖组织架构)。
// 返回空说明完全没配 IM,调用方记日志后放弃。
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

// NotifyRechargeSubmitted 提交后通知第一级审批人,并给申请人回执一条
// "提交成功"确认。commit 后异步调用。
func NotifyRechargeSubmitted(requestId int) {
	provider := rechargeNotifyProvider()
	if provider == "" {
		common.SysLog("recharge approval notify skipped: no enterprise IM enabled and no IM notify enabled")
		return
	}
	req, err := model.GetRechargeRequestById(requestId)
	if err != nil {
		common.SysLog(fmt.Sprintf("recharge approval notify skipped: load request %d failed: %s", requestId, err.Error()))
		return
	}
	steps, err := model.PendingRechargeSteps(requestId, req.CurrentLevel)
	if err != nil {
		common.SysLog(fmt.Sprintf("recharge approval notify skipped: load pending steps of request %d failed: %s", requestId, err.Error()))
		return
	}
	fanOutRechargeNotice(provider, steps,
		"充值申请待审批",
		rechargeRequestDigest(req)+fmt.Sprintf("\n请登录 %s 平台,在「充值申请-我的审批」处理。", common.SystemName))
	notifyRechargeApplicant(req, "充值申请已提交",
		fmt.Sprintf("你的充值申请已提交成功,等待第 1/%d 级审批。\n事由: %s\n充值金额: $%.2f(额度 %s)\n具体信息: %s\n审批人会收到 IM 提醒;请登录 %s 平台,在「充值申请-我的申请」查看进度,必要时可催批(最多 %d 次)。",
			req.TotalLevels, rechargeCategoryText(req.Category), req.AmountUsd, logger.FormatQuota(req.Quota), req.Detail, common.SystemName, model.RechargeUrgeMax))
}

// NotifyRechargeLevelApproved 某级通过后通知下一级审批人。
func NotifyRechargeLevelApproved(req *model.RechargeRequest) {
	provider := rechargeNotifyProvider()
	if provider == "" {
		common.SysLog("recharge approval notify skipped: no enterprise IM enabled and no IM notify enabled")
		return
	}
	steps, err := model.PendingRechargeSteps(req.Id, req.CurrentLevel)
	if err != nil {
		common.SysLog(fmt.Sprintf("recharge approval notify skipped: load pending steps of request %d failed: %s", req.Id, err.Error()))
		return
	}
	fanOutRechargeNotice(provider, steps,
		fmt.Sprintf("充值申请待审批(第 %d/%d 级)", req.CurrentLevel, req.TotalLevels),
		rechargeRequestDigest(req)+fmt.Sprintf("\n请登录 %s 平台,在「充值申请-我的审批」处理。", common.SystemName))
}

// NotifyRechargeUrge 申请人催批:提醒当前级审批人尽快登录平台处理。
func NotifyRechargeUrge(req *model.RechargeRequest) {
	provider := rechargeNotifyProvider()
	if provider == "" {
		common.SysLog("recharge approval notify skipped: no enterprise IM enabled and no IM notify enabled")
		return
	}
	steps, err := model.PendingRechargeSteps(req.Id, req.CurrentLevel)
	if err != nil {
		common.SysLog(fmt.Sprintf("recharge approval notify skipped: load pending steps of request %d failed: %s", req.Id, err.Error()))
		return
	}
	fanOutRechargeNotice(provider, steps,
		"充值申请催办提醒",
		fmt.Sprintf("【催办】申请人 %s 请求你尽快审批:\n%s\n请尽快登录 %s 平台,在「充值申请-我的审批」处理。",
			rechargeApplicantName(req), rechargeRequestDigest(req), common.SystemName))
}

// notifyRechargeApplicant 终审通过/拒绝/提交回执后通知申请人本人。
func notifyRechargeApplicant(req *model.RechargeRequest, title string, text string) {
	provider := rechargeNotifyProvider()
	if provider == "" {
		common.SysLog("recharge approval notify skipped: no enterprise IM enabled and no IM notify enabled")
		return
	}
	if !rechargeNotifyEnabled(provider) {
		common.SysLog(fmt.Sprintf("recharge approval notify skipped: %s notify_enabled is off", provider))
		return
	}
	unionId := rechargeNotifyUnionId(provider, req.UserId, "")
	if unionId == "" {
		common.SysLog(fmt.Sprintf("recharge approval notify skipped: applicant %d has no IM binding", req.UserId))
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
