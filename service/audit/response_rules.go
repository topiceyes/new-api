package audit

import (
	"regexp"
	"sync"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// 入方向(模型返回)恶意内容规则。与 prompt 规则分离:扫描对象是上游原始响应字节
// (SSE data 行或非流式 JSON),恶意代码特征在转义文本中原样可见。
// 二期响应规则全部内置,不支持自定义(自定义 audit.rules 只作用于 prompt)。
// Id 为稳定常量,事件按 RuleId 关联,请勿改名。
var responseRuleSpecs = []struct {
	id       string
	name     string
	severity string
	pattern  string
}{
	{"resp.reverse_shell_bash", "反弹 Shell (bash)", system_setting.AuditSeverityCritical, `bash\s+-i\b|/dev/tcp/`},
	{"resp.reverse_shell_nc", "反弹 Shell (nc)", system_setting.AuditSeverityCritical, `\b(?:nc|ncat|netcat)\s+[^\n]{0,40}-e\s`},
	{"resp.pipe_to_shell", "管道执行远程脚本", system_setting.AuditSeverityCritical, `\b(?:curl|wget)\s+[^|\n]{4,200}\|\s*(?:sudo\s+)?(?:ba|z|da)?sh\b`},
	{"resp.webshell_eval", "Webshell 特征", system_setting.AuditSeverityCritical, `eval\s*\(\s*(?:base64_decode|\$_(?:POST|GET|REQUEST))`},
	{"resp.base64_pipe_sh", "Base64 解码执行", system_setting.AuditSeverityWarning, `base64\s+(?:-d|--decode)[^|\n]{0,100}\|\s*(?:ba)?sh\b`},
	{"resp.read_ssh_key", "读取 SSH 私钥", system_setting.AuditSeverityWarning, `\.ssh/id_rsa\b`},
	{"resp.read_aws_credentials", "读取 AWS 凭据", system_setting.AuditSeverityWarning, `\.aws/credentials`},
	{"resp.read_etc_passwd", "读取 /etc/passwd", system_setting.AuditSeverityWarning, `/etc/passwd`},
	{"resp.long_base64_blob", "超长 Base64 数据段", system_setting.AuditSeverityWarning, `\b[A-Za-z0-9+/]{200,}={0,2}\b`},
}

var (
	responseRulesOnce sync.Once
	responseRules     []compiledRule
)

// activeResponseRules 惰性编译内置响应规则;坏 regex 跳过并记日志(编译期错误属代码 bug,
// 但宁可跳过单条也不能让整个扫描失效)。
func activeResponseRules() []compiledRule {
	responseRulesOnce.Do(func() {
		compiled := make([]compiledRule, 0, len(responseRuleSpecs))
		for _, spec := range responseRuleSpecs {
			re, err := regexp.Compile(spec.pattern)
			if err != nil {
				common.SysError("audit response rule compile failed: " + spec.id + ": " + err.Error())
				continue
			}
			compiled = append(compiled, compiledRule{spec.id, spec.name, spec.severity, re, nil})
		}
		responseRules = compiled
	})
	return responseRules
}

// responseContextWindow 响应命中上下文窗口长度(字符),命中位置前后各半。
const responseContextWindow = 200

// contextAround 返回命中位置前后的上下文窗口,命中片段用【】标出,
// 窗口内的密钥/PII 再做打码。窗口超长(命中本身极长)时按上限截断。
func contextAround(text string, start, end int, window int) string {
	runes := []rune(text)
	s := utf8.RuneCountInString(text[:start])
	e := utf8.RuneCountInString(text[:end])
	half := window / 2
	ws := s - half
	if ws < 0 {
		ws = 0
	}
	we := e + half
	if we > len(runes) {
		we = len(runes)
	}
	// 命中段本身超长时,窗口至少覆盖完整命中(否则截断处语义不明);
	// 否则把总长压回窗口上限。
	if we < e {
		we = e
	} else if we-ws > window && e-s <= window {
		we = ws + window
	}
	return maskSensitiveIn(string(runes[ws:s]) + "【" + string(runes[s:e]) + "】" + string(runes[e:we]))
}

// ScanResponse 对上游响应文本执行全部响应规则,返回命中列表(与 ScanPrompt 同语义)。
// 响应侧不存原文,命中额外带 200 字符打码上下文供排障。
func ScanResponse(text string) []RuleHit {
	if text == "" {
		return nil
	}
	var hits []RuleHit
	for _, rule := range activeResponseRules() {
		locs := rule.re.FindAllStringIndex(text, maxHitsPerRule+1)
		if len(locs) == 0 {
			continue
		}
		hits = append(hits, RuleHit{
			RuleId:   rule.id,
			RuleName: rule.name,
			Severity: rule.severity,
			Excerpt:  MaskExcerpt(text[locs[0][0]:locs[0][1]]),
			Context:  contextAround(text, locs[0][0], locs[0][1], responseContextWindow),
			Count:    len(locs),
		})
	}
	return hits
}
