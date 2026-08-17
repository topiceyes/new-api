package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelScheduleTest(t *testing.T) *gorm.DB {
	t.Helper()
	originalDB := model.DB
	originalMemoryCache := common.MemoryCacheEnabled
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	model.DB = db
	// 关掉 MemoryCache，让 UpdateChannelStatus 直接走 DB 分支；生产环境 InitChannelCache 会把 channel 放进 cache。
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = originalDB
		common.MemoryCacheEnabled = originalMemoryCache
		sqlDB, err := db.DB()
		if err == nil {
			require.NoError(t, sqlDB.Close())
		}
	})
	return db
}

func seedChannelForSchedule(t *testing.T, db *gorm.DB, status int, otherInfo map[string]interface{}, schedule string, name string) *model.Channel {
	t.Helper()
	sch := &schedule
	if schedule == "" {
		sch = nil
	}
	ch := &model.Channel{
		Type:    1,
		Key:     "sk-test",
		Name:    name,
		Status:  status,
		Group:   "default",
		Models:  "test-model",
		OtherInfo: func() string {
			if otherInfo == nil {
				return ""
			}
			s, _ := common.Marshal(otherInfo)
			return string(s)
		}(),
		Schedule: sch,
	}
	require.NoError(t, db.Create(ch).Error)
	// 不调 model.InitChannelCache：该函数依赖未初始化的全局 cache map，测试只断言 DB 状态转换。
	return ch
}

// 窗口覆盖"全天"(00:00-23:59)，任何时间 desired=true
const scheduleAlwaysOn = `{"enabled":true,"timezone":"UTC","windows":[{"days":[0,1,2,3,4,5,6],"start":"00:00","end":"23:59"}]}`
// 窗口不覆盖当前时刻（极短窗口：00:00-00:01）
const scheduleAlwaysOff = `{"enabled":true,"timezone":"UTC","windows":[{"days":[0,1,2,3,4,5,6],"start":"00:00","end":"00:01"}]}`

func TestRunChannelScheduleOnce_EnabledChannel_OutsideWindow_Disables(t *testing.T) {
	db := setupChannelScheduleTest(t)
	ch := seedChannelForSchedule(t, db, common.ChannelStatusEnabled, nil, scheduleAlwaysOff, "C1")

	summary := RunChannelScheduleOnce(nil)
	require.GreaterOrEqual(t, summary.ScannedCount, 1)
	require.Equal(t, 1, summary.DisabledCount)

	reloaded, err := model.GetChannelById(ch.Id, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusManuallyDisabled, reloaded.Status)
	require.True(t, reloaded.IsScheduledOff(), "reason should carry scheduled_off prefix")
}

func TestRunChannelScheduleOnce_ScheduledOffChannel_InsideWindow_ReEnables(t *testing.T) {
	db := setupChannelScheduleTest(t)
	ch := seedChannelForSchedule(t, db, common.ChannelStatusManuallyDisabled,
		map[string]interface{}{"status_reason": "scheduled_off: outside window", "status_time": int64(1)},
		scheduleAlwaysOn, "C2")

	summary := RunChannelScheduleOnce(nil)
	require.Equal(t, 1, summary.EnabledCount)

	reloaded, err := model.GetChannelById(ch.Id, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusEnabled, reloaded.Status)
}

func TestRunChannelScheduleOnce_ManualDisabledChannel_LeftAlone(t *testing.T) {
	db := setupChannelScheduleTest(t)
	ch := seedChannelForSchedule(t, db, common.ChannelStatusManuallyDisabled,
		map[string]interface{}{"status_reason": "manual operation", "status_time": int64(1)},
		scheduleAlwaysOn, "C3")

	summary := RunChannelScheduleOnce(nil)
	require.Equal(t, 0, summary.EnabledCount, "scheduler must not touch manual-2 channels")
	require.Equal(t, 0, summary.DisabledCount)

	reloaded, err := model.GetChannelById(ch.Id, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusManuallyDisabled, reloaded.Status, "manual disable must persist")
}

// 人工禁用 + 当前在关段：调度器不应把 reason 覆写成 scheduled_off（否则会被当成定时禁用并在下轮开启时段被拉起）。
func TestRunChannelScheduleOnce_ManualDisabledChannel_OutsideWindow_ReasonNotOverwritten(t *testing.T) {
	db := setupChannelScheduleTest(t)
	ch := seedChannelForSchedule(t, db, common.ChannelStatusManuallyDisabled,
		map[string]interface{}{"status_reason": "manual operation", "status_time": int64(1)},
		scheduleAlwaysOff, "C3b")

	summary := RunChannelScheduleOnce(nil)
	require.Equal(t, 0, summary.DisabledCount, "manual-2 channels must not be re-marked scheduled_off")

	reloaded, err := model.GetChannelById(ch.Id, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusManuallyDisabled, reloaded.Status)
	require.False(t, reloaded.IsScheduledOff(), "reason must stay manual")
}

