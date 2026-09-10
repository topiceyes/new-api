package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

// rechargeDetailMaxRunes 申请具体信息的长度上限。
const rechargeDetailMaxRunes = 500

// rechargeDetailMinRunes 申请具体信息的长度下限,避免一句话敷衍的审批材料。
const rechargeDetailMinRunes = 10

// rechargeChainLevel 审批链预览中的一级。
type rechargeChainLevel struct {
	Level     int                              `json:"level"`
	Type      string                           `json:"type"`
	Approvers []model.ResolvedRechargeApprover `json:"approvers"`
}

// GetRechargeRequestConfig 返回流程配置与当前用户的审批链预览。
// 审批链解析失败不报错,而是 resolvable=false + 原因,让发起页直接展示阻断原因。
func GetRechargeRequestConfig(c *gin.Context) {
	settings := system_setting.GetRechargeApprovalSettings()
	quota := 0
	if settings.QuotaUsd > 0 {
		if q, err := service.RechargeQuotaFromUsd(settings.QuotaUsd); err == nil {
			quota = q
		}
	}
	resp := gin.H{
		"enabled":    settings.Enabled,
		"quota_usd":  settings.QuotaUsd,
		"quota":      quota,
		"categories": []string{model.RechargeCategoryProjectDelivery, model.RechargeCategoryTechResearch},
		"resolvable": false,
		"chain":      []rechargeChainLevel{},
	}
	if !settings.Enabled || len(settings.Levels) == 0 {
		resp["resolve_error"] = "充值审批流程未启用或未配置"
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": resp})
		return
	}
	chain, err := service.ResolveApprovalChain(c.GetInt("id"), settings.Levels)
	if err != nil {
		resp["resolve_error"] = err.Error()
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": resp})
		return
	}
	levels := make([]rechargeChainLevel, 0, len(chain))
	for i, approvers := range chain {
		levels = append(levels, rechargeChainLevel{
			Level:     i + 1,
			Type:      settings.Levels[i].Type,
			Approvers: approvers,
		})
	}
	resp["resolvable"] = true
	resp["chain"] = levels
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": resp})
}

type submitRechargeRequestBody struct {
	Category string `json:"category"`
	Detail   string `json:"detail"`
}

// SubmitRechargeRequest 用户提交充值申请:校验事由,解析审批链并快照,
// 创建申请后异步通知第一级审批人。
func SubmitRechargeRequest(c *gin.Context) {
	settings := system_setting.GetRechargeApprovalSettings()
	if !settings.Enabled {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "充值申请功能未启用"})
		return
	}
	if err := settings.Validate(); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "充值审批流程配置无效,请联系管理员: " + err.Error()})
		return
	}
	var body submitRechargeRequestBody
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	body.Category = strings.TrimSpace(body.Category)
	body.Detail = strings.TrimSpace(body.Detail)
	if !model.IsValidRechargeCategory(body.Category) {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的申请事由"})
		return
	}
	if body.Detail == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "请填写具体信息"})
		return
	}
	if utf8.RuneCountInString(body.Detail) < rechargeDetailMinRunes {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "具体信息至少填写 10 个字"})
		return
	}
	if utf8.RuneCountInString(body.Detail) > rechargeDetailMaxRunes {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "具体信息过长(最多 500 字)"})
		return
	}

	userId := c.GetInt("id")
	chain, err := service.ResolveApprovalChain(userId, settings.Levels)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	quota, err := service.RechargeQuotaFromUsd(settings.QuotaUsd)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "充值额度配置无效,请联系管理员"})
		return
	}

	user, err := model.GetUserById(userId, false)
	if err != nil || user == nil || user.Id == 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "用户不存在"})
		return
	}
	req := &model.RechargeRequest{
		UserId:    userId,
		Username:  user.Username,
		Category:  body.Category,
		Detail:    body.Detail,
		AmountUsd: settings.QuotaUsd,
		Quota:     quota,
	}
	if err := model.CreateRechargeRequest(req, chain); err != nil {
		if errors.Is(err, model.ErrRechargePendingExists) {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		common.SysError("create recharge request failed: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "提交失败,请稍后重试"})
		return
	}
	recordUserSecurityAudit(c, userId, "recharge.submit", map[string]interface{}{
		"request_id": req.Id, "category": req.Category, "quota": req.Quota,
	})
	go service.NotifyRechargeSubmitted(req.Id)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": req.Id})
}

func GetMyRechargeRequests(c *gin.Context) {
	requests, total, err := model.GetUserRechargeRequests(c.GetInt("id"), common.GetPageQuery(c))
	if err != nil {
		common.SysError("get my recharge requests failed: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{
		"items": requests, "total": total,
	}})
}

