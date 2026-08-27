package audit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/go-redis/redis/v8"
)

// KeyShareSignal 一次请求触发的 key 分享信号(可能为空)。
// Severity 为空时按 warning 处理(一期三种信号全是 warning);
// 不可能移动等高危信号显式置 critical。
type KeyShareSignal struct {
	EventType string         `json:"event_type"`
	Severity  string         `json:"severity,omitempty"`
	Detail    map[string]any `json:"detail"`
}

var redisOpTimeout = 3 * time.Second

// TrackKeyShare 记录本次请求的 IP/UA 指纹并评估分享信号。
// Redis 可用时用 SET+TTL 实现滚动窗口,否则退化到内存实现(单实例语义)。
// 触发阈值且不在抑制期内时返回信号列表,由调用方写审计事件。
func TrackKeyShare(tokenId int, ip string, ua string, ks *system_setting.AuditSettings) []KeyShareSignal {
	if tokenId == 0 {
		return nil
	}
	var signals []KeyShareSignal
	redisOK := false
	if common.RedisEnabled {
		redisSignals, err := trackKeyShareRedis(tokenId, ip, ua, ks)
		if err != nil {
			common.SysError("audit keyshare redis failed, fallback to memory: " + err.Error())
		} else {
			signals = redisSignals
			redisOK = true
		}
	}
	if !redisOK {
		signals = trackKeyShareMemory(tokenId, ip, ua, ks, time.Now().Unix())
	}
	// GeoIP 不可能移动:独立于 IP 计数信号,任一存储路径都会追加评估
	if signal, ok := trackImpossibleTravel(tokenId, ip, ks); ok {
		signals = append(signals, signal)
	}
	return signals
}

// evaluateKeyShare 纯函数:根据各窗口去重计数判定信号类型。
func evaluateKeyShare(distinctIPs int64, rapidIPs int64, distinctUAs int64, ks *system_setting.AuditSettings) []string {
	var types []string
	if ks.KeyShareDistinctIPThreshold > 0 && distinctIPs >= int64(ks.KeyShareDistinctIPThreshold) {
		types = append(types, model.AuditEventTypeKeyShareMultiIP)
	}
	if ks.KeyShareRapidIPThreshold > 0 && rapidIPs >= int64(ks.KeyShareRapidIPThreshold) {
		types = append(types, model.AuditEventTypeKeyShareRapidIP)
	}
	if ks.KeyShareDistinctIPThreshold > 0 && distinctUAs >= int64(ks.KeyShareDistinctIPThreshold) {
		types = append(types, model.AuditEventTypeKeyShareMultiUA)
	}
	return types
}

func redisKeyShareKey(tokenId int, suffix string) string {
	return fmt.Sprintf("audit:ks:%d:%s", tokenId, suffix)
}

func trackKeyShareRedis(tokenId int, ip string, ua string, ks *system_setting.AuditSettings) ([]KeyShareSignal, error) {
	ctx, cancel := context.WithTimeout(context.Background(), redisOpTimeout)
	defer cancel()

	window := time.Duration(ks.KeyShareWindowMinutes) * time.Minute
	rapidWindow := time.Duration(ks.KeyShareRapidWindowMinutes) * time.Minute

	ipsKey := redisKeyShareKey(tokenId, "ips")
	rapidKey := redisKeyShareKey(tokenId, "rips")
	uasKey := redisKeyShareKey(tokenId, "uas")

	pipe := common.RDB.TxPipeline()
	pipe.SAdd(ctx, ipsKey, ip)
	ipsCount := pipe.SCard(ctx, ipsKey)
	pipe.Expire(ctx, ipsKey, window)
	pipe.SAdd(ctx, rapidKey, ip)
	rapidCount := pipe.SCard(ctx, rapidKey)
	pipe.Expire(ctx, rapidKey, rapidWindow)
	pipe.SAdd(ctx, uasKey, ua)
	uasCount := pipe.SCard(ctx, uasKey)
	pipe.Expire(ctx, uasKey, window)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}

	eventTypes := evaluateKeyShare(ipsCount.Val(), rapidCount.Val(), uasCount.Val(), ks)
	var signals []KeyShareSignal
	for _, eventType := range eventTypes {
		suppressKey := redisKeyShareKey(tokenId, "sup:"+eventType)
		// SET NX 原子完成"检查抑制 + 落抑制",只在真正抢到(未抑制)时返回信号。
		ok, err := common.RDB.SetNX(ctx, suppressKey, 1, time.Duration(ks.KeyShareSuppressHours)*time.Hour).Result()
		if err != nil {
			return signals, err
		}
		if !ok {
			continue
		}
		signals = append(signals, KeyShareSignal{
			EventType: eventType,
			Detail: map[string]any{
				"distinct_ips":   ipsCount.Val(),
				"rapid_ips":      rapidCount.Val(),
				"distinct_uas":   uasCount.Val(),
				"window_minutes": ks.KeyShareWindowMinutes,
			},
		})
	}
	return signals, nil
}

// ---- GeoIP 不可能移动(二期④) ----
// 对比同一令牌上一次请求的地理快照:快速窗口内距离超阈值且时间差物理不可达
// 则产生 critical 信号。mmdb 未配置/私网 IP 时 geoLookup 返回 ok=false,整体停用。

