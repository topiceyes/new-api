package planmonitor

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

// 火山方舟(Volcengine Ark)套餐用量查询,覆盖 Agent Plan 与 Coding Plan。
// 官方接口(控制面统一网关):
//
//	POST https://open.volcengineapi.com/?Action={GetAFPUsage|GetCodingPlanUsage}&Version=2024-01-01&Region=cn-beijing
//
// 认证:火山引擎签名 V4(AK/SK),与 AWS SigV4 同构但有两处致命差异:
//  1. canonical headers 与 SignedHeaders 用**固定顺序**
//     `host;x-date;x-content-sha256;content-type`(**不按字母序**,照搬标准 SigV4 会签名失败);
//  2. algorithm 串 `HMAC-SHA256`(无 `AWS4_` 前缀)、credential scope 结尾 `request`
//     (非 `aws4_request`)、签名密钥派生 `kDate=HMAC(SK, date)`(SK 不加 `AWS4` 前缀)。
//
// 配置方式(凭证字段填 JSON):
//
//	{"ak":"AKLT...","sk":"...","region":"cn-beijing"}
//
// region 缺省 cn-beijing。AK/SK 在火山控制台「访问控制 IAM → API 访问密钥」开通,
// 与推理用的 Bearer API Key 是两套凭据,复用 Bearer Key 会被网关 400 InvalidAuthorization 拒绝。
//
// ⚠️ 两种套餐同一份 AK/SK 通用,但拆成两个独立 provider,各查各的,互不干扰:
//   - volcengine        = Agent Plan(GetAFPUsage,绝对值 AFP Quota/Used)
//   - volcengine_coding = Coding Plan(GetCodingPlanUsage,只给百分比)
//
// 同一份 AK/SK 配两条监控(各选一个 provider),即可同时看到两个套餐。
// 参考实现(带实测注释):github.com/farion1231/cc-switch src-tauri/src/services/coding_plan.rs
// (volcengine_sign / parse_afp_tiers / parse_coding_plan_tiers)。
type arkProvider struct{}       // volcengine = Agent Plan
type arkCodingProvider struct{} // volcengine_coding = Coding Plan

func init() {
	registerProvider(arkProvider{})
	registerProvider(arkCodingProvider{})
}

func (arkProvider) Name() string       { return "volcengine" }
func (arkCodingProvider) Name() string { return "volcengine_coding" }

const (
	arkAPIHost    = "open.volcengineapi.com"
	arkVersion    = "2024-01-01"
	arkService    = "ark"
	arkTerminator = "request"
	arkAlgorithm  = "HMAC-SHA256"
	// ⚠️ 火山特有固定顺序(非字母序),与 cc-switch 实测一致。
	arkSignedHeaders = "host;x-date;x-content-sha256;content-type"
	arkContentType   = "application/json; charset=utf-8"
)

// arkCred 火山引擎 AK/SK 凭证,从 JSON 解析。
type arkCred struct {
	AK     string `json:"ak"`
	SK     string `json:"sk"`
	Region string `json:"region"` // 缺省 cn-beijing
}

func (c arkCred) region() string {
	if r := strings.TrimSpace(c.Region); r != "" {
		return r
	}
	return "cn-beijing"
}

// parseArkCred 解析凭证 JSON。ak/sk 必填。
func parseArkCred(apiKey string) (arkCred, error) {
	k := strings.TrimSpace(apiKey)
	if k == "" {
		return arkCred{}, fmt.Errorf("empty api key")
	}
	var cred arkCred
	if err := common.Unmarshal([]byte(k), &cred); err != nil {
		return arkCred{}, fmt.Errorf("volcengine 凭证需为 JSON({\"ak\":\"...\",\"sk\":\"...\",\"region\":\"cn-beijing\"}): %w", err)
	}
	cred.AK = strings.TrimSpace(cred.AK)
	cred.SK = strings.TrimSpace(cred.SK)
	if cred.AK == "" || cred.SK == "" {
		return arkCred{}, fmt.Errorf("volcengine 凭证缺少 ak 或 sk")
	}
	return cred, nil
}

// arkEnvelope 火山 OpenAPI 标准信封。业务错误可能 HTTP 200 但 ResponseMetadata.Error 非空。
type arkEnvelope struct {
	ResponseMetadata *struct {
		Error *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error"`
	} `json:"ResponseMetadata"`
	Error *struct { // 兼容顶层 Error
		Code    string `json:"Code"`
		Message string `json:"Message"`
	} `json:"Error"`
	Result json.RawMessage `json:"Result"`
}

// arkAFPPanel GetAFPUsage 的单个窗口(绝对值 AFP 点数)。
type arkAFPPanel struct {
	Quota     float64 `json:"Quota"`
	Used      float64 `json:"Used"`
	ResetTime int64   `json:"ResetTime"` // 秒或毫秒,<=0 无重置
}

