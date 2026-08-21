package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupPublicPlanMonitorDB 为本测试换一个全新的内存 sqlite 并建表,结束恢复原 model.DB。
// 不能复用包内共享的 model.DB:其他测试可能已将其关闭(sql: database is closed)。
func setupPublicPlanMonitorDB(t *testing.T) {
	t.Helper()
	prev := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.PlanMonitor{},
		&model.PlanMonitorUsage{},
		&model.PlanMonitorUsageHistory{},
	))
	model.DB = db
	t.Cleanup(func() {
		model.DB = prev
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
}

func seedPlanMonitor(t *testing.T, provider string, enabled, isPublic bool, sortOrder int) *model.PlanMonitor {
	t.Helper()
	p := &model.PlanMonitor{
		Provider:  provider,
		PlanName:  provider + "-plan",
		ApiKey:    "sk-test",
		Enabled:   enabled,
		IsPublic:  isPublic,
		SortOrder: sortOrder,
	}
	require.NoError(t, model.CreatePlanMonitor(p))
	return p
}

func doPublicOverview() *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/plan_monitor/overview", nil)
	GetPublicPlanMonitorOverview(c)
	return rec
}

func doPublicHistory(id int64, period, range_ string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/plan_monitor/plans/"+itoa(id)+"/history?period="+period+"&range="+range_,
		nil,
	)
	c.Params = gin.Params{{Key: "id", Value: itoa(id)}}
	GetPublicPlanMonitorHistory(c)
	return rec
}

func itoa(i int64) string {
	// strconv.FormatInt 即可,这里仅避免再 import 一个包
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// 公开 overview 仅返回公开+启用的套餐,字段裁剪为 {id, provider, plan_name, usages},
// 不得泄漏 api_url / api_key_masked / alert_threshold / last_error 等管理字段。
func TestGetPublicPlanMonitorOverview_FiltersAndFieldTrim(t *testing.T) {
	setupPublicPlanMonitorDB(t)

	publicOn := seedPlanMonitor(t, "kimi", true, true, 0)
	require.NoError(t, model.UpsertPlanMonitorUsages(publicOn.Id, []model.PlanMonitorUsage{
		{PlanId: publicOn.Id, Period: model.PlanPeriod5Hour, UsedPercent: 12.5, RemainingPercent: 87.5, FetchedAt: time.Now().Unix()},
	}))
	seedPlanMonitor(t, "minimax", false, true, 0)  // 公开但停用
	seedPlanMonitor(t, "volcengine", true, false, 0) // 启用但未公开
	// 给被过滤掉的也塞 usage/history,确认不会泄漏
	hidden := seedPlanMonitor(t, "bigmodel", true, true, 0)
	require.NoError(t, model.UpsertPlanMonitorUsages(hidden.Id, []model.PlanMonitorUsage{
		{PlanId: hidden.Id, Period: model.PlanPeriodWeekly, UsedPercent: 99, RemainingPercent: 1, FetchedAt: time.Now().Unix()},
	}))
	// 翻回 hidden 为未公开,模拟管理员改配置
	hidden.IsPublic = false
	require.NoError(t, model.UpdatePlanMonitor(hidden))

	rec := doPublicOverview()
	assert.Equal(t, http.StatusOK, rec.Code)

	var payload struct {
		Success bool   `json:"success"`
		Data    struct {
			Groups []struct {
				Provider string `json:"provider"`
				Plans    []map[string]any `json:"plans"`
			} `json:"groups"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Len(t, payload.Data.Groups, 1, "只有一个公开+启用的供应商")
	assert.Equal(t, "kimi", payload.Data.Groups[0].Provider)
	require.Len(t, payload.Data.Groups[0].Plans, 1)
	plan := payload.Data.Groups[0].Plans[0]
	// 仅含裁剪字段
	assert.Contains(t, plan, "id")
	assert.Contains(t, plan, "provider")
	assert.Contains(t, plan, "plan_name")
	assert.Contains(t, plan, "usages")
	for _, forbidden := range []string{
		"api_url", "api_key_masked", "refresh_interval_min",
		"alert_threshold", "enabled", "is_public",
		"created_time", "updated_time", "last_fetch_time", "last_error",
	} {
		assert.NotContains(t, plan, forbidden, "公开 DTO 不应包含管理字段 "+forbidden)
	}
}

// 历史接口:仅公开+启用的套餐可查;非公开/已停用/不存在统一报「套餐不存在」。
func TestGetPublicPlanMonitorHistory_RejectsNonPublicAndDisabled(t *testing.T) {
	setupPublicPlanMonitorDB(t)

	publicOn := seedPlanMonitor(t, "kimi", true, true, 0)
	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&[]model.PlanMonitorUsageHistory{
		{PlanId: publicOn.Id, Period: model.PlanPeriod5Hour, FetchedAt: now - 600, UsedPercent: 10, RemainingPercent: 90},
		{PlanId: publicOn.Id, Period: model.PlanPeriod5Hour, FetchedAt: now - 60, UsedPercent: 30, RemainingPercent: 70},
	}).Error)

	publicOff := seedPlanMonitor(t, "minimax", false, true, 0) // 停用
	hidden := seedPlanMonitor(t, "bigmodel", true, false, 0)   // 未公开

	cases := []struct {
		name string
		id   int64
	}{
		{"disabled", publicOff.Id},
		{"not_public", hidden.Id},
		{"not_found", 999999},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doPublicHistory(tc.id, model.PlanPeriod5Hour, "24h")
			assert.Equal(t, http.StatusOK, rec.Code)
			var payload struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &payload))
			assert.False(t, payload.Success)
			assert.Equal(t, "套餐不存在", payload.Message, "必须统一报错,防探测")
		})
	}

	// 公开+启用:正常返回 points
	rec := doPublicHistory(publicOn.Id, model.PlanPeriod5Hour, "24h")
	assert.Equal(t, http.StatusOK, rec.Code)
	var ok struct {
		Success bool `json:"success"`
		Data    struct {
			Points []map[string]any `json:"points"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &ok))
	require.True(t, ok.Success)
	assert.Len(t, ok.Data.Points, 2)
}

// 缺 period → 报错;range 非法 → 静默回退到 24h,正常返回。
func TestGetPublicPlanMonitorHistory_Validation(t *testing.T) {
	setupPublicPlanMonitorDB(t)
	p := seedPlanMonitor(t, "kimi", true, true, 0)

	// 缺 period
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/plan_monitor/plans/"+itoa(p.Id)+"/history", nil)
	c.Params = gin.Params{{Key: "id", Value: itoa(p.Id)}}
	GetPublicPlanMonitorHistory(c)
	require.Equal(t, http.StatusOK, rec.Code)
	var miss struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &miss))
	assert.False(t, miss.Success)
	assert.Contains(t, miss.Message, "period")

	// 非法 range → 回退 24h
	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&model.PlanMonitorUsageHistory{
		PlanId: p.Id, Period: model.PlanPeriodWeekly,
		FetchedAt: now - 60, UsedPercent: 50, RemainingPercent: 50,
	}).Error)
	rec2 := doPublicHistory(p.Id, model.PlanPeriodWeekly, "bogus")
	require.Equal(t, http.StatusOK, rec2.Code)
	var ok struct {
		Success bool `json:"success"`
		Data    struct {
			Points []map[string]any `json:"points"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(rec2.Body.Bytes(), &ok))
	require.True(t, ok.Success)
	assert.Len(t, ok.Data.Points, 1)
}