// 熔断禁用 + 当前在关段：调度器不应再去禁用（熔断优先）。
func TestRunChannelScheduleOnce_AutoDisabledChannel_OutsideWindow_LeftAlone(t *testing.T) {
	db := setupChannelScheduleTest(t)
	ch := seedChannelForSchedule(t, db, common.ChannelStatusAutoDisabled,
		map[string]interface{}{"status_reason": "circuit breaker", "status_time": int64(1)},
		scheduleAlwaysOff, "C4b")

	summary := RunChannelScheduleOnce(nil)
	require.Equal(t, 0, summary.DisabledCount)
	require.Equal(t, 0, summary.EnabledCount)

	reloaded, err := model.GetChannelById(ch.Id, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusAutoDisabled, reloaded.Status)
}

func TestRunChannelScheduleOnce_AutoDisabledChannel_LeftAlone(t *testing.T) {
	db := setupChannelScheduleTest(t)
	ch := seedChannelForSchedule(t, db, common.ChannelStatusAutoDisabled,
		map[string]interface{}{"status_reason": "circuit breaker", "status_time": int64(1)},
		scheduleAlwaysOn, "C4")

	summary := RunChannelScheduleOnce(nil)
	require.Equal(t, 0, summary.EnabledCount)
	require.Equal(t, 0, summary.DisabledCount)

	reloaded, err := model.GetChannelById(ch.Id, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusAutoDisabled, reloaded.Status)
}

func TestRunChannelScheduleOnce_ScheduleDisabled_RestoresScheduledOffChannel(t *testing.T) {
	db := setupChannelScheduleTest(t)
	ch := seedChannelForSchedule(t, db, common.ChannelStatusManuallyDisabled,
		map[string]interface{}{"status_reason": "scheduled_off: outside window", "status_time": int64(1)},
		`{"enabled":false,"timezone":"UTC","windows":[]}`, "C5")

	summary := RunChannelScheduleOnce(nil)
	require.Equal(t, 1, summary.RestoredCount, "schedule disabled must restore scheduled-off channels")

	reloaded, err := model.GetChannelById(ch.Id, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusEnabled, reloaded.Status)
}

func TestRunChannelScheduleOnce_InsideWindow_NoOp(t *testing.T) {
	db := setupChannelScheduleTest(t)
	ch := seedChannelForSchedule(t, db, common.ChannelStatusEnabled, nil, scheduleAlwaysOn, "C6")

	summary := RunChannelScheduleOnce(nil)
	require.GreaterOrEqual(t, summary.ScannedCount, 1)
	require.Equal(t, 0, summary.DisabledCount)
	require.Equal(t, 0, summary.EnabledCount)
	require.Equal(t, 0, summary.RestoredCount)

	reloaded, err := model.GetChannelById(ch.Id, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusEnabled, reloaded.Status)
}

func TestRunChannelScheduleOnce_ParseErrorSkipped(t *testing.T) {
	db := setupChannelScheduleTest(t)
	// schedule 字段存在但是非法 JSON
	seedChannelForSchedule(t, db, common.ChannelStatusEnabled, nil, "{not-json", "C7")

	summary := RunChannelScheduleOnce(nil)
	require.Equal(t, 1, summary.ParseErrorSkip)
	require.Equal(t, 0, summary.DisabledCount)
	require.Equal(t, 0, summary.EnabledCount)
}

// 多 key 渠道：定时禁用应走整体渠道级 Status 分支（usingKey=""），不应去碰 MultiKeyStatusList。
func TestRunChannelScheduleOnce_MultiKeyChannel_DisablesWholeChannel(t *testing.T) {
	db := setupChannelScheduleTest(t)
	ch := seedChannelForSchedule(t, db, common.ChannelStatusEnabled, nil, scheduleAlwaysOff, "MultiKey")
	// 把渠道改成多 key：2 个 key，per-key 状态全启用
	ch.ChannelInfo = model.ChannelInfo{
		IsMultiKey:         true,
		MultiKeySize:       2,
		MultiKeyStatusList: map[int]int{0: common.ChannelStatusEnabled, 1: common.ChannelStatusEnabled},
	}
	require.NoError(t, db.Save(ch).Error)

	summary := RunChannelScheduleOnce(nil)
	require.Equal(t, 1, summary.DisabledCount)

	reloaded, err := model.GetChannelById(ch.Id, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusManuallyDisabled, reloaded.Status)
	require.True(t, reloaded.IsScheduledOff(), "channel-level reason should carry scheduled_off prefix")
	// per-key 状态未被改动
	require.Equal(t, common.ChannelStatusEnabled, reloaded.ChannelInfo.MultiKeyStatusList[0])
	require.Equal(t, common.ChannelStatusEnabled, reloaded.ChannelInfo.MultiKeyStatusList[1])
}