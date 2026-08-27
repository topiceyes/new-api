package audit

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

// sendAdminAlert 告警发送入口,测试中可替换为替身(同 service/planmonitor 的 seam 模式)。
var sendAdminAlert = service.SendAdminAlert

// AuditNotifyType 是 SendAdminAlert 限流使用的通知类型标识。
const AuditNotifyType = "security_audit"

// requestMeta 从 gin 上下文与 RelayInfo 同步快照的审计元数据。
// 必须同步快照:handler 返回后 gin.Context 即失效。
type requestMeta struct {
	userId    int
	username  string
	tokenId   int
	tokenName string
	channelId int
	modelName string
	group     string
	requestId string
	ip        string
	userAgent string
}

// auditSettingsSnapshot 返回配置的值拷贝。GetAuditSettings 返回的是共享指针,
// 配置热更新(UpdateConfigFromMap reflect 原地改写)时把指针传进异步闭包构成
// 数据竞争,且一次扫描内多次读可能拿到半新半旧的撕裂配置;入口处拷贝快照。
func auditSettingsSnapshot() *system_setting.AuditSettings {
	settings := *system_setting.GetAuditSettings()
	// key_share_suppress_hours 两条路径对 0 的语义相反(Redis 0 TTL=永久抑制,
	// 内存 0=每条都报),钳制下限统一为 1 小时。
	if settings.KeyShareSuppressHours < 1 {
		settings.KeyShareSuppressHours = 1
	}
	return &settings
}

// InspectRequest 安全审计入口,由 relaycommon.OnRelayInfoReady 回调触发。
// 只做轻量同步快照,扫描/追踪/写库全部异步,不阻塞 relay 主路径。
func InspectRequest(c *gin.Context, info *relaycommon.RelayInfo) {
	settings := auditSettingsSnapshot()
	if !settings.Enabled {
		return
	}
	if info == nil || info.UserId == 0 {
		return
	}

	meta := requestMeta{
		userId:    info.UserId,
		username:  c.GetString("username"),
		tokenId:   info.TokenId,
		tokenName: c.GetString("token_name"),
		channelId: common.GetContextKeyInt(c, constant.ContextKeyChannelId),
		modelName: info.OriginModelName,
		group:     info.UsingGroup,
		requestId: info.RequestId,
		ip:        c.ClientIP(),
	}
	if c.Request != nil {
		meta.userAgent = truncateRunes(c.Request.UserAgent(), 512)
	}

	promptText := ""
	if settings.PromptScanEnabled && info.Request != nil {
		promptText = ExtractPromptText(info.Request, settings.MaxScanBytes)
	}

	storeMode := effectiveStorePromptMode(settings, info.UserSetting.AuditStorePromptMode, meta.group)

	gopool.Go(func() {
		inspectAsync(settings, meta, promptText, storeMode)
	})
}

func inspectAsync(settings *system_setting.AuditSettings, meta requestMeta, promptText string, storeMode string) {
	hits := ScanPrompt(promptText)

	// 1. PII 命中事件
	storedPrompt := storedPromptFor(storeMode, promptText, len(hits) > 0)
	for _, hit := range hits {
		event := &model.AuditEvent{
			EventType: model.AuditEventTypePiiHit,
			Severity:  hit.Severity,
			RuleId:    hit.RuleId,
			RuleName:  hit.RuleName,
			Excerpt:   hit.Excerpt,
			Detail:    common.MapToJsonStr(map[string]any{"count": hit.Count}),
			Prompt:    storedPrompt,
		}
		fillEventMeta(event, meta)
		if err := model.CreateAuditEvent(event); err != nil {
			common.SysError("audit: failed to create pii event: " + err.Error())
			continue
		}
		if settings.AlertEnabled && hit.Severity == system_setting.AuditSeverityCritical {
			alertCriticalHit(meta, hit)
		}
	}

	// 2. 有效存储模式为 "all":无命中也留 prompt 快照,供行为分析。
	// 音量由管理员显式开启自负,保留期由 RetentionDays 兜底。
	if settings.PromptScanEnabled && storeMode == system_setting.AuditStorePromptAll && promptText != "" {
		event := &model.AuditEvent{
			EventType: model.AuditEventTypePromptSnapshot,
			Severity:  system_setting.AuditSeverityInfo,
			Prompt:    storedPromptFor(storeMode, promptText, true),
		}
		fillEventMeta(event, meta)
		if err := model.CreateAuditEvent(event); err != nil {
			common.SysError("audit: failed to create prompt snapshot: " + err.Error())
		}
	}

	// 3. key 分享信号
	if settings.KeyShareEnabled {
		for _, signal := range TrackKeyShare(meta.tokenId, meta.ip, meta.userAgent, settings) {
			severity := signal.Severity
			if severity == "" {
				severity = system_setting.AuditSeverityWarning
			}
			event := &model.AuditEvent{
				EventType: signal.EventType,
				Severity:  severity,
				Detail:    common.MapToJsonStr(signal.Detail),
			}
			fillEventMeta(event, meta)
			if err := model.CreateAuditEvent(event); err != nil {
				common.SysError("audit: failed to create keyshare event: " + err.Error())
				continue
			}
			if settings.AlertEnabled && severity == system_setting.AuditSeverityCritical {
				alertKeyShareCritical(meta, signal)
			}
		}
	}
}

