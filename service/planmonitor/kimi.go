package planmonitor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

// Kimi(Moonshot AI Coding Plan)套餐用量查询。
// 接口:GET {api_url}/usages  (api_url 填 https://api.kimi.com/coding/v1)
// 404 时回退 {api_url}/usage。
// ⚠️ host 是 api.kimi.com(Coding Plan 专用面),不是 api.moonshot.cn / platform.kimi.com。
// ⚠️ key 必须是 Kimi Code 控制台的 sk-kimi-xxx,开放平台 sk-xxx 不互通(401)。
// ⚠️ User-Agent 必须伪装 Kimi 官方 CLI,否则返回 access_terminated_error。
// 用量是计数制(used/limit,字符串数字),这里换算成百分比。
type kimiProvider struct{}

func init() { registerProvider(kimiProvider{}) }

func (kimiProvider) Name() string { return "kimi" }

// kimiUserAgent 伪装官方 CLI,避免 access_terminated_error。
const kimiUserAgent = "KimiCLI/1.6"

// kimiUsageResponse /usages 响应。usage 是周(7天)窗口汇总,limits 含各滚动窗口。
// 数值字段是字符串数字,用 json.Number 兼容字符串与数值两种返回。
type kimiUsageResponse struct {
	Usage  *kimiUsageEntry `json:"usage"`
	Limits []struct {
		Detail *kimiUsageEntry `json:"detail"`
		Window struct {
			Duration int    `json:"duration"`
			TimeUnit string `json:"timeUnit"` // TIME_UNIT_MINUTE 等
		} `json:"window"`
	} `json:"limits"`
}

// kimiUsageEntry 单个窗口的用量。used/limit/remaining 为字符串计数;resetTime 是 ISO8601(可带纳秒)。
type kimiUsageEntry struct {
	Used      json.Number `json:"used"`
	Limit     json.Number `json:"limit"`
	Remaining json.Number `json:"remaining"`
	ResetTime string      `json:"resetTime"` // ISO8601,如 2026-01-09T15:23:13.716839300Z
}

func (kimiProvider) FetchUsage(ctx context.Context, apiUrl string, apiKey string) ([]PeriodUsage, error) {
	base, err := ResolveAPIURL("kimi", apiUrl)
	if err != nil {
		return nil, err
	}
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return nil, fmt.Errorf("empty api key")
	}

	// 先试 /usages,404 回退 /usage。
	body, err := kimiGet(ctx, base+"/usages", key)
	if err != nil && is404Err(err) {
		body, err = kimiGet(ctx, base+"/usage", key)
	}
	if err != nil {
		return nil, err
	}

	var parsed kimiUsageResponse
	if err := common.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse kimi usages: %w", err)
	}

	var out []PeriodUsage
	// 各滚动窗口:5h(duration=300, TIME_UNIT_MINUTE)等,取 detail。
	for _, item := range parsed.Limits {
		entry := item.Detail
		if entry == nil {
			continue
		}
		period := kimiWindowToPeriod(item.Window.Duration, item.Window.TimeUnit)
		if period == "" {
			period = model.PlanPeriod5Hour // 无窗口信息时默认 5h
		}
		if pu, ok := entry.toPeriodUsage(period); ok {
			out = append(out, pu)
		}
	}
	// 周窗口汇总(usage 字段,7 天)。
	if parsed.Usage != nil {
		if pu, ok := parsed.Usage.toPeriodUsage(model.PlanPeriodWeekly); ok {
			out = append(out, pu)
		}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("kimi usages: no usable period data")
	}
	return out, nil
}

// toPeriodUsage 把计数制 used/limit(字符串)换算成百分比。无法换算时返回 false。
func (e *kimiUsageEntry) toPeriodUsage(period string) (PeriodUsage, bool) {
	limit, err1 := e.Limit.Float64()
	used, err2 := e.Used.Float64()
	if err1 != nil || err2 != nil {
		// used 缺失时尝试 remaining 反推。
		if rem, err := e.Remaining.Float64(); err == nil && err1 == nil && limit > 0 {
			used = limit - rem
		} else if err1 != nil {
			return PeriodUsage{}, false
		}
	}
	if used == 0 {
		if rem, err := e.Remaining.Float64(); err == nil && limit > 0 {
			used = limit - rem
		}
	}
	if limit <= 0 {
		return PeriodUsage{}, false
	}
	usedPct := clampPercent(used / limit * 100)
	return PeriodUsage{
		Period:           period,
		UsedPercent:      usedPct,
		RemainingPercent: clampPercent(100 - usedPct),
		PeriodEndTime:    kimiResetToSec(e.ResetTime),
	}, true
}

// kimiWindowToPeriod 把窗口 duration+timeUnit 映射到已知周期。
// 5h 窗口是 duration=300 + TIME_UNIT_MINUTE(300 分钟)。
func kimiWindowToPeriod(duration int, timeUnit string) string {
	u := strings.ToUpper(timeUnit)
	switch {
	case strings.Contains(u, "MINUTE"):
		if duration == 300 { // 300 分钟 = 5 小时
			return model.PlanPeriod5Hour
		}
	case strings.Contains(u, "HOUR"):
		if duration == 5 {
			return model.PlanPeriod5Hour
		}
	case strings.Contains(u, "DAY"), strings.Contains(u, "WEEK"):
		if duration == 7 || strings.Contains(u, "WEEK") {
			return model.PlanPeriodWeekly
		}
	case strings.Contains(u, "MONTH"):
		return model.PlanPeriodMonthly
	}
	return ""
}

// kimiResetToSec 解析 ISO8601 resetTime(可带纳秒)为秒时间戳。
// Go 的 time.RFC3339 不接受任意位纳秒,需用 RFC3339Nano 并容忍格式变体。
func kimiResetToSec(resetTime string) int64 {
	if resetTime == "" {
		return 0
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.999999999Z0700"} {
		if t, err := time.Parse(layout, resetTime); err == nil {
			return t.Unix()
		}
	}
	return 0
}

// kimiGet 发起一次 GET 并返回响应体。Bearer 认证 + Kimi CLI UA;非 200 返回带状态码错误。
func kimiGet(ctx context.Context, url string, apiKey string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", kimiUserAgent)

	client := service.GetHttpClient()
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &httpStatusError{status: resp.StatusCode, body: string(body)}
	}
	return body, nil
}
