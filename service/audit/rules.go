package audit

import (
	"regexp"
	"sync"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// ruleValidator 命中后校验,用于降低误报(身份证校验位、银行卡 Luhn)。
type ruleValidator func(match string) bool

type compiledRule struct {
	id       string
	name     string
	severity string
	re       *regexp.Regexp
	validate ruleValidator // nil 表示命中即有效
}

// RuleHit 单条规则的扫描结果。
type RuleHit struct {
	RuleId   string `json:"rule_id"`
	RuleName string `json:"rule_name"`
	Severity string `json:"severity"`
	Excerpt  string `json:"excerpt"` // 打码后的首个命中片段
	Count    int    `json:"count"`   // 有效命中次数
}

// 内置规则。Id 为稳定常量,事件按 RuleId 关联,请勿改名。
var builtinRuleSpecs = []struct {
	id       string
	name     string
	severity string
	pattern  string
	validate ruleValidator
}{
	{"builtin.api_key_sk", "API 密钥 (sk-)", system_setting.AuditSeverityCritical, `sk-[A-Za-z0-9_-]{20,}`, nil},
	{"builtin.aws_access_key", "AWS Access Key", system_setting.AuditSeverityCritical, `\bAKIA[0-9A-Z]{16}\b`, nil},
	{"builtin.private_key_pem", "PEM 私钥", system_setting.AuditSeverityCritical, `-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`, nil},
	{"builtin.jwt", "JWT Token", system_setting.AuditSeverityCritical, `\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{4,}`, nil},
	{"builtin.id_card_cn", "身份证号", system_setting.AuditSeverityWarning, `[1-9]\d{5}(?:19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[\dXx]`, validIDCardCN},
	{"builtin.bank_card", "银行卡号", system_setting.AuditSeverityWarning, `\b\d{16,19}\b`, validLuhn},
	{"builtin.phone_cn", "手机号", system_setting.AuditSeverityInfo, `\b1[3-9]\d{9}\b`, nil},
	{"builtin.email", "邮箱地址", system_setting.AuditSeverityInfo, `[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`, nil},
	{"builtin.generic_secret", "疑似口令赋值", system_setting.AuditSeverityWarning, `(?i)(?:password|passwd|api[_-]?key|secret|access[_-]?token)\s*[:=]\s*["'` + "`" + `]?[A-Za-z0-9_\-./+]{8,}`, nil},
}

var (
	rulesMu        sync.RWMutex
	rulesCache     []compiledRule
	rulesCacheHash string
)

// activeRules 返回内置规则 + 管理员自定义规则的编译结果。
// 按自定义规则的 JSON 指纹缓存,配置不变不重编译;单条坏 regex 跳过并记日志。
func activeRules() []compiledRule {
	custom := system_setting.GetAuditSettings().Rules
	hash := common.GetJsonString(custom)

	rulesMu.RLock()
	if rulesCacheHash == hash && rulesCache != nil {
		defer rulesMu.RUnlock()
		return rulesCache
	}
	rulesMu.RUnlock()

	compiled := make([]compiledRule, 0, len(builtinRuleSpecs)+len(custom))
	for _, spec := range builtinRuleSpecs {
		re, err := regexp.Compile(spec.pattern)
		if err != nil {
			common.SysError("audit builtin rule compile failed: " + spec.id + ": " + err.Error())
			continue
		}
		compiled = append(compiled, compiledRule{spec.id, spec.name, spec.severity, re, spec.validate})
	}
	for _, rule := range custom {
		if !rule.Enabled || rule.Id == "" || rule.Regex == "" {
			continue
		}
		re, err := regexp.Compile(rule.Regex)
		if err != nil {
			common.SysError("audit custom rule compile failed: " + rule.Id + ": " + err.Error())
			continue
		}
		severity := rule.Severity
		if severity != system_setting.AuditSeverityWarning && severity != system_setting.AuditSeverityCritical {
			severity = system_setting.AuditSeverityInfo
		}
		compiled = append(compiled, compiledRule{rule.Id, rule.Name, severity, re, nil})
	}

	rulesMu.Lock()
	rulesCache = compiled
	rulesCacheHash = hash
	rulesMu.Unlock()
	return compiled
}

// maxHitsPerRule 限制单规则命中计数上限,避免病态输入拖慢扫描。
const maxHitsPerRule = 32

// ScanPrompt 对提取出的 prompt 文本执行全部启用规则,返回命中列表。
func ScanPrompt(text string) []RuleHit {
	if text == "" {
		return nil
	}
	var hits []RuleHit
	for _, rule := range activeRules() {
		matches := rule.re.FindAllString(text, maxHitsPerRule+1)
		if len(matches) == 0 {
			continue
		}
		count := 0
		first := ""
		for _, m := range matches {
			if rule.validate != nil && !rule.validate(m) {
				continue
			}
			count++
			if first == "" {
				first = m
			}
		}
		if count == 0 {
			continue
		}
		hits = append(hits, RuleHit{
			RuleId:   rule.id,
			RuleName: rule.name,
			Severity: rule.severity,
			Excerpt:  MaskExcerpt(first),
			Count:    count,
		})
	}
	return hits
}

// MaskExcerpt 打码命中片段:长度 >8 保留前 3 后 2,中间 *;否则全 *。
// 审计事件只存打码版,明文只在 StorePromptMode 允许时进 Prompt 字段。
func MaskExcerpt(s string) string {
	n := utf8.RuneCountInString(s)
	if n <= 8 {
		return "********"
	}
	runes := []rune(s)
	masked := string(runes[:3]) + "****" + string(runes[n-2:])
	if len(masked) > 64 {
		masked = string([]rune(masked)[:64])
	}
	return masked
}

// validIDCardCN GB11643 身份证加权校验位。
func validIDCardCN(s string) bool {
	if len(s) != 18 {
		return false
	}
	weights := [17]int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}
	checkCodes := [11]byte{'1', '0', 'X', '9', '8', '7', '6', '5', '4', '3', '2'}
	sum := 0
	for i := 0; i < 17; i++ {
		d := s[i] - '0'
		if d > 9 {
			return false
		}
		sum += int(d) * weights[i]
	}
	want := checkCodes[sum%11]
	last := s[17]
	if last == 'x' {
		last = 'X'
	}
	return last == want
}

// validLuhn 银行卡号 Luhn 校验。
func validLuhn(s string) bool {
	sum := 0
	double := false
	for i := len(s) - 1; i >= 0; i-- {
		d := s[i] - '0'
		if d > 9 {
			return false
		}
		n := int(d)
		if double {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		sum += n
		double = !double
	}
	return sum%10 == 0
}

// truncateRunes 按字节上限截断 UTF-8 文本,不切断多字节字符。
func truncateRunes(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
