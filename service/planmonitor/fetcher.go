package planmonitor

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
)

// sendAdminAlert 告警发送入口,测试中可替换为替身。
var sendAdminAlert = service.SendAdminAlert

// periodLabel 告警文案里的周期中文名。
var periodLabel = map[string]string{
	model.PlanPeriod5Hour:   "每5小时",
	model.PlanPeriodWeekly:  "每周",
	model.PlanPeriodMonthly: "每月",
}

// FetchSummary 一次拉取周期的汇总。
type FetchSummary struct {
	ScannedCount int `json:"scanned_count"`
	FetchedCount int `json:"fetched_count"`
	SkippedCount int `json:"skipped_count"` // 未到刷新间隔
	FailedCount  int `json:"failed_count"`
}

// RunFetchOnce 对所有启用的套餐做一次到期检查并拉取。
// 逐套餐按 RefreshIntervalMin 判断是否到期,避免被最短间隔绑架。
// 成功:覆盖写最新快照并清错误;失败:只记 LastError,保留上次成功快照。
func RunFetchOnce(ctx context.Context) FetchSummary {
	var summary FetchSummary
	plans, err := model.GetEnabledPlanMonitors()
	if err != nil {
		common.SysError("plan monitor: list enabled plans failed: " + err.Error())
		return summary
	}
	now := time.Now().Unix()

	for _, plan := range plans {
		summary.ScannedCount++
		if !due(plan, now) {
			summary.SkippedCount++
			continue
		}
		provider, err := GetProvider(plan.Provider)
		if err != nil {
			summary.FailedCount++
			recordFetchError(plan, err)
			continue
		}
		fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		usages, err := provider.FetchUsage(fetchCtx, plan.ApiUrl, plan.ApiKey, plan.UserAgent)
		cancel()
		if err != nil {
			summary.FailedCount++
			recordFetchError(plan, err)
			continue
		}

		if err := saveFetchSuccess(plan, usages, now); err != nil {
			summary.FailedCount++
			recordFetchError(plan, err)
			continue
		}
		summary.FetchedCount++
	}
	return summary
}

// saveFetchSuccess 成功拉取后的统一落库与告警:
// 1. 按旧快照继承/重置告警去抖状态,判定本次需要告警的周期
// 2. 事务写入快照+历史(见 model.SavePlanMonitorFetch)
// 3. 落库成功后发送告警(发送失败不影响拉取结果)
func saveFetchSuccess(plan *model.PlanMonitor, usages []PeriodUsage, now int64) error {
	oldRows, err := model.GetPlanMonitorUsages(plan.Id)
	if err != nil {
		return err
	}
	oldAlertSentAt := make(map[string]int64, len(oldRows))
	for _, u := range oldRows {
		oldAlertSentAt[u.Period] = u.AlertSentAt
	}

	rows := make([]model.PlanMonitorUsage, 0, len(usages))
	var alertRows []model.PlanMonitorUsage
	for _, u := range usages {
		row := model.PlanMonitorUsage{
			PlanId:           plan.Id,
			Period:           u.Period,
			UsedPercent:      u.UsedPercent,
			RemainingPercent: u.RemainingPercent,
			PeriodEndTime:    u.PeriodEndTime,
			FetchedAt:        now,
		}
		alertSentAt := oldAlertSentAt[u.Period]
		switch {
		case plan.AlertThreshold <= 0:
			// 告警关闭时清掉残留状态,重新开启后按最新用量重新判定
			alertSentAt = 0
		case u.UsedPercent >= float64(plan.AlertThreshold):
			if alertSentAt == 0 {
				alertSentAt = now
				row.AlertSentAt = now
				alertRows = append(alertRows, row)
			}
		default:
			// 用量回落(新周期窗口),重置告警状态允许再次告警
			alertSentAt = 0
		}
		row.AlertSentAt = alertSentAt
		rows = append(rows, row)
	}

	if err := model.SavePlanMonitorFetch(plan.Id, rows); err != nil {
		return err
	}

	// 失败告警恢复:先清零计数与告警状态,再清 last_error/写 last_fetch_time。
	wasAlerting, resetErr := model.ResetPlanMonitorFailAlert(plan.Id)
	if resetErr != nil {
		common.SysError("plan monitor: reset fail alert state failed: " + resetErr.Error())
	}

	if err := model.RecordPlanMonitorFetchResult(plan.Id, nil); err != nil {
		common.SysError("plan monitor: record fetch result failed: " + err.Error())
	}
	for _, row := range alertRows {
		sendUsageAlert(plan, row)
	}
	if wasAlerting {
		sendFetchRecoveredAlert(plan)
	}
	return nil
}

