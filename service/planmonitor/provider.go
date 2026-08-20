package planmonitor

import (
	"context"
	"fmt"
	"strings"
)

// PeriodUsage 一个统计周期的用量快照(已换算为"已用"视角)。
type PeriodUsage struct {
	Period           string  // model.PlanPeriod*: 5h / weekly / monthly
	UsedPercent      float64 // 已用百分比(0-100)
	RemainingPercent float64 // 剩余百分比(0-100)
	PeriodEndTime    int64   // 周期截止(重置)时间戳(秒)
}

// Provider 一个供应商的套餐用量查询实现。按配置里的 provider 字段选择。
type Provider interface {
	// Name 供应商标识,与 PlanMonitor.Provider 对应(如 "minimax")。
	Name() string
	// FetchUsage 调用供应商接口,返回各周期用量。
	FetchUsage(ctx context.Context, apiUrl string, apiKey string) ([]PeriodUsage, error)
}

var providers = map[string]Provider{}

// defaultAPIUrls 各供应商的默认查询地址。配置页 API URL 留空时后端按此兜底,
// 用户无需手填;填了则优先用用户值(如 MiniMax 国际站、智谱国际站)。
var defaultAPIUrls = map[string]string{
	"minimax":             "https://api.minimaxi.com",
	"kimi":                "https://api.kimi.com/coding/v1",
	"bigmodel":            "https://bigmodel.cn",
	"bigmodel_enterprise": "https://bigmodel.cn",
	"volcengine":          "https://open.volcengineapi.com",
	"volcengine_coding":   "https://open.volcengineapi.com",
	"opencode":            "https://opencode.ai",
}

// DefaultAPIURL 返回供应商默认查询地址,无默认返回空串。
func DefaultAPIURL(provider string) string {
	return defaultAPIUrls[provider]
}

// ResolveAPIURL 归一查询地址:去空白与尾斜杠;为空时回落默认地址;仍无默认报错。
// 所有 provider 的 FetchUsage 统一用它处理 apiUrl,不再各自判空。
func ResolveAPIURL(provider string, apiUrl string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(apiUrl), "/")
	if base != "" {
		return base, nil
	}
	if def := DefaultAPIURL(provider); def != "" {
		return def, nil
	}
	return "", fmt.Errorf("empty api url")
}

func registerProvider(p Provider) {
	providers[p.Name()] = p
}

// GetProvider 按供应商标识取实现,未注册返回错误。
func GetProvider(name string) (Provider, error) {
	p, ok := providers[name]
	if !ok {
		return nil, fmt.Errorf("unsupported plan monitor provider: %s", name)
	}
	return p, nil
}

// SupportedProviders 返回已实现的供应商标识列表(供前端下拉)。
func SupportedProviders() []string {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	return names
}
