package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/stretchr/testify/assert"
)

func TestShouldRunDingTalkPatrol(t *testing.T) {
	t.Parallel()

	// Fixed reference points: 2026-08-05 is a Wednesday.
	at := func(hour, min int) time.Time {
		return time.Date(2026, 8, 5, hour, min, 0, 0, time.Local)
	}
	dayMarker := func(t time.Time) int64 { return dingTalkPatrolDayMarker(t) }

	testCases := []struct {
		name       string
		settings   system_setting.DingTalkSettings
		now        time.Time
		lastRunDay int64
		lastRun    time.Time
		expected   bool
	}{
		{
			name:       "daily before configured hour does not run",
			settings:   system_setting.DingTalkSettings{PatrolMode: system_setting.DingTalkPatrolModeDaily, PatrolHour: 3},
			now:        at(2, 59),
			lastRunDay: 0,
			expected:   false,
		},
		{
			name:       "daily at configured hour runs",
			settings:   system_setting.DingTalkSettings{PatrolMode: system_setting.DingTalkPatrolModeDaily, PatrolHour: 3},
			now:        at(3, 0),
			lastRunDay: 0,
			expected:   true,
		},
		{
			name:       "daily after configured hour runs as catch-up",
			settings:   system_setting.DingTalkSettings{PatrolMode: system_setting.DingTalkPatrolModeDaily, PatrolHour: 3},
			now:        at(15, 30),
			lastRunDay: 0,
			expected:   true,
		},
		{
			name:       "daily already ran today is skipped",
			settings:   system_setting.DingTalkSettings{PatrolMode: system_setting.DingTalkPatrolModeDaily, PatrolHour: 3},
			now:        at(4, 0),
			lastRunDay: dayMarker(at(3, 5)),
			expected:   false,
		},
		{
			name:       "daily runs again after midnight rollover",
			settings:   system_setting.DingTalkSettings{PatrolMode: system_setting.DingTalkPatrolModeDaily, PatrolHour: 3},
			now:        at(3, 1),
			lastRunDay: dayMarker(at(3, 0).AddDate(0, 0, -1)),
			expected:   true,
		},
		{
			name:       "daily with hour zero runs any time once per day",
			settings:   system_setting.DingTalkSettings{PatrolMode: system_setting.DingTalkPatrolModeDaily, PatrolHour: 0},
			now:        at(0, 30),
			lastRunDay: 0,
			expected:   true,
		},
		{
			name:     "interval due after full gap",
			settings: system_setting.DingTalkSettings{PatrolMode: system_setting.DingTalkPatrolModeInterval, PatrolIntervalHours: 6},
			now:      at(12, 0),
			lastRun:  at(6, 0),
			expected: true,
		},
		{
			name:     "interval not due before gap elapses",
			settings: system_setting.DingTalkSettings{PatrolMode: system_setting.DingTalkPatrolModeInterval, PatrolIntervalHours: 6},
			now:      at(11, 59),
			lastRun:  at(6, 0),
			expected: false,
		},
		{
			name:     "interval with zero last run is immediately due",
			settings: system_setting.DingTalkSettings{PatrolMode: system_setting.DingTalkPatrolModeInterval, PatrolIntervalHours: 24},
			now:      at(1, 0),
			lastRun:  time.Time{},
			expected: true,
		},
		{
			name:     "interval with non-positive hours never runs",
			settings: system_setting.DingTalkSettings{PatrolMode: system_setting.DingTalkPatrolModeInterval, PatrolIntervalHours: 0},
			now:      at(12, 0),
			lastRun:  at(1, 0),
			expected: false,
		},
		{
			name:       "unknown mode never runs",
			settings:   system_setting.DingTalkSettings{PatrolMode: "weekly", PatrolHour: 3},
			now:        at(12, 0),
			lastRunDay: 0,
			expected:   false,
		},
		{
			name:     "empty mode never runs",
			settings: system_setting.DingTalkSettings{},
			now:      at(12, 0),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := shouldRunDingTalkPatrol(&tc.settings, tc.now, tc.lastRunDay, tc.lastRun)
			assert.Equal(t, tc.expected, got)
		})
	}
}
