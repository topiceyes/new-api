package model

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// usageStatTestDay 返回本地时区某天的起止 unix 秒,测试用固定日期避免依赖"今天"。
func usageStatTestDay(t *testing.T, offsetDays int) (string, int64, int64) {
	t.Helper()
	now := time.Now().In(time.Local)
	base := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, offsetDays)
	return base.Format("2006-01-02"), base.Unix(), base.AddDate(0, 0, 1).Unix()
}

func seedUsageStatLog(t *testing.T, log *Log) {
	t.Helper()
	require.NoError(t, createLog(log))
}

// 聚合正确性: consume/error/refund 三类分桶、quota 与 refund 分离、token 与
// use_time 求和、小时落格、跨天边界(23:59:59 vs 00:00:01)不串天。
func TestAggregateLogsForDay(t *testing.T) {
	truncateTables(t)
	ctx := context.Background()
	date, dayStart, dayEnd := usageStatTestDay(t, -2)

	mkLog := func(createdAt int64, logType, userId int, modelName, tokenName, group string, quota, prompt, completion, useTime int) *Log {
		return &Log{
			UserId: userId, CreatedAt: createdAt, Type: logType,
			Username: "u" + string(rune('0'+userId)), TokenName: tokenName,
			ModelName: modelName, Quota: quota, PromptTokens: prompt,
			CompletionTokens: completion, UseTime: useTime, ChannelId: 7, Group: group,
		}
	}

	// 当天: 用户1 两条 consume(同模型同令牌,应合并)、一条 error、一条 refund;用户2 一条 consume。
	seedUsageStatLog(t, mkLog(dayStart+3600, LogTypeConsume, 1, "gpt-4o", "cli", "default", 1000, 100, 50, 5))
	seedUsageStatLog(t, mkLog(dayStart+2*3600, LogTypeConsume, 1, "gpt-4o", "cli", "default", 2000, 200, 100, 10))
	seedUsageStatLog(t, mkLog(dayStart+3*3600, LogTypeError, 1, "gpt-4o", "cli", "default", 0, 0, 0, 0))
	seedUsageStatLog(t, mkLog(dayStart+4*3600, LogTypeRefund, 1, "gpt-4o", "cli", "default", 500, 0, 0, 0))
	seedUsageStatLog(t, mkLog(dayStart+5*3600, LogTypeConsume, 2, "claude-sonnet", "web", "vip", 3000, 300, 150, 20))

	// 用户2 当天改名后的一条日志: username 不同但其余维度相同,必须合并进同一行
	// (username 不在唯一索引里,不合并会在写入时撞唯一键)。
	renamed := mkLog(dayStart+6*3600, LogTypeConsume, 2, "claude-sonnet", "web", "vip", 700, 70, 35, 2)
	renamed.Username = "u2-renamed"
	seedUsageStatLog(t, renamed)

	// 边界: 当天最后一秒 vs 次日第一秒;次日第一秒不应进今天的聚合。
	seedUsageStatLog(t, mkLog(dayEnd-1, LogTypeConsume, 1, "gpt-4o", "cli", "default", 7000, 0, 0, 1))
	seedUsageStatLog(t, mkLog(dayEnd, LogTypeConsume, 1, "gpt-4o", "cli", "default", 9000, 0, 0, 1))

	// 不相关类型(topup)不参与聚合。
	seedUsageStatLog(t, mkLog(dayStart+100, LogTypeTopup, 1, "", "", "", 100000, 0, 0, 0))

	daily, hourly, err := AggregateLogsForDay(ctx, dayStart, dayEnd)
	require.NoError(t, err)
	require.Len(t, daily, 2)

	byUser := map[int]UsageStatDaily{}
	for _, d := range daily {
		assert.Equal(t, date, d.Date)
		byUser[d.UserId] = d
	}
	u1 := byUser[1]
	assert.Equal(t, 3, u1.RequestCount)
	assert.Equal(t, 1, u1.FailCount)
	assert.Equal(t, 1, u1.RefundCount)
	assert.Equal(t, int64(1000+2000+7000), u1.Quota)
	assert.Equal(t, int64(500), u1.RefundQuota)
	assert.Equal(t, int64(300), u1.PromptTokens)
	assert.Equal(t, int64(150), u1.CompletionTokens)
	assert.Equal(t, int64(16), u1.TotalUseTime)
	assert.Equal(t, "gpt-4o", u1.ModelName)
	assert.Equal(t, "cli", u1.TokenName)
	assert.Equal(t, "default", u1.UseGroup)

	u2 := byUser[2]
	assert.Equal(t, 2, u2.RequestCount)
	assert.Equal(t, 0, u2.FailCount)
	assert.Equal(t, int64(3700), u2.Quota)
	assert.Equal(t, "vip", u2.UseGroup)
	assert.NotEmpty(t, u2.Username)

	// 小时落格: 用户1 consume 在 1h/2h/23h 三格, 用户2 在 5h。
	hourlyByUserHour := map[int]map[int]UsageStatHourly{}
	for _, h := range hourly {
		assert.Equal(t, date, h.Date)
		if hourlyByUserHour[h.UserId] == nil {
			hourlyByUserHour[h.UserId] = map[int]UsageStatHourly{}
		}
		hourlyByUserHour[h.UserId][h.Hour] = h
	}
	u1Hours := hourlyByUserHour[1]
	require.Len(t, u1Hours, 3)
	assert.Equal(t, 1, u1Hours[1].RequestCount)
	assert.Equal(t, int64(1000), u1Hours[1].Quota)
	assert.Equal(t, 1, u1Hours[2].RequestCount)
	assert.Equal(t, 1, u1Hours[23].RequestCount)
	assert.Equal(t, int64(7000), u1Hours[23].Quota)
	u2Hours := hourlyByUserHour[2]
	require.Len(t, u2Hours, 2)
	assert.Equal(t, 1, u2Hours[5].RequestCount)
	assert.Equal(t, int64(3000), u2Hours[5].Quota)
	assert.Equal(t, 1, u2Hours[6].RequestCount)
	assert.Equal(t, int64(700), u2Hours[6].Quota)

	// 空窗口: 聚合出零行不报错。
	dailyEmpty, hourlyEmpty, err := AggregateLogsForDay(ctx, dayStart-2*86400, dayStart-86400)
	require.NoError(t, err)
	assert.Empty(t, dailyEmpty)
	assert.Empty(t, hourlyEmpty)
}

