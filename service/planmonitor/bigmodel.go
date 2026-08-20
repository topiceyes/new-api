package planmonitor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

// 智谱 bigmodel(GLM Coding Plan)个人版套餐用量查询。
// 接口:GET {api_url}/api/monitor/usage/quota/limit
// 国内站 open.bigmodel.cn,国际站 api.z.ai。
// ⚠️ 认证特殊:Authorization 直接放 API key,不带 Bearer 前缀(与推理接口不同)。
// ⚠️ 与 MiniMax 相反:返回的 percentage 是「已用」百分比,不是剩余。
// 企业版(团队版)见 bigmodel_enterprise provider(?type=2 + JWT + org/project 头)。
type bigmodelProvider struct{}

func init() { registerProvider(bigmodelProvider{}) }

func (bigmodelProvider) Name() string { return "bigmodel" }

// bigmodel 配额窗口 unit 值(社区工具实测约定):
const (
	bigmodelUnitFiveHour = 3 // 5 小时滚动窗口
	bigmodelUnitWeekly   = 6 // 每周窗口(老 V1 套餐可能没有)
)

// bigmodelQuotaResponse quota/limit 响应。limits 在 data 下,部分套餐可能平铺在顶层。
// 注意:业务错误(如「不存在coding plan」)也是 HTTP 200,需看 code/success 字段。
type bigmodelQuotaResponse struct {
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
	Success *bool  `json:"success"`
	Data    struct {
		Limits []bigmodelLimit `json:"limits"`
	} `json:"data"`
	Limits []bigmodelLimit `json:"limits"` // 兼容平铺
}

type bigmodelLimit struct {
	Type          string  `json:"type"`          // TOKENS_LIMIT / TIME_LIMIT
	Unit          int     `json:"unit"`          // 3=5h, 6=weekly
	Percentage    float64 `json:"percentage"`    // 已用百分比(0-100)
	NextResetTime int64   `json:"nextResetTime"` // 周期重置时间戳(毫秒)
}

func (bigmodelProvider) FetchUsage(ctx context.Context, apiUrl string, apiKey string) ([]PeriodUsage, error) {
	base, err := ResolveAPIURL("bigmodel", apiUrl)
	if err != nil {
		return nil, err
	}
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return nil, fmt.Errorf("empty api key")
	}
	// JSON 凭证是企业版格式,误配到个人版时直接引导,避免上游报难懂的 401。
	if strings.HasPrefix(key, "{") {
		return nil, fmt.Errorf("bigmodel 个人版凭证应为 API key;企业版(团队版)请在 provider 选 bigmodel_enterprise")
	}

	body, err := bigmodelGet(ctx, base, key)
	if err != nil {
		return nil, err
	}
	return parseBigmodelQuota(body)
}

// parseBigmodelQuota 解析 quota/limit 响应为各周期用量。个人版/企业版共用(响应结构一致)。
func parseBigmodelQuota(body []byte) ([]PeriodUsage, error) {
	var parsed bigmodelQuotaResponse
	if err := common.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse bigmodel quota: %w", err)
	}
	// 业务错误识别:success==false 或 code 非 0/200 视为失败(如「不存在coding plan」)。
	if parsed.Success != nil && !*parsed.Success {
		return nil, fmt.Errorf("bigmodel quota: %s", parsed.Msg)
	}
	if parsed.Code != 0 && parsed.Code != 200 {
		return nil, fmt.Errorf("bigmodel quota: code=%d %s", parsed.Code, parsed.Msg)
	}
	limits := parsed.Data.Limits
	if len(limits) == 0 {
		limits = parsed.Limits
	}
	if len(limits) == 0 {
		return nil, fmt.Errorf("bigmodel quota: empty limits")
	}

	// 只取 TOKENS_LIMIT(token 配额),按 unit 区分 5h / 周窗口;TIME_LIMIT 是 MCP 月配额,不纳入。
	var fiveHour, weekly *bigmodelLimit
	for i := range limits {
		l := &limits[i]
		if l.Type != "TOKENS_LIMIT" {
			continue
		}
		switch l.Unit {
		case bigmodelUnitFiveHour:
			if fiveHour == nil {
				fiveHour = l
			}
		case bigmodelUnitWeekly:
			if weekly == nil {
				weekly = l
			}
		}
	}
	// 无 unit 标识时,按出现顺序兜底(第一个是 5h,第二个是周)。
	if fiveHour == nil {
		for i := range limits {
			if limits[i].Type == "TOKENS_LIMIT" {
				fiveHour = &limits[i]
				break
			}
		}
	}

	var out []PeriodUsage
	if fiveHour != nil {
		used := clampPercent(fiveHour.Percentage)
		out = append(out, PeriodUsage{
			Period:           model.PlanPeriod5Hour,
			UsedPercent:      used,
			RemainingPercent: clampPercent(100 - used),
			PeriodEndTime:    msToSec(fiveHour.NextResetTime),
		})
	}
	if weekly != nil {
		used := clampPercent(weekly.Percentage)
		out = append(out, PeriodUsage{
			Period:           model.PlanPeriodWeekly,
			UsedPercent:      used,
			RemainingPercent: clampPercent(100 - used),
			PeriodEndTime:    msToSec(weekly.NextResetTime),
		})
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("bigmodel quota: no usable TOKENS_LIMIT data")
	}
	return out, nil
}

