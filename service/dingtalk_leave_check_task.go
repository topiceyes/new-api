package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	// The tick only evaluates the schedule against live settings, so a short
	// interval keeps configuration changes effective within a minute without
	// any restart.
	dingTalkLeaveCheckTickInterval = 1 * time.Minute
	// Gap between membership queries to stay well under DingTalk rate limits.
	dingTalkLeaveCheckRequestGap = 200 * time.Millisecond
)

var (
	dingTalkLeaveCheckOnce    sync.Once
	dingTalkLeaveCheckRunning atomic.Bool
	// YYYYMMDD of the last completed audit; daily mode runs at most once per day.
	dingTalkLeaveCheckLastDay atomic.Int64
	// Unix seconds of the last completed audit; anchors the interval mode.
	dingTalkPatrolLastRun atomic.Int64
)

// DingTalkLeaveCheckResult summarizes one patrol run.
type DingTalkLeaveCheckResult struct {
	Checked  int `json:"checked"`
	Disabled int `json:"disabled"`
	Unknown  int `json:"unknown"`
}

var (
	ErrDingTalkLeaveCheckRunning       = errors.New("dingtalk leave check is already running")
	ErrDingTalkLeaveCheckNotConfigured = errors.New("dingtalk app credentials are not configured")
)

func StartDingTalkLeaveCheckTask() {
	dingTalkLeaveCheckOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf("dingtalk leave check task started: tick=%s, schedule configurable via dingtalk.patrol_* options", dingTalkLeaveCheckTickInterval))
			ticker := time.NewTicker(dingTalkLeaveCheckTickInterval)
			defer ticker.Stop()

			runDingTalkLeaveCheckIfDue()
			for range ticker.C {
				runDingTalkLeaveCheckIfDue()
			}
		})
	})
}

// shouldRunDingTalkPatrol decides whether a patrol run is due. It is pure so
// the schedule semantics can be tested directly. Unknown modes never run:
// direct database tampering cannot bypass option validation this way.
func shouldRunDingTalkPatrol(settings *system_setting.DingTalkSettings, now time.Time, lastRunDay int64, lastRun time.Time) bool {
	switch settings.PatrolMode {
	case system_setting.DingTalkPatrolModeDaily:
		// Catch-up semantics: if the process was down at the configured
		// hour, the day's audit runs as soon as it is back and the hour
		// gate passes.
		return now.Hour() >= settings.PatrolHour && lastRunDay != dingTalkPatrolDayMarker(now)
	case system_setting.DingTalkPatrolModeInterval:
		interval := time.Duration(settings.PatrolIntervalHours) * time.Hour
		if interval <= 0 {
			return false
		}
		return lastRun.IsZero() || now.Sub(lastRun) >= interval
	default:
		return false
	}
}

func dingTalkPatrolDayMarker(t time.Time) int64 {
	return int64(t.Year())*10000 + int64(t.Month())*100 + int64(t.Day())
}

// markDingTalkPatrolCompleted records a finished run so the scheduler does
// not immediately repeat it. Both markers are updated regardless of mode so
// a later mode switch anchors on the real last completion.
func markDingTalkPatrolCompleted(now time.Time) {
	dingTalkLeaveCheckLastDay.Store(dingTalkPatrolDayMarker(now))
	dingTalkPatrolLastRun.Store(now.Unix())
}

func patrolLastRunTime() time.Time {
	if ts := dingTalkPatrolLastRun.Load(); ts > 0 {
		return time.Unix(ts, 0)
	}
	return time.Time{}
}

func runDingTalkLeaveCheckIfDue() {
	settings := system_setting.GetDingTalkSettings()
	if !settings.PatrolEnabled || settings.AppKey == "" || settings.AppSecret == "" {
		return
	}
	if !shouldRunDingTalkPatrol(settings, time.Now(), dingTalkLeaveCheckLastDay.Load(), patrolLastRunTime()) {
		return
	}
	if !dingTalkLeaveCheckRunning.CompareAndSwap(false, true) {
		return
	}
	defer dingTalkLeaveCheckRunning.Store(false)

	ctx := context.Background()
	if _, err := runDingTalkLeaveCheck(ctx); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("dingtalk leave check task failed: %v", err))
		// Do not advance the markers on failure: retry on the next tick.
		// A failed run only costs a cheap database query; DingTalk API
		// errors per user are swallowed as "unknown" and do not fail the run.
		return
	}
	markDingTalkPatrolCompleted(time.Now())
}

