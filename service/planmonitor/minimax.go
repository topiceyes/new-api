package planmonitor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

// MiniMax 套餐用量查询。
// 接口:GET {api_url}/v1/token_plan/remains
// 国际站 api.minimax.io,中国站 api.minimaxi.com。
// 旧接口 /v1/api/openplatform/coding_plan/remains 部分套餐返回 404,
// 故先试 token_plan,若 404 再回退旧接口。
// ⚠️ 关键坑:返回的是「剩余」百分比/次数,不是「已用」,多个第三方工具在此取反出错。
type miniMaxProvider struct{}

func init() { registerProvider(miniMaxProvider{}) }

func (miniMaxProvider) Name() string { return "minimax" }

// miniMaxRemainsResponse coding_plan/remains 的响应结构(按实际返回为准,字段容忍缺失)。
type miniMaxRemainsResponse struct {
	ModelRemains []struct {
		CurrentIntervalRemainingPercent float64 `json:"current_interval_remaining_percent"` // 5h 窗口剩余%
		CurrentIntervalUsageCount       float64 `json:"current_interval_usage_count"`       // 5h 窗口剩余次数(名字误导,实为剩余)
		CurrentWeeklyRemainingPercent   float64 `json:"current_weekly_remaining_percent"`   // 每周剩余%
		CurrentWeeklyStatus             int     `json:"current_weekly_status"`              // 1 表示周窗口有效
		EndTime                         int64   `json:"end_time"`                           // 5h 窗口重置时间(毫秒)
		WeeklyEndTime                   int64   `json:"weekly_end_time"`                    // 周窗口重置时间(毫秒)
	} `json:"model_remains"`
}

func (miniMaxProvider) FetchUsage(ctx context.Context, apiUrl string, apiKey string, userAgent string) ([]PeriodUsage, error) {
	base, err := ResolveAPIURL("minimax", apiUrl)
	if err != nil {
		return nil, err
	}
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return nil, fmt.Errorf("empty api key")
	}

	// 优先新接口 token_plan/remains;部分套餐仅支持旧接口 coding_plan/remains,
	// 新接口 404 时回退旧接口。
	body, err := miniMaxGet(ctx, base+"/v1/token_plan/remains", key, userAgent)
	if err != nil && is404Err(err) {
		body, err = miniMaxGet(ctx, base+"/v1/api/openplatform/coding_plan/remains", key, userAgent)
	}
	if err != nil {
		return nil, err
	}

	var parsed miniMaxRemainsResponse
	if err := common.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse minimax remains: %w", err)
	}
	if len(parsed.ModelRemains) == 0 {
		return nil, fmt.Errorf("minimax remains: empty model_remains")
	}

	// 取第一条 model_remains(套餐级用量;多模型时后续可扩展为按模型细分)。
	mr := parsed.ModelRemains[0]
	var out []PeriodUsage

	// 5 小时窗口:剩余% → 已用%
	if mr.CurrentIntervalRemainingPercent > 0 || mr.EndTime > 0 {
		remaining := clampPercent(mr.CurrentIntervalRemainingPercent)
		out = append(out, PeriodUsage{
			Period:           model.PlanPeriod5Hour,
			UsedPercent:      clampPercent(100 - remaining),
			RemainingPercent: remaining,
			PeriodEndTime:    msToSec(mr.EndTime),
		})
	}
	// 每周窗口:仅当周窗口有效时返回
	if mr.CurrentWeeklyStatus == 1 {
		remaining := clampPercent(mr.CurrentWeeklyRemainingPercent)
		out = append(out, PeriodUsage{
			Period:           model.PlanPeriodWeekly,
			UsedPercent:      clampPercent(100 - remaining),
			RemainingPercent: remaining,
			PeriodEndTime:    msToSec(mr.WeeklyEndTime),
		})
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("minimax remains: no usable period data")
	}
	return out, nil
}

// miniMaxGet 发起一次 GET 并返回响应体;非 200 时返回带状态码的错误。
func miniMaxGet(ctx context.Context, url string, apiKey string, userAgent string) ([]byte, error) {
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
		return nil, &httpStatusError{status: resp.StatusCode, body: string(body)}
	}
	return body, nil
}

// httpStatusError 带状态码的 HTTP 错误,便于识别 404 做接口回退。
type httpStatusError struct {
	status int
	body   string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("minimax remains returned status %d: %s", e.status, truncateForErr(e.body))
}

func is404Err(err error) bool {
	var se *httpStatusError
	if errors.As(err, &se) {
		return se.status == http.StatusNotFound
	}
	return false
}

// clampPercent 把百分比收敛到 [0,100],容忍上游毛刺。
func clampPercent(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// msToSec 毫秒时间戳转秒;小于 1e12 视为已是秒,原样返回。
func msToSec(ms int64) int64 {
	if ms > 1_000_000_000_000 {
		return ms / 1000
	}
	return ms
}

func truncateForErr(s string) string {
	const n = 200
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