// arkAFPResult GetAFPUsage 的 Result。
type arkAFPResult struct {
	PlanType    string       `json:"PlanType"`
	AFPFiveHour *arkAFPPanel `json:"AFPFiveHour"`
	AFPDaily    *arkAFPPanel `json:"AFPDaily"` // 控制台隐藏,跳过
	AFPWeekly   *arkAFPPanel `json:"AFPWeekly"`
	AFPMonthly  *arkAFPPanel `json:"AFPMonthly"`
}

// arkCodingResult GetCodingPlanUsage 的 Result(只给百分比)。
type arkCodingResult struct {
	Status     string `json:"Status"`
	QuotaUsage []struct {
		Level          string      `json:"Level"`
		Percent        float64     `json:"Percent"`
		ResetTimestamp int64       `json:"ResetTimestamp"` // 秒或毫秒,<=0 无重置
		ResetTime      interface{} `json:"ResetTime"`      // 兼容字段
	} `json:"QuotaUsage"`
}

// FetchUsage Agent Plan(volcengine):只查 GetAFPUsage,绝对值 Quota/Used → 百分比。
func (arkProvider) FetchUsage(ctx context.Context, apiUrl string, apiKey string) ([]PeriodUsage, error) {
	cred, err := parseArkCred(apiKey)
	if err != nil {
		return nil, err
	}
	body, err := arkOpenAPICall(ctx, cred, "GetAFPUsage", time.Now().UTC())
	if err != nil {
		return nil, err
	}
	var afp arkAFPResult
	if rerr := common.Unmarshal(arkResultOf(body), &afp); rerr != nil {
		return nil, fmt.Errorf("parse volcengine GetAFPUsage: %w", rerr)
	}
	out := afp.toPeriodUsages()
	if len(out) == 0 {
		return nil, fmt.Errorf("volcengine: 无有效 Agent Plan 订阅(签名已通过);若只订了 Coding Plan 请改用 volcengine_coding")
	}
	return out, nil
}

// FetchUsage Coding Plan(volcengine_coding):只查 GetCodingPlanUsage,Percent 即已用。
func (arkCodingProvider) FetchUsage(ctx context.Context, apiUrl string, apiKey string) ([]PeriodUsage, error) {
	cred, err := parseArkCred(apiKey)
	if err != nil {
		return nil, err
	}
	body, err := arkOpenAPICall(ctx, cred, "GetCodingPlanUsage", time.Now().UTC())
	if err != nil {
		return nil, err
	}
	var coding arkCodingResult
	if rerr := common.Unmarshal(arkResultOf(body), &coding); rerr != nil {
		return nil, fmt.Errorf("parse volcengine GetCodingPlanUsage: %w", rerr)
	}
	out := coding.toPeriodUsages()
	if len(out) == 0 {
		return nil, fmt.Errorf("volcengine_coding: 无有效 Coding Plan 订阅(签名已通过);若订的是 Agent Plan 请改用 volcengine")
	}
	return out, nil
}

// toPeriodUsages Agent Plan 绝对值 → 百分比。跳过 AFPDaily 与 Quota<=0 窗口。
func (r *arkAFPResult) toPeriodUsages() []PeriodUsage {
	var out []PeriodUsage
	for _, w := range []struct {
		period string
		panel  *arkAFPPanel
	}{
		{model.PlanPeriod5Hour, r.AFPFiveHour},
		{model.PlanPeriodWeekly, r.AFPWeekly},
		{model.PlanPeriodMonthly, r.AFPMonthly},
	} {
		p := w.panel
		if p == nil || p.Quota <= 0 {
			continue
		}
		used := clampPercent(p.Used / p.Quota * 100)
		out = append(out, PeriodUsage{
			Period:           w.period,
			UsedPercent:      used,
			RemainingPercent: clampPercent(100 - used),
			PeriodEndTime:    arkResetToSec(p.ResetTime),
		})
	}
	return out
}

// toPeriodUsages Coding Plan 百分比窗口 → PeriodUsage。
func (r *arkCodingResult) toPeriodUsages() []PeriodUsage {
	var out []PeriodUsage
	for _, q := range r.QuotaUsage {
		period := arkLevelToPeriod(q.Level)
		if period == "" {
			continue
		}
		reset := q.ResetTimestamp
		if reset <= 0 {
			// 兼容 ResetTime(可能是数字或 ISO 字符串)。
			reset = arkResetTimeValue(q.ResetTime)
		}
		used := clampPercent(q.Percent)
		out = append(out, PeriodUsage{
			Period:           period,
			UsedPercent:      used,
			RemainingPercent: clampPercent(100 - used),
			PeriodEndTime:    arkResetToSec(reset),
		})
	}
	return out
}

// arkLevelToPeriod 映射 Level:session/5h=5h 窗口,weekly=周,monthly=月。
func arkLevelToPeriod(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "session", "5h", "fivehour", "five_hour", "rolling_5h":
		return model.PlanPeriod5Hour
	case "weekly", "week", "7d":
		return model.PlanPeriodWeekly
	case "monthly", "month":
		return model.PlanPeriodMonthly
	}
	return ""
}

// arkResetTimeValue 兼容 ResetTime 是 float64 / string(数字或 ISO)。
func arkResetTimeValue(v interface{}) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case string:
		if n, err := strconv.ParseInt(t, 10, 64); err == nil {
			return n
		}
		if ts, err := time.Parse(time.RFC3339, t); err == nil {
			return ts.Unix()
		}
	}
	return 0
}

