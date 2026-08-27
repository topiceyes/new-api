package audit

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

// captureReader 包装上游响应 Body:Read 透传的同时把字节追加进上限截断的 buffer,
// 不影响流式转发的实时性。非线程安全——HTTP body 本就单 goroutine 顺序消费。
type captureReader struct {
	r   io.Reader
	buf []byte
	max int
}

func (c *captureReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 && len(c.buf) < c.max {
		remain := c.max - len(c.buf)
		take := n
		if take > remain {
			take = remain
		}
		c.buf = append(c.buf, p[:take]...)
	}
	return n, err
}

func (c *captureReader) Close() error {
	if rc, ok := c.r.(io.Closer); ok {
		return rc.Close()
	}
	return nil
}

// CaptureResponse 在 handler 调用 adaptor.DoResponse 前包装上游响应体进行旁路捕获,
// 返回 finish 函数:DoResponse 返回后(含提前 return 分支)必须调用,finish 幂等。
// 审计未启用 / 响应扫描关闭 / 响应为空时返回 nil,调用方零开销。
// gin.Context 只在本函数内同步读,异步扫描不持有 c。
func CaptureResponse(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (finish func()) {
	settings := auditSettingsSnapshot()
	if !settings.Enabled || !settings.ResponseScanEnabled {
		return nil
	}
	if resp == nil || resp.Body == nil || info == nil {
		return nil
	}

	// 同步快照元数据:handler 返回后 gin.Context 即失效
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

	maxBytes := settings.ResponseMaxScanBytes
	if maxBytes <= 0 {
		maxBytes = 65536
	}
	cr := &captureReader{r: resp.Body, max: maxBytes}
	resp.Body = cr

	var once sync.Once
	return func() {
		once.Do(func() {
			if len(cr.buf) == 0 {
				return
			}
			// 复制一份再进异步,避免后续(理论上)继续读造成数据竞争
			captured := make([]byte, len(cr.buf))
			copy(captured, cr.buf)
			gopool.Go(func() {
				inspectResponse(meta, string(captured))
			})
		})
	}
}

// jsonEscapeReplacer 把捕获到的 JSON/SSE 原始字节里的转义序列还原成真实字符。
// 响应体是 JSON 转义文本(`\n` 是两个字符),不还原时恶意载荷前的字面 'n' 会破坏
// 规则的 \b 词边界导致漏扫。仅用于扫描,不回写捕获内容。
var jsonEscapeReplacer = strings.NewReplacer(
	`\n`, "\n",
	`\t`, "\t",
	`\r`, "\r",
	`\"`, `"`,
	`\\`, `\`,
)

// inspectResponse 异步扫描上游响应文本,命中写 response_malicious 事件。
// 响应侧不存原文(一期原则:observe-only,只留打码摘要)。
func inspectResponse(meta requestMeta, text string) {
	hits := ScanResponse(jsonEscapeReplacer.Replace(text))
	for _, hit := range hits {
		event := &model.AuditEvent{
			EventType: model.AuditEventTypeResponseMalicious,
			Severity:  hit.Severity,
			RuleId:    hit.RuleId,
			RuleName:  hit.RuleName,
			Excerpt:   hit.Excerpt,
			Detail:    common.MapToJsonStr(map[string]any{"count": hit.Count}),
		}
		fillEventMeta(event, meta)
		if err := model.CreateAuditEvent(event); err != nil {
			common.SysError("audit: failed to create response event: " + err.Error())
			continue
		}
		if hit.Severity == system_setting.AuditSeverityCritical && auditSettingsSnapshot().AlertEnabled {
			alertCriticalResponseHit(meta, hit)
		}
	}
}

// alertCriticalResponseHit 入方向 critical 命中告警,subject 与出方向区分。
func alertCriticalResponseHit(meta requestMeta, hit RuleHit) {
	subject := "安全审计告警：模型返回内容命中恶意特征"
	content := fmt.Sprintf(
		"用户 %s(ID:%d) 请求的模型返回命中规则「%s」(%d 次)\n令牌: %s(ID:%d)\n模型: %s\n渠道ID: %d\n命中摘要: %s\nIP: %s\n时间: %s\n可能原因:上游渠道/中转站被投毒,建议核查渠道来源",
		meta.username, meta.userId, hit.RuleName, hit.Count,
		meta.tokenName, meta.tokenId, meta.modelName, meta.channelId, hit.Excerpt, meta.ip,
		time.Now().Format("2006-01-02 15:04:05"),
	)
	sendAdminAlert(AuditNotifyType, subject, content)
}