// storedPromptFor 按有效存储模式决定事件是否携带 prompt 原文。
func storedPromptFor(storeMode string, promptText string, hasHit bool) string {
	switch storeMode {
	case system_setting.AuditStorePromptAll:
		return promptText
	case system_setting.AuditStorePromptHits:
		if hasHit {
			return promptText
		}
	}
	return ""
}

// effectiveStorePromptMode 计算请求的有效 prompt 存储模式,优先级:
// 用户级(UserSetting)> 分组策略(GroupStorePromptModes)> 全局(StorePromptMode)。
// 非法值归一为"跟随全局",继续向下回退。
func effectiveStorePromptMode(settings *system_setting.AuditSettings, userMode string, group string) string {
	if isValidStorePromptMode(userMode) {
		return userMode
	}
	for _, policy := range settings.GroupStorePromptModes {
		if policy.Group == group && isValidStorePromptMode(policy.Mode) {
			return policy.Mode
		}
	}
	return settings.StorePromptMode
}

func isValidStorePromptMode(mode string) bool {
	switch mode {
	case system_setting.AuditStorePromptNone, system_setting.AuditStorePromptHits, system_setting.AuditStorePromptAll:
		return true
	}
	return false
}

func fillEventMeta(event *model.AuditEvent, meta requestMeta) {
	event.UserId = meta.userId
	event.Username = meta.username
	event.TokenId = meta.tokenId
	event.TokenName = meta.tokenName
	event.ChannelId = meta.channelId
	event.ModelName = meta.modelName
	event.Group = meta.group
	event.RequestId = meta.requestId
	event.Ip = meta.ip
	event.UserAgent = meta.userAgent
}

// alertCriticalHit critical 级命中走管理告警(自带限流与钉钉/飞书 fan-out)。
func alertCriticalHit(meta requestMeta, hit RuleHit) {
	subject := "安全审计告警：检测到高危敏感信息"
	content := fmt.Sprintf(
		"用户 %s(ID:%d) 的请求命中规则「%s」(%d 次)\n令牌: %s(ID:%d)\n模型: %s\n命中摘要: %s\nIP: %s\n时间: %s",
		meta.username, meta.userId, hit.RuleName, hit.Count,
		meta.tokenName, meta.tokenId, meta.modelName, hit.Excerpt, meta.ip,
		time.Now().Format("2006-01-02 15:04:05"),
	)
	sendAdminAlert(AuditNotifyType, subject, content)
}

// alertKeyShareCritical critical 级 key 分享信号(如不可能移动)告警。
func alertKeyShareCritical(meta requestMeta, signal KeyShareSignal) {
	subject := "安全审计告警：检测到令牌异常使用"
	content := fmt.Sprintf(
		"用户 %s(ID:%d) 的令牌 %s(ID:%d) 触发信号「%s」\n详情: %s\nIP: %s\n时间: %s",
		meta.username, meta.userId, meta.tokenName, meta.tokenId, signal.EventType,
		common.MapToJsonStr(signal.Detail), meta.ip,
		time.Now().Format("2006-01-02 15:04:05"),
	)
	sendAdminAlert(AuditNotifyType, subject, content)
}
