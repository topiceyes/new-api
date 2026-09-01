package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/usage_analytics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withUsageAnalyticsSettings 临时改设置,测试后还原。
func withUsageAnalyticsSettings(t *testing.T, mutate func(*usage_analytics.UsageAnalyticsSettings)) {
	t.Helper()
	settings := usage_analytics.GetUsageAnalyticsSettings()
	original := *settings
	mutate(settings)
	t.Cleanup(func() { *settings = original })
}

// 首次运行回填全部缺漏日(含零日志日),水位表落行;
// 紧接着第二轮只重算昨天+今天,不再重扫空窗口。
func TestRunUsageAggregateOnceBackfillThenIncremental(t *testing.T) {
	truncate(t)
	withUsageAnalyticsSettings(t, func(s *usage_analytics.UsageAnalyticsSettings) {
		s.BackfillDays = 5
		s.AggregateRetentionDays = 365
		s.IncludeToday = true
	})
	ctx := context.Background()

	summary, err := RunUsageAggregateOnce(ctx, nil)
	require.NoError(t, err)
	// 回填 [今天-5, 昨天] 共 5 天 + 今天。
	assert.Equal(t, 6, summary["days_aggregated"])

	// 全部 6 天都落了水位,包括没有任何日志的日子。
	localNow := time.Now().In(time.Local)
	todayStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.Local)
	existing, err := model.ListAggregatedDates(todayStart.AddDate(0, 0, -5).Format("2006-01-02"))
	require.NoError(t, err)
	assert.Len(t, existing, 6)

	summary, err = RunUsageAggregateOnce(ctx, nil)
	require.NoError(t, err)
	// 第二轮只剩昨天重算 + 今天刷新,空日志日不再重复聚合。
	assert.Equal(t, 2, summary["days_aggregated"])
}

// retention < backfill 时回填下限被钳到保留期: 更老的窗口聚合完立刻被清理,
// 不钳会每小时永久重扫 [今天-backfill, 今天-retention) 这段空转区间。
func TestRunUsageAggregateOnceClampsBackfillToRetention(t *testing.T) {
	truncate(t)
	withUsageAnalyticsSettings(t, func(s *usage_analytics.UsageAnalyticsSettings) {
		s.BackfillDays = 90
		s.AggregateRetentionDays = 10
		s.IncludeToday = false
	})

	summary, err := RunUsageAggregateOnce(context.Background(), nil)
	require.NoError(t, err)
	// 回填窗口被钳到 [今天-10, 昨天] 共 10 天,而不是 90 天。
	assert.Equal(t, 10, summary["days_aggregated"])

	localNow := time.Now().In(time.Local)
	todayStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.Local)
	existing, err := model.ListAggregatedDates(todayStart.AddDate(0, 0, -90).Format("2006-01-02"))
	require.NoError(t, err)
	assert.Len(t, existing, 10)
	_, ok := existing[todayStart.AddDate(0, 0, -11).Format("2006-01-02")]
	assert.False(t, ok)
}
