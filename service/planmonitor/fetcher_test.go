package planmonitor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fakeProvider 可编程的测试 provider:按 plan 的 api_key 决定成功/失败与返回值。
type fakeProvider struct {
	name   string
	usages []PeriodUsage
	err    error
	calls  int
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) FetchUsage(ctx context.Context, apiUrl, apiKey, userAgent string) ([]PeriodUsage, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.usages, nil
}

func setupPlanMonitorTest(t *testing.T) *gorm.DB {
	t.Helper()
	originalDB := model.DB
	originalMemoryCache := common.MemoryCacheEnabled
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PlanMonitor{}, &model.PlanMonitorUsage{}, &model.PlanMonitorUsageHistory{}))
	model.DB = db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = originalDB
		common.MemoryCacheEnabled = originalMemoryCache
		sqlDB, err := db.DB()
		if err == nil {
			require.NoError(t, sqlDB.Close())
		}
	})
	return db
}

func seedPlan(t *testing.T, db *gorm.DB, provider string, refreshMin int, lastFetch int64) *model.PlanMonitor {
	t.Helper()
	p := &model.PlanMonitor{
		Provider:           provider,
		PlanName:           "test-plan",
		ApiUrl:             "https://api.example.com",
		ApiKey:             "sk-test",
		RefreshIntervalMin: refreshMin,
		Enabled:            true,
		LastFetchTime:      lastFetch,
	}
	require.NoError(t, db.Create(p).Error)
	return p
}

func registerFake(t *testing.T, f *fakeProvider) {
	t.Helper()
	providers[f.name] = f
	t.Cleanup(func() { delete(providers, f.name) })
}

func TestRunFetchOnce_SuccessWritesSnapshotAndClearsError(t *testing.T) {
	db := setupPlanMonitorTest(t)
	fake := &fakeProvider{
		name: "fake_ok",
		usages: []PeriodUsage{
			{Period: model.PlanPeriod5Hour, UsedPercent: 60, RemainingPercent: 40, PeriodEndTime: 1760000000},
			{Period: model.PlanPeriodWeekly, UsedPercent: 20, RemainingPercent: 80, PeriodEndTime: 1760600000},
		},
	}
	registerFake(t, fake)
	plan := seedPlan(t, db, "fake_ok", 5, 0) // LastFetchTime=0 → 立即拉取

	summary := RunFetchOnce(context.Background())
	require.Equal(t, 1, summary.ScannedCount)
	require.Equal(t, 1, summary.FetchedCount)
	require.Equal(t, 0, summary.FailedCount)
	require.Equal(t, 1, fake.calls)

	usages, err := model.GetPlanMonitorUsages(plan.Id)
	require.NoError(t, err)
	require.Len(t, usages, 2)

	reloaded, err := model.GetPlanMonitorById(plan.Id)
	require.NoError(t, err)
	assert.Greater(t, reloaded.LastFetchTime, int64(0), "success should set last_fetch_time")
	assert.Empty(t, reloaded.LastError, "success should clear last_error")
}

func TestRunFetchOnce_FailureKeepsOldSnapshot(t *testing.T) {
	db := setupPlanMonitorTest(t)
	// 先放一个旧快照
	old := []model.PlanMonitorUsage{
		{PlanId: 0, Period: model.PlanPeriod5Hour, UsedPercent: 30, RemainingPercent: 70, PeriodEndTime: 1750000000},
	}
	fake := &fakeProvider{name: "fake_fail", err: errors.New("upstream 401")}
	registerFake(t, fake)
	plan := seedPlan(t, db, "fake_fail", 5, 0)
	old[0].PlanId = plan.Id
	require.NoError(t, db.Create(&old).Error)

	summary := RunFetchOnce(context.Background())
	require.Equal(t, 1, summary.FailedCount)
	require.Equal(t, 0, summary.FetchedCount)

	// 旧快照未被清除
	usages, err := model.GetPlanMonitorUsages(plan.Id)
	require.NoError(t, err)
	require.Len(t, usages, 1, "old snapshot must be preserved on failure")
	assert.InDelta(t, 30, usages[0].UsedPercent, 0.001)

	// 错误被记录
	reloaded, err := model.GetPlanMonitorById(plan.Id)
	require.NoError(t, err)
	assert.Contains(t, reloaded.LastError, "upstream 401")
	assert.Equal(t, int64(0), reloaded.LastFetchTime, "failed fetch must not set last_fetch_time")
}

func TestRunFetchOnce_SkipsNotDue(t *testing.T) {
	db := setupPlanMonitorTest(t)
	fake := &fakeProvider{name: "fake_skip", usages: []PeriodUsage{{Period: model.PlanPeriod5Hour}}}
	registerFake(t, fake)
	// 刚拉过(lastFetch=now),间隔 5 分钟,不应再拉
	seedPlan(t, db, "fake_skip", 5, time.Now().Unix())

	summary := RunFetchOnce(context.Background())
	assert.Equal(t, 1, summary.SkippedCount)
	assert.Equal(t, 0, summary.FetchedCount)
	assert.Equal(t, 0, fake.calls, "not-due plan must not be fetched")
}

func TestRunFetchOnce_UnsupportedProviderMarkedFailed(t *testing.T) {
	db := setupPlanMonitorTest(t)
	plan := seedPlan(t, db, "no_such_provider", 5, 0)

	summary := RunFetchOnce(context.Background())
	assert.Equal(t, 1, summary.FailedCount)

	reloaded, err := model.GetPlanMonitorById(plan.Id)
	require.NoError(t, err)
	assert.Contains(t, reloaded.LastError, "unsupported plan monitor provider")
}

func TestDueLogic(t *testing.T) {
	now := time.Now().Unix()
	assert.True(t, due(&model.PlanMonitor{LastFetchTime: 0}, now), "never fetched → due")
	assert.True(t, due(&model.PlanMonitor{LastFetchTime: now - 400, RefreshIntervalMin: 5}, now), ">5min → due")
	assert.False(t, due(&model.PlanMonitor{LastFetchTime: now - 100, RefreshIntervalMin: 5}, now), "<5min → not due")
	assert.True(t, due(&model.PlanMonitor{LastFetchTime: now - 400, RefreshIntervalMin: 0}, now), "interval 0 → default 5min, due")
}