// 幂等: 同一天重复 Replace 结果一致;新日志进来后重算会覆盖旧行而不是叠加。
func TestReplaceUsageStatsForDayIdempotent(t *testing.T) {
	truncateTables(t)
	ctx := context.Background()
	_, dayStart, dayEnd := usageStatTestDay(t, -1)

	seedUsageStatLog(t, &Log{UserId: 1, CreatedAt: dayStart + 60, Type: LogTypeConsume, Username: "u1", ModelName: "m", Quota: 100})

	daily, hourly, err := AggregateLogsForDay(ctx, dayStart, dayEnd)
	require.NoError(t, err)
	require.Len(t, daily, 1)
	require.NoError(t, ReplaceUsageStatsForDay(daily[0].Date, daily, hourly))
	require.NoError(t, ReplaceUsageStatsForDay(daily[0].Date, daily, hourly))

	var count int64
	require.NoError(t, DB.Model(&UsageStatDaily{}).Where("date = ?", daily[0].Date).Count(&count).Error)
	assert.Equal(t, int64(1), count)

	// 新增一条日志后重算同一天,结果应被替换为 2 条 consume 的新值。
	seedUsageStatLog(t, &Log{UserId: 1, CreatedAt: dayStart + 120, Type: LogTypeConsume, Username: "u1", ModelName: "m", Quota: 200})
	daily2, hourly2, err := AggregateLogsForDay(ctx, dayStart, dayEnd)
	require.NoError(t, err)
	require.NoError(t, ReplaceUsageStatsForDay(daily2[0].Date, daily2, hourly2))

	var rows []UsageStatDaily
	require.NoError(t, DB.Where("date = ?", daily[0].Date).Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, 2, rows[0].RequestCount)
	assert.Equal(t, int64(300), rows[0].Quota)
	assert.NotZero(t, rows[0].UpdatedAt)

	var hourlyCount int64
	require.NoError(t, DB.Model(&UsageStatHourly{}).Where("date = ?", daily[0].Date).Count(&hourlyCount).Error)
	assert.Equal(t, int64(1), hourlyCount)
}

