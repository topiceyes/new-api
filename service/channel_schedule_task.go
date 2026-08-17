package service

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// ChannelScheduleRunSummary 调度器单次执行的统计。
type ChannelScheduleRunSummary struct {
	ScannedCount   int `json:"scanned_count"`
	DisabledCount  int `json:"disabled_count"`
	EnabledCount   int `json:"enabled_count"`
	RestoredCount  int `json:"restored_count"`
	ParseErrorSkip int `json:"parse_error_skip"`
}

// RunChannelScheduleOnce 调和一次所有渠道的定时开关状态。供 SystemTask handler 调用，也可直接单测。
// 决策表：
//
//	status==1, desired disable  → 置 2 (reason: scheduled_off: ...)
//	status==1, desired enable   → 无
//	status==2 (scheduled_off), desired enable  → 置 1
//	status==2 (scheduled_off), desired disable → 无
//	status==2 (manual)         → 无（人工优先）
//	status==3                  → 无（熔断优先）
//
// 另：若 schedule 被关闭/清空但渠道仍处于 scheduled_off 状态，恢复为 1。
func RunChannelScheduleOnce(ctx context.Context) ChannelScheduleRunSummary {
	summary := ChannelScheduleRunSummary{}

	channels, err := model.GetChannelsWithEnabledSchedules()
	if err != nil {
		common.SysLog(fmt.Sprintf("channel schedule reconcile: query failed: %v", err))
		return summary
	}

	now := time.Now()
	changed := false
	for _, ch := range channels {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		summary.ScannedCount++

		sch := ch.GetSchedule()
		if sch == nil {
			summary.ParseErrorSkip++
			continue
		}

		if !sch.Enabled || len(sch.Windows) == 0 {
			// 调度被关闭或配置清空：若该渠道处于 scheduled_off 则恢复
			if ch.IsScheduledOff() {
				if restoreScheduledChannel(ch) {
					summary.RestoredCount++
					changed = true
				}
			}
			continue
		}

		desiredEnabled := sch.IsActiveNow(now)

		switch {
		case ch.Status == common.ChannelStatusEnabled && !desiredEnabled:
			if disableForSchedule(ch) {
				summary.DisabledCount++
				changed = true
			}
		case ch.Status == common.ChannelStatusManuallyDisabled && desiredEnabled:
			if ch.IsScheduledOff() {
				if enableForSchedule(ch) {
					summary.EnabledCount++
					changed = true
				}
			}
		}
	}

	if changed {
		// 多实例 cache 漂移双保险；UpdateChannelStatus 内部已处理 cache status + abilities。
		model.InitChannelCache()
	}
	return summary
}

// disableForSchedule 将渠道置为 ManuallyDisabled 并写入 scheduled_off reason。
func disableForSchedule(ch *model.Channel) bool {
	reason := model.ChannelScheduleOffReasonPrefix + ": outside window"
	ok := model.UpdateChannelStatus(ch.Id, "", common.ChannelStatusManuallyDisabled, reason)
	if ok {
		common.SysLog(fmt.Sprintf("通道「%s」（#%d）定时关闭：不在运行时段", ch.Name, ch.Id))
	}
	return ok
}

// enableForSchedule 将 scheduled_off 渠道恢复为 Enabled。reason 留空，status_reason 仍带 scheduled_off 前缀由前端显示来源。
func enableForSchedule(ch *model.Channel) bool {
	ok := model.UpdateChannelStatus(ch.Id, "", common.ChannelStatusEnabled, "")
	if ok {
		common.SysLog(fmt.Sprintf("通道「%s」（#%d）定时开启：回到运行时段", ch.Name, ch.Id))
	}
	return ok
}

// restoreScheduledChannel 把 scheduled_off 渠道恢复为 Enabled（用于调度被关闭或配置清空场景）。
func restoreScheduledChannel(ch *model.Channel) bool {
	ok := model.UpdateChannelStatus(ch.Id, "", common.ChannelStatusEnabled, "")
	if ok {
		common.SysLog(fmt.Sprintf("通道「%s」（#%d）定时开关已关闭，恢复启用", ch.Name, ch.Id))
	}
	return ok
}