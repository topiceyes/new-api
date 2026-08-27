package model

import (
	"github.com/QuantumNous/new-api/common"
)

// 安全审计事件类型(字符串而非 int,便于后续扩展新类型不改语义)。
const (
	AuditEventTypePiiHit          = "pii_hit"
	AuditEventTypeKeyShareMultiIP = "key_share_multi_ip"
	AuditEventTypeKeyShareRapidIP = "key_share_rapid_ip"
	AuditEventTypeKeyShareMultiUA = "key_share_multi_ua"
	// AuditEventTypePromptSnapshot StorePromptMode=all 时的全量 prompt 快照事件。
	AuditEventTypePromptSnapshot = "prompt_snapshot"
	// AuditEventTypeResponseMalicious 入方向模型返回命中恶意代码特征。
	AuditEventTypeResponseMalicious = "response_malicious"
	// AuditEventTypeKeyShareImpossibleTravel GeoIP 判定的"不可能移动"(同一令牌
	// 在物理不可达的时间内从两个相距遥远的地点使用)。
	AuditEventTypeKeyShareImpossibleTravel = "key_share_impossible_travel"
)

// AuditEvent 安全/行为审计事件:prompt 敏感内容命中、key 分享信号等。
// 只记录命中(低音量),存主库而非 LOG_DB(可能是 ClickHouse)。
// 与 logs 表的管理操作审计(LogTypeManage)相互独立。
type AuditEvent struct {
	Id        int    `json:"id"`
	CreatedAt int64  `json:"created_at" gorm:"bigint;index:idx_audit_created_type,priority:1"`
	EventType string `json:"event_type" gorm:"type:varchar(32);index:idx_audit_created_type,priority:2"`
	Severity  string `json:"severity" gorm:"type:varchar(16);index"`
	UserId    int    `json:"user_id" gorm:"index"`
	Username  string `json:"username" gorm:"default:''"`
	TokenId   int    `json:"token_id" gorm:"index;default:0"`
	TokenName string `json:"token_name" gorm:"default:''"`
	ChannelId int    `json:"channel_id" gorm:"default:0"`
	ModelName string `json:"model_name" gorm:"default:''"`
	Group     string `json:"group" gorm:"default:''"`
	Ip        string `json:"ip" gorm:"type:varchar(64);default:''"`
	UserAgent string `json:"user_agent" gorm:"type:varchar(512);default:''"`
	RequestId string `json:"request_id" gorm:"type:varchar(64);index;default:''"`
	// RuleId 命中的规则标识(内置规则为稳定常量,自定义规则由管理员指定)。
	RuleId   string `json:"rule_id" gorm:"type:varchar(64);default:''"`
	RuleName string `json:"rule_name" gorm:"type:varchar(128);default:''"`
	// Excerpt 命中片段打码后的摘要(保留前3后2,中间 *),绝不存明文。
	Excerpt string `json:"excerpt" gorm:"type:varchar(256);default:''"`
	// Detail JSON:命中次数、key 分享信号的去重 IP/UA 列表等结构化详情。
	Detail string `json:"detail" gorm:"type:text"`
	// Prompt 原文,受 system_setting.AuditSettings.StorePromptMode 控制,未存时为""。
	Prompt string `json:"prompt" gorm:"type:text"`
	// Category LLM 分类结果(二期②),未分类时为""。
	Category string `json:"category" gorm:"type:varchar(32);index;default:''"`
}

func (AuditEvent) TableName() string { return "audit_events" }

func CreateAuditEvent(event *AuditEvent) error {
	if event.CreatedAt == 0 {
		event.CreatedAt = common.GetTimestamp()
	}
	return DB.Create(event).Error
}

func GetAuditEventById(id int) (*AuditEvent, error) {
	var event AuditEvent
	if err := DB.First(&event, id).Error; err != nil {
		return nil, err
	}
	return &event, nil
}

// GetAuditEvents 管理端分页查询,全部走 GORM 方法保证三库兼容。
func GetAuditEvents(eventType string, severity string, userId int, tokenId int, keyword string, startTimestamp int64, endTimestamp int64, startIdx int, num int) (events []*AuditEvent, total int64, err error) {
	tx := DB.Model(&AuditEvent{})
	if eventType != "" {
		tx = tx.Where("event_type = ?", eventType)
	}
	if severity != "" {
		tx = tx.Where("severity = ?", severity)
	}
	if userId != 0 {
		tx = tx.Where("user_id = ?", userId)
	}
	if tokenId != 0 {
		tx = tx.Where("token_id = ?", tokenId)
	}
	if keyword != "" {
		tx = tx.Where("username LIKE ? OR rule_name LIKE ? OR model_name LIKE ?", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if err = tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	// 列表页不返回 prompt 原文,详情接口单独取,避免大字段拖慢列表。
	err = tx.Select("id", "created_at", "event_type", "severity", "user_id", "username",
		"token_id", "token_name", "channel_id", "model_name", commonGroupCol,
		"ip", "user_agent", "request_id", "rule_id", "rule_name", "excerpt", "detail", "category").
		Order("created_at desc, id desc").Limit(num).Offset(startIdx).Find(&events).Error
	return events, total, err
}

// AuditEventStatRow 聚合统计行:按事件类型 + 规则分组计数。
type AuditEventStatRow struct {
	EventType string `json:"event_type"`
	RuleId    string `json:"rule_id"`
	Count     int64  `json:"count"`
}

func GetAuditEventStats(startTimestamp int64, endTimestamp int64) ([]AuditEventStatRow, error) {
	var rows []AuditEventStatRow
	tx := DB.Model(&AuditEvent{}).
		Select("event_type, rule_id, COUNT(*) as count").
		Group("event_type, rule_id")
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	err := tx.Scan(&rows).Error
	return rows, err
}

// DeleteExpiredAuditEvents 清理超过保留期的事件,由定时任务调用。
func DeleteExpiredAuditEvents(retentionDays int, now int64) (int64, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	cutoff := now - int64(retentionDays)*86400
	tx := DB.Where("created_at < ?", cutoff).Delete(&AuditEvent{})
	return tx.RowsAffected, tx.Error
}

// GetUnclassifiedPromptEvents 取带 prompt 原文且尚未分类的事件,供 LLM 分类任务。
// 只取 prompt_snapshot(hits 模式下 pii_hit 存的恰恰是命中密钥/证件的敏感原文,
// 不应整段送第三方分类渠道);已标 classify_failed 的不再重试,防"毒批"队头阻塞。
func GetUnclassifiedPromptEvents(batchSize int) ([]*AuditEvent, error) {
	var events []*AuditEvent
	err := DB.Where("prompt != '' AND category = '' AND event_type = ?", AuditEventTypePromptSnapshot).
		Select("id", "user_id", "prompt").
		Order("id asc").Limit(batchSize).Find(&events).Error
	return events, err
}

// UpdateAuditEventCategory 写入单条事件的分类结果。
func UpdateAuditEventCategory(id int, category string) error {
	return DB.Model(&AuditEvent{}).Where("id = ?", id).Update("category", category).Error
}
