package planmonitor

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 火山 provider 测试分三层:
//   - 解析层(不依赖网络):Agent/Coding 的 toPeriodUsages、-1 重置、毫秒转换、daily/Quota=0 跳过
//   - 签名层:arkSign 确定性/敏感性、canonical query 排序、arkEscape、空 body payloadHash
//   - 双探测流程:用 arkCallHook 注入伪响应,验证 Agent 命中短路 / Coding 回落 / 无订阅报错
//
// 端到端 HTTP mock 不可行是因为 arkOpenAPICall 写死了 arkAPIHost 常量(生产正确),
// 这里用 hook 注入代替,签名正确性由签名层单测保证。

// --- 测试辅助 ---

// arkResetCallHook 清空注入的 hook。
func arkResetCallHook() { arkCallHook = nil }

// arkTestTime 固定时间,保证签名可复现。
func arkTestTime() time.Time {
	t, _ := time.Parse("20060102T150405Z", "20260818T023000Z")
	return t
}

// --- 解析层测试(不依赖网络) ---

// Agent Plan:绝对值 Quota/Used → 百分比;AFPDaily 跳过;Quota<=0 窗口跳过。
func TestArkAFPResult_ToPeriodUsages(t *testing.T) {
	r := arkAFPResult{
		PlanType:    "Medium",
		AFPFiveHour: &arkAFPPanel{Quota: 2000, Used: 500, ResetTime: 1787000000},
		AFPDaily:    &arkAFPPanel{Quota: 99999, Used: 1, ResetTime: 1787000000},       // 应跳过
		AFPWeekly:   &arkAFPPanel{Quota: 10000, Used: 6200, ResetTime: 1787500000000}, // 毫秒
		AFPMonthly:  &arkAFPPanel{Quota: 0, Used: 0},                                  // Quota=0 应跳过
	}
	out := r.toPeriodUsages()
	require.Len(t, out, 2, "daily 与 Quota=0 应跳过")

	byPeriod := map[string]PeriodUsage{}
	for _, u := range out {
		byPeriod[u.Period] = u
	}
	fiveH := byPeriod[model.PlanPeriod5Hour]
	assert.InDelta(t, 25, fiveH.UsedPercent, 0.001, "500/2000=25%")
	assert.InDelta(t, 75, fiveH.RemainingPercent, 0.001)
	assert.Equal(t, int64(1787000000), fiveH.PeriodEndTime)

	weekly := byPeriod[model.PlanPeriodWeekly]
	assert.InDelta(t, 62, weekly.UsedPercent, 0.001, "6200/10000=62%")
	assert.Equal(t, int64(1787500000), weekly.PeriodEndTime, "毫秒应转秒")

	_, hasMonthly := byPeriod[model.PlanPeriodMonthly]
	assert.False(t, hasMonthly, "Quota=0 的月窗口应跳过")
}

// Agent Plan:session 无活跃窗口时 ResetTime=-1 → PeriodEndTime=0。
func TestArkAFPResult_NegativeResetTime(t *testing.T) {
	r := arkAFPResult{
		AFPFiveHour: &arkAFPPanel{Quota: 2000, Used: 100, ResetTime: -1},
	}
	out := r.toPeriodUsages()
	require.Len(t, out, 1)
	assert.Equal(t, int64(0), out[0].PeriodEndTime, "-1 应视为无重置时间")
}

// Coding Plan:Level 映射 + Percent 即已用。
func TestArkCodingResult_ToPeriodUsages(t *testing.T) {
	var r arkCodingResult
	r.QuotaUsage = []struct {
		Level          string      `json:"Level"`
		Percent        float64     `json:"Percent"`
		ResetTimestamp int64       `json:"ResetTimestamp"`
		ResetTime      interface{} `json:"ResetTime"`
	}{
		{Level: "session", Percent: 35.5, ResetTimestamp: 1787000000},
		{Level: "weekly", Percent: 62, ResetTimestamp: 1787500000},
		{Level: "monthly", Percent: 18.2, ResetTimestamp: -1},     // 无活跃窗口
		{Level: "daily", Percent: 99, ResetTimestamp: 1787000000}, // 未知 Level 跳过
	}
	out := r.toPeriodUsages()
	require.Len(t, out, 3, "daily 应跳过")

	byPeriod := map[string]PeriodUsage{}
	for _, u := range out {
		byPeriod[u.Period] = u
	}
	assert.InDelta(t, 35.5, byPeriod[model.PlanPeriod5Hour].UsedPercent, 0.001)
	assert.InDelta(t, 62, byPeriod[model.PlanPeriodWeekly].UsedPercent, 0.001)
	assert.Equal(t, int64(0), byPeriod[model.PlanPeriodMonthly].PeriodEndTime, "-1 应归一为 0")
}

