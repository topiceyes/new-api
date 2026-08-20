package controller

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/planmonitor"

	"github.com/gin-gonic/gin"
)

// ---------- 请求/响应 DTO ----------

type planMonitorRequest struct {
	Provider           string `json:"provider"`
	PlanName           string `json:"plan_name"`
	ApiUrl             string `json:"api_url"`
	ApiKey             string `json:"api_key"` // 编辑时留空表示不修改
	RefreshIntervalMin int    `json:"refresh_interval_min"`
	SortOrder          int    `json:"sort_order"`
	Enabled            bool   `json:"enabled"`
}

// planMonitorResponse 列表/详情返回,key 脱敏。
type planMonitorResponse struct {
	Id                 int64  `json:"id"`
	Provider           string `json:"provider"`
	PlanName           string `json:"plan_name"`
	ApiUrl             string `json:"api_url"`
	ApiKeyMasked       string `json:"api_key_masked"`
	RefreshIntervalMin int    `json:"refresh_interval_min"`
	SortOrder          int    `json:"sort_order"`
	Enabled            bool   `json:"enabled"`
	CreatedTime        int64  `json:"created_time"`
	UpdatedTime        int64  `json:"updated_time"`
	LastFetchTime      int64  `json:"last_fetch_time"`
	LastError          string `json:"last_error"`
}

func toPlanMonitorResponse(p *model.PlanMonitor) planMonitorResponse {
	return planMonitorResponse{
		Id:                 p.Id,
		Provider:           p.Provider,
		PlanName:           p.PlanName,
		ApiUrl:             p.ApiUrl,
		ApiKeyMasked:       p.MaskApiKey(),
		RefreshIntervalMin: p.RefreshIntervalMin,
		SortOrder:          p.SortOrder,
		Enabled:            p.Enabled,
		CreatedTime:        p.CreatedTime,
		UpdatedTime:        p.UpdatedTime,
		LastFetchTime:      p.LastFetchTime,
		LastError:          p.LastError,
	}
}

func validatePlanMonitorRequest(req *planMonitorRequest) string {
	if _, err := planmonitor.GetProvider(strings.TrimSpace(req.Provider)); err != nil {
		return "不支持的供应商: " + req.Provider
	}
	if strings.TrimSpace(req.PlanName) == "" {
		return "套餐名称不能为空"
	}
	// API URL 可留空,provider 端按 defaultAPIUrls 兜底。
	if req.RefreshIntervalMin <= 0 {
		req.RefreshIntervalMin = 5
	}
	return ""
}

// ---------- 配置 CRUD ----------

// AdminListPlanMonitors GET /plan_monitor/admin/plans
func AdminListPlanMonitors(c *gin.Context) {
	plans, err := model.GetAllPlanMonitors()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取套餐列表失败: " + err.Error()})
		return
	}
	out := make([]planMonitorResponse, 0, len(plans))
	for _, p := range plans {
		out = append(out, toPlanMonitorResponse(p))
	}
	common.ApiSuccess(c, gin.H{
		"plans":               out,
		"supported_providers": planmonitor.SupportedProviders(),
	})
}

// AdminCreatePlanMonitor POST /plan_monitor/admin/plans
func AdminCreatePlanMonitor(c *gin.Context) {
	var req planMonitorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if msg := validatePlanMonitorRequest(&req); msg != "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
		return
	}
	if strings.TrimSpace(req.ApiKey) == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "API Key 不能为空"})
		return
	}
	plan := &model.PlanMonitor{
		Provider:           strings.TrimSpace(req.Provider),
		PlanName:           strings.TrimSpace(req.PlanName),
		ApiUrl:             strings.TrimSpace(req.ApiUrl),
		ApiKey:             strings.TrimSpace(req.ApiKey),
		RefreshIntervalMin: req.RefreshIntervalMin,
		SortOrder:          req.SortOrder,
		Enabled:            req.Enabled,
	}
	if err := model.CreatePlanMonitor(plan); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建套餐失败: " + err.Error()})
		return
	}
	common.ApiSuccess(c, gin.H{"plan": toPlanMonitorResponse(plan)})
}