func GetMyRechargeApprovalTasks(c *gin.Context) {
	pendingOnly := c.Query("pending") == "true"
	tasks, total, err := model.GetRechargeApprovalTasks(c.GetInt("id"), pendingOnly, common.GetPageQuery(c))
	if err != nil {
		common.SysError("get recharge approval tasks failed: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{
		"items": tasks, "total": total,
	}})
}

// loadVisibleRechargeRequest 加载申请并校验可见性:申请人本人、任一审批人或管理员。
func loadVisibleRechargeRequest(c *gin.Context) (*model.RechargeRequest, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return nil, false
	}
	req, err := model.GetRechargeRequestById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "申请不存在"})
		return nil, false
	}
	userId := c.GetInt("id")
	if req.UserId != userId && c.GetInt("role") < common.RoleAdminUser && !model.IsRechargeRequestApprover(id, userId) {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无权查看该申请"})
		return nil, false
	}
	return req, true
}

func GetRechargeRequestDetail(c *gin.Context) {
	req, ok := loadVisibleRechargeRequest(c)
	if !ok {
		return
	}
	steps, err := model.GetRechargeRequestSteps(req.Id)
	if err != nil {
		common.SysError("get recharge request steps failed: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{
		"request": req, "steps": steps,
	}})
}

type decideRechargeRequestBody struct {
	Comment string `json:"comment"`
}

func ApproveRechargeRequest(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	var body decideRechargeRequestBody
	_ = common.DecodeJson(c.Request.Body, &body)
	approverId := c.GetInt("id")
	req, _, finalApproved, alreadyDone, err := model.ApproveRechargeStep(id, approverId, strings.TrimSpace(body.Comment))
	if err != nil {
		writeRechargeDecideError(c, err)
		return
	}
	if alreadyDone {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已处理过,无需重复操作", "data": req})
		return
	}
	recordUserSecurityAudit(c, approverId, "recharge.approve", map[string]interface{}{
		"request_id": id, "target_user_id": req.UserId, "quota": req.Quota,
	})
	if finalApproved {
		go service.NotifyRechargeFinalApproved(req)
	} else {
		go service.NotifyRechargeLevelApproved(req)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": req})
}

func RejectRechargeRequest(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	var body decideRechargeRequestBody
	_ = common.DecodeJson(c.Request.Body, &body)
	comment := strings.TrimSpace(body.Comment)
	if comment == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "拒绝时必须填写原因"})
		return
	}
	approverId := c.GetInt("id")
	req, _, alreadyDone, err := model.RejectRechargeStep(id, approverId, comment)
	if err != nil {
		writeRechargeDecideError(c, err)
		return
	}
	if alreadyDone {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已处理过,无需重复操作", "data": req})
		return
	}
	recordUserSecurityAudit(c, approverId, "recharge.reject", map[string]interface{}{
		"request_id": id, "target_user_id": req.UserId, "reason": comment,
	})
	go service.NotifyRechargeRejected(req)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": req})
}

func writeRechargeDecideError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, model.ErrRechargeRequestNotFound),
		errors.Is(err, model.ErrRechargeRequestStatusInvalid),
		errors.Is(err, model.ErrRechargeNotApprover):
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
	case errors.Is(err, model.ErrTopUpQuotaLimitExceeded):
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "申请人额度已达系统上限,暂时无法入账,请稍后再试"})
	default:
		common.SysError("decide recharge request failed: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "操作失败,请稍后重试"})
	}
}

func AdminGetRechargeRequests(c *gin.Context) {
	requests, total, err := model.GetAllRechargeRequests(
		strings.TrimSpace(c.Query("status")),
		strings.TrimSpace(c.Query("category")),
		strings.TrimSpace(c.Query("keyword")),
		common.GetPageQuery(c),
	)
	if err != nil {
		common.SysError("admin get recharge requests failed: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{
		"items": requests, "total": total,
	}})
}
