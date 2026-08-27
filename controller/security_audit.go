package controller

import (
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// 安全/行为审计事件的管理端查询接口(独立于本包 audit.go 的管理操作审计,
// 数据在 audit_events 表)。

// GetAuditEvents 安全审计事件分页查询。
// 列表不返回 prompt 原文(模型层已 Select 剔除),详情走 GetAuditEvent。
func GetAuditEvents(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	eventType := c.Query("event_type")
	severity := c.Query("severity")
	userId, _ := strconv.Atoi(c.Query("user_id"))
	tokenId, _ := strconv.Atoi(c.Query("token_id"))
	keyword := c.Query("keyword")
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)

	events, total, err := model.GetAuditEvents(eventType, severity, userId, tokenId, keyword, startTimestamp, endTimestamp, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(events)
	common.ApiSuccess(c, pageInfo)
}

// GetAuditEvent 单条事件详情,含 prompt 原文(仅当配置允许且已存储)。
func GetAuditEvent(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的事件ID")
		return
	}
	event, err := model.GetAuditEventById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, event)
}

// GetAuditEventStats 按事件类型 + 规则聚合计数,供管理端概览/趋势使用。
func GetAuditEventStats(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	stats, err := model.GetAuditEventStats(startTimestamp, endTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, stats)
}

// ---- skill 库(二期②):LLM 分类沉淀的候选,管理员审核后发布 ----

// GetSkillCandidates 候选分页查询,status 为空查全部。
func GetSkillCandidates(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.GetSkillCandidates(c.Query("status"), c.Query("keyword"), pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

// ApproveSkillCandidateRequest 审核通过时可顺手修订标题/分类/描述。
type ApproveSkillCandidateRequest struct {
	Title       string `json:"title"`
	Category    string `json:"category"`
	Description string `json:"description"`
}

// ApproveSkillCandidate 候选审核通过:建 Skill 条目并标记候选已发布。
func ApproveSkillCandidate(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的候选ID")
		return
	}
	candidate, err := model.GetSkillCandidateById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if candidate.Status != model.SkillCandidateStatusPending {
		common.ApiErrorMsg(c, "候选已处理,请刷新列表")
		return
	}
	var req ApproveSkillCandidateRequest
	_ = c.ShouldBindJSON(&req)
	title := req.Title
	if title == "" {
		title = candidate.Title
	}
	category := req.Category
	if category == "" {
		category = candidate.Category
	}
	skill, err := model.PublishSkillCandidate(candidate, title, category, req.Description, time.Now().Unix())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, skill)
}

// RejectSkillCandidate 候选审核拒绝。
func RejectSkillCandidate(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的候选ID")
		return
	}
	candidate, err := model.GetSkillCandidateById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if candidate.Status != model.SkillCandidateStatusPending {
		common.ApiErrorMsg(c, "候选已处理,请刷新列表")
		return
	}
	if err := model.RejectSkillCandidate(id, time.Now().Unix()); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// GetSkills 已发布/已下架 skill 分页查询。
func GetSkills(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.GetSkills(c.Query("status"), c.Query("keyword"), pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

// UpdateSkillRequest skill 条目编辑。
type UpdateSkillRequest struct {
	Title        string `json:"title" binding:"required"`
	Category     string `json:"category"`
	Description  string `json:"description"`
	SamplePrompt string `json:"sample_prompt"`
}

func UpdateSkill(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的 skill ID")
		return
	}
	var req UpdateSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "无效的参数")
		return
	}
	if err := model.UpdateSkill(id, req.Title, req.Category, req.Description, req.SamplePrompt, time.Now().Unix()); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// ArchiveSkill 下架 skill 条目(数据保留,不再出现在默认库视图)。
func ArchiveSkill(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的 skill ID")
		return
	}
	if err := model.ArchiveSkill(id, time.Now().Unix()); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
