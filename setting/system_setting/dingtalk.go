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

	// Org-structure sync: periodic snapshot of departments and members from
	// the DingTalk address book, shown on the admin organization page.
	OrgSyncEnabled       bool   `json:"orgsync_enabled"`
	OrgSyncIntervalHours int    `json:"orgsync_interval_hours"` // 1-168, gap between scheduled syncs
	// 主管分组映射:开启后,部门主管自动加入 OrgSyncTargetGroup;卸任时恢复
	// 到映射前分组。只动同步写入的分组,不覆盖管理员手动调整。
	OrgSyncMapGroup    bool   `json:"orgsync_map_group"`
	OrgSyncTargetGroup string `json:"orgsync_target_group"`
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
	PatrolEnabled:        true,
	PatrolMode:           DingTalkPatrolModeDaily,
	PatrolHour:           3,
	PatrolIntervalHours:  24,
	OrgSyncEnabled:       false,
	OrgSyncIntervalHours: 6,
	OrgSyncMapGroup:      false,
	OrgSyncTargetGroup:   "",
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("dingtalk", &defaultDingTalkSettings)
}

func GetDingTalkSettings() *DingTalkSettings {
	return &defaultDingTalkSettings
}
