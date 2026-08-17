package dto

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustParseSchedule(t *testing.T, raw string) *ChannelSchedule {
	t.Helper()
	s, err := ParseChannelSchedule(raw)
	require.NoError(t, err)
	require.NotNil(t, s)
	return s
}

func TestParseChannelSchedule_EmptyReturnsNil(t *testing.T) {
	s, err := ParseChannelSchedule("")
	require.NoError(t, err)
	assert.Nil(t, s)
}

func TestParseChannelSchedule_BadJSON(t *testing.T) {
	_, err := ParseChannelSchedule("{not-json")
	require.Error(t, err)
}

func TestValidate_BadTimezone(t *testing.T) {
	s := &ChannelSchedule{Enabled: true, Timezone: "Nowhere/Land", Windows: []ChannelScheduleWindow{{Start: "00:00", End: "01:00"}}}
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid timezone")
}

func TestValidate_EnabledNoWindows(t *testing.T) {
	s := &ChannelSchedule{Enabled: true, Timezone: "UTC", Windows: nil}
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "window is required")
}

func TestValidate_BadHHMM(t *testing.T) {
	cases := []string{"9:00", "24:00", "ab:cd", "00:60", "12:30 "}
	for _, start := range cases {
		s := &ChannelSchedule{Enabled: true, Timezone: "UTC", Windows: []ChannelScheduleWindow{{Start: start, End: "01:00"}}}
		err := s.Validate()
		require.Error(t, err, "input=%q", start)
	}
}

func TestValidate_StartEqualsEnd(t *testing.T) {
	s := &ChannelSchedule{Enabled: true, Timezone: "UTC", Windows: []ChannelScheduleWindow{{Start: "08:00", End: "08:00"}}}
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be equal")
}

func TestValidate_DayOutOfRangeAndDuplicate(t *testing.T) {
	s := &ChannelSchedule{Enabled: true, Timezone: "UTC", Windows: []ChannelScheduleWindow{{Start: "08:00", End: "09:00", Days: []int{0, 7}}}}
	require.Error(t, s.Validate())

	s2 := &ChannelSchedule{Enabled: true, Timezone: "UTC", Windows: []ChannelScheduleWindow{{Start: "08:00", End: "09:00", Days: []int{1, 1}}}}
	require.Error(t, s2.Validate())
}

func TestValidate_DisabledNoTimezone(t *testing.T) {
	s := &ChannelSchedule{Enabled: false}
	require.NoError(t, s.Validate())
}

func TestValidate_OK(t *testing.T) {
	s := &ChannelSchedule{
		Enabled:  true,
		Timezone: "Asia/Shanghai",
		Windows: []ChannelScheduleWindow{
			{Days: []int{1, 2, 3, 4, 5}, Start: "00:30", End: "08:30"},
		},
	}
	require.NoError(t, s.Validate())
}

// at 构建一个指定年/月/日/小时/分钟/星期的时间（Asia/Shanghai 时区，便于可读）。
func at(year int, month time.Month, day, hour, minute int, weekday time.Weekday) time.Time {
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(nil, nil) // intentionally unused; see below
	if err != nil {
		loc = time.UTC
	}
	t := time.Date(year, month, day, hour, minute, 0, 0, loc)
	if t.Weekday() != weekday {
		// 调整日期以匹配 weekday（用于测试；非生产代码路径）
		shift := int(weekday) - int(t.Weekday())
		t = t.AddDate(0, 0, shift)
	}
	return t
}

func TestIsActiveNow_SameDayWindow(t *testing.T) {
	s := mustParseSchedule(t, `{"enabled":true,"timezone":"Asia/Shanghai","windows":[{"days":[1,2,3,4,5],"start":"09:00","end":"18:00"}]}`)

	monday9 := at(2026, time.March, 2, 9, 0, time.Monday)
	monday12 := at(2026, time.March, 2, 12, 0, time.Monday)
	monday18 := at(2026, time.March, 2, 18, 0, time.Monday) // 边界 end, 应不激活
	monday830 := at(2026, time.March, 2, 8, 30, time.Monday)
	saturday12 := at(2026, time.March, 7, 12, 0, time.Saturday)

	assert.True(t, s.IsActiveNow(monday9), "周一9点应在窗口内")
	assert.True(t, s.IsActiveNow(monday12), "周一12点应在窗口内")
	assert.False(t, s.IsActiveNow(monday18), "周一18点整应在窗口外（end边界）")
	assert.False(t, s.IsActiveNow(monday830), "周一8:30在窗口外")
	assert.False(t, s.IsActiveNow(saturday12), "周六不在 days 中")
}

