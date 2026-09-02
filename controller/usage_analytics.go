package controller

import (
	"net/http"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/usage_analytics"

	"github.com/gin-gonic/gin"
)

// resolveAnalyticsScope 解析当前用户的使用分析可见范围;无权时写错误响应并返回 false。
func resolveAnalyticsScope(c *gin.Context) (*service.AnalyticsScope, bool) {
	scope, err := service.ResolveAnalyticsScope(c.GetInt("id"), c.GetInt("role"))
	if err != nil {
		common.ApiErrorMsg(c, "无使用分析权限")
		return nil, false
	}
	return scope, true
}

// analyticsDateRange 解析 start_timestamp/end_timestamp(unix 秒)并转换为本地时区
// 'YYYY-MM-DD' 闭区间,右端不超过今天,左端不早于聚合保留期。
func analyticsDateRange(c *gin.Context) (string, string, bool) {
	startTimestamp, err1 := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, err2 := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if err1 != nil || err2 != nil || startTimestamp <= 0 || endTimestamp <= 0 || endTimestamp < startTimestamp {
		common.ApiErrorMsg(c, "invalid time range")
		return "", "", false
	}
	now := time.Now().In(time.Local)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	if endTimestamp >= today.AddDate(0, 0, 1).Unix() {
		endTimestamp = now.Unix()
	}
	// AggregateRetentionDays <= 0 表示聚合数据永久保留(与任务侧语义一致),
	// 此时不做下限钳制; >0 时左端不早于保留期。无论是否开启保留期都设 10 年
	// 硬上限: activity 端点按天展开,1970 年起点的病态请求会产出上万行空条目。
	retentionDays := usage_analytics.GetUsageAnalyticsSettings().AggregateRetentionDays
	if retentionDays > 0 {
		earliest := today.AddDate(0, 0, -retentionDays).Unix()
		if startTimestamp < earliest {
			startTimestamp = earliest
		}
	}
	hardEarliest := today.AddDate(-10, 0, 0).Unix()
	if startTimestamp < hardEarliest {
		startTimestamp = hardEarliest
	}
	startDate := time.Unix(startTimestamp, 0).In(time.Local).Format("2006-01-02")
	endDate := time.Unix(endTimestamp, 0).In(time.Local).Format("2006-01-02")
	return startDate, endDate, true
}

func analyticsSuccess(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": data})
}

// GetAnalyticsAccess 前端 section 可见性探测:普通员工调用返回 allowed=false 而不是报错。
func GetAnalyticsAccess(c *gin.Context) {
	scope, err := service.ResolveAnalyticsScope(c.GetInt("id"), c.GetInt("role"))
	if err != nil {
		analyticsSuccess(c, gin.H{"allowed": false})
		return
	}
	analyticsSuccess(c, gin.H{
		"allowed":  true,
		"scope":    scope.Scope,
		"dept_ids": scope.DeptIds,
	})
}

func GetAnalyticsOverview(c *gin.Context) {
	scope, ok := resolveAnalyticsScope(c)
	if !ok {
		return
	}
	startDate, endDate, ok := analyticsDateRange(c)
	if !ok {
		return
	}
	totals, err := model.QueryUsageDailyTotals(startDate, endDate, scope.UserIds)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	failRate := 0.0
	if totals.RequestCount+totals.FailCount > 0 {
		failRate = float64(totals.FailCount) / float64(totals.RequestCount+totals.FailCount)
	}
	analyticsSuccess(c, gin.H{
		"request_count":     totals.RequestCount,
		"fail_count":        totals.FailCount,
		"fail_rate":         failRate,
		"quota":             totals.Quota,
		"refund_quota":      totals.RefundQuota,
		"net_quota":         totals.Quota - totals.RefundQuota,
		"active_users":      totals.ActiveUsers,
		"prompt_tokens":     totals.PromptTokens,
		"completion_tokens": totals.CompletionTokens,
	})
}

