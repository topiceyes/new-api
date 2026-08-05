package system_setting

import "github.com/QuantumNous/new-api/setting/config"

type FeishuSettings struct {
	Enabled   bool   `json:"enabled"`
	AppId     string `json:"app_id"`
	AppSecret string `json:"app_secret"`

	// Departed-employee patrol: independently configurable schedule for the
	// audit that disables accounts of users who left the organization.
	PatrolEnabled       bool   `json:"patrol_enabled"`
	PatrolMode          string `json:"patrol_mode"`           // FeishuPatrolModeDaily or FeishuPatrolModeInterval
	PatrolHour          int    `json:"patrol_hour"`           // 0-23, daily mode: first run at or after this local hour
	PatrolIntervalHours int    `json:"patrol_interval_hours"` // 1-24, interval mode: gap between runs
}

// Patrol schedule modes.
const (
	FeishuPatrolModeDaily    = "daily"    // run once per day at/after PatrolHour
	FeishuPatrolModeInterval = "interval" // run every PatrolIntervalHours
)

// 默认配置
var defaultFeishuSettings = FeishuSettings{
	PatrolEnabled:       true,
	PatrolMode:          FeishuPatrolModeDaily,
	PatrolHour:          3,
	PatrolIntervalHours: 24,
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("feishu", &defaultFeishuSettings)
}

func GetFeishuSettings() *FeishuSettings {
	return &defaultFeishuSettings
}
