package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/audit"
	"github.com/QuantumNous/new-api/service/planmonitor"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// RegisterScheduledSystemTasks wires the periodic channel test, upstream model
// update, and async task polling (Midjourney / Suno / video) jobs into the
// system task framework so a DB lease dedups execution across multiple master
// instances and each run is recorded as one task row. Call this before
// service.StartSystemTaskRunner.
func RegisterScheduledSystemTasks() {
	service.RegisterSystemTaskHandler(channelTestHandler{})
	service.RegisterSystemTaskHandler(modelUpdateHandler{})
	service.RegisterSystemTaskHandler(midjourneyPollHandler{})
	service.RegisterSystemTaskHandler(asyncTaskPollHandler{})
	service.RegisterSystemTaskHandler(channelScheduleHandler{})
	service.RegisterSystemTaskHandler(planMonitorHandler{})
	service.RegisterSystemTaskHandler(orgSyncHandler{})
	service.RegisterSystemTaskHandler(auditCleanupHandler{})
	service.RegisterSystemTaskHandler(auditClassifyHandler{})
}

// channelTestHandler runs the scheduled "test all channels" job. Enablement and
// cadence still come from the monitor settings; only the execution path moved
// into the system task runner.
type channelTestHandler struct{}

func (channelTestHandler) Type() string { return model.SystemTaskTypeChannelTest }

func (channelTestHandler) Enabled() bool {
	return operation_setting.GetMonitorSetting().AutoTestChannelEnabled
}

func (channelTestHandler) Interval() time.Duration {
	minutes := operation_setting.GetMonitorSetting().AutoTestChannelMinutes
	if minutes <= 0 {
		minutes = 10
	}
	return time.Duration(minutes * float64(time.Minute))
}

func (channelTestHandler) NewPayload() any { return nil }

// channelTestTaskPayload controls one channel_test run. A nil/empty payload is a
// scheduled run, which uses the configured monitor ChannelTestMode and does not
// notify. A manual "test all channels" trigger sets Mode=scheduled_all and
// Notify=true to reproduce the legacy manual behavior (test every channel and
// notify root on completion).
type channelTestTaskPayload struct {
	Mode   string `json:"mode,omitempty"`
	Notify bool   `json:"notify,omitempty"`
}

