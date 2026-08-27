package audit

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
)

// TestEffectiveStorePromptMode 锁定存储模式优先级:user > group > global,
// 非法值归一为跟随全局(继续向下回退)。
func TestEffectiveStorePromptMode(t *testing.T) {
	settings := &system_setting.AuditSettings{
		StorePromptMode: system_setting.AuditStorePromptHits,
		GroupStorePromptModes: []system_setting.GroupPromptPolicy{
			{Group: "vip", Mode: system_setting.AuditStorePromptAll},
			{Group: "intern", Mode: system_setting.AuditStorePromptNone},
			{Group: "broken", Mode: "everything"}, // 非法 mode,视为未配置
		},
	}

	cases := []struct {
		name     string
		userMode string
		group    string
		want     string
	}{
		{"user overrides group and global", system_setting.AuditStorePromptNone, "vip", system_setting.AuditStorePromptNone},
		{"user overrides global without group policy", system_setting.AuditStorePromptAll, "default", system_setting.AuditStorePromptAll},
		{"invalid user mode falls back to group", "whatever", "vip", system_setting.AuditStorePromptAll},
		{"empty user mode uses group policy", "", "intern", system_setting.AuditStorePromptNone},
		{"group policy with invalid mode falls back to global", "", "broken", system_setting.AuditStorePromptHits},
		{"no user mode no group policy uses global", "", "default", system_setting.AuditStorePromptHits},
		{"empty group never matches policy", "", "", system_setting.AuditStorePromptHits},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, effectiveStorePromptMode(settings, tc.userMode, tc.group))
		})
	}
}

// TestStoredPromptFor 三种模式的存储语义:none 不存 / hits 仅命中存 / all 全量存。
func TestStoredPromptFor(t *testing.T) {
	assert.Equal(t, "", storedPromptFor(system_setting.AuditStorePromptNone, "hello", true))
	assert.Equal(t, "", storedPromptFor(system_setting.AuditStorePromptHits, "hello", false))
	assert.Equal(t, "hello", storedPromptFor(system_setting.AuditStorePromptHits, "hello", true))
	assert.Equal(t, "hello", storedPromptFor(system_setting.AuditStorePromptAll, "hello", false))
	// 非法模式不存,与 none 等价(回退路径的最后一道防线)
	assert.Equal(t, "", storedPromptFor("bogus", "hello", true))
}
