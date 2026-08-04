package system_setting

import "github.com/QuantumNous/new-api/setting/config"

type DingTalkSettings struct {
	Enabled      bool   `json:"enabled"`
	AppKey       string `json:"app_key"`
	AppSecret    string `json:"app_secret"`
	CorpId       string `json:"corp_id"`
	ClientId     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`

	// Departed-employee patrol: independently configurable schedule for the
	// daily audit that disables accounts of users who left the organization.
	PatrolEnabled       bool   `json:"patrol_enabled"`
	PatrolMode          string `json:"patrol_mode"`           // DingTalkPatrolModeDaily or DingTalkPatrolModeInterval
	PatrolHour          int    `json:"patrol_hour"`           // 0-23, daily mode: first run at or after this local hour
	PatrolIntervalHours int    `json:"patrol_interval_hours"` // 1-24, interval mode: gap between runs
}

// Patrol schedule modes.
const (
	DingTalkPatrolModeDaily    = "daily"    // run once per day at/after PatrolHour
	DingTalkPatrolModeInterval = "interval" // run every PatrolIntervalHours
)

// 默认配置
// Patrol defaults preserve the historical behavior (daily audit at local
// hour 3) so upgrades keep running without any configuration change.
var defaultDingTalkSettings = DingTalkSettings{
	PatrolEnabled:       true,
	PatrolMode:          DingTalkPatrolModeDaily,
	PatrolHour:          3,
	PatrolIntervalHours: 24,
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("dingtalk", &defaultDingTalkSettings)
}

func GetDingTalkSettings() *DingTalkSettings {
	return &defaultDingTalkSettings
}