// AdminUpdatePlanMonitor PUT /plan_monitor/admin/plans/:id
func AdminUpdatePlanMonitor(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		common.ApiErrorMsg(c, "无效的套餐 ID")
		return
	}
	existing, err := model.GetPlanMonitorById(id)
	if err != nil {
		common.ApiErrorMsg(c, "套餐不存在")
		return
	}
	var req planMonitorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if msg := validatePlanMonitorRequest(&req); msg != "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
		return
	}
	existing.Provider = strings.TrimSpace(req.Provider)
	existing.PlanName = strings.TrimSpace(req.PlanName)
	existing.ApiUrl = strings.TrimSpace(req.ApiUrl)
	existing.RefreshIntervalMin = req.RefreshIntervalMin
	existing.SortOrder = req.SortOrder
	existing.Enabled = req.Enabled
	// key 留空表示不修改
	if k := strings.TrimSpace(req.ApiKey); k != "" {
		existing.ApiKey = k
	}
	if err := model.UpdatePlanMonitor(existing); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "更新套餐失败: " + err.Error()})
		return
	}
	common.ApiSuccess(c, gin.H{"plan": toPlanMonitorResponse(existing)})
}

// AdminDeletePlanMonitor DELETE /plan_monitor/admin/plans/:id
func AdminDeletePlanMonitor(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		common.ApiErrorMsg(c, "无效的套餐 ID")
		return
	}
	if err := model.DeletePlanMonitor(id); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "删除套餐失败: " + err.Error()})
		return
	}
	common.ApiSuccess(c, nil)
}

// AdminUpdatePlanMonitorStatus PATCH /plan_monitor/admin/plans/:id/status
func AdminUpdatePlanMonitorStatus(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		common.ApiErrorMsg(c, "无效的套餐 ID")
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DB.Model(&model.PlanMonitor{}).Where("id = ?", id).
		Update("enabled", body.Enabled).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "更新状态失败: " + err.Error()})
		return
	}
	common.ApiSuccess(c, nil)
}

// AdminRefreshPlanMonitor POST /plan_monitor/admin/plans/:id/refresh 手动立即拉取
func AdminRefreshPlanMonitor(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		common.ApiErrorMsg(c, "无效的套餐 ID")
		return
	}
	if err := planmonitor.FetchOneNow(context.Background(), id); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "拉取失败: " + err.Error()})
		return
	}
	common.ApiSuccess(c, nil)
}

// ---------- 展示页数据 ----------

type planUsageView struct {
	Period           string  `json:"period"`
	UsedPercent      float64 `json:"used_percent"`
	RemainingPercent float64 `json:"remaining_percent"`
	PeriodEndTime    int64   `json:"period_end_time"`
	FetchedAt        int64   `json:"fetched_at"`
}

type planOverviewItem struct {
	planMonitorResponse
	Usages []planUsageView `json:"usages"`
}

type planOverviewGroup struct {
	Provider string             `json:"provider"`
	Plans    []planOverviewItem `json:"plans"`
}

// AdminGetPlanMonitorOverview GET /plan_monitor/admin/overview
// 按供应商分组返回套餐 + 各周期最新用量。
func AdminGetPlanMonitorOverview(c *gin.Context) {
	plans, err := model.GetAllPlanMonitors()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取套餐失败: " + err.Error()})
		return
	}
	groupIndex := map[string]int{}
	groups := make([]planOverviewGroup, 0)
	for _, p := range plans {
		usages, err := model.GetPlanMonitorUsages(p.Id)
		if err != nil {
			common.SysError("plan monitor overview: load usages failed: " + err.Error())
		}
		views := make([]planUsageView, 0, len(usages))
		for _, u := range usages {
			views = append(views, planUsageView{
				Period:           u.Period,
				UsedPercent:      u.UsedPercent,
				RemainingPercent: u.RemainingPercent,
				PeriodEndTime:    u.PeriodEndTime,
				FetchedAt:        u.FetchedAt,
			})
		}
		item := planOverviewItem{planMonitorResponse: toPlanMonitorResponse(p), Usages: views}
		idx, ok := groupIndex[p.Provider]
		if !ok {
			groups = append(groups, planOverviewGroup{Provider: p.Provider, Plans: []planOverviewItem{}})
			idx = len(groups) - 1
			groupIndex[p.Provider] = idx
		}
		groups[idx].Plans = append(groups[idx].Plans, item)
	}
	common.ApiSuccess(c, gin.H{"groups": groups})
}