// arkResetToSec 归一重置时间:<=0 视为无重置(火山 session 无活跃窗口回 -1);>1e12 按毫秒转秒。
func arkResetToSec(ts int64) int64 {
	if ts <= 0 {
		return 0
	}
	return msToSec(ts)
}

// arkResultOf 提取信封里的 Result(为空时回退整 body,兼容平铺)。
func arkResultOf(body []byte) []byte {
	var env arkEnvelope
	if err := common.Unmarshal(body, &env); err == nil && len(env.Result) > 0 && string(env.Result) != "null" {
		return env.Result
	}
	return body
}

// arkCallHook 测试注入点:非 nil 时替代真实 HTTP 调用(参数 action,返回响应体)。
var arkCallHook func(action string) ([]byte, error)

// arkOpenAPICall 发起一次签名后的 POST(空 body),校验信封业务错误,返回响应体。
func arkOpenAPICall(ctx context.Context, cred arkCred, action string, now time.Time) ([]byte, error) {
	if arkCallHook != nil {
		return arkCallHook(action)
	}
	canonicalQuery := arkCanonicalQuery(action, cred.region())
	u := "https://" + arkAPIHost + "/?" + canonicalQuery

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return nil, err
	}
	arkSign(req, cred, canonicalQuery, now)

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

	// 信封业务错误(200 或非 2xx 都可能带)。
	var env arkEnvelope
	if uerr := common.Unmarshal(body, &env); uerr == nil {
		var e *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		}
		if env.ResponseMetadata != nil && env.ResponseMetadata.Error != nil {
			e = env.ResponseMetadata.Error
		} else if env.Error != nil {
			e = env.Error
		}
		if e != nil && (e.Code != "" || e.Message != "") {
			return nil, fmt.Errorf("volcengine %s: %s %s", action, e.Code, e.Message)
		}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("volcengine %s returned status %d: %s", action, resp.StatusCode, truncateForErr(string(body)))
	}
	return body, nil
}

// arkSign 火山签名 V4。canonicalQuery 必须与实际请求 URL 的 query 逐字一致。
func arkSign(req *http.Request, cred arkCred, canonicalQuery string, now time.Time) {
	timestamp := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	payloadHash := sha256Hex(nil)

	// 固定顺序 canonical headers(火山特有,不排序)。
	canonicalHeaders := "host:" + arkAPIHost + "\nx-date:" + timestamp +
		"\nx-content-sha256:" + payloadHash + "\ncontent-type:" + arkContentType + "\n"
	canonicalRequest := strings.Join([]string{
		http.MethodPost,
		"/",
		canonicalQuery,
		canonicalHeaders,
		arkSignedHeaders,
		payloadHash,
	}, "\n")

	credentialScope := dateStamp + "/" + cred.region() + "/" + arkService + "/" + arkTerminator
	stringToSign := strings.Join([]string{
		arkAlgorithm,
		timestamp,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	signature := arkSignature(stringToSign, cred.SK, dateStamp, cred.region())

	req.Header.Set("Content-Type", arkContentType)
	req.Header.Set("X-Date", timestamp)
	req.Header.Set("X-Content-Sha256", payloadHash)
	req.Header.Set("Authorization", arkAlgorithm+" Credential="+cred.AK+"/"+credentialScope+
		", SignedHeaders="+arkSignedHeaders+", Signature="+signature)
}

// arkSignature 派生签名密钥:HMAC 链 date→region→service→request,最后签 stringToSign。
// SK 不加 AWS4 前缀(与标准 SigV4 的关键差异)。
func arkSignature(stringToSign, sk, dateStamp, region string) string {
	dateKey := hmacSHA256([]byte(sk), dateStamp)
	regionKey := hmacSHA256(dateKey, region)
	serviceKey := hmacSHA256(regionKey, arkService)
	signingKey := hmacSHA256(serviceKey, arkTerminator)
	return hex.EncodeToString(hmacSHA256(signingKey, stringToSign))
}

func hmacSHA256(key []byte, msg string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(msg))
	return h.Sum(nil)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// arkCanonicalQuery 按 key 字母序排序、逐段 URL 编码;同一份字符串既用于签名也用于实际 URL。
func arkCanonicalQuery(action, region string) string {
	pairs := [][2]string{
		{"Action", action},
		{"Region", region},
		{"Version", arkVersion},
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i][0] < pairs[j][0] })
	var sb strings.Builder
	for i, p := range pairs {
		if i > 0 {
			sb.WriteByte('&')
		}
		sb.WriteString(arkEscape(p[0]))
		sb.WriteByte('=')
		sb.WriteString(arkEscape(p[1]))
	}
	return sb.String()
}

// arkEscape RFC3986 unreserved 之外全部按 %XX 编码。
func arkEscape(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			sb.WriteByte(c)
		} else {
			fmt.Fprintf(&sb, "%%%02X", c)
		}
	}
	return sb.String()
}
