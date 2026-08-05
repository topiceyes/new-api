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
	feishuLeaveCheckTickInterval = 1 * time.Minute
	// Gap between membership queries to stay well under Feishu rate limits
	// (contact/v3/users: 1000/min & 50/sec per app per tenant).
	feishuLeaveCheckRequestGap = 200 * time.Millisecond
)

var (
	feishuLeaveCheckOnce    sync.Once
	feishuLeaveCheckRunning atomic.Bool
	// YYYYMMDD of the last completed audit; daily mode runs at most once per day.
	feishuLeaveCheckLastDay atomic.Int64
	// Unix seconds of the last completed audit; anchors the interval mode.
	feishuPatrolLastRun atomic.Int64
)

// FeishuLeaveCheckResult summarizes one patrol run.
type FeishuLeaveCheckResult struct {
	Checked  int `json:"checked"`
	Disabled int `json:"disabled"`
	Unknown  int `json:"unknown"`
}

var (
	ErrFeishuLeaveCheckRunning       = errors.New("feishu leave check is already running")
	ErrFeishuLeaveCheckNotConfigured = errors.New("feishu app credentials are not configured")
)

func StartFeishuLeaveCheckTask() {
	feishuLeaveCheckOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf("feishu leave check task started: tick=%s, schedule configurable via feishu.patrol_* options", feishuLeaveCheckTickInterval))
			ticker := time.NewTicker(feishuLeaveCheckTickInterval)
			defer ticker.Stop()

			runFeishuLeaveCheckIfDue()
			for range ticker.C {
				runFeishuLeaveCheckIfDue()
			}
		})
	})
}

// shouldRunFeishuPatrol decides whether a patrol run is due. It is pure so
// the schedule semantics can be tested directly. Unknown modes never run:
// direct database tampering cannot bypass option validation this way.
func shouldRunFeishuPatrol(settings *system_setting.FeishuSettings, now time.Time, lastRunDay int64, lastRun time.Time) bool {
	switch settings.PatrolMode {
	case system_setting.FeishuPatrolModeDaily:
		// Catch-up semantics: if the process was down at the configured
		// hour, the day's audit runs as soon as it is back and the hour
		// gate passes.
		return now.Hour() >= settings.PatrolHour && lastRunDay != feishuPatrolDayMarker(now)
	case system_setting.FeishuPatrolModeInterval:
		interval := time.Duration(settings.PatrolIntervalHours) * time.Hour
		if interval <= 0 {
			return false
		}
		return lastRun.IsZero() || now.Sub(lastRun) >= interval
	default:
		return false
	}
}

func feishuPatrolDayMarker(t time.Time) int64 {
	return int64(t.Year())*10000 + int64(t.Month())*100 + int64(t.Day())
}

// markFeishuPatrolCompleted records a finished run so the scheduler does
// not immediately repeat it. Both markers are updated regardless of mode so
// a later mode switch anchors on the real last completion.
func markFeishuPatrolCompleted(now time.Time) {
	feishuLeaveCheckLastDay.Store(feishuPatrolDayMarker(now))
	feishuPatrolLastRun.Store(now.Unix())
}

func feishuPatrolLastRunTime() time.Time {
	if ts := feishuPatrolLastRun.Load(); ts > 0 {
		return time.Unix(ts, 0)
	}
	return time.Time{}
}

func runFeishuLeaveCheckIfDue() {
	settings := system_setting.GetFeishuSettings()
	if !settings.PatrolEnabled || settings.AppId == "" || settings.AppSecret == "" {
		return
	}
	if !shouldRunFeishuPatrol(settings, time.Now(), feishuLeaveCheckLastDay.Load(), feishuPatrolLastRunTime()) {
		return
	}
	if !feishuLeaveCheckRunning.CompareAndSwap(false, true) {
		return
	}
	defer feishuLeaveCheckRunning.Store(false)

	ctx := context.Background()
	if _, err := runFeishuLeaveCheck(ctx); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("feishu leave check task failed: %v", err))
		// Do not advance the markers on failure: retry on the next tick.
		// A failed run only costs a cheap database query; Feishu API
		// errors per user are swallowed as "unknown" and do not fail the run.
		return
	}
	markFeishuPatrolCompleted(time.Now())
}

// RunFeishuLeaveCheckNow performs one patrol run immediately, regardless
// of the configured schedule (an explicit admin action). It shares the
// running lock with the scheduler, so concurrent scheduled runs are
// impossible on the same node. A successful manual run advances the
// schedule markers, consuming the day's/interval's slot.
func RunFeishuLeaveCheckNow(ctx context.Context) (*FeishuLeaveCheckResult, error) {
	if !feishuLeaveCheckRunning.CompareAndSwap(false, true) {
		return nil, ErrFeishuLeaveCheckRunning
	}
	defer feishuLeaveCheckRunning.Store(false)

	result, err := runFeishuLeaveCheck(ctx)
	if err != nil {
		return nil, err
	}
	markFeishuPatrolCompleted(time.Now())
	return result, nil
}

// runFeishuLeaveCheck executes one full audit. It is gated only on the app
// credentials (not on the login switch or PatrolEnabled) so both the
// scheduler and the manual trigger can share it.
func runFeishuLeaveCheck(ctx context.Context) (*FeishuLeaveCheckResult, error) {
	settings := system_setting.GetFeishuSettings()
	if settings.AppId == "" || settings.AppSecret == "" {
		return nil, ErrFeishuLeaveCheckNotConfigured
	}
	provider, ok := oauth.GetProvider("feishu").(*oauth.FeishuProvider)
	if !ok {
		return nil, fmt.Errorf("feishu oauth provider not registered")
	}

	users, err := model.GetEnabledFeishuUsers()
	if err != nil {
		return nil, fmt.Errorf("query feishu-bound users: %w", err)
	}

	result := &FeishuLeaveCheckResult{}
	for _, user := range users {
		active, err := provider.CheckUserActive(ctx, user.FeishuId)
		time.Sleep(feishuLeaveCheckRequestGap)
		result.Checked++
		if err != nil {
			// Unknown state (network failure, out of contact permission scope,
			// rate limit, unexpected code): never disable, record and retry
			// on the next run.
			result.Unknown++
			logger.LogWarn(ctx, fmt.Sprintf("feishu leave check: user %d (union_id=%s) state unknown: %v", user.Id, user.FeishuId, err))
			continue
		}
		if active {
			continue
		}
		if err := disableDepartedFeishuUser(user); err != nil {
			logger.LogError(ctx, fmt.Sprintf("feishu leave check: failed to disable departed user %d: %v", user.Id, err))
			continue
		}
		result.Disabled++
		common.SysLog(fmt.Sprintf("feishu leave check: user %d (username=%s) left the Feishu organization, account and tokens disabled", user.Id, user.Username))
	}
	common.SysLog(fmt.Sprintf("feishu leave check finished: checked=%d, disabled=%d, unknown=%d", result.Checked, result.Disabled, result.Unknown))
	return result, nil
}

// disableDepartedFeishuUser disables a user confirmed to have left the
// Feishu organization: the account, every enabled API token, and all live
// sessions. Nothing is deleted so the action stays reversible and auditable.
func disableDepartedFeishuUser(user *model.User) error {
	user.Status = common.UserStatusDisabled
	if err := user.Update(false); err != nil {
		return err
	}
	if err := model.DisableAllEnabledTokensByUserId(user.Id); err != nil {
		return err
	}
	if _, err := model.RevokeAllUserSessions(user.Id, "feishu_leave"); err != nil {
		return err
	}
	model.RecordLog(user.Id, model.LogTypeSystem, "检测到已退出飞书企业，系统自动禁用账号及全部 API 令牌")
	return nil
}