// RunDingTalkLeaveCheckNow performs one patrol run immediately, regardless
// of the configured schedule (an explicit admin action). It shares the
// running lock with the scheduler, so concurrent scheduled runs are
// impossible on the same node. A successful manual run advances the
// schedule markers, consuming the day's/interval's slot.
func RunDingTalkLeaveCheckNow(ctx context.Context) (*DingTalkLeaveCheckResult, error) {
	if !dingTalkLeaveCheckRunning.CompareAndSwap(false, true) {
		return nil, ErrDingTalkLeaveCheckRunning
	}
	defer dingTalkLeaveCheckRunning.Store(false)

	result, err := runDingTalkLeaveCheck(ctx)
	if err != nil {
		return nil, err
	}
	markDingTalkPatrolCompleted(time.Now())
	return result, nil
}

// runDingTalkLeaveCheck executes one full audit. It is gated only on the app
// credentials (not on the login switch or PatrolEnabled) so both the
// scheduler and the manual trigger can share it.
func runDingTalkLeaveCheck(ctx context.Context) (*DingTalkLeaveCheckResult, error) {
	settings := system_setting.GetDingTalkSettings()
	if settings.AppKey == "" || settings.AppSecret == "" {
		return nil, ErrDingTalkLeaveCheckNotConfigured
	}
	provider, ok := oauth.GetProvider("dingtalk").(*oauth.DingTalkProvider)
	if !ok {
		return nil, fmt.Errorf("dingtalk oauth provider not registered")
	}

	users, err := model.GetEnabledDingTalkUsers()
	if err != nil {
		return nil, fmt.Errorf("query dingtalk-bound users: %w", err)
	}

	result := &DingTalkLeaveCheckResult{}
	for _, user := range users {
		active, err := provider.CheckUserActive(ctx, user.DingTalkId)
		time.Sleep(dingTalkLeaveCheckRequestGap)
		result.Checked++
		if err != nil {
			// Unknown state (network failure, missing permission, unexpected
			// errcode): never disable, record and retry on the next run.
			result.Unknown++
			logger.LogWarn(ctx, fmt.Sprintf("dingtalk leave check: user %d (unionId=%s) state unknown: %v", user.Id, user.DingTalkId, err))
			continue
		}
		if active {
			continue
		}
		if err := disableDepartedDingTalkUser(user); err != nil {
			logger.LogError(ctx, fmt.Sprintf("dingtalk leave check: failed to disable departed user %d: %v", user.Id, err))
			continue
		}
		result.Disabled++
		common.SysLog(fmt.Sprintf("dingtalk leave check: user %d (username=%s) left the DingTalk organization, account and tokens disabled", user.Id, user.Username))
	}
	common.SysLog(fmt.Sprintf("dingtalk leave check finished: checked=%d, disabled=%d, unknown=%d", result.Checked, result.Disabled, result.Unknown))
	return result, nil
}

// disableDepartedDingTalkUser disables a user confirmed to have left the
// DingTalk organization: the account, every enabled API token, and all live
// sessions. Nothing is deleted so the action stays reversible and auditable.
func disableDepartedDingTalkUser(user *model.User) error {
	user.Status = common.UserStatusDisabled
	if err := user.Update(false); err != nil {
		return err
	}
	if err := model.DisableAllEnabledTokensByUserId(user.Id); err != nil {
		return err
	}
	if _, err := model.RevokeAllUserSessions(user.Id, "dingtalk_leave"); err != nil {
		return err
	}
	model.RecordLog(user.Id, model.LogTypeSystem, "检测到已退出钉钉企业，系统自动禁用账号及全部 API 令牌")
	return nil
}