// userIds 过滤的 IN 分块: 超过 500 个 id 时多块查询结果合并后不丢不重。
func TestQueryUsageByUserChunkedMerge(t *testing.T) {
	truncateTables(t)

	const userCount = 501
	rows := make([]UsageStatDaily, 0, userCount)
	userIds := make([]int, 0, userCount)
	for i := 1; i <= userCount; i++ {
		rows = append(rows, UsageStatDaily{Date: "2026-08-01", UserId: i, Username: "u", Quota: int64(i)})
		userIds = append(userIds, i)
	}
	require.NoError(t, DB.CreateInBatches(rows, 200).Error)

	got, err := QueryUsageByUser("2026-08-01", "2026-08-01", userIds)
	require.NoError(t, err)
	assert.Len(t, got, userCount)

	// 空非 nil 集合 = 无可见用户,必须返回空而不是全量。
	got, err = QueryUsageByUser("2026-08-01", "2026-08-01", []int{})
	require.NoError(t, err)
	assert.Empty(t, got)

	// nil = 不限制。
	got, err = QueryUsageByUser("2026-08-01", "2026-08-01", nil)
	require.NoError(t, err)
	assert.Len(t, got, userCount)
}

func TestListAggregatedDatesAndDelete(t *testing.T) {
	truncateTables(t)

	d1, s1, _ := usageStatTestDay(t, -3)
	d2, s2, _ := usageStatTestDay(t, -2)
	require.NoError(t, ReplaceUsageStatsForDay(d1, []UsageStatDaily{{Date: d1, UserId: 1}}, nil))
	require.NoError(t, ReplaceUsageStatsForDay(d2, []UsageStatDaily{{Date: d2, UserId: 1}}, []UsageStatHourly{{Date: d2, Hour: 3, UserId: 1}}))
	_ = s1
	_ = s2

	existing, err := ListAggregatedDates(d1)
	require.NoError(t, err)
	assert.True(t, existing[d1])
	assert.True(t, existing[d2])

	// since 晚于所有数据时为空集。
	future, _, _ := usageStatTestDay(t, 1)
	existing, err = ListAggregatedDates(future)
	require.NoError(t, err)
	assert.Empty(t, existing)

	// 清理 d2 之前: d1 两张表都被删掉, d2 保留。
	require.NoError(t, DeleteUsageStatsBefore(d2))
	existing, err = ListAggregatedDates(d1)
	require.NoError(t, err)
	assert.False(t, existing[d1])
	assert.True(t, existing[d2])
	var hourlyCount int64
	require.NoError(t, DB.Model(&UsageStatHourly{}).Count(&hourlyCount).Error)
	assert.Equal(t, int64(1), hourlyCount)
}

// 零日志日也写水位行: Replace 空切片后 ListAggregatedDates 仍认为该日已聚合,
// 否则补缺机制会每小时重扫永远没有日志的日子。
func TestReplaceUsageStatsForDayMarksEmptyDay(t *testing.T) {
	truncateTables(t)
	emptyDate, _, _ := usageStatTestDay(t, -5)

	require.NoError(t, ReplaceUsageStatsForDay(emptyDate, nil, nil))
	existing, err := ListAggregatedDates(emptyDate)
	require.NoError(t, err)
	assert.True(t, existing[emptyDate])

	// 水位行随保留期一起被清理。
	day, err := time.ParseInLocation("2006-01-02", emptyDate, time.Local)
	require.NoError(t, err)
	require.NoError(t, DeleteUsageStatsBefore(day.AddDate(0, 0, 1).Format("2006-01-02")))
	existing, err = ListAggregatedDates(emptyDate)
	require.NoError(t, err)
	assert.Empty(t, existing)
}