func (channelTestHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := channelTestTaskPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	summary, err := runChannelTestTask(ctx, payload.Mode, payload.Notify, service.NewSystemTaskProgressReporter(task, runnerID))
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// modelUpdateHandler runs the scheduled upstream model update detection job.
type modelUpdateHandler struct{}

func (modelUpdateHandler) Type() string { return model.SystemTaskTypeModelUpdate }

func (modelUpdateHandler) Enabled() bool {
	return common.GetEnvOrDefaultBool("CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED", true)
}

func (modelUpdateHandler) Interval() time.Duration {
	intervalMinutes := common.GetEnvOrDefault(
		"CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_INTERVAL_MINUTES",
		channelUpstreamModelUpdateTaskDefaultIntervalMinutes,
	)
	if intervalMinutes < 1 {
		intervalMinutes = channelUpstreamModelUpdateTaskDefaultIntervalMinutes
	}
	return time.Duration(intervalMinutes) * time.Minute
}

func (modelUpdateHandler) NewPayload() any { return nil }

// modelUpdateTaskPayload controls one model_update run. A scheduled run
// (Manual=false) respects the per-channel minimum check interval and may
// auto-apply detected models when a channel has auto-sync enabled. A manual
// "detect all" trigger sets Manual=true to reproduce the legacy detect-all
// semantics: force a re-check regardless of the interval and never auto-apply,
// so the admin reviews and applies changes explicitly.
type modelUpdateTaskPayload struct {
	Manual bool `json:"manual,omitempty"`
}

func (modelUpdateHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := modelUpdateTaskPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	summary := runChannelUpstreamModelUpdateTaskOnce(ctx, payload.Manual, !payload.Manual, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// midjourneyPollHandler runs one Midjourney polling pass per scheduled run.
// Enabled() folds the "are there unfinished tasks?" check into enablement so the
// scheduler creates no row when the system is idle; only when at least one
// Midjourney task is in progress does a row get scheduled.
type midjourneyPollHandler struct{}

func (midjourneyPollHandler) Type() string { return model.SystemTaskTypeMidjourneyPoll }

func (midjourneyPollHandler) Enabled() bool {
	return constant.UpdateTask && model.HasUnfinishedMidjourneyTasks()
}

func (midjourneyPollHandler) Interval() time.Duration { return 15 * time.Second }

func (midjourneyPollHandler) NewPayload() any { return nil }

func (midjourneyPollHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary := runMidjourneyTaskUpdateOnce(ctx, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// asyncTaskPollHandler runs one async-task (Suno/video) polling pass per
// scheduled run. Like midjourneyPollHandler, Enabled() folds in the unfinished
// task existence check so an idle system schedules no rows.
type asyncTaskPollHandler struct{}

func (asyncTaskPollHandler) Type() string { return model.SystemTaskTypeAsyncTaskPoll }

func (asyncTaskPollHandler) Enabled() bool {
	return constant.UpdateTask && model.HasUnfinishedSyncTasks()
}

func (asyncTaskPollHandler) Interval() time.Duration { return 15 * time.Second }

func (asyncTaskPollHandler) NewPayload() any { return nil }

func (asyncTaskPollHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary := service.RunTaskPollingOnce(ctx, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

func finishSystemTaskHandler(task *model.SystemTask, runnerID string, status model.SystemTaskStatus, result any, runErr error) {
	errorMessage := ""
	if runErr != nil {
		errorMessage = runErr.Error()
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, status, result, errorMessage); err != nil {
		common.SysLog(fmt.Sprintf("system task %s failed to persist result: %v", task.TaskID, err))
	}
}

// channelScheduleHandler runs the scheduled channel enable/disable reconcile
// job. It iterates channels with a non-empty schedule and flips each between
// enabled (status=1) and manually-disabled (status=2 with a "scheduled_off"
// reason) so cost-control windows stay enforced. Multi-instance execution is
// deduped by the per-type DB lease; the scheduler creates one task row per
// Interval() so the run history stays auditable in the system-tasks panel.
type channelScheduleHandler struct{}

func (channelScheduleHandler) Type() string  { return model.SystemTaskTypeChannelSchedule }
func (channelScheduleHandler) Enabled() bool { return true }
func (channelScheduleHandler) Interval() time.Duration {
	return 30 * time.Second
}
func (channelScheduleHandler) NewPayload() any { return nil }

func (channelScheduleHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary := service.RunChannelScheduleOnce(ctx)
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// planMonitorHandler periodically pulls upstream token-plan usage (MiniMax etc.)
// for every enabled plan that is due. Cadence is a fixed 60s tick; each plan's
// own RefreshIntervalMin decides whether it is actually refetched this tick, so
// the shortest configured interval does not force-refresh the rest. Multi-instance
// execution is deduped by the per-type DB lease.
type planMonitorHandler struct{}

func (planMonitorHandler) Type() string  { return model.SystemTaskTypePlanMonitor }
func (planMonitorHandler) Enabled() bool { return true }
func (planMonitorHandler) Interval() time.Duration {
	return 60 * time.Second
}
func (planMonitorHandler) NewPayload() any { return nil }

func (planMonitorHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary := planmonitor.RunFetchOnce(ctx)
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// orgSyncHandler periodically snapshots the enterprise org structure
// (departments + members + leader flags) from whichever provider
// (DingTalk/Feishu, mutually exclusive) is enabled. Cadence comes from the
// provider's orgsync_interval_hours setting; enablement additionally requires
// credentials so a misconfigured tenant schedules no rows. Manual "sync now"
// triggers enqueue the same task type, so scheduled and manual runs share the
// DB lease and can never overlap.
type orgSyncHandler struct{}

func (orgSyncHandler) Type() string { return model.SystemTaskTypeOrgSync }
func (orgSyncHandler) Enabled() bool {
	return service.OrgSyncScheduleEnabled()
}
func (orgSyncHandler) Interval() time.Duration {
	return service.OrgSyncInterval()
}
func (orgSyncHandler) NewPayload() any { return nil }

func (orgSyncHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary, err := service.RunOrgSyncOnce(ctx)
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// auditCleanupHandler 每日清理超过保留期(RetentionDays)的安全审计事件。
// 仅在审计总开关开启时调度,避免未启用审计的部署产生空跑任务行。
type auditCleanupHandler struct{}

func (auditCleanupHandler) Type() string { return model.SystemTaskTypeAuditCleanup }
func (auditCleanupHandler) Enabled() bool {
	return system_setting.GetAuditSettings().Enabled
}
func (auditCleanupHandler) Interval() time.Duration { return 24 * time.Hour }
func (auditCleanupHandler) NewPayload() any         { return nil }

func (auditCleanupHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	retentionDays := system_setting.GetAuditSettings().RetentionDays
	affected, err := model.DeleteExpiredAuditEvents(retentionDays, time.Now().Unix())
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, map[string]any{
		"retention_days": retentionDays,
		"deleted":        affected,
	}, nil)
}

// auditClassifyHandler 按配置间隔对带 prompt 原文的未分类审计事件做 LLM 分类
// 并归并 skill 候选。分类未启用或渠道未配置时不调度(Enabled 返回 false)。
type auditClassifyHandler struct{}

func (auditClassifyHandler) Type() string { return model.SystemTaskTypeAuditClassify }
func (auditClassifyHandler) Enabled() bool {
	settings := system_setting.GetAuditSettings()
	return settings.Enabled && settings.ClassifyEnabled &&
		settings.ClassifyChannelId != 0 && settings.ClassifyModel != ""
}
func (auditClassifyHandler) Interval() time.Duration {
	minutes := system_setting.GetAuditSettings().ClassifyIntervalMinutes
	if minutes <= 0 {
		minutes = 60
	}
	return time.Duration(minutes) * time.Minute
}
func (auditClassifyHandler) NewPayload() any { return nil }

func (auditClassifyHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	processed, classified, err := audit.ClassifyPendingEvents()
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, map[string]any{
		"processed":  processed,
		"classified": classified,
	}, nil)
}