func trackImpossibleTravel(tokenId int, ip string, ks *system_setting.AuditSettings) (KeyShareSignal, bool) {
	cur, ok := geoLookup(ip)
	if !ok {
		return KeyShareSignal{}, false
	}
	cur.TS = time.Now().Unix()
	rapidWindowSec := int64(ks.KeyShareRapidWindowMinutes) * 60
	if common.RedisEnabled {
		signal, err := trackImpossibleTravelRedis(tokenId, cur, rapidWindowSec, ks)
		if err != nil {
			common.SysError("audit impossible-travel redis failed, fallback to memory: " + err.Error())
		} else {
			if signal == nil {
				return KeyShareSignal{}, false
			}
			return *signal, true
		}
	}
	if signal := trackImpossibleTravelMemory(tokenId, cur, rapidWindowSec, ks, cur.TS); signal != nil {
		return *signal, true
	}
	return KeyShareSignal{}, false
}

func trackImpossibleTravelRedis(tokenId int, cur GeoFix, rapidWindowSec int64, ks *system_setting.AuditSettings) (*KeyShareSignal, error) {
	ctx, cancel := context.WithTimeout(context.Background(), redisOpTimeout)
	defer cancel()

	key := redisKeyShareKey(tokenId, "geolast")
	oldStr, err := common.RDB.Get(ctx, key).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	data, err := common.Marshal(cur)
	if err != nil {
		return nil, err
	}
	ttl := time.Duration(ks.KeyShareRapidWindowMinutes) * time.Minute
	if err := common.RDB.Set(ctx, key, data, ttl).Err(); err != nil {
		return nil, err
	}
	if oldStr == "" {
		return nil, nil
	}
	var prev GeoFix
	if err := common.UnmarshalJsonStr(oldStr, &prev); err != nil {
		// 坏快照只当无历史,不阻断本次记录
		return nil, nil
	}
	signal := impossibleTravelSignal(prev, cur, rapidWindowSec, cur.TS)
	if signal == nil {
		return nil, nil
	}
	suppressKey := redisKeyShareKey(tokenId, "sup:"+signal.EventType)
	ok, err := common.RDB.SetNX(ctx, suppressKey, 1, time.Duration(ks.KeyShareSuppressHours)*time.Hour).Result()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return signal, nil
}

func trackImpossibleTravelMemory(tokenId int, cur GeoFix, rapidWindowSec int64, ks *system_setting.AuditSettings, now int64) *KeyShareSignal {
	fp := memFingerprintOf(tokenId)
	fp.mu.Lock()
	defer fp.mu.Unlock()

	cur.TS = now
	prev := fp.lastFix
	fp.lastFix = cur

	signal := impossibleTravelSignal(prev, cur, rapidWindowSec, now)
	if signal == nil {
		return nil
	}
	if until, ok := fp.suppress[signal.EventType]; ok && until > now {
		return nil
	}
	fp.suppress[signal.EventType] = now + int64(ks.KeyShareSuppressHours)*3600
	return signal
}

// ---- 内存 fallback(单实例语义,进程重启即清空,可接受:信号会随流量重新累积) ----

type memFingerprint struct {
	mu       sync.Mutex
	ips      map[string]int64 // member -> lastSeen unix
	rapidIPs map[string]int64
	uas      map[string]int64
	suppress map[string]int64 // eventType -> suppressedUntil unix
	lastFix  GeoFix           // 上一次可定位 IP 的地理快照(不可能移动判定)
}

var memFingerprints sync.Map // tokenId -> *memFingerprint

func memFingerprintOf(tokenId int) *memFingerprint {
	if v, ok := memFingerprints.Load(tokenId); ok {
		return v.(*memFingerprint)
	}
	fp := &memFingerprint{
		ips:      map[string]int64{},
		rapidIPs: map[string]int64{},
		uas:      map[string]int64{},
		suppress: map[string]int64{},
	}
	actual, _ := memFingerprints.LoadOrStore(tokenId, fp)
	return actual.(*memFingerprint)
}

func pruneExpired(m map[string]int64, windowSeconds int64, now int64) {
	cutoff := now - windowSeconds
	for member, lastSeen := range m {
		if lastSeen < cutoff {
			delete(m, member)
		}
	}
}

func trackKeyShareMemory(tokenId int, ip string, ua string, ks *system_setting.AuditSettings, now int64) []KeyShareSignal {
	fp := memFingerprintOf(tokenId)
	fp.mu.Lock()
	defer fp.mu.Unlock()

	windowSec := int64(ks.KeyShareWindowMinutes) * 60
	rapidSec := int64(ks.KeyShareRapidWindowMinutes) * 60

	fp.ips[ip] = now
	fp.rapidIPs[ip] = now
	fp.uas[ua] = now
	pruneExpired(fp.ips, windowSec, now)
	pruneExpired(fp.rapidIPs, rapidSec, now)
	pruneExpired(fp.uas, windowSec, now)

	eventTypes := evaluateKeyShare(int64(len(fp.ips)), int64(len(fp.rapidIPs)), int64(len(fp.uas)), ks)
	var signals []KeyShareSignal
	suppressSec := int64(ks.KeyShareSuppressHours) * 3600
	for _, eventType := range eventTypes {
		if until, ok := fp.suppress[eventType]; ok && until > now {
			continue
		}
		fp.suppress[eventType] = now + suppressSec
		signals = append(signals, KeyShareSignal{
			EventType: eventType,
			Detail: map[string]any{
				"distinct_ips":   len(fp.ips),
				"rapid_ips":      len(fp.rapidIPs),
				"distinct_uas":   len(fp.uas),
				"window_minutes": ks.KeyShareWindowMinutes,
			},
		})
	}
	return signals
}
