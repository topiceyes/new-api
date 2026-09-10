package service

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// 限流触发记录:每次有 IP 被限流(429)就记一条,管理员在 IP 访问控制
// 区块里查看并一键加白/加黑。Redis 启用时存 hash(重启保留),否则存内存。

// RateLimitedIPRecord 一个触发过限流的 IP 的汇总记录。
type RateLimitedIPRecord struct {
	IP        string   `json:"ip"`
	Hits      int64    `json:"hits"`       // 累计被限流次数
	Marks     []string `json:"marks"`      // 触发过的限流桶(GW/GA/CT/...)
	FirstSeen int64    `json:"first_seen"` // 首次触发(Unix 秒)
	LastSeen  int64    `json:"last_seen"`  // 最近触发(Unix 秒)
}

const rateLimitedIPRecordsKey = "rateLimit:v2:blocked_ips"

var rateLimitedIPMemory = struct {
	sync.Mutex
	records map[string]*RateLimitedIPRecord
}{records: map[string]*RateLimitedIPRecord{}}

// seam 便于单测
var rateLimitedIPNow = func() int64 { return time.Now().Unix() }

// RecordRateLimitedIP 记录一次限流触发。失败只记日志,不影响主流程。
func RecordRateLimitedIP(ip string, mark string) {
	if ip == "" {
		return
	}
	now := rateLimitedIPNow()
	if !common.RedisEnabled {
		rateLimitedIPMemory.Lock()
		rec, ok := rateLimitedIPMemory.records[ip]
		if !ok {
			rec = &RateLimitedIPRecord{IP: ip, FirstSeen: now}
			rateLimitedIPMemory.records[ip] = rec
		}
		rec.Hits++
		rec.LastSeen = now
		rec.Marks = appendRecordMark(rec.Marks, mark)
		rateLimitedIPMemory.Unlock()
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, err := common.RDB.HGet(ctx, rateLimitedIPRecordsKey, ip).Result()
	rec := RateLimitedIPRecord{IP: ip, FirstSeen: now}
	if err == nil && raw != "" {
		if err := common.UnmarshalJsonStr(raw, &rec); err != nil {
			rec = RateLimitedIPRecord{IP: ip, FirstSeen: now}
		}
	}
	rec.Hits++
	rec.LastSeen = now
	rec.Marks = appendRecordMark(rec.Marks, mark)
	data, err := common.Marshal(rec)
	if err != nil {
		common.SysError("marshal rate-limited ip record failed: " + err.Error())
		return
	}
	if err := common.RDB.HSet(ctx, rateLimitedIPRecordsKey, ip, data).Err(); err != nil {
		common.SysError("save rate-limited ip record failed: " + err.Error())
	}
}

func appendRecordMark(marks []string, mark string) []string {
	for _, m := range marks {
		if m == mark {
			return marks
		}
	}
	return append(marks, mark)
}

// ListRateLimitedIPs 返回全部限流触发记录,按最近触发时间倒序。
func ListRateLimitedIPs() ([]RateLimitedIPRecord, error) {
	var records []RateLimitedIPRecord
	if !common.RedisEnabled {
		rateLimitedIPMemory.Lock()
		for _, rec := range rateLimitedIPMemory.records {
			records = append(records, *rec)
		}
		rateLimitedIPMemory.Unlock()
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		raw, err := common.RDB.HGetAll(ctx, rateLimitedIPRecordsKey).Result()
		if err != nil {
			return nil, err
		}
		for ip, data := range raw {
			rec := RateLimitedIPRecord{IP: ip}
			if err := common.UnmarshalJsonStr(data, &rec); err != nil {
				continue // 脏数据跳过
			}
			records = append(records, rec)
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].LastSeen > records[j].LastSeen })
	return records, nil
}

// DeleteRateLimitedIPRecord 删除一条限流触发记录。
func DeleteRateLimitedIPRecord(ip string) error {
	if !common.RedisEnabled {
		rateLimitedIPMemory.Lock()
		delete(rateLimitedIPMemory.records, ip)
		rateLimitedIPMemory.Unlock()
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return common.RDB.HDel(ctx, rateLimitedIPRecordsKey, ip).Err()
}
