package planmonitor

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capturedNotify 记录一次告警发送。
type capturedNotify struct {
	notifyType string
	subject    string
	content    string
}

func fetchNow(t *testing.T, planId int64) {
	t.Helper()
	require.NoError(t, FetchOneNow(context.Background(), planId))
}

func TestUsageAlert_FiresOnceResetsAfterDrop(t *testing.T) {
	db := setupPlanMonitorTest(t)
	fake := &fakeProvider{
		name: "alert-test",
		usages: []PeriodUsage{
			{Period: model.PlanPeriod5Hour, UsedPercent: 95, RemainingPercent: 5, PeriodEndTime: time.Now().Unix() + 3600},
		},
	}
	registerFake(t, fake)
	notifies := stubSendAdminAlert(t)

	plan := seedPlan(t, db, "alert-test", 5, 0)
	require.NoError(t, db.Model(plan).Update("alert_threshold", 90).Error)

	// 超阈值:告警一次,快照记录告警状态
	fetchNow(t, plan.Id)
	require.Len(t, *notifies, 1)
	assert.Contains(t, (*notifies)[0].subject, "95.0%")
	assert.Contains(t, (*notifies)[0].notifyType, "plan_usage_threshold")
	var usage model.PlanMonitorUsage
	require.NoError(t, db.Where("plan_id = ?", plan.Id).First(&usage).Error)
	assert.Greater(t, usage.AlertSentAt, int64(0))

	// 持续超阈值:不重复告警
	fetchNow(t, plan.Id)
	assert.Len(t, *notifies, 1)

	// 用量回落:告警状态重置,不告警
	fake.usages[0].UsedPercent = 50
	fetchNow(t, plan.Id)
	assert.Len(t, *notifies, 1)
	require.NoError(t, db.Where("plan_id = ?", plan.Id).First(&usage).Error)
	assert.Equal(t, int64(0), usage.AlertSentAt)

	// 再次超阈值:重新告警
	fake.usages[0].UsedPercent = 96
	fetchNow(t, plan.Id)
	assert.Len(t, *notifies, 2)
}

func TestUsageAlert_DisabledThresholdNeverAlerts(t *testing.T) {
	db := setupPlanMonitorTest(t)
	fake := &fakeProvider{
		name:   "alert-disabled-test",
		usages: []PeriodUsage{{Period: model.PlanPeriod5Hour, UsedPercent: 99}},
	}
	registerFake(t, fake)
	notifies := stubSendAdminAlert(t)

	// alert_threshold 默认 0 = 不告警
	plan := seedPlan(t, db, "alert-disabled-test", 5, 0)
	fetchNow(t, plan.Id)
	assert.Empty(t, *notifies)
	var usage model.PlanMonitorUsage
	require.NoError(t, db.Where("plan_id = ?", plan.Id).First(&usage).Error)
	assert.Equal(t, int64(0), usage.AlertSentAt)
}

func TestSaveFetchSuccess_WritesHistoryAndPrunes(t *testing.T) {
	db := setupPlanMonitorTest(t)
	fake := &fakeProvider{
		name:   "history-test",
		usages: []PeriodUsage{{Period: model.PlanPeriodWeekly, UsedPercent: 60, RemainingPercent: 40}},
	}
	registerFake(t, fake)
	stubSendAdminAlert(t)

	plan := seedPlan(t, db, "history-test", 5, 0)
	// 预置一条 31 天前的历史,应被清理
	old := model.PlanMonitorUsageHistory{
		PlanId: plan.Id, Period: model.PlanPeriodWeekly,
		FetchedAt: time.Now().Add(-31 * 24 * time.Hour).Unix(), UsedPercent: 10,
	}
	require.NoError(t, db.Create(&old).Error)

	fetchNow(t, plan.Id)
	fetchNow(t, plan.Id)

	var histories []model.PlanMonitorUsageHistory
	require.NoError(t, db.Where("plan_id = ?", plan.Id).Order("fetched_at asc").Find(&histories).Error)
	require.Len(t, histories, 2, "两次拉取各写一行历史,过期行被清理")
	for _, h := range histories {
		assert.Equal(t, 60.0, h.UsedPercent)
	}

	// 趋势查询:24h 返回原始点
	points, err := GetUsageHistoryPoints(plan.Id, model.PlanPeriodWeekly, 24)
	require.NoError(t, err)
	assert.Len(t, points, 2)
	// 7d 按小时聚合,同小时两点合并为一个
	points7d, err := GetUsageHistoryPoints(plan.Id, model.PlanPeriodWeekly, 24*7)
	require.NoError(t, err)
	assert.Len(t, points7d, 1)
	assert.Equal(t, 60.0, points7d[0].UsedPercent)
}
