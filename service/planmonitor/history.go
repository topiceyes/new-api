package planmonitor

import (
	"time"

	"github.com/QuantumNous/new-api/model"
)

// UsageHistoryPoint 趋势图数据点。
type UsageHistoryPoint struct {
	Ts          int64   `json:"ts"`           // 点时间戳(秒),聚合时为小时桶起点
	UsedPercent float64 `json:"used_percent"` // 已用百分比
}

// GetUsageHistoryPoints 取某套餐某周期近 rangeHours 小时的趋势点(时间升序)。
// 24h 内返回原始点(刷新间隔最密 5 分钟,一天约 288 点,可直接画);
// 更长范围按小时桶取平均,30 天最多 720 点,避免前端一次渲染过多。
// 聚合在 Go 侧做,避开 SQLite/MySQL/PostgreSQL 的时间函数方言差异。
func GetUsageHistoryPoints(planId int64, period string, rangeHours int) ([]UsageHistoryPoint, error) {
	since := time.Now().Add(-time.Duration(rangeHours) * time.Hour).Unix()
	rows, err := model.GetPlanMonitorUsageHistory(planId, period, since)
	if err != nil {
		return nil, err
	}
	if rangeHours <= 24 {
		points := make([]UsageHistoryPoint, 0, len(rows))
		for _, r := range rows {
			points = append(points, UsageHistoryPoint{Ts: r.FetchedAt, UsedPercent: r.UsedPercent})
		}
		return points, nil
	}
	type bucket struct {
		sum   float64
		count int
	}
	buckets := make(map[int64]*bucket)
	var order []int64
	for _, r := range rows {
		ts := r.FetchedAt - r.FetchedAt%3600
		b, ok := buckets[ts]
		if !ok {
			b = &bucket{}
			buckets[ts] = b
			order = append(order, ts)
		}
		b.sum += r.UsedPercent
		b.count++
	}
	points := make([]UsageHistoryPoint, 0, len(order))
	for _, ts := range order {
		b := buckets[ts]
		points = append(points, UsageHistoryPoint{Ts: ts, UsedPercent: b.sum / float64(b.count)})
	}
	return points, nil
}