// sendUsageAlert 发送套餐用量超阈值告警。type 带 planId+period 后缀,
// 让每个套餐每个周期独立计数限流(默认每 10 分钟最多 2 条)。
func sendUsageAlert(plan *model.PlanMonitor, usage model.PlanMonitorUsage) {
	period := periodLabel[usage.Period]
	if period == "" {
		period = usage.Period
	}
	notifyType := fmt.Sprintf("%s_%d_%s", dto.NotifyTypePlanUsageThreshold, plan.Id, usage.Period)
	subject := fmt.Sprintf("套餐「%s」%s用量已达 %.1f%%", plan.PlanName, period, usage.UsedPercent)
	content := fmt.Sprintf(
		"套餐「%s」(%s) %s用量已达 %.1f%%,超过告警阈值 %d%%,周期重置时间 %s。请关注用量或及时续费/调整。",
		plan.PlanName, plan.Provider, period, usage.UsedPercent, plan.AlertThreshold,
		time.Unix(usage.PeriodEndTime, 0).Format("2006-01-02 15:04"),
	)
	sendAdminAlert(notifyType, subject, content)
}

// sendFetchFailedAlert 发送套餐拉取连续失败告警。
func sendFetchFailedAlert(plan *model.PlanMonitor, count int, fetchErr error) {
	notifyType := fmt.Sprintf("%s_%d", dto.NotifyTypePlanFetchFailed, plan.Id)
	subject := fmt.Sprintf("套餐「%s」连续拉取失败 %d 次", plan.PlanName, count)
	content := fmt.Sprintf(
		"套餐「%s」(%s) 已连续失败 %d 次(告警阈值 %d 次)。最近错误: %s。请检查 API Key、网络或上游状态。",
		plan.PlanName, plan.Provider, count, plan.FailAlertThreshold, fetchErr.Error(),
	)
	sendAdminAlert(notifyType, subject, content)
}

// sendFetchRecoveredAlert 发送套餐从连续失败中恢复的通知。
func sendFetchRecoveredAlert(plan *model.PlanMonitor) {
	notifyType := fmt.Sprintf("%s_%d", dto.NotifyTypePlanFetchRecovered, plan.Id)
	subject := fmt.Sprintf("套餐「%s」拉取已恢复", plan.PlanName)
	content := fmt.Sprintf(
		"套餐「%s」(%s) 已重新拉取成功,连续失败计数已清零。",
		plan.PlanName, plan.Provider,
	)
	sendAdminAlert(notifyType, subject, content)
}

// due 判断套餐是否到达刷新时间。LastFetchTime 为 0(从未成功)时立即拉取。
func due(plan *model.PlanMonitor, now int64) bool {
	if plan.LastFetchTime == 0 {
		return true
	}
	interval := int64(plan.RefreshIntervalMin)
	if interval <= 0 {
		interval = 5
	}
	return now-plan.LastFetchTime >= interval*60
}

// FetchOneNow 手动立即拉取单个套餐(配置页"立即刷新"),返回错误供前端提示。
func FetchOneNow(ctx context.Context, planId int64) error {
	plan, err := model.GetPlanMonitorById(planId)
	if err != nil {
		return err
	}
	provider, err := GetProvider(plan.Provider)
	if err != nil {
		recordFetchError(plan, err)
		return err
	}
	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	usages, err := provider.FetchUsage(fetchCtx, plan.ApiUrl, plan.ApiKey, plan.UserAgent)
	if err != nil {
		recordFetchError(plan, err)
		return err
	}
	return saveFetchSuccess(plan, usages, time.Now().Unix())
}

func recordFetchError(plan *model.PlanMonitor, err error) {
	common.SysError("plan monitor fetch failed for plan " + strconv.FormatInt(plan.Id, 10) + ": " + err.Error())

	if plan.FailAlertThreshold <= 0 {
		// 失败告警关闭时清残留状态,仍要记录本次错误。
		if dbErr := model.ClearPlanMonitorFailAlertState(plan.Id); dbErr != nil {
			common.SysError("plan monitor: clear fail alert state failed: " + dbErr.Error())
		}
		if dbErr := model.RecordPlanMonitorFetchResult(plan.Id, err); dbErr != nil {
			common.SysError("plan monitor: record fetch error failed: " + dbErr.Error())
		}
		return
	}

	count, dbErr := model.IncrementPlanMonitorFetchFail(plan.Id, err)
	if dbErr != nil {
		common.SysError("plan monitor: increment fetch fail count failed: " + dbErr.Error())
		return
	}
	if count >= plan.FailAlertThreshold {
		won, markErr := model.MarkPlanMonitorFailAlertSent(plan.Id, time.Now().Unix())
		if markErr != nil {
			common.SysError("plan monitor: mark fail alert sent failed: " + markErr.Error())
			return
		}
		if won {
			sendFetchFailedAlert(plan, count, err)
		}
	}
}
