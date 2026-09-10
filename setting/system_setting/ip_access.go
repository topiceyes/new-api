package system_setting

import (
	"fmt"
	"net/netip"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/setting/config"
)

// IP 访问控制:白名单内的 IP 跳过所有按 IP 的限流(全局 web/API/关键接口),
// 黑名单内的 IP 全站拒绝(403)。条目支持单个 IP 和 CIDR 网段。
// 触发限流的 IP 记录见 service/ip_access_record.go。

type IPAccessSettings struct {
	Whitelist []string `json:"whitelist"` // options 表存 JSON 字符串
	Blacklist []string `json:"blacklist"`
}

var defaultIPAccessSettings = IPAccessSettings{}

func init() {
	config.GlobalConfig.Register("ip_access", &defaultIPAccessSettings)
}

func GetIPAccessSettings() *IPAccessSettings {
	return &defaultIPAccessSettings
}

// ValidateIPAccessEntries 校验一批 IP/CIDR 条目,返回第一个不合法的条目错误。
func ValidateIPAccessEntries(entries []string) error {
	for _, entry := range entries {
		if _, err := parseIPAccessEntry(entry); err != nil {
			return fmt.Errorf("「%s」不是合法的 IP 或 CIDR 网段", entry)
		}
	}
	return nil
}

// parseIPAccessEntry 把单个条目解析为 netip.Prefix;裸 IP 视为 /32 或 /128 主机前缀。
func parseIPAccessEntry(entry string) (netip.Prefix, error) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return netip.Prefix{}, fmt.Errorf("empty entry")
	}
	if prefix, err := netip.ParsePrefix(entry); err == nil {
		return prefix.Masked(), nil
	}
	addr, err := netip.ParseAddr(entry)
	if err != nil {
		return netip.Prefix{}, err
	}
	return netip.PrefixFrom(addr, addr.BitLen()), nil
}

// parsedIPAccessCache 请求热路径上的解析缓存:配置列表内容不变时复用解析结果,
// 避免每个请求都重新 ParsePrefix。
var parsedIPAccessCache = struct {
	sync.RWMutex
	whitelistKey string
	blacklistKey string
	whitelist    []netip.Prefix
	blacklist    []netip.Prefix
}{}

// parseIPAccessList 把配置条目解析成前缀列表;非法条目跳过(保存时已被
// option 校验拦截,这里只是防御存量脏数据)。
func parseIPAccessList(entries []string) []netip.Prefix {
	prefixes := make([]netip.Prefix, 0, len(entries))
	for _, entry := range entries {
		if prefix, err := parseIPAccessEntry(entry); err == nil {
			prefixes = append(prefixes, prefix)
		}
	}
	return prefixes
}

func ipAccessListKey(entries []string) string {
	return strings.Join(entries, "\x00")
}

func matchIPAccessList(entries []string, addr netip.Addr, whitelist bool) bool {
	key := ipAccessListKey(entries)

	parsedIPAccessCache.RLock()
	var cachedKey string
	var cached []netip.Prefix
	if whitelist {
		cachedKey, cached = parsedIPAccessCache.whitelistKey, parsedIPAccessCache.whitelist
	} else {
		cachedKey, cached = parsedIPAccessCache.blacklistKey, parsedIPAccessCache.blacklist
	}
	parsedIPAccessCache.RUnlock()

	if cachedKey != key {
		fresh := parseIPAccessList(entries)
		parsedIPAccessCache.Lock()
		if whitelist {
			parsedIPAccessCache.whitelistKey, parsedIPAccessCache.whitelist = key, fresh
		} else {
			parsedIPAccessCache.blacklistKey, parsedIPAccessCache.blacklist = key, fresh
		}
		parsedIPAccessCache.Unlock()
		cached = fresh
	}

	for _, prefix := range cached {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// IsIPWhitelisted 该 IP 是否命中白名单(跳过按 IP 的限流)。
func IsIPWhitelisted(ip string) bool {
	entries := GetIPAccessSettings().Whitelist
	if len(entries) == 0 {
		return false
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return false
	}
	return matchIPAccessList(entries, addr, true)
}

// IsIPBlacklisted 该 IP 是否命中黑名单(全站拒绝)。
func IsIPBlacklisted(ip string) bool {
	entries := GetIPAccessSettings().Blacklist
	if len(entries) == 0 {
		return false
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return false
	}
	return matchIPAccessList(entries, addr, false)
}

// IPAccessEntriesContain 给定的一批条目是否覆盖该 IP(用于保存黑名单前的
// 防自杀校验:不允许管理员把自己当前的出口 IP 加入黑名单,否则连同管理接口
// 一起被 403,只能改库恢复)。
func IPAccessEntriesContain(entries []string, ip string) bool {
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return false
	}
	for _, prefix := range parseIPAccessList(entries) {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
