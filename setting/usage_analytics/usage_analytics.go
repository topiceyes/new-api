package usage_analytics

import "github.com/QuantumNous/new-api/setting/config"

// UsageAnalyticsSettings 使用分析配置:日聚合任务与聚合表保留期。
// 字段保持扁平(config 包按字段序列化为 usage_analytics.xxx 独立 option)。
type UsageAnalyticsSettings struct {
	// Enabled 为 false 时 usage_aggregate 定时任务不调度,聚合表停止更新(已有数据仍可查)。
	Enabled bool `json:"enabled"`
	// BackfillDays 首次运行/缺漏时向前回填的天数,应与日志保留时长(约 90 天)对齐。
	BackfillDays int `json:"backfill_days"`
	// AggregateRetentionDays 聚合表自身保留天数,独立于日志清理,聚合可以比日志活得久。
	AggregateRetentionDays int `json:"aggregate_retention_days"`
	// IncludeToday 为 true 时每小时重算"今天至今"的部分天,看板能见到当天数据。
	IncludeToday bool `json:"include_today"`
}

var defaultUsageAnalyticsSettings = UsageAnalyticsSettings{
	Enabled:                true,
	BackfillDays:           90,
	AggregateRetentionDays: 365,
	IncludeToday:           true,
}

func init() {
	config.GlobalConfig.Register("usage_analytics", &defaultUsageAnalyticsSettings)
}

func GetUsageAnalyticsSettings() *UsageAnalyticsSettings {
	return &defaultUsageAnalyticsSettings
}
