package dto

import (
	"encoding/json"
	"fmt"
	"time"
)

// ChannelScheduleWindow 表示一个开启时段。Days 为 0=周日..6=周六，空数组或全 7 天表示每天。
// Start/End 为 HH:MM。当 End <= Start 时视为跨零点窗口（如 22:00-02:00，归属 Start 当天）。
// 边界约定为左闭右开 [start, end)。
type ChannelScheduleWindow struct {
	Days  []int  `json:"days"`
	Start string `json:"start"`
	End   string `json:"end"`
}

// ChannelSchedule 渠道定时开关配置。渠道在任一窗口内视为应启用，窗口外视为应禁用。
type ChannelSchedule struct {
	Enabled  bool                    `json:"enabled"`
	Timezone string                  `json:"timezone"`
	Windows  []ChannelScheduleWindow `json:"windows"`
}

// ParseChannelSchedule 解析 schedule JSON。空串返回 (nil, nil)。
func ParseChannelSchedule(raw string) (*ChannelSchedule, error) {
	if raw == "" {
		return nil, nil
	}
	s := &ChannelSchedule{}
	if err := json.Unmarshal([]byte(raw), s); err != nil {
		return nil, err
	}
	return s, nil
}

// Validate 校验时区、时间格式、星期范围与窗口合法性。
func (s *ChannelSchedule) Validate() error {
	if s == nil {
		return nil
	}
	if s.Timezone == "" {
		if s.Enabled {
			return fmt.Errorf("timezone is required when schedule is enabled")
		}
	} else if _, err := time.LoadLocation(s.Timezone); err != nil {
		return fmt.Errorf("invalid timezone %q: %w", s.Timezone, err)
	}
	if s.Enabled && len(s.Windows) == 0 {
		return fmt.Errorf("at least one window is required when schedule is enabled")
	}
	for i, w := range s.Windows {
		if err := w.validate(); err != nil {
			return fmt.Errorf("window %d: %w", i, err)
		}
	}
	return nil
}

func (w *ChannelScheduleWindow) validate() error {
	sM, err := parseHHMM(w.Start)
	if err != nil {
		return fmt.Errorf("invalid start time %q", w.Start)
	}
	eM, err := parseHHMM(w.End)
	if err != nil {
		return fmt.Errorf("invalid end time %q", w.End)
	}
	if sM == eM {
		return fmt.Errorf("start and end time cannot be equal")
	}
	seen := make(map[int]struct{}, len(w.Days))
	for _, d := range w.Days {
		if d < 0 || d > 6 {
			return fmt.Errorf("day %d out of range [0,6]", d)
		}
		if _, dup := seen[d]; dup {
			return fmt.Errorf("duplicate day %d", d)
		}
		seen[d] = struct{}{}
	}
	return nil
}

// IsActiveNow 判断 now 是否落在任一开启窗口内（按 Timezone 时区）。
func (s *ChannelSchedule) IsActiveNow(now time.Time) bool {
	if s == nil || !s.Enabled || len(s.Windows) == 0 {
		return false
	}
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return false
	}
	t := now.In(loc)
	mins := t.Hour()*60 + t.Minute()
	wd := int(t.Weekday())
	for i := range s.Windows {
		if s.Windows[i].contains(wd, mins) {
			return true
		}
	}
	return false
}

// contains 判断（星期几 wd，分钟 mins）是否在该窗口内。跨零点窗口归属 start 当天。
func (w *ChannelScheduleWindow) contains(wd, mins int) bool {
	sM, err1 := parseHHMM(w.Start)
	eM, err2 := parseHHMM(w.End)
	if err1 != nil || err2 != nil {
		return false
	}
	if sM < eM {
		// 同日窗口
		return w.hasDay(wd) && mins >= sM && mins < eM
	}
	// 跨零点窗口：[start, 24:00) 归属 start 当天；[00:00, end) 归属次日（即 start+1 天）
	if w.hasDay(wd) && mins >= sM {
		return true
	}
	return w.hasDay((wd+6)%7) && mins < eM
}

// hasDay 空数组或全 7 天均视为每天。
func (w *ChannelScheduleWindow) hasDay(wd int) bool {
	if len(w.Days) == 0 || len(w.Days) == 7 {
		return true
	}
	for _, d := range w.Days {
		if d == wd {
			return true
		}
	}
	return false
}

// parseHHMM 解析 HH:MM 为当日分钟数。要求严格两位小时两位分钟。
func parseHHMM(s string) (int, error) {
	if len(s) != 5 || s[2] != ':' {
		return 0, fmt.Errorf("not HH:MM format")
	}
	var h, m int
	for _, c := range s[:2] {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid hour")
		}
		h = h*10 + int(c-'0')
	}
	for _, c := range s[3:] {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid minute")
		}
		m = m*10 + int(c-'0')
	}
	if h > 23 || m > 59 {
		return 0, fmt.Errorf("time out of range")
	}
	return h*60 + m, nil
}
