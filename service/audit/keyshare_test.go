package audit

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testKeyShareSettings() *system_setting.AuditSettings {
	return &system_setting.AuditSettings{
		KeyShareEnabled:             true,
		KeyShareWindowMinutes:       60,
		KeyShareDistinctIPThreshold: 3,
		KeyShareRapidWindowMinutes:  10,
		KeyShareRapidIPThreshold:    2,
		KeyShareSuppressHours:       24,
	}
}

func signalTypes(signals []KeyShareSignal) []string {
	types := make([]string, 0, len(signals))
	for _, s := range signals {
		types = append(types, s.EventType)
	}
	return types
}

// TestTrackKeyShareMemoryMultiIP 长窗口去重 IP 数达阈值触发 multi_ip 信号。
// tokenId 用测试唯一值,避免共享 memFingerprints 全局状态。
func TestTrackKeyShareMemoryMultiIP(t *testing.T) {
	ks := testKeyShareSettings()
	now := int64(1_700_000_000)
	// 间隔拉开到短窗口(10 分钟)之外,隔离出纯长窗口 multi_ip 信号
	step := int64(ks.KeyShareRapidWindowMinutes)*60 + 1

	assert.Empty(t, trackKeyShareMemory(90001, "10.0.0.1", "ua-a", ks, now))
	assert.Empty(t, trackKeyShareMemory(90001, "10.0.0.2", "ua-a", ks, now+step))

	signals := trackKeyShareMemory(90001, "10.0.0.3", "ua-a", ks, now+2*step)
	require.Len(t, signals, 1)
	assert.Equal(t, model.AuditEventTypeKeyShareMultiIP, signals[0].EventType)
	assert.Equal(t, 3, signals[0].Detail["distinct_ips"])
}

// TestTrackKeyShareMemoryRapidIP 短窗口内快速切换 IP 触发 rapid_ip 信号。
func TestTrackKeyShareMemoryRapidIP(t *testing.T) {
	ks := testKeyShareSettings()
	now := int64(1_700_000_000)

	assert.Empty(t, trackKeyShareMemory(90002, "10.1.0.1", "ua-a", ks, now))
	signals := trackKeyShareMemory(90002, "10.1.0.2", "ua-a", ks, now+60)
	require.Contains(t, signalTypes(signals), model.AuditEventTypeKeyShareRapidIP)

	// 短窗口(10 分钟)外的 IP 不计入 rapid 信号:旧记录过期后重新累积不足阈值
	far := now + int64(ks.KeyShareRapidWindowMinutes)*60 + 1
	assert.Empty(t, trackKeyShareMemory(90003, "10.2.0.1", "ua-a", ks, now))
	assert.Empty(t, trackKeyShareMemory(90003, "10.2.0.2", "ua-a", ks, far))
}

// TestTrackKeyShareMemoryMultiUA 同一阈值复用于 UA 多样性。
func TestTrackKeyShareMemoryMultiUA(t *testing.T) {
	ks := testKeyShareSettings()
	now := int64(1_700_000_000)

	for i, ua := range []string{"ua-a", "ua-b"} {
		assert.Empty(t, trackKeyShareMemory(90004, "10.3.0.1", ua, ks, now+int64(i)))
	}
	signals := trackKeyShareMemory(90004, "10.3.0.1", "ua-c", ks, now+2)
	require.Contains(t, signalTypes(signals), model.AuditEventTypeKeyShareMultiUA)
}

// TestTrackKeyShareMemorySuppression 同一 token 同一信号在抑制期内不重复上报,过期后恢复。
func TestTrackKeyShareMemorySuppression(t *testing.T) {
	ks := testKeyShareSettings()
	// 窗口拉长到 48h,使抑制期(24h)过后早期 IP 仍在窗口内,信号可复现
	ks.KeyShareWindowMinutes = 60 * 48
	now := int64(1_700_000_000)

	for i := 1; i <= 3; i++ {
		trackKeyShareMemory(90005, fmt.Sprintf("10.4.0.%d", i), "ua-a", ks, now)
	}
	// 阈值已触发过一次(第 3 个 IP),抑制期内新 IP 不再产生 multi_ip 信号
	signals := trackKeyShareMemory(90005, "10.4.0.4", "ua-a", ks, now+10)
	assert.NotContains(t, signalTypes(signals), model.AuditEventTypeKeyShareMultiIP)

	// 抑制期过后恢复上报
	after := now + int64(ks.KeyShareSuppressHours)*3600 + 1
	signals = trackKeyShareMemory(90005, "10.4.0.5", "ua-a", ks, after)
	assert.Contains(t, signalTypes(signals), model.AuditEventTypeKeyShareMultiIP)
}

// TestTrackKeyShareMemoryWindowExpiry 长窗口过期后去重 IP 数回落,不再触发。
func TestTrackKeyShareMemoryWindowExpiry(t *testing.T) {
	ks := testKeyShareSettings()
	now := int64(1_700_000_000)
	// 间隔避开短窗口,防止 rapid_ip 信号干扰断言
	step := int64(ks.KeyShareRapidWindowMinutes)*60 + 1

	assert.Empty(t, trackKeyShareMemory(90006, "10.5.0.1", "ua-a", ks, now))
	assert.Empty(t, trackKeyShareMemory(90006, "10.5.0.2", "ua-a", ks, now+step))
	// 第三个 IP 到达时前两个已超出 60 分钟长窗口
	later := now + int64(ks.KeyShareWindowMinutes)*60 + step
	assert.Empty(t, trackKeyShareMemory(90006, "10.5.0.3", "ua-a", ks, later))
}

func TestEvaluateKeyShare(t *testing.T) {
	ks := testKeyShareSettings()
	assert.Empty(t, evaluateKeyShare(2, 1, 2, ks))
	assert.Equal(t, []string{model.AuditEventTypeKeyShareMultiIP}, evaluateKeyShare(3, 1, 1, ks))
	assert.Equal(t, []string{model.AuditEventTypeKeyShareRapidIP}, evaluateKeyShare(1, 2, 1, ks))
	assert.Equal(t, []string{model.AuditEventTypeKeyShareMultiUA}, evaluateKeyShare(1, 1, 3, ks))

	// 阈值为 0 表示关闭该信号
	off := testKeyShareSettings()
	off.KeyShareDistinctIPThreshold = 0
	off.KeyShareRapidIPThreshold = 0
	assert.Empty(t, evaluateKeyShare(100, 100, 100, off))
}

// TestTrackKeyShareZeroToken tokenId 为 0(非令牌请求)时不追踪。
func TestTrackKeyShareZeroToken(t *testing.T) {
	assert.Empty(t, TrackKeyShare(0, "1.2.3.4", "ua", testKeyShareSettings()))
}