func GetAnalyticsActivity(c *gin.Context) {
	scope, ok := resolveAnalyticsScope(c)
	if !ok {
		return
	}
	startDate, endDate, ok := analyticsDateRange(c)
	if !ok {
		return
	}
	rows, err := model.QueryUsageDailyByDate(startDate, endDate, scope.UserIds)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	type dayBucket struct {
		activeSet    map[int]bool
		requestCount int64
		failCount    int64
		quota        int64
	}
	byDate := make(map[string]*dayBucket)
	endTime, _ := time.ParseInLocation("2006-01-02", endDate, time.Local)
	wauStart := endTime.AddDate(0, 0, -6).Format("2006-01-02")
	mauStart := endTime.AddDate(0, 0, -29).Format("2006-01-02")
	wauSet := make(map[int]bool)
	mauSet := make(map[int]bool)
	for _, r := range rows {
		if r.RequestCount <= 0 {
			continue
		}
		bucket := byDate[r.Date]
		if bucket == nil {
			bucket = &dayBucket{activeSet: make(map[int]bool)}
			byDate[r.Date] = bucket
		}
		bucket.activeSet[r.UserId] = true
		if r.Date >= wauStart {
			wauSet[r.UserId] = true
		}
		if r.Date >= mauStart {
			mauSet[r.UserId] = true
		}
	}
	for _, r := range rows {
		bucket := byDate[r.Date]
		if bucket == nil {
			bucket = &dayBucket{activeSet: make(map[int]bool)}
			byDate[r.Date] = bucket
		}
		bucket.requestCount += r.RequestCount
		bucket.failCount += r.FailCount
		bucket.quota += r.Quota - r.RefundQuota
	}
	// 输出连续日期序列,缺口补零,前端折线不断点。
	days := []gin.H{}
	for d, _ := time.ParseInLocation("2006-01-02", startDate, time.Local); !d.After(endTime); d = d.AddDate(0, 0, 1) {
		date := d.Format("2006-01-02")
		bucket := byDate[date]
		entry := gin.H{"date": date, "active_users": 0, "request_count": 0, "fail_count": 0, "quota": 0}
		if bucket != nil {
			entry["active_users"] = len(bucket.activeSet)
			entry["request_count"] = bucket.requestCount
			entry["fail_count"] = bucket.failCount
			entry["quota"] = bucket.quota
		}
		days = append(days, entry)
	}
	analyticsSuccess(c, gin.H{
		"days": days,
		"wau":  len(wauSet),
		"mau":  len(mauSet),
	})
}

// GetAnalyticsUserTable 用户分析全量表(每用户一行,含零活跃与未绑定成员),
// 筛选/排序/分页全部在前端完成,这里一次返回全集(内部员工量级,数百行)。
func GetAnalyticsUserTable(c *gin.Context) {
	scope, ok := resolveAnalyticsScope(c)
	if !ok {
		return
	}
	startDate, endDate, ok := analyticsDateRange(c)
	if !ok {
		return
	}
	entries, err := service.BuildAnalyticsUserTable(scope, startDate, endDate)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	analyticsSuccess(c, entries)
}

