package audit

import (
	"math"
	"net"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/oschwald/maxminddb-golang"
)

// GeoFix 一个 IP 的地理位置快照,用于"不可能移动"判定。
type GeoFix struct {
	IP      string  `json:"ip"`
	Country string  `json:"country"`
	City    string  `json:"city"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	TS      int64   `json:"ts"`
}

// geoLookup 测试 seam(同 sendAdminAlert):单测替换为替身,不依赖真实 mmdb。
var geoLookup = geoLookupImpl

var (
	geoMu     sync.Mutex
	geoReader *maxminddb.Reader
	geoPath   string // 当前 reader 对应的配置路径;路径变更时重载
)

func geoLookupImpl(ip string) (GeoFix, bool) {
	path := system_setting.GetAuditSettings().GeoIPDBPath
	if path == "" {
		return GeoFix{}, false
	}
	parsed := net.ParseIP(ip)
	if parsed == nil || !parsed.IsGlobalUnicast() || parsed.IsPrivate() || parsed.IsLoopback() {
		return GeoFix{}, false
	}

	geoMu.Lock()
	defer geoMu.Unlock()
	if geoPath != path {
		if geoReader != nil {
			_ = geoReader.Close()
			geoReader = nil
		}
		reader, err := maxminddb.Open(path)
		if err != nil {
			// 记录路径防反复重试;配置修复(路径变更)时会重新加载
			geoPath = path
			common.SysError("audit geoip: failed to open mmdb " + path + ": " + err.Error())
			return GeoFix{}, false
		}
		geoReader = reader
		geoPath = path
	}
	if geoReader == nil {
		return GeoFix{}, false
	}

	var record struct {
		Country struct {
			Names map[string]string `maxminddb:"names"`
		} `maxminddb:"country"`
		City struct {
			Names map[string]string `maxminddb:"names"`
		} `maxminddb:"city"`
		Location struct {
			Latitude  float64 `maxminddb:"latitude"`
			Longitude float64 `maxminddb:"longitude"`
		} `maxminddb:"location"`
	}
	if err := geoReader.Lookup(parsed, &record); err != nil {
		return GeoFix{}, false
	}
	if record.Location.Latitude == 0 && record.Location.Longitude == 0 {
		return GeoFix{}, false
	}
	return GeoFix{
		IP:      ip,
		Country: record.Country.Names["en"],
		City:    record.City.Names["en"],
		Lat:     record.Location.Latitude,
		Lng:     record.Location.Longitude,
	}, true
}

// 不可能移动判定阈值:距离超过 500km 且时间差不足以按民航速度(约 900km/h)抵达。
const (
	impossibleTravelMinKm       = 500.0
	impossibleTravelMaxSpeedKmh = 900.0
)

// haversineKm 两经纬度间的大圆距离。
func haversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusKm = 6371.0
	const degToRad = math.Pi / 180
	dLat := (lat2 - lat1) * degToRad
	dLng := (lng2 - lng1) * degToRad
	sinDLat := math.Sin(dLat / 2)
	sinDLng := math.Sin(dLng / 2)
	a := sinDLat*sinDLat + math.Cos(lat1*degToRad)*math.Cos(lat2*degToRad)*sinDLng*sinDLng
	return 2 * earthRadiusKm * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// isImpossibleTravel 纯函数:前后两个地理位置快照是否物理不可达。
func isImpossibleTravel(from GeoFix, to GeoFix, now int64) (distKm float64, impossible bool) {
	if from.IP == to.IP {
		return 0, false
	}
	dist := haversineKm(from.Lat, from.Lng, to.Lat, to.Lng)
	if dist < impossibleTravelMinKm {
		return dist, false
	}
	deltaSec := float64(now - from.TS)
	if deltaSec < 0 {
		deltaSec = 0
	}
	needSec := dist / impossibleTravelMaxSpeedKmh * 3600
	return dist, deltaSec < needSec
}

// impossibleTravelSignal 对比上次地理快照,不可达则返回 critical 信号。
// 上次快照不存在或已超出快速窗口时返回 nil(由调用方负责更新快照)。
func impossibleTravelSignal(prev GeoFix, cur GeoFix, rapidWindowSec int64, now int64) *KeyShareSignal {
	if prev.IP == "" || now-prev.TS > rapidWindowSec {
		return nil
	}
	dist, impossible := isImpossibleTravel(prev, cur, now)
	if !impossible {
		return nil
	}
	return &KeyShareSignal{
		EventType: model.AuditEventTypeKeyShareImpossibleTravel,
		Severity:  system_setting.AuditSeverityCritical,
		Detail: map[string]any{
			"from_ip":       prev.IP,
			"to_ip":         cur.IP,
			"from_location": locationLabel(prev),
			"to_location":   locationLabel(cur),
			"distance_km":   int(dist + 0.5),
			"minutes":       int((now - prev.TS) / 60),
		},
	}
}

func locationLabel(fix GeoFix) string {
	label := fix.Country
	if fix.City != "" {
		if label != "" {
			label += "/"
		}
		label += fix.City
	}
	if label == "" {
		label = "unknown"
	}
	return label
}