// 智谱 bigmodel 企业版(团队版)套餐用量查询。
// 与个人版(bigmodel_personal.go)共用同一接口路径 /api/monitor/usage/quota/limit,
// 差异在认证与参数:
//   - 个人版:Authorization 裸 API key,无 type 参数
//   - 企业版:Authorization 网页登录态 JWT + bigmodel-organization/bigmodel-project 头,加 ?type=2
//
// ⚠️ 业务错误也是 HTTP 200:{"code":500,"success":false,"msg":"当前用户不存在coding plan"},
// 需看 success/code 字段识别。
// 凭证字段(Key)填 JSON:{"token":"<登录JWT>","org":"org-xxx","project":"proj_xxx"},
// 三项从浏览器抓 bigmodel.cn/coding-plan/team/usage-stats 页 Network 请求头。
type bigmodelEnterpriseProvider struct{}

func init() { registerProvider(bigmodelEnterpriseProvider{}) }

func (bigmodelEnterpriseProvider) Name() string { return "bigmodel_enterprise" }

// bigmodelEntCred 企业版凭证,从 Key 字段的 JSON 解析。
type bigmodelEntCred struct {
	Token string `json:"token"`
	Org   string `json:"org"`
	Proj  string `json:"project"`
}

func parseBigmodelEntCred(apiKey string) (bigmodelEntCred, error) {
	k := strings.TrimSpace(apiKey)
	if k == "" {
		return bigmodelEntCred{}, fmt.Errorf("empty credential")
	}
	var cred bigmodelEntCred
	if err := common.Unmarshal([]byte(k), &cred); err != nil {
		return bigmodelEntCred{}, fmt.Errorf("bigmodel 企业版凭证需为 JSON({\"token\":\"...\",\"org\":\"org-xxx\",\"project\":\"proj_xxx\"}): %w", err)
	}
	cred.Token = strings.TrimSpace(cred.Token)
	cred.Org = strings.TrimSpace(cred.Org)
	cred.Proj = strings.TrimSpace(cred.Proj)
	if cred.Token == "" {
		return bigmodelEntCred{}, fmt.Errorf("bigmodel 企业版凭证缺少 token")
	}
	return cred, nil
}

func (bigmodelEnterpriseProvider) FetchUsage(ctx context.Context, apiUrl string, apiKey string) ([]PeriodUsage, error) {
	base, err := ResolveAPIURL("bigmodel_enterprise", apiUrl)
	if err != nil {
		return nil, err
	}
	cred, err := parseBigmodelEntCred(apiKey)
	if err != nil {
		return nil, err
	}

	body, err := bigmodelEntGet(ctx, base, cred)
	if err != nil {
		return nil, err
	}
	return parseBigmodelQuota(body)
}

// bigmodelEntGet 企业版请求:?type=2 + JWT + 组织/项目头。
func bigmodelEntGet(ctx context.Context, base string, cred bigmodelEntCred) ([]byte, error) {
	url := base + "/api/monitor/usage/quota/limit?type=2"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", cred.Token)
	req.Header.Set("Accept", "application/json")
	if cred.Org != "" {
		req.Header.Set("bigmodel-organization", cred.Org)
	}
	if cred.Proj != "" {
		req.Header.Set("bigmodel-project", cred.Proj)
	}

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
		return nil, fmt.Errorf("bigmodel enterprise quota returned status %d: %s", resp.StatusCode, truncateForErr(string(body)))
	}
	return body, nil
}

// bigmodelGet 个人版请求:Authorization 直接放 API key(不带 Bearer),路径无 type 参数。
func bigmodelGet(ctx context.Context, base string, key string) ([]byte, error) {
	url := base + "/api/monitor/usage/quota/limit"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", key)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

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
		return nil, fmt.Errorf("bigmodel quota returned status %d: %s", resp.StatusCode, truncateForErr(string(body)))
	}
	return body, nil
}