func GetAnalyticsTopUsers(c *gin.Context) {
	scope, ok := resolveAnalyticsScope(c)
	if !ok {
		return
	}
	startDate, endDate, ok := analyticsDateRange(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := model.QueryUsageByUser(startDate, endDate, scope.UserIds)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	sort.Slice(rows, func(i, j int) bool {
		netI := rows[i].Quota - rows[i].RefundQuota
		netJ := rows[j].Quota - rows[j].RefundQuota
		if netI != netJ {
			return netI > netJ
		}
		// 净消耗相同按 user_id 定序,避免跨请求顺序抖动。
		return rows[i].UserId < rows[j].UserId
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	userIds := make([]int, 0, len(rows))
	for _, r := range rows {
		userIds = append(userIds, r.UserId)
	}
	displayNames, err := model.GetUserDisplayNames(userIds)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items := []gin.H{}
	for _, r := range rows {
		failRate := 0.0
		if r.RequestCount+r.FailCount > 0 {
			failRate = float64(r.FailCount) / float64(r.RequestCount+r.FailCount)
		}
		items = append(items, gin.H{
			"user_id":       r.UserId,
			"username":      r.Username,
			"display_name":  displayNames[r.UserId],
			"quota":         r.Quota - r.RefundQuota,
			"request_count": r.RequestCount,
			"fail_count":    r.FailCount,
			"fail_rate":     failRate,
		})
	}
	analyticsSuccess(c, items)
}

func GetAnalyticsDepartments(c *gin.Context) {
	scope, ok := resolveAnalyticsScope(c)
	if !ok {
		return
	}
	startDate, endDate, ok := analyticsDateRange(c)
	if !ok {
		return
	}
	provider := service.ActiveOrgSyncProvider()
	if provider == "" {
		analyticsSuccess(c, []gin.H{})
		return
	}
	attribution, err := service.LoadOrgDeptAttribution(provider)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	rows, err := model.QueryUsageByUser(startDate, endDate, scope.UserIds)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	type deptBucket struct {
		activeUsers int
		quota       int64
		requests    int64
		fails       int64
	}
	buckets := make(map[string]*deptBucket)
	for _, r := range rows {
		var deptId string
		var memberOfScope bool
		if scope.Scope == "admin" {
			deptId, memberOfScope = attribution.PrimaryDept[r.UserId]
		} else {
			deptId, memberOfScope = scope.PrimaryDept[r.UserId]
		}
		if !memberOfScope || deptId == "" {
			continue
		}
		bucket := buckets[deptId]
		if bucket == nil {
			bucket = &deptBucket{}
			buckets[deptId] = bucket
		}
		if r.RequestCount > 0 {
			bucket.activeUsers++
		}
		bucket.quota += r.Quota - r.RefundQuota
		bucket.requests += r.RequestCount
		bucket.fails += r.FailCount
	}
	// 列出的部门: 管理员=全部有绑定成员的部门; 负责人=自己子树内的部门。
	deptIds := make([]string, 0, len(buckets))
	if scope.Scope == "admin" {
		for deptId := range attribution.MemberCount {
			deptIds = append(deptIds, deptId)
		}
	} else {
		// scope 对象在缓存里被并发共享,排序前必须克隆,不能原地排。
		deptIds = slices.Clone(scope.DeptIds)
	}
	sort.Strings(deptIds)
	// 负责人范围的成员数先按主部门一次遍历统计,避免 部门数x成员数 的嵌套循环。
	scopeMemberCount := make(map[string]int, len(deptIds))
	if scope.Scope != "admin" {
		for _, userId := range scope.UserIds {
			if deptId := scope.PrimaryDept[userId]; deptId != "" {
				scopeMemberCount[deptId]++
			}
		}
	}
	items := []gin.H{}
	for _, deptId := range deptIds {
		bucket := buckets[deptId]
		memberCount := 0
		activeUsers := 0
		var quota, requests, fails int64
		if scope.Scope == "admin" {
			memberCount = attribution.MemberCount[deptId]
		} else {
			memberCount = scopeMemberCount[deptId]
		}
		if bucket != nil {
			activeUsers = bucket.activeUsers
			quota = bucket.quota
			requests = bucket.requests
			fails = bucket.fails
		}
		failRate := 0.0
		if requests+fails > 0 {
			failRate = float64(fails) / float64(requests+fails)
		}
		deptName := attribution.DeptNames[deptId]
		if name, ok := scope.DeptNames[deptId]; ok && name != "" {
			deptName = name
		}
		items = append(items, gin.H{
			"dept_id":       deptId,
			"dept_name":     deptName,
			"member_count":  memberCount,
			"active_users":  activeUsers,
			"quota":         quota,
			"request_count": requests,
			"fail_rate":     failRate,
		})
	}
	analyticsSuccess(c, items)
}

func GetAnalyticsModels(c *gin.Context) {
	scope, ok := resolveAnalyticsScope(c)
	if !ok {
		return
	}
	startDate, endDate, ok := analyticsDateRange(c)
	if !ok {
		return
	}
	rows, err := model.QueryUsageByModel(startDate, endDate, scope.UserIds)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Quota > rows[j].Quota })
	const topN = 12
	items := []gin.H{}
	var other model.UsageModelRow
	other.ModelName = "other"
	for i, r := range rows {
		if i < topN {
			items = append(items, gin.H{
				"model_name":        r.ModelName,
				"quota":             r.Quota,
				"request_count":     r.RequestCount,
				"prompt_tokens":     r.PromptTokens,
				"completion_tokens": r.CompletionTokens,
			})
		} else {
			other.Quota += r.Quota
			other.RequestCount += r.RequestCount
			other.PromptTokens += r.PromptTokens
			other.CompletionTokens += r.CompletionTokens
		}
	}
	if other.RequestCount > 0 {
		items = append(items, gin.H{
			"model_name":        other.ModelName,
			"quota":             other.Quota,
			"request_count":     other.RequestCount,
			"prompt_tokens":     other.PromptTokens,
			"completion_tokens": other.CompletionTokens,
		})
	}
	analyticsSuccess(c, items)
}

func GetAnalyticsHeatmap(c *gin.Context) {
	scope, ok := resolveAnalyticsScope(c)
	if !ok {
		return
	}
	startDate, endDate, ok := analyticsDateRange(c)
	if !ok {
		return
	}
	rows, err := model.QueryUsageHourly(startDate, endDate, scope.UserIds)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 7(星期) x 24(小时) 网格;星期几由 date 按本地时区推导。
	type cell struct {
		requests int64
		quota    int64
	}
	grid := make(map[int]map[int]*cell)
	for _, r := range rows {
		day, err := time.ParseInLocation("2006-01-02", r.Date, time.Local)
		if err != nil {
			continue
		}
		hour := r.Hour
		// DST 秋季回拨日有 25 个小时,桶位会算出 hour=24;网格只有 24 列,
		// 并入当天最后一格而不是静默丢弃。
		if hour > 23 {
			hour = 23
		}
		if hour < 0 {
			continue
		}
		dow := int(day.Weekday()) // 0=Sunday
		if grid[dow] == nil {
			grid[dow] = make(map[int]*cell)
		}
		if grid[dow][hour] == nil {
			grid[dow][hour] = &cell{}
		}
		grid[dow][hour].requests += r.RequestCount
		grid[dow][hour].quota += r.Quota
	}
	cells := []gin.H{}
	for dow := 0; dow < 7; dow++ {
		for hour := 0; hour < 24; hour++ {
			entry := gin.H{"day_of_week": dow, "hour": hour, "request_count": 0, "quota": 0}
			if cl := grid[dow][hour]; cl != nil {
				entry["request_count"] = cl.requests
				entry["quota"] = cl.quota
			}
			cells = append(cells, entry)
		}
	}
	analyticsSuccess(c, gin.H{"cells": cells})
}