// --- 签名层测试 ---

// 签名确定性 + 敏感性(SK/Action 变化必改签名)。
func TestArkSign_DeterministicAndSensitive(t *testing.T) {
	now := arkTestTime()
	cred := arkCred{AK: "AKxxx", SK: "sk-one"}
	q1 := arkCanonicalQuery("GetCodingPlanUsage", "cn-beijing")

	req1, _ := http.NewRequest(http.MethodPost, "https://"+arkAPIHost+"/?"+q1, nil)
	arkSign(req1, cred, q1, now, "")
	auth1 := req1.Header.Get("Authorization")
	assert.Contains(t, auth1, "SignedHeaders=host;x-date;x-content-sha256;content-type")

	// 同输入同签名。
	req2, _ := http.NewRequest(http.MethodPost, "https://"+arkAPIHost+"/?"+q1, nil)
	arkSign(req2, cred, q1, now, "")
	assert.Equal(t, auth1, req2.Header.Get("Authorization"))

	// 换 SK 必变。
	cred.SK = "sk-two"
	req3, _ := http.NewRequest(http.MethodPost, "https://"+arkAPIHost+"/?"+q1, nil)
	arkSign(req3, cred, q1, now, "")
	assert.NotEqual(t, auth1, req3.Header.Get("Authorization"))

	// 换 Action(query 变)必变。
	cred.SK = "sk-one"
	q2 := arkCanonicalQuery("GetAFPUsage", "cn-beijing")
	req4, _ := http.NewRequest(http.MethodPost, "https://"+arkAPIHost+"/?"+q2, nil)
	arkSign(req4, cred, q2, now, "")
	assert.NotEqual(t, auth1, req4.Header.Get("Authorization"))
}

// canonical query 按 key 字母序。
func TestArkCanonicalQuery_Sorted(t *testing.T) {
	q := arkCanonicalQuery("GetCodingPlanUsage", "cn-beijing")
	assert.Equal(t, "Action=GetCodingPlanUsage&Region=cn-beijing&Version=2024-01-01", q)
}

// 空 body payloadHash 固定。
func TestArkEmptyPayloadHash(t *testing.T) {
	assert.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", sha256Hex(nil))
}

// arkEscape:unreserved 原样,其余 %XX 大写。
func TestArkEscape(t *testing.T) {
	assert.Equal(t, "abc-XYZ_019.~", arkEscape("abc-XYZ_019.~"))
	assert.Equal(t, "a%2Fb%3Dc", arkEscape("a/b=c"))
}

// --- 凭证解析 ---

func TestParseArkCred(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		c, err := parseArkCred(`{"ak":"AK1","sk":"SK1","region":"cn-shanghai"}`)
		require.NoError(t, err)
		assert.Equal(t, "AK1", c.AK)
		assert.Equal(t, "SK1", c.SK)
		assert.Equal(t, "cn-shanghai", c.region())
	})
	t.Run("default region", func(t *testing.T) {
		c, err := parseArkCred(`{"ak":"AK1","sk":"SK1"}`)
		require.NoError(t, err)
		assert.Equal(t, "cn-beijing", c.region())
	})
	t.Run("not json", func(t *testing.T) {
		_, err := parseArkCred("plain-key")
		require.Error(t, err)
	})
	t.Run("missing sk", func(t *testing.T) {
		_, err := parseArkCred(`{"ak":"AK1"}`)
		require.Error(t, err)
	})
	t.Run("empty", func(t *testing.T) {
		_, err := parseArkCred("  ")
		require.Error(t, err)
	})
}

