package audit

import (
	"regexp"
	"sync"

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

// ScanResponse 对上游响应文本执行全部响应规则,返回命中列表(与 ScanPrompt 同语义)。
func ScanResponse(text string) []RuleHit {
	if text == "" {
		return nil
	}
	var hits []RuleHit
	for _, rule := range activeResponseRules() {
		matches := rule.re.FindAllString(text, maxHitsPerRule+1)
		if len(matches) == 0 {
			continue
		}
		hits = append(hits, RuleHit{
			RuleId:   rule.id,
			RuleName: rule.name,
			Severity: rule.severity,
			Excerpt:  MaskExcerpt(matches[0]),
			Count:    len(matches),
		})
	}
	return hits
}
