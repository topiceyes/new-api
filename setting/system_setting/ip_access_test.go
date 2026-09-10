package system_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetIPAccessSettings(t *testing.T) {
	t.Helper()
	old := defaultIPAccessSettings
	defaultIPAccessSettings = IPAccessSettings{}
	t.Cleanup(func() { defaultIPAccessSettings = old })
}

func TestValidateIPAccessEntries(t *testing.T) {
	require.NoError(t, ValidateIPAccessEntries(nil))
	require.NoError(t, ValidateIPAccessEntries([]string{
		"203.0.113.10",
		"198.51.100.0/24",
		"2001:db8::1",
		"2001:db8::/32",
	}))
	require.Error(t, ValidateIPAccessEntries([]string{"not-an-ip"}))
	require.Error(t, ValidateIPAccessEntries([]string{"203.0.113.10", "999.1.1.1"}))
	require.Error(t, ValidateIPAccessEntries([]string{"203.0.113.0/33"}))
	require.Error(t, ValidateIPAccessEntries([]string{""}))
}

func TestIsIPWhitelisted(t *testing.T) {
	resetIPAccessSettings(t)
	defaultIPAccessSettings.Whitelist = []string{"203.0.113.10", "198.51.100.0/24", "2001:db8::/32"}

	assert.True(t, IsIPWhitelisted("203.0.113.10"))
	assert.True(t, IsIPWhitelisted("198.51.100.42"))
	assert.True(t, IsIPWhitelisted("2001:db8::dead:beef"))
	assert.False(t, IsIPWhitelisted("203.0.113.11"))
	assert.False(t, IsIPWhitelisted("198.51.101.1"))
	assert.False(t, IsIPWhitelisted("10.0.0.1"))
	// 非法 IP 输入不命中
	assert.False(t, IsIPWhitelisted("garbage"))
	assert.False(t, IsIPWhitelisted(""))
}

func TestIsIPBlacklisted(t *testing.T) {
	resetIPAccessSettings(t)
	defaultIPAccessSettings.Blacklist = []string{"192.0.2.55", "192.0.2.0/25"}

	assert.True(t, IsIPBlacklisted("192.0.2.55"))
	assert.True(t, IsIPBlacklisted("192.0.2.100"))
	assert.False(t, IsIPBlacklisted("192.0.2.200"))
	assert.False(t, IsIPBlacklisted("8.8.8.8"))
}

func TestIPAccessEmptyListsNeverMatch(t *testing.T) {
	resetIPAccessSettings(t)
	assert.False(t, IsIPWhitelisted("203.0.113.10"))
	assert.False(t, IsIPBlacklisted("203.0.113.10"))
}

func TestIPAccessCIDRIsMasked(t *testing.T) {
	resetIPAccessSettings(t)
	// 非对齐的 CIDR 写法应按掩码归一化:198.51.100.77/24 等价于 198.51.100.0/24
	defaultIPAccessSettings.Whitelist = []string{"198.51.100.77/24"}
	assert.True(t, IsIPWhitelisted("198.51.100.1"))
	assert.False(t, IsIPWhitelisted("198.51.101.1"))
}

func TestIPAccessCacheRebuildsOnChange(t *testing.T) {
	resetIPAccessSettings(t)
	defaultIPAccessSettings.Whitelist = []string{"203.0.113.10"}
	assert.True(t, IsIPWhitelisted("203.0.113.10"))
	assert.False(t, IsIPWhitelisted("203.0.113.20"))

	// 配置变更后缓存必须失效重建
	defaultIPAccessSettings.Whitelist = []string{"203.0.113.20"}
	assert.False(t, IsIPWhitelisted("203.0.113.10"))
	assert.True(t, IsIPWhitelisted("203.0.113.20"))
}

func TestIPAccessEntriesContain(t *testing.T) {
	entries := []string{"192.0.2.55", "198.51.100.0/24"}
	assert.True(t, IPAccessEntriesContain(entries, "192.0.2.55"))
	assert.True(t, IPAccessEntriesContain(entries, "198.51.100.9"))
	assert.False(t, IPAccessEntriesContain(entries, "203.0.113.1"))
	assert.False(t, IPAccessEntriesContain(entries, "garbage"))
	assert.False(t, IPAccessEntriesContain(nil, "192.0.2.55"))
}

func TestIPAccessSkipsInvalidStoredEntries(t *testing.T) {
	resetIPAccessSettings(t)
	// 存量脏数据(绕过 option 校验直接写库)不应 panic,合法条目仍生效
	defaultIPAccessSettings.Blacklist = []string{"garbage", "192.0.2.55"}
	assert.True(t, IsIPBlacklisted("192.0.2.55"))
	assert.False(t, IsIPBlacklisted("192.0.2.56"))
}
