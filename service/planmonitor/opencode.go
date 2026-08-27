package planmonitor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

// OpenCode Go(opencode.ai)套餐用量查询。
// 接口:GET {api_url}/zen/go/v1/usage  (api_url 填 https://opencode.ai)
// 认证:Authorization: Bearer <OPENCODE_API_KEY>(与推理用的 key 相同)。
// 响应(JSON,也可能被 usage/data/result 包裹):
//
//	{"usage":{"rolling":{"percent":12,"resetsAt":"2026-08-12T02:00:00.000Z"},
//	          "weekly":{"percent":8,"resetsAt":"..."},
//	          "monthly":{"percent":35,"resetsAt":"..."}}}
//
// rolling=5h 滚动窗口,percent=已用,resetsAt=ISO8601 截止时间(也兼容 resetInSec 相对秒)。
// 参考:github.com/steipete/CodexBar OpenCodeGoUsageFetcher.fetchAPIUsage / parseSubscriptionJSON。
type opencodeProvider struct{}

func init() { registerProvider(opencodeProvider{}) }

func (opencodeProvider) Name() string { return "opencode" }

// opencodeUsageResponse zen/go/v1/usage 响应。窗口字段兼容多种键名与包裹层。
type opencodeUsageResponse struct {
	Usage   *opencodeWindows `json:"usage"`
	Data    *opencodeWindows `json:"data"`
	Result  *opencodeWindows `json:"result"`
	Payload *opencodeWindows `json:"payload"`
	// 平铺兜底
	Rolling *opencodeWindow `json:"rolling"`
	Weekly  *opencodeWindow `json:"weekly"`
	Monthly *opencodeWindow `json:"monthly"`
}

type opencodeWindows struct {
	Rolling *opencodeWindow `json:"rolling"`
	Weekly  *opencodeWindow `json:"weekly"`
	Monthly *opencodeWindow `json:"monthly"`
	// 兼容 seroval 风格键名
	RollingUsage *opencodeWindow `json:"rollingUsage"`
	WeeklyUsage  *opencodeWindow `json:"weeklyUsage"`
	MonthlyUsage *opencodeWindow `json:"monthlyUsage"`
}

// opencodeWindow 单个周期窗口。percent=已用;resetsAt=ISO8601,resetInSec=相对秒。
type opencodeWindow struct {
	Percent      *float64 `json:"percent"`
	UsagePercent *float64 `json:"usagePercent"`
	ResetsAt     string   `json:"resetsAt"`
	ResetAt      string   `json:"resetAt"`
	ResetInSec   *int64   `json:"resetInSec"`
}

func (opencodeProvider) FetchUsage(ctx context.Context, apiUrl string, apiKey string, userAgent string) ([]PeriodUsage, error) {
	base, err := ResolveAPIURL("opencode", apiUrl)
	if err != nil {
		return nil, err
	}
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return nil, fmt.Errorf("empty api key")
	}

	body, err := opencodeGet(ctx, base+"/zen/go/v1/usage", key, userAgent)
	if err != nil {
		return nil, err
	}

	var parsed opencodeUsageResponse
	if err := common.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse opencode usage: %w", err)
	}

	// 依次尝试各包裹层:usage > data > result > payload > 平铺。
	windows := []*opencodeWindows{parsed.Usage, parsed.Data, parsed.Result, parsed.Payload}
	var out []PeriodUsage
	for _, w := range windows {
		if w == nil {
			continue
		}
		if out = w.toPeriodUsages(); len(out) > 0 {
			return out, nil
		}
	}
	// 平铺兜底。
	flat := &opencodeWindows{Rolling: parsed.Rolling, Weekly: parsed.Weekly, Monthly: parsed.Monthly}
	if out = flat.toPeriodUsages(); len(out) > 0 {
		return out, nil
	}
	return nil, fmt.Errorf("opencode usage: no usable window data")
}

func (w *opencodeWindows) toPeriodUsages() []PeriodUsage {
	var out []PeriodUsage
	for _, item := range []struct {
		period string
		win    *opencodeWindow
	}{
		{model.PlanPeriod5Hour, firstNonNil(w.Rolling, w.RollingUsage)},
		{model.PlanPeriodWeekly, firstNonNil(w.Weekly, w.WeeklyUsage)},
		{model.PlanPeriodMonthly, firstNonNil(w.Monthly, w.MonthlyUsage)},
	} {
		if pu, ok := item.win.toPeriodUsage(item.period); ok {
			out = append(out, pu)
		}
	}
	return out
}

// toPeriodUsage 单窗口换算。percent 与 usagePercent 二选一,均为已用。
func (w *opencodeWindow) toPeriodUsage(period string) (PeriodUsage, bool) {
	if w == nil {
		return PeriodUsage{}, false
	}
	var pct float64
	if w.Percent != nil {
		pct = *w.Percent
	} else if w.UsagePercent != nil {
		pct = *w.UsagePercent
	} else {
		return PeriodUsage{}, false
	}
	used := clampPercent(pct)
	return PeriodUsage{
		Period:           period,
		UsedPercent:      used,
		RemainingPercent: clampPercent(100 - used),
		PeriodEndTime:    w.resetToSec(),
	}, true
}

// resetToSec 归一截止时间:优先 ISO8601 resetsAt,其次相对秒 resetInSec。
func (w *opencodeWindow) resetToSec() int64 {
	for _, s := range []string{w.ResetsAt, w.ResetAt} {
		if s == "" {
			continue
		}
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
			if t, err := time.Parse(layout, s); err == nil {
				return t.Unix()
			}
		}
	}
	if w.ResetInSec != nil && *w.ResetInSec > 0 {
		return time.Now().Unix() + *w.ResetInSec
	}
	return 0
}

func firstNonNil(a, b *opencodeWindow) *opencodeWindow {
	if a != nil {
		return a
	}
	return b
}

// opencodeGet 发起一次 GET 并返回响应体。Bearer 认证;非 200 返回带状态码错误。
func opencodeGet(ctx context.Context, url string, apiKey string, userAgent string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	applyUserAgent(req, userAgent)

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
		return nil, fmt.Errorf("opencode usage returned status %d: %s", resp.StatusCode, truncateForErr(string(body)))
	}
	return body, nil
}