func TestIsActiveNow_OvernightWindow(t *testing.T) {
	s := mustParseSchedule(t, `{"enabled":true,"timezone":"Asia/Shanghai","windows":[{"days":[0,1,2,3,4,5,6],"start":"22:00","end":"02:00"}]}`)

	monday23 := at(2026, time.March, 2, 23, 0, time.Monday)
	tuesday1 := at(2026, time.March, 3, 1, 0, time.Tuesday)
	tuesday3 := at(2026, time.March, 3, 3, 0, time.Tuesday)

	assert.True(t, s.IsActiveNow(monday23), "周一23点在窗口内（start day）")
	assert.True(t, s.IsActiveNow(tuesday1), "周二1点在窗口内（次日）")
	assert.False(t, s.IsActiveNow(tuesday3), "周二3点在窗口外")
}

func TestIsActiveNow_MultipleWindows(t *testing.T) {
	s := mustParseSchedule(t, `{"enabled":true,"timezone":"Asia/Shanghai","windows":[{"days":[0,1,2,3,4,5,6],"start":"09:00","end":"12:00"},{"days":[0,1,2,3,4,5,6],"start":"14:00","end":"18:00"}]}`)
	monday10 := at(2026, time.March, 2, 10, 0, time.Monday)
	monday13 := at(2026, time.March, 2, 13, 0, time.Monday)
	monday16 := at(2026, time.March, 2, 16, 0, time.Monday)
	assert.True(t, s.IsActiveNow(monday10))
	assert.False(t, s.IsActiveNow(monday13))
	assert.True(t, s.IsActiveNow(monday16))
}

func TestIsActiveNow_EmptyDaysMeansEveryDay(t *testing.T) {
	s := mustParseSchedule(t, `{"enabled":true,"timezone":"Asia/Shanghai","windows":[{"days":[],"start":"09:00","end":"18:00"}]}`)
	sunday10 := at(2026, time.March, 1, 10, 0, time.Sunday)
	saturday10 := at(2026, time.March, 7, 10, 0, time.Saturday)
	assert.True(t, s.IsActiveNow(sunday10))
	assert.True(t, s.IsActiveNow(saturday10))

	s2 := mustParseSchedule(t, `{"enabled":true,"timezone":"Asia/Shanghai","windows":[{"days":[0,1,2,3,4,5,6],"start":"09:00","end":"18:00"}]}`)
	assert.True(t, s2.IsActiveNow(sunday10))
	assert.True(t, s2.IsActiveNow(saturday10))
}

func TestIsActiveNow_TimezoneDifference(t *testing.T) {
	// 同一 UTC 时刻：Shanghai +8 时为 23:00（周二），UTC 为 15:00
	// 窗口在 Shanghai 时区 22:00-02:00 → 23:00 Shanghai 应激活
	sShanghai := mustParseSchedule(t, `{"enabled":true,"timezone":"Asia/Shanghai","windows":[{"days":[0,1,2,3,4,5,6],"start":"22:00","end":"02:00"}]}`)
	// 窗口在 UTC 时区 09:00-18:00 → 15:00 UTC 应激活
	sUTC := mustParseSchedule(t, `{"enabled":true,"timezone":"UTC","windows":[{"days":[0,1,2,3,4,5,6],"start":"09:00","end":"18:00"}]}`)

	utcTime := time.Date(2026, time.March, 3, 15, 0, 0, 0, time.UTC)
	assert.True(t, sShanghai.IsActiveNow(utcTime), "Shanghai 23点窗口内")
	assert.True(t, sUTC.IsActiveNow(utcTime), "UTC 15点窗口内")

	// 另一时刻：UTC 12:00 → Shanghai 20:00 → Shanghai 窗口外，UTC 12:00 → UTC 12点 → UTC 窗口内
	utcTime2 := time.Date(2026, time.March, 3, 12, 0, 0, 0, time.UTC)
	assert.False(t, sShanghai.IsActiveNow(utcTime2), "Shanghai 20点在跨零点窗口外（start前）")
	assert.True(t, sUTC.IsActiveNow(utcTime2), "UTC 12点在窗口内")
}

func TestIsActiveNow_DisabledReturnsFalse(t *testing.T) {
	s := &ChannelSchedule{Enabled: false, Timezone: "UTC", Windows: []ChannelScheduleWindow{{Start: "00:00", End: "23:59"}}}
	assert.False(t, s.IsActiveNow(time.Now()))
}

func TestIsActiveNow_InvalidTimezoneReturnsFalse(t *testing.T) {
	s := &ChannelSchedule{Enabled: true, Timezone: "Nowhere/Land", Windows: []ChannelScheduleWindow{{Start: "00:00", End: "23:59"}}}
	assert.False(t, s.IsActiveNow(time.Now()))
}