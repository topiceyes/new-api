package planmonitor

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubSendAdminAlert 替换 fan-out 入口,返回捕获切片指针。
func stubSendAdminAlert(t *testing.T) *[]capturedNotify {
	t.Helper()
	captured := &[]capturedNotify{}
	original := sendAdminAlert
	sendAdminAlert = func(notifyType, subject, content string) {
		*captured = append(*captured, capturedNotify{notifyType: notifyType, subject: subject, content: content})
	}
	t.Cleanup(func() { sendAdminAlert = original })
	return captured
}

func countFailedAlerts(notifies *[]capturedNotify, planId int64) int {
	prefix := fmt.Sprintf("%s_%d", dto.NotifyTypePlanFetchFailed, planId)
	count := 0
	for _, n := range *notifies {
		if n.notifyType == prefix {
			count++
		}
	}
	return count
}

func countRecoveredAlerts(notifies *[]capturedNotify, planId int64) int {
	prefix := fmt.Sprintf("%s_%d", dto.NotifyTypePlanFetchRecovered, planId)
	count := 0
	for _, n := range *notifies {
		if n.notifyType == prefix {
			count++
		}
	}
	return count
}

func TestFetchFailAlert_FiresOnceAfterThreshold(t *testing.T) {
	db := setupPlanMonitorTest(t)
	fake := &fakeProvider{name: "fail-alert", err: errors.New("upstream 403")}
	registerFake(t, fake)
	notifies := stubSendAdminAlert(t)

	plan := seedPlan(t, db, "fail-alert", 0, 0)
	require.NoError(t, db.Model(plan).Update("fail_alert_threshold", 3).Error)

	for i := 0; i < 3; i++ {
		require.Error(t, FetchOneNow(context.Background(), plan.Id))
	}

	require.Equal(t, 1, countFailedAlerts(notifies, plan.Id), "达到阈值后应只发一次失败告警")
	assert.Contains(t, (*notifies)[0].subject, "连续拉取失败 3 次")

	reloaded, err := model.GetPlanMonitorById(plan.Id)
	require.NoError(t, err)
	assert.Equal(t, 3, reloaded.FetchFailCount)
	assert.Greater(t, reloaded.FailAlertSentAt, int64(0))
}

func TestFetchFailAlert_DebouncesAfterSent(t *testing.T) {
	db := setupPlanMonitorTest(t)
	fake := &fakeProvider{name: "fail-debounce", err: errors.New("upstream 500")}
	registerFake(t, fake)
	notifies := stubSendAdminAlert(t)

	plan := seedPlan(t, db, "fail-debounce", 0, 0)
	require.NoError(t, db.Model(plan).Update("fail_alert_threshold", 2).Error)

	for i := 0; i < 5; i++ {
		require.Error(t, FetchOneNow(context.Background(), plan.Id))
	}

	assert.Equal(t, 1, countFailedAlerts(notifies, plan.Id), "已发送后应去抖,不再重复告警")
}

func TestFetchFailAlert_RecoversAndResets(t *testing.T) {
	db := setupPlanMonitorTest(t)
	fake := &fakeProvider{name: "fail-recover", err: errors.New("upstream timeout")}
	registerFake(t, fake)
	notifies := stubSendAdminAlert(t)

	plan := seedPlan(t, db, "fail-recover", 0, 0)
	require.NoError(t, db.Model(plan).Update("fail_alert_threshold", 2).Error)

	require.Error(t, FetchOneNow(context.Background(), plan.Id))
	require.Error(t, FetchOneNow(context.Background(), plan.Id))
	require.Equal(t, 1, countFailedAlerts(notifies, plan.Id))

	fake.err = nil
	fake.usages = []PeriodUsage{{Period: model.PlanPeriod5Hour, UsedPercent: 10, RemainingPercent: 90, PeriodEndTime: time.Now().Unix() + 3600}}
	require.NoError(t, FetchOneNow(context.Background(), plan.Id))

	require.Equal(t, 1, countRecoveredAlerts(notifies, plan.Id), "恢复成功后应发送一次已恢复通知")

	reloaded, err := model.GetPlanMonitorById(plan.Id)
	require.NoError(t, err)
	assert.Equal(t, 0, reloaded.FetchFailCount)
	assert.Equal(t, int64(0), reloaded.FailAlertSentAt)
	assert.Empty(t, reloaded.LastError)

	fake.err = errors.New("upstream timeout again")
	require.Error(t, FetchOneNow(context.Background(), plan.Id))
	require.Error(t, FetchOneNow(context.Background(), plan.Id))
	assert.Equal(t, 2, countFailedAlerts(notifies, plan.Id), "重置后应能再次告警")
}

func TestFetchFailAlert_DisabledNeverAlerts(t *testing.T) {
	db := setupPlanMonitorTest(t)
	fake := &fakeProvider{name: "fail-disabled", err: errors.New("upstream 401")}
	registerFake(t, fake)
	notifies := stubSendAdminAlert(t)

	plan := seedPlan(t, db, "fail-disabled", 0, 0)
	require.NoError(t, db.Model(plan).Update("fail_alert_threshold", 0).Error)

	for i := 0; i < 3; i++ {
		require.Error(t, FetchOneNow(context.Background(), plan.Id))
	}

	assert.Empty(t, *notifies, "threshold=0 时不应产生告警")

	reloaded, err := model.GetPlanMonitorById(plan.Id)
	require.NoError(t, err)
	assert.Equal(t, 0, reloaded.FetchFailCount)
	assert.Equal(t, int64(0), reloaded.FailAlertSentAt)
}

func TestFetchFailAlert_NoRecoveryWithoutPriorAlert(t *testing.T) {
	db := setupPlanMonitorTest(t)
	fake := &fakeProvider{
		name:   "fail-no-prior",
		usages: []PeriodUsage{{Period: model.PlanPeriod5Hour, UsedPercent: 20, RemainingPercent: 80, PeriodEndTime: time.Now().Unix() + 3600}},
	}
	registerFake(t, fake)
	notifies := stubSendAdminAlert(t)

	plan := seedPlan(t, db, "fail-no-prior", 0, 0)
	require.NoError(t, db.Model(plan).Update("fail_alert_threshold", 2).Error)

	require.NoError(t, FetchOneNow(context.Background(), plan.Id))

	assert.Empty(t, *notifies, "从未失败过时成功拉取不应发恢复通知")
	reloaded, err := model.GetPlanMonitorById(plan.Id)
	require.NoError(t, err)
	assert.Equal(t, 0, reloaded.FetchFailCount)
	assert.Equal(t, int64(0), reloaded.FailAlertSentAt)
}
