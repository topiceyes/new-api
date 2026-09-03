package audit

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	geoBeijing  = GeoFix{IP: "1.1.1.1", Country: "China", City: "Beijing", Lat: 39.9042, Lng: 116.4074}
	geoShanghai = GeoFix{IP: "2.2.2.2", Country: "China", City: "Shanghai", Lat: 31.2304, Lng: 121.4737}
	geoShenzhen = GeoFix{IP: "3.3.3.3", Country: "China", City: "Shenzhen", Lat: 22.5431, Lng: 114.0579}
)

// TestGeoLookupDegrade 未配置 mmdb / 私网与保留地址一律 ok=false(安静降级)。
func TestGeoLookupDegrade(t *testing.T) {
	withAuditSettings(t, func(s *system_setting.AuditSettings) {
		s.GeoIPDBPath = ""
	})
	_, ok := geoLookupImpl("8.8.8.8")
	assert.False(t, ok, "no mmdb path configured")

	// 归属地展示串同样安静降级为空。
	assert.Equal(t, "", ResolveIPLocation("8.8.8.8"))

	// names 表 -> 展示串: 中文优先,缺中文回退英文,城市缺失只出国家。
	assert.Equal(t, "中国/北京", geoNamesLabel(
		map[string]string{"zh": "中国", "en": "China"},
		map[string]string{"zh": "北京", "en": "Beijing"},
	))
	assert.Equal(t, "China/Beijing", geoNamesLabel(
		map[string]string{"en": "China"},
		map[string]string{"en": "Beijing"},
	))
	assert.Equal(t, "中国", geoNamesLabel(map[string]string{"zh": "中国"}, nil))

	withAuditSettings(t, func(s *system_setting.AuditSettings) {
		s.GeoIPDBPath = "/nonexistent/GeoLite2-City.mmdb"
	})
	for _, ip := range []string{"8.8.8.8", "10.0.0.1", "192.168.1.1", "127.0.0.1", "not-an-ip"} {
		_, ok := geoLookupImpl(ip)
		assert.False(t, ok, "ip %s must not resolve without a real mmdb", ip)
	}
}

// TestHaversineKm 大圆距离的已知城市对校验(北京-上海约 1070km,北京-深圳约 1940km)。
func TestHaversineKm(t *testing.T) {
	// 同城零距离
	assert.InDelta(t, 0, haversineKm(39.9042, 116.4074, 39.9042, 116.4074), 0.01)
	assert.InDelta(t, 1070, haversineKm(39.9042, 116.4074, 31.2304, 121.4737), 30)
	assert.InDelta(t, 1940, haversineKm(39.9042, 116.4074, 22.5431, 114.0579), 40)
}

// TestIsImpossibleTravel 距离阈值与时间可达性的纯函数契约。
func TestIsImpossibleTravel(t *testing.T) {
	now := int64(1_700_000_000)

	// 同 IP 永不判定
	dist, impossible := isImpossibleTravel(geoBeijing, geoBeijing, now)
	assert.False(t, impossible)
	assert.Zero(t, dist)

	// 近距离(同城/邻城)不判定
	near := GeoFix{IP: "1.1.1.2", Lat: 39.0, Lng: 117.0} // 距北京约 160km
	_, impossible = isImpossibleTravel(geoBeijing, near, now)
	assert.False(t, impossible)

	// 远距离但时间足够(北京-深圳约 1940km,900km/h 需约 2.2h;给 3h)
	from := geoBeijing
	from.TS = now - 3*3600
	dist, impossible = isImpossibleTravel(from, geoShenzhen, now)
	assert.False(t, impossible)
	assert.Greater(t, dist, impossibleTravelMinKm)

	// 远距离且时间不足 → 不可达
	from.TS = now - 10*60 // 10 分钟
	_, impossible = isImpossibleTravel(from, geoShenzhen, now)
	assert.True(t, impossible)

	// 时钟回拨(delta<0)按 0 处理,远距离即不可达
	from.TS = now + 3600
	_, impossible = isImpossibleTravel(from, geoShenzhen, now)
	assert.True(t, impossible)
}

// TestTrackImpossibleTravelMemory 内存路径:首次定位只记快照;窗口内不可达跳
// 变触发 critical 信号并按 suppress 抑制;快照超窗后重新累积。
// tokenId 用测试唯一值,避免共享 memFingerprints 全局状态。
func TestTrackImpossibleTravelMemory(t *testing.T) {
	ks := testKeyShareSettings()
	now := int64(1_700_000_000)
	rapidWindowSec := int64(ks.KeyShareRapidWindowMinutes) * 60

	// 首次请求只有快照,无信号
	assert.Nil(t, trackImpossibleTravelMemory(91001, geoBeijing, rapidWindowSec, ks, now))

	// 5 分钟后出现在深圳 → 不可达,critical 信号
	signal := trackImpossibleTravelMemory(91001, geoShenzhen, rapidWindowSec, ks, now+300)
	require.NotNil(t, signal)
	assert.Equal(t, model.AuditEventTypeKeyShareImpossibleTravel, signal.EventType)
	assert.Equal(t, system_setting.AuditSeverityCritical, signal.Severity)
	assert.Equal(t, geoBeijing.IP, signal.Detail["from_ip"])
	assert.Equal(t, geoShenzhen.IP, signal.Detail["to_ip"])
	assert.Equal(t, "China/Beijing", signal.Detail["from_location"])
	assert.Equal(t, 5, signal.Detail["minutes"])

	// 抑制期内再次不可达跳变(深圳→北京)不再产生信号
	assert.Nil(t, trackImpossibleTravelMemory(91001, geoBeijing, rapidWindowSec, ks, now+600))

	// 另一个 token:窗口外才出现远端 IP,快照已过期,只更新不报警
	assert.Nil(t, trackImpossibleTravelMemory(91002, geoBeijing, rapidWindowSec, ks, now))
	assert.Nil(t, trackImpossibleTravelMemory(91002, geoShenzhen, rapidWindowSec, ks, now+rapidWindowSec+1))

	// 时间充足的长距离移动(北京→深圳 3 小时)不报警
	assert.Nil(t, trackImpossibleTravelMemory(91003, geoBeijing, rapidWindowSec, ks, now))
	assert.Nil(t, trackImpossibleTravelMemory(91003, geoShenzhen, rapidWindowSec, ks, now+3*3600))
}
