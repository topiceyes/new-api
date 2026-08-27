package system_setting

import "github.com/QuantumNous/new-api/setting/config"

// AuditSettings 安全审计配置:出方向 prompt 敏感内容识别(一期 observe-only,
// 不做处置)与 key 分享行为信号检测。事件写入 model.AuditEvent,与 logs 表的
// 管理操作审计(LogTypeManage)相互独立。
//
// 字段保持扁平(config 包按字段序列化为 audit.xxx 独立 option,前端按类型
// 自动解析);Rules 与 GroupStorePromptModes 是例外,各自整体序列化为一个
// JSON 数组 option。
type AuditSettings struct {
	Enabled           bool `json:"enabled"`
	PromptScanEnabled bool `json:"prompt_scan_enabled"`
	// StorePromptMode 控制 prompt 原文存储: "none" 不存 / "hits" 仅命中存 / "all" 全量存。
	StorePromptMode string `json:"store_prompt_mode"`
	// MaxScanBytes 限制单次扫描的提取文本长度,避免超长上下文拖慢规则匹配。
	MaxScanBytes int `json:"max_scan_bytes"`
	// AlertEnabled 为 true 时,critical 严重度事件通过 service.SendAdminAlert 推送。
	AlertEnabled bool `json:"alert_enabled"`
	// RetentionDays 审计事件保留天数,超出后由定时清理任务删除。
	RetentionDays int `json:"retention_days"`

	// key 分享检测:同一令牌在时间窗内的 IP/UA 多样性信号。纯 observe,
	// 触发即写审计事件,不做封禁。
	KeyShareEnabled bool `json:"key_share_enabled"`
	// KeyShareWindowMinutes 长窗口(默认 1440=24h),窗口内去重 IP 数超过
	// KeyShareDistinctIPThreshold 判定疑似分享;UA 多样性复用同一阈值。
	KeyShareWindowMinutes       int `json:"key_share_window_minutes"`
	KeyShareDistinctIPThreshold int `json:"key_share_distinct_ip_threshold"`
	// KeyShareRapidWindowMinutes 短窗口(默认 10 分钟),窗口内去重 IP 数超过
	// KeyShareRapidIPThreshold 判定快速切换(无 GeoIP 时"不可能移动"的近似)。
	KeyShareRapidWindowMinutes int `json:"key_share_rapid_window_minutes"`
	KeyShareRapidIPThreshold   int `json:"key_share_rapid_ip_threshold"`
	// KeyShareSuppressHours 同一 token 同一信号类型的事件抑制时长,避免告警轰炸。
	KeyShareSuppressHours int `json:"key_share_suppress_hours"`

	// Rules 自定义正则审计规则,与内置规则合并。Id 需稳定(事件按 RuleId 关联),
	// 坏 regex 在编译期被跳过并记录 SysError,不影响其他规则。
	Rules []AuditRule `json:"rules"`

	// 入方向(模型返回)恶意内容审计:捕获上游响应字节(含 SSE 流),命中反弹 shell、
	// 管道执行、凭据窃取等特征时写 response_malicious 事件。observe-only,不阻断返回。
	ResponseScanEnabled bool `json:"response_scan_enabled"`
	// ResponseMaxScanBytes 单次响应的最大捕获/扫描字节数,超出部分不扫。
	ResponseMaxScanBytes int `json:"response_max_scan_bytes"`

	// GroupStorePromptModes 分组级 prompt 存储策略(二期③)。用户级设置
	// (UserSetting.AuditStorePromptMode)优先于分组策略,分组策略优先于全局
	// StorePromptMode;非法 mode 值归一为"跟随全局"。
	GroupStorePromptModes []GroupPromptPolicy `json:"group_store_prompt_modes"`

	// GeoIPDBPath MaxMind GeoLite2-City.mmdb 文件路径(二期④)。为空或文件
	// 不可用时"不可能移动"检测自动停用,其余 key 分享信号不受影响。
	GeoIPDBPath string `json:"geoip_db_path"`

	// LLM 分类 + skill 沉淀(二期②):定时任务取带 prompt 原文的未分类事件,
	// 经 OpenAI 兼容渠道分类后写入 audit_events.category 并归并 skill 候选。
	// 未配置渠道/模型时任务自动停用(安静降级)。
	ClassifyEnabled   bool   `json:"classify_enabled"`
	ClassifyChannelId int    `json:"classify_channel_id"`
	ClassifyModel     string `json:"classify_model"`
	// ClassifyIntervalMinutes 分类任务的运行间隔。
	ClassifyIntervalMinutes int `json:"classify_interval_minutes"`
	// ClassifyBatchSize 每轮最多分类的事件条数。
	ClassifyBatchSize int `json:"classify_batch_size"`
}

// GroupPromptPolicy 分组级 prompt 存储策略:命中 Group 的请求按 Mode 存储。
type GroupPromptPolicy struct {
	Group string `json:"group"`
	Mode  string `json:"mode"` // ""=跟随全局,none/hits/all
}

// AuditRule 自定义正则审计规则。
type AuditRule struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	Regex    string `json:"regex"`
	Severity string `json:"severity"` // info / warning / critical
	Enabled  bool   `json:"enabled"`
}

// StorePromptMode 取值。
const (
	AuditStorePromptNone = "none"
	AuditStorePromptHits = "hits"
	AuditStorePromptAll  = "all"
)

// 审计事件严重度。
const (
	AuditSeverityInfo     = "info"
	AuditSeverityWarning  = "warning"
	AuditSeverityCritical = "critical"
)

var defaultAuditSettings = AuditSettings{
	Enabled:                     false,
	PromptScanEnabled:           true,
	StorePromptMode:             AuditStorePromptNone,
	MaxScanBytes:                32768,
	AlertEnabled:                true,
	RetentionDays:               90,
	KeyShareEnabled:             true,
	KeyShareWindowMinutes:       1440,
	KeyShareDistinctIPThreshold: 5,
	KeyShareRapidWindowMinutes:  10,
	KeyShareRapidIPThreshold:    3,
	KeyShareSuppressHours:       24,
	Rules:                       []AuditRule{},
	ResponseScanEnabled:         true,
	ResponseMaxScanBytes:        65536,
	GroupStorePromptModes:       []GroupPromptPolicy{},
	GeoIPDBPath:                 "",
	ClassifyEnabled:             false,
	ClassifyChannelId:           0,
	ClassifyModel:               "",
	ClassifyIntervalMinutes:     60,
	ClassifyBatchSize:           20,
}

func init() {
	config.GlobalConfig.Register("audit", &defaultAuditSettings)
}

func GetAuditSettings() *AuditSettings {
	return &defaultAuditSettings
}
