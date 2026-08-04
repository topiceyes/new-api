package service

import (
	"context"
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
	dingTalkLeaveCheckTickInterval = 1 * time.Hour
	// First tick at or after this local hour performs the day's audit; later
	// ticks on the same day are skipped via the date marker.
	dingTalkLeaveCheckEarliestHour = 3
	// Gap between membership queries to stay well under DingTalk rate limits.
	dingTalkLeaveCheckRequestGap = 200 * time.Millisecond
)

var (
	dingTalkLeaveCheckOnce    sync.Once
	dingTalkLeaveCheckRunning atomic.Bool
	// YYYYMMDD of the last successful audit; the task runs at most once per day.
	dingTalkLeaveCheckLastDay atomic.Int64
)

func StartDingTalkLeaveCheckTask() {
	dingTalkLeaveCheckOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf("dingtalk leave check task started: tick=%s, run_hour=%d", dingTalkLeaveCheckTickInterval, dingTalkLeaveCheckEarliestHour))
			ticker := time.NewTicker(dingTalkLeaveCheckTickInterval)
			defer ticker.Stop()

			runDingTalkLeaveCheckIfDue()
			for range ticker.C {
				runDingTalkLeaveCheckIfDue()
			}
		})
	})
}

func runDingTalkLeaveCheckIfDue() {
	now := time.Now()
	if now.Hour() < dingTalkLeaveCheckEarliestHour {
		return
	}
	today := int64(now.Year())*10000 + int64(now.Month())*100 + int64(now.Day())
	if dingTalkLeaveCheckLastDay.Load() == today {
		return
	}
	if !dingTalkLeaveCheckRunning.CompareAndSwap(false, true) {
		return
	}
	defer dingTalkLeaveCheckRunning.Store(false)
	if dingTalkLeaveCheckLastDay.Load() == today {
		return
	}

	ctx := context.Background()
	if err := runDingTalkLeaveCheck(ctx); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("dingtalk leave check task failed: %v", err))
		// Do not advance the date marker on failure: retry on the next tick.
		return
	}
	dingTalkLeaveCheckLastDay.Store(today)
}

func runDingTalkLeaveCheck(ctx context.Context) error {
	settings := system_setting.GetDingTalkSettings()
	if !settings.Enabled || settings.AppKey == "" || settings.AppSecret == "" {
		return nil
	}
	provider, ok := oauth.GetProvider("dingtalk").(*oauth.DingTalkProvider)
	if !ok {
		return nil
	}

	users, err := model.GetEnabledDingTalkUsers()
	if err != nil {
		return fmt.Errorf("query dingtalk-bound users: %w", err)
	}
	if len(users) == 0 {
		return nil
	}

	checked, disabled, unknown := 0, 0, 0
	for _, user := range users {
		active, err := provider.CheckUserActive(ctx, user.DingTalkId)
		time.Sleep(dingTalkLeaveCheckRequestGap)
		checked++
		if err != nil {
			// Unknown state (network failure, missing permission, unexpected
			// errcode): never disable, record and retry on the next run.
			unknown++
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
		disabled++
		common.SysLog(fmt.Sprintf("dingtalk leave check: user %d (username=%s) left the DingTalk organization, account and tokens disabled", user.Id, user.Username))
	}
	common.SysLog(fmt.Sprintf("dingtalk leave check finished: checked=%d, disabled=%d, unknown=%d", checked, disabled, unknown))
	return nil
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
