package planmonitor

import (
	"context"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

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

		rows := make([]model.PlanMonitorUsage, 0, len(usages))
		for _, u := range usages {
			rows = append(rows, model.PlanMonitorUsage{
				PlanId:           plan.Id,
				Period:           u.Period,
				UsedPercent:      u.UsedPercent,
				RemainingPercent: u.RemainingPercent,
				PeriodEndTime:    u.PeriodEndTime,
				FetchedAt:        now,
			})
		}
		if err := model.UpsertPlanMonitorUsages(plan.Id, rows); err != nil {
			summary.FailedCount++
			recordFetchError(plan.Id, err)
			continue
		}
		if err := model.RecordPlanMonitorFetchResult(plan.Id, nil); err != nil {
			common.SysError("plan monitor: record fetch result failed: " + err.Error())
		}
		summary.FetchedCount++
	}
	return summary
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
	now := time.Now().Unix()
	rows := make([]model.PlanMonitorUsage, 0, len(usages))
	for _, u := range usages {
		rows = append(rows, model.PlanMonitorUsage{
			PlanId:           plan.Id,
			Period:           u.Period,
			UsedPercent:      u.UsedPercent,
			RemainingPercent: u.RemainingPercent,
			PeriodEndTime:    u.PeriodEndTime,
			FetchedAt:        now,
		})
	}
	if err := model.UpsertPlanMonitorUsages(plan.Id, rows); err != nil {
		recordFetchError(plan.Id, err)
		return err
	}
	return model.RecordPlanMonitorFetchResult(plan.Id, nil)
}

func recordFetchError(planId int64, err error) {
	common.SysError("plan monitor fetch failed for plan " + strconv.FormatInt(planId, 10) + ": " + err.Error())
	if dbErr := model.RecordPlanMonitorFetchResult(planId, err); dbErr != nil {
		common.SysError("plan monitor: record fetch error failed: " + dbErr.Error())
	}
}
