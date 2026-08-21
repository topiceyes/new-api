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

// notifyRootUser 告警发送入口,测试中可替换为替身。
var notifyRootUser = service.NotifyRootUser

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
			recordFetchError(plan.Id, err)
			continue
		}
		fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		usages, err := provider.FetchUsage(fetchCtx, plan.ApiUrl, plan.ApiKey)
		cancel()
		if err != nil {
			summary.FailedCount++
			recordFetchError(plan.Id, err)
			continue
		}

		if err := saveFetchSuccess(plan, usages, now); err != nil {
			summary.FailedCount++
			recordFetchError(plan.Id, err)
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
	if err := model.RecordPlanMonitorFetchResult(plan.Id, nil); err != nil {
		common.SysError("plan monitor: record fetch result failed: " + err.Error())
	}
	for _, row := range alertRows {
		sendUsageAlert(plan, row)
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
	notifyRootUser(notifyType, subject, content)
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
		recordFetchError(plan.Id, err)
		return err
	}
	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	usages, err := provider.FetchUsage(fetchCtx, plan.ApiUrl, plan.ApiKey)
	if err != nil {
		recordFetchError(plan.Id, err)
		return err
	}
	return saveFetchSuccess(plan, usages, time.Now().Unix())
}

func recordFetchError(planId int64, err error) {
	common.SysError("plan monitor fetch failed for plan " + strconv.FormatInt(planId, 10) + ": " + err.Error())
	if dbErr := model.RecordPlanMonitorFetchResult(planId, err); dbErr != nil {
		common.SysError("plan monitor: record fetch error failed: " + dbErr.Error())
	}
}