// --- 单套餐拉取流程(用可替换的 call hook) ---
// 拆分后 volcengine 只调 GetAFPUsage,volcengine_coding 只调 GetCodingPlanUsage,互不探测。

// Agent Plan 命中:只调 GetAFPUsage。
func TestArkFetchUsage_AgentPlanHit(t *testing.T) {
	defer arkResetCallHook()
	calls := []string{}
	arkCallHook = func(action string) ([]byte, error) {
		calls = append(calls, action)
		return []byte(`{"Result":{"PlanType":"Medium","AFPFiveHour":{"Quota":2000,"Used":500,"ResetTime":1787000000},"AFPWeekly":{"Quota":10000,"Used":6200,"ResetTime":1787500000},"AFPMonthly":{"Quota":250000,"Used":1000,"ResetTime":1789900000}}}`), nil
	}
	usages, err := arkProvider{}.FetchUsage(context.Background(), "", `{"ak":"A","sk":"S"}`, "")
	require.NoError(t, err)
	require.Len(t, usages, 3)
	assert.Equal(t, []string{"GetAFPUsage"}, calls, "volcengine 只应调 GetAFPUsage")
}

// Agent Plan 无有效窗口(Quota 全 0)→ 报错并引导改用 volcengine_coding,不再回落。
func TestArkFetchUsage_AgentPlanNoWindow(t *testing.T) {
	defer arkResetCallHook()
	arkCallHook = func(action string) ([]byte, error) {
		return []byte(`{"Result":{"PlanType":"","AFPFiveHour":{"Quota":0,"Used":0}}}`), nil
	}
	_, err := arkProvider{}.FetchUsage(context.Background(), "", `{"ak":"A","sk":"S"}`, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "volcengine_coding", "应引导改用 coding provider")
}

// Coding Plan 命中:只调 GetCodingPlanUsage,Level 映射 + Percent 即已用。
func TestArkCodingFetchUsage_Hit(t *testing.T) {
	defer arkResetCallHook()
	calls := []string{}
	arkCallHook = func(action string) ([]byte, error) {
		calls = append(calls, action)
		return []byte(`{"Result":{"Status":"active","QuotaUsage":[{"Level":"session","Percent":42,"ResetTimestamp":1787000000},{"Level":"weekly","Percent":17,"ResetTimestamp":1787500000}]}}`), nil
	}
	usages, err := arkCodingProvider{}.FetchUsage(context.Background(), "", `{"ak":"A","sk":"S"}`, "")
	require.NoError(t, err)
	require.Len(t, usages, 2)
	assert.Equal(t, []string{"GetCodingPlanUsage"}, calls, "volcengine_coding 只应调 GetCodingPlanUsage")
	byPeriod := map[string]PeriodUsage{}
	for _, u := range usages {
		byPeriod[u.Period] = u
	}
	assert.InDelta(t, 42, byPeriod[model.PlanPeriod5Hour].UsedPercent, 0.001)
	assert.InDelta(t, 17, byPeriod[model.PlanPeriodWeekly].UsedPercent, 0.001)
}

// Coding Plan 无有效订阅 → 报错并引导改用 volcengine。
func TestArkCodingFetchUsage_NoSubscription(t *testing.T) {
	defer arkResetCallHook()
	arkCallHook = func(action string) ([]byte, error) {
		return []byte(`{"Result":{"Status":"inactive","QuotaUsage":[]}}`), nil
	}
	_, err := arkCodingProvider{}.FetchUsage(context.Background(), "", `{"ak":"A","sk":"S"}`, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "volcengine", "应引导改用 agent provider")
}

// 信封业务错误(HTTP 200 + ResponseMetadata.Error)应报错。
func TestArkFetchUsage_EnvelopeError(t *testing.T) {
	defer arkResetCallHook()
	arkCallHook = func(action string) ([]byte, error) {
		return nil, assert.AnError // 走 arkOpenAPICall 的错误路径,这里 hook 直接返回错误
	}
	_, err := arkProvider{}.FetchUsage(context.Background(), "", `{"ak":"A","sk":"S"}`, "")
	require.Error(t, err)
}
