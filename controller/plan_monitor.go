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
	AlertThreshold     int    `json:"alert_threshold"`      // 用量告警阈值(百分比),0=不告警
	FailAlertThreshold *int   `json:"fail_alert_threshold"` // 连续失败告警次数,0=不告警;nil 表示新建默认 3
	Enabled            bool   `json:"enabled"`
	IsPublic           bool   `json:"is_public"`
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
	AlertThreshold     int    `json:"alert_threshold"`
	FailAlertThreshold int    `json:"fail_alert_threshold"`
	FetchFailCount     int    `json:"fetch_fail_count"`
	FailAlertSentAt    int64  `json:"fail_alert_sent_at"`
	Enabled            bool   `json:"enabled"`
	IsPublic           bool   `json:"is_public"`
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
		AlertThreshold:     p.AlertThreshold,
		FailAlertThreshold: p.FailAlertThreshold,
		FetchFailCount:     p.FetchFailCount,
		FailAlertSentAt:    p.FailAlertSentAt,
		Enabled:            p.Enabled,
		IsPublic:           p.IsPublic,
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
	if req.AlertThreshold < 0 || req.AlertThreshold > 100 {
		return "告警阈值必须在 0-100 之间(0 表示不告警)"
	}
	if req.FailAlertThreshold != nil && (*req.FailAlertThreshold < 0 || *req.FailAlertThreshold > 100) {
		return "连续失败告警次数必须在 0-100 之间(0 表示不告警)"
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
		AlertThreshold:     req.AlertThreshold,
		Enabled:            req.Enabled,
		IsPublic:           req.IsPublic,
	}
	// 新建默认连续失败告警 3 次;显式传 0 表示关闭。
	plan.FailAlertThreshold = 3
	if req.FailAlertThreshold != nil {
		plan.FailAlertThreshold = *req.FailAlertThreshold
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
	existing.AlertThreshold = req.AlertThreshold
	if req.FailAlertThreshold != nil {
		existing.FailAlertThreshold = *req.FailAlertThreshold
	}
	existing.Enabled = req.Enabled
	existing.IsPublic = req.IsPublic
	// key 留空表示不修改
	if k := strings.TrimSpace(req.ApiKey); k != "" {
		existing.ApiKey = k
	}
	if err := model.UpdatePlanMonitor(existing); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "更新套餐失败: " + err.Error()})
		return
	}
	// 阈值被关闭(0)时清掉残留的失败计数/告警标记,避免列表页显示过期状态
	if existing.FailAlertThreshold <= 0 {
		if err := model.ClearPlanMonitorFailAlertState(existing.Id); err != nil {
			common.SysError("plan monitor: clear fail alert state on threshold disable failed: " + err.Error())
		}
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

// planHistoryRanges 允许的历史查询范围(小时)。
var planHistoryRanges = map[string]int{
	"24h": 24,
	"7d":  24 * 7,
	"30d": 24 * 30,
}

// AdminGetPlanMonitorHistory GET /plan_monitor/admin/plans/:id/history?period=&range=24h|7d|30d
// 返回某套餐某周期的用量趋势点,供展示页趋势图使用。
func AdminGetPlanMonitorHistory(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		common.ApiErrorMsg(c, "无效的套餐 ID")
		return
	}
	period := strings.TrimSpace(c.Query("period"))
	if period == "" {
		common.ApiErrorMsg(c, "缺少 period 参数")
		return
	}
	rangeHours, ok := planHistoryRanges[c.Query("range")]
	if !ok {
		rangeHours = planHistoryRanges["24h"]
	}
	if _, err := model.GetPlanMonitorById(id); err != nil {
		common.ApiErrorMsg(c, "套餐不存在")
		return
	}
	points, err := planmonitor.GetUsageHistoryPoints(id, period, rangeHours)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取历史数据失败: " + err.Error()})
		return
	}
	common.ApiSuccess(c, gin.H{"points": points})
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

// ---------- 用户端「套餐余量」 ----------

// publicPlanItem 用户端裁剪后的套餐条目:不含 key/url/阈值/错误等管理字段。
type publicPlanItem struct {
	Id       int64           `json:"id"`
	Provider string          `json:"provider"`
	PlanName string          `json:"plan_name"`
	Usages   []planUsageView `json:"usages"`
}

type publicPlanGroup struct {
	Provider string           `json:"provider"`
	Plans    []publicPlanItem `json:"plans"`
}

// GetPublicPlanMonitorOverview GET /plan_monitor/overview
// 仅返回已启用且公开的套餐,按供应商分组,字段裁剪。
func GetPublicPlanMonitorOverview(c *gin.Context) {
	plans, err := model.GetPublicPlanMonitors()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取套餐失败: " + err.Error()})
		return
	}
	groupIndex := map[string]int{}
	groups := make([]publicPlanGroup, 0)
	for _, p := range plans {
		usages, err := model.GetPlanMonitorUsages(p.Id)
		if err != nil {
			common.SysError("plan monitor public overview: load usages failed: " + err.Error())
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
		item := publicPlanItem{Id: p.Id, Provider: p.Provider, PlanName: p.PlanName, Usages: views}
		idx, ok := groupIndex[p.Provider]
		if !ok {
			groups = append(groups, publicPlanGroup{Provider: p.Provider, Plans: []publicPlanItem{}})
			idx = len(groups) - 1
			groupIndex[p.Provider] = idx
		}
		groups[idx].Plans = append(groups[idx].Plans, item)
	}
	common.ApiSuccess(c, gin.H{"groups": groups})
}

// GetPublicPlanMonitorHistory GET /plan_monitor/plans/:id/history?period=&range=24h|7d|30d
// 仅当套餐公开且启用时返回;不存在/不公开/已停用统一报「套餐不存在」,避免探测非公开套餐。
func GetPublicPlanMonitorHistory(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		common.ApiErrorMsg(c, "无效的套餐 ID")
		return
	}
	period := strings.TrimSpace(c.Query("period"))
	if period == "" {
		common.ApiErrorMsg(c, "缺少 period 参数")
		return
	}
	rangeHours, ok := planHistoryRanges[c.Query("range")]
	if !ok {
		rangeHours = planHistoryRanges["24h"]
	}
	plan, err := model.GetPlanMonitorById(id)
	if err != nil || !plan.IsPublic || !plan.Enabled {
		common.ApiErrorMsg(c, "套餐不存在")
		return
	}
	points, err := planmonitor.GetUsageHistoryPoints(id, period, rangeHours)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取历史数据失败: " + err.Error()})
		return
	}
	common.ApiSuccess(c, gin.H{"points": points})
}
