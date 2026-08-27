package audit

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hitByRuleId(hits []RuleHit, ruleId string) *RuleHit {
	for i := range hits {
		if hits[i].RuleId == ruleId {
			return &hits[i]
		}
	}
	return nil
}

// TestScanPromptBuiltinRules 逐条内置规则验证真阳性样本命中与易误报样本豁免。
// 身份证/银行卡样本的校验位由 GB11643 加权算法与 Luhn 算法算出。
func TestScanPromptBuiltinRules(t *testing.T) {
	cases := []struct {
		name       string
		text       string
		wantRule   string
		wantAbsent bool // true 表示断言该规则不命中(误报防护)
	}{
		{"openai api key", "用这个 key 试试 sk-abcdefghijklmnopqrstuvwxyz12", "builtin.api_key_sk", false},
		{"aws access key", "AKIAIOSFODNN7EXAMPLE 是我的测试凭证", "builtin.aws_access_key", false},
		{"private key pem", "-----BEGIN RSA PRIVATE KEY-----\nMIIEow...", "builtin.private_key_pem", false},
		{"jwt", "token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5t", "builtin.jwt", false},
		{"valid id card", "我的身份证号是 110101199003077539 请保密", "builtin.id_card_cn", false},
		{"id card wrong checksum", "我的身份证号是 110101199003077530", "builtin.id_card_cn", true},
		{"id card 15-digit legacy", "老证号 110101900307753", "builtin.id_card_cn", true},
		{"valid luhn bank card", "卡号 6222021234567894", "builtin.bank_card", false},
		{"non-luhn 16-digit number", "单号 6222021234567895", "builtin.bank_card", true},
		{"short number not bank card", "订单号 12345678", "builtin.bank_card", true},
		{"phone cn", "联系电话 13800138000", "builtin.phone_cn", false},
		{"email", "邮箱 someone@example.com", "builtin.email", false},
		{"generic secret assignment", "password: abcdef123456", "builtin.generic_secret", false},
		{"generic secret in prose not assignment", "请记住密码安全的重要性", "builtin.generic_secret", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits := ScanPrompt(tc.text)
			hit := hitByRuleId(hits, tc.wantRule)
			if tc.wantAbsent {
				assert.Nil(t, hit, "rule %s must not hit %q", tc.wantRule, tc.text)
				return
			}
			require.NotNil(t, hit, "rule %s must hit %q", tc.wantRule, tc.text)
			assert.Equal(t, 1, hit.Count)
			assert.NotEmpty(t, hit.Excerpt)
			assert.NotContains(t, hit.Excerpt, tc.text, "excerpt must be masked, never the raw input")
		})
	}

	assert.Empty(t, ScanPrompt("今天天气怎么样,帮我写个周报"))
	assert.Empty(t, ScanPrompt(""))
}

// TestScanPromptSeverity 内置规则严重度契约:密钥类 critical,证件/卡 warning,联系方式 info。
func TestScanPromptSeverity(t *testing.T) {
	hits := ScanPrompt("sk-abcdefghijklmnopqrstuvwxyz12 和 110101199003077539 和 13800138000")
	assert.Equal(t, system_setting.AuditSeverityCritical, hitByRuleId(hits, "builtin.api_key_sk").Severity)
	assert.Equal(t, system_setting.AuditSeverityWarning, hitByRuleId(hits, "builtin.id_card_cn").Severity)
	assert.Equal(t, system_setting.AuditSeverityInfo, hitByRuleId(hits, "builtin.phone_cn").Severity)
}

// TestScanPromptCustomRules 自定义规则合并:启用才生效、坏 regex 跳过、未知严重度归一为 info。
func TestScanPromptCustomRules(t *testing.T) {
	settings := system_setting.GetAuditSettings()
	original := settings.Rules
	t.Cleanup(func() { settings.Rules = original })

	settings.Rules = []system_setting.AuditRule{
		{Id: "custom.internal_domain", Name: "内部域名", Regex: `internal\.example\.com`, Severity: system_setting.AuditSeverityWarning, Enabled: true},
		{Id: "custom.disabled", Name: "停用规则", Regex: `should-not-match`, Severity: system_setting.AuditSeverityCritical, Enabled: false},
		{Id: "custom.bad_regex", Name: "坏正则", Regex: `([a-z`, Severity: system_setting.AuditSeverityCritical, Enabled: true},
		{Id: "custom.unknown_severity", Name: "未知严重度", Regex: `secret-marker-\d+`, Severity: "bogus", Enabled: true},
	}

	hits := ScanPrompt("请访问 internal.example.com 获取数据,标记 secret-marker-42")
	customHit := hitByRuleId(hits, "custom.internal_domain")
	require.NotNil(t, customHit)
	assert.Equal(t, system_setting.AuditSeverityWarning, customHit.Severity)

	assert.Nil(t, hitByRuleId(hits, "custom.disabled"))

	severityHit := hitByRuleId(hits, "custom.unknown_severity")
	require.NotNil(t, severityHit)
	assert.Equal(t, system_setting.AuditSeverityInfo, severityHit.Severity)

	// 坏 regex 不影响其他规则(上面的命中已证明),重新扫描不报错
	assert.NotPanics(t, func() { ScanPrompt("anything") })
}

// TestScanPromptHitCount 同规则多次命中聚合为一条 RuleHit,Count 为有效命中数。
func TestScanPromptHitCount(t *testing.T) {
	hits := ScanPrompt("电话 13800138000,备用 13912345678,无关 12345")
	hit := hitByRuleId(hits, "builtin.phone_cn")
	require.NotNil(t, hit)
	assert.Equal(t, 2, hit.Count)
}

func TestMaskExcerpt(t *testing.T) {
	assert.Equal(t, "********", MaskExcerpt(""))
	assert.Equal(t, "********", MaskExcerpt("12345678"))
	assert.Equal(t, "138****00", MaskExcerpt("13800138000"))
	assert.Equal(t, "汉字测****89", MaskExcerpt("汉字测试123456789"))
}

// TestTruncateRunes 截断不得切断多字节 UTF-8 字符;maxBytes<=0 表示不截断。
func TestTruncateRunes(t *testing.T) {
	assert.Equal(t, "abc", truncateRunes("abc", 0))
	assert.Equal(t, "abc", truncateRunes("abc", 10))
	// "汉字" 6 字节,上限 4 落在"字"中间,应退回到"汉"(3 字节)
	assert.Equal(t, "汉", truncateRunes("汉字测试", 4))
	assert.Equal(t, "汉字", truncateRunes("汉字测试", 6))
}
