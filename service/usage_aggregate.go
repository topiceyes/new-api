package service

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/usage_analytics"
)

// UsageAggregationDay 一个待聚合的本地日历日,[Start, End) 为 unix 秒窗口。
type UsageAggregationDay struct {
	Date  string
	Start int64
	End   int64
}

const usageStatDateLayout = "2006-01-02"

// PlanUsageAggregationDays 计算本轮要聚合的日期清单(纯函数,便于单测):
//  1. [今天-backfillDays, 昨天] 中 existing 里没有的日期(首次运行=历史回填,之后的运行=补缺);
//  2. 总是重算昨天(迟到日志);
//  3. includeToday 时追加今天(部分天,窗口到 now,每次运行覆盖重写)。
// 日界按服务器本地时区。
func PlanUsageAggregationDays(now time.Time, backfillDays int, includeToday bool, existing map[string]bool) []UsageAggregationDay {
	localNow := now.In(time.Local)
	todayStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.Local)
	yesterdayStart := todayStart.AddDate(0, 0, -1)

	var days []UsageAggregationDay
	if backfillDays > 0 {
		earliest := todayStart.AddDate(0, 0, -backfillDays)
		for d := earliest; d.Before(todayStart); d = d.AddDate(0, 0, 1) {
			if !existing[d.Format(usageStatDateLayout)] {
				days = append(days, UsageAggregationDay{
					Date:  d.Format(usageStatDateLayout),
					Start: d.Unix(),
					End:   d.AddDate(0, 0, 1).Unix(),
				})
			}
		}
	}
	// 昨天总是重算;若恰好也在补缺清单里(首次运行回填窗口含昨天)会重复,先去重再追加。
	yesterdayDate := yesterdayStart.Format(usageStatDateLayout)
	alreadyPlanned := false
	for _, d := range days {
		if d.Date == yesterdayDate {
			alreadyPlanned = true
			break
		}
	}
	if !alreadyPlanned {
		days = append(days, UsageAggregationDay{
			Date:  yesterdayDate,
			Start: yesterdayStart.Unix(),
			End:   todayStart.Unix(),
		})
	}
	if includeToday {
		days = append(days, UsageAggregationDay{
			Date:  todayStart.Format(usageStatDateLayout),
			Start: todayStart.Unix(),
			End:   localNow.Unix(),
		})
	}
	return days
}

// RunUsageAggregateOnce 执行一轮使用分析聚合:补缺回填 + 重算昨天/今天 + 清理过期聚合。
// reporter 为系统任务进度回调(可为 nil)。整个循环尊重 ctx 取消(lease 丢失时退出)。
func RunUsageAggregateOnce(ctx context.Context, reporter func(processed, total int)) (map[string]any, error) {
	settings := usage_analytics.GetUsageAnalyticsSettings()
	now := time.Now()
	localNow := now.In(time.Local)
	todayStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.Local)

	backfillDays := settings.BackfillDays
	if backfillDays < 0 {
		backfillDays = 0
	}
	// 比保留期更老的窗口聚合完立刻被 DeleteUsageStatsBefore 删掉,纯属白扫;
	// 回填下限钳到保留期之后,避免 retention < backfill 时每小时永久空转。
	if settings.AggregateRetentionDays > 0 && settings.AggregateRetentionDays < backfillDays {
		backfillDays = settings.AggregateRetentionDays
	}
	earliest := todayStart.AddDate(0, 0, -backfillDays)
	existing, err := model.ListAggregatedDates(earliest.Format(usageStatDateLayout))
	if err != nil {
		return nil, err
	}

	days := PlanUsageAggregationDays(now, backfillDays, settings.IncludeToday, existing)
	for i, day := range days {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		daily, hourly, err := model.AggregateLogsForDay(ctx, day.Start, day.End)
		if err != nil {
			return nil, err
		}
		if err := model.ReplaceUsageStatsForDay(day.Date, daily, hourly); err != nil {
			return nil, err
		}
		if reporter != nil {
			reporter(i+1, len(days))
		}
	}

	deleted := false
	if settings.AggregateRetentionDays > 0 {
		cutoff := todayStart.AddDate(0, 0, -settings.AggregateRetentionDays)
		if err := model.DeleteUsageStatsBefore(cutoff.Format(usageStatDateLayout)); err != nil {
			return nil, err
		}
		deleted = true
	}

	return map[string]any{
		"days_aggregated": len(days),
		"retention_swept": deleted,
	}, nil
}
