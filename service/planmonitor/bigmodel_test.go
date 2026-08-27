package planmonitor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newBigmodelServer 起 mock,校验 Authorization 直接放 key(不带 Bearer),按 quota/limit 路径应答。
func newBigmodelServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/monitor/usage/quota/limit", r.URL.Path)
		assert.Equal(t, "test-key", r.Header.Get("Authorization"), "Authorization 应直接放 key,不带 Bearer")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestBigmodelFetchUsage_ParsesFiveHourAndWeekly(t *testing.T) {
	// percentage 是「已用」(与 MiniMax 相反),unit 3=5h, 6=weekly,nextResetTime 是毫秒。
	body := `{"data":{"limits":[
		{"type":"TIME_LIMIT","percentage":10,"currentValue":100,"usage":1000,"nextResetTime":1787000000000},
		{"type":"TOKENS_LIMIT","unit":3,"percentage":32,"nextResetTime":1786982400000},
		{"type":"TOKENS_LIMIT","unit":6,"percentage":15,"nextResetTime":1787500800000}
	]}}`
	srv := newBigmodelServer(t, http.StatusOK, body)

	usages, err := bigmodelProvider{}.FetchUsage(context.Background(), srv.URL, "test-key", "")
	require.NoError(t, err)
	require.Len(t, usages, 2, "TIME_LIMIT(MCP 月配额)应被忽略")

	byPeriod := map[string]PeriodUsage{}
	for _, u := range usages {
		byPeriod[u.Period] = u
	}

	fiveH, ok := byPeriod[model.PlanPeriod5Hour]
	require.True(t, ok, "unit=3 应映射为 5h")
	assert.InDelta(t, 32, fiveH.UsedPercent, 0.001, "percentage 即已用,直接采用")
	assert.InDelta(t, 68, fiveH.RemainingPercent, 0.001, "remaining = 100 - used")
	assert.Equal(t, int64(1786982400000)/1000, fiveH.PeriodEndTime, "ms 转 s")

	weekly, ok := byPeriod[model.PlanPeriodWeekly]
	require.True(t, ok, "unit=6 应映射为 weekly")
	assert.InDelta(t, 15, weekly.UsedPercent, 0.001)
	assert.Equal(t, int64(1787500800000)/1000, weekly.PeriodEndTime)
}

// 老 V1 套餐可能只有 5h 一个 TOKENS_LIMIT,无周窗口。
func TestBigmodelFetchUsage_FiveHourOnly(t *testing.T) {
	body := `{"data":{"limits":[
		{"type":"TOKENS_LIMIT","unit":3,"percentage":55,"nextResetTime":1786982400000}
	]}}`
	srv := newBigmodelServer(t, http.StatusOK, body)

	usages, err := bigmodelProvider{}.FetchUsage(context.Background(), srv.URL, "test-key", "")
	require.NoError(t, err)
	require.Len(t, usages, 1)
	assert.Equal(t, model.PlanPeriod5Hour, usages[0].Period)
	assert.InDelta(t, 55, usages[0].UsedPercent, 0.001)
}

// 兼容 limits 平铺在顶层(无 data 包裹)。
func TestBigmodelFetchUsage_FlattenedLimits(t *testing.T) {
	body := `{"limits":[
		{"type":"TOKENS_LIMIT","unit":3,"percentage":20,"nextResetTime":1786982400000}
	]}`
	srv := newBigmodelServer(t, http.StatusOK, body)

	usages, err := bigmodelProvider{}.FetchUsage(context.Background(), srv.URL, "test-key", "")
	require.NoError(t, err)
	require.Len(t, usages, 1)
	assert.InDelta(t, 20, usages[0].UsedPercent, 0.001)
}

// 无 unit 字段时按出现顺序兜底:第一个 TOKENS_LIMIT 当作 5h。
func TestBigmodelFetchUsage_NoUnitFallback(t *testing.T) {
	body := `{"data":{"limits":[
		{"type":"TOKENS_LIMIT","percentage":42,"nextResetTime":1786982400000}
	]}}`
	srv := newBigmodelServer(t, http.StatusOK, body)

	usages, err := bigmodelProvider{}.FetchUsage(context.Background(), srv.URL, "test-key", "")
	require.NoError(t, err)
	require.Len(t, usages, 1)
	assert.Equal(t, model.PlanPeriod5Hour, usages[0].Period)
	assert.InDelta(t, 42, usages[0].UsedPercent, 0.001)
}

func TestBigmodelFetchUsage_Errors(t *testing.T) {
	t.Run("non-200 status", func(t *testing.T) {
		srv := newBigmodelServer(t, http.StatusUnauthorized, `{"error":"bad key"}`)
		_, err := bigmodelProvider{}.FetchUsage(context.Background(), srv.URL, "test-key", "")
		require.Error(t, err)
	})

	t.Run("invalid json", func(t *testing.T) {
		srv := newBigmodelServer(t, http.StatusOK, `{not-json`)
		_, err := bigmodelProvider{}.FetchUsage(context.Background(), srv.URL, "test-key", "")
		require.Error(t, err)
	})

	t.Run("empty limits", func(t *testing.T) {
		srv := newBigmodelServer(t, http.StatusOK, `{"data":{"limits":[]}}`)
		_, err := bigmodelProvider{}.FetchUsage(context.Background(), srv.URL, "test-key", "")
		require.Error(t, err)
	})

	t.Run("no TOKENS_LIMIT", func(t *testing.T) {
		srv := newBigmodelServer(t, http.StatusOK, `{"data":{"limits":[{"type":"TIME_LIMIT","percentage":10}]}}`)
		_, err := bigmodelProvider{}.FetchUsage(context.Background(), srv.URL, "test-key", "")
		require.Error(t, err, "只有 TIME_LIMIT 应视为无可用数据")
	})

	t.Run("empty api url", func(t *testing.T) {
		_, err := bigmodelProvider{}.FetchUsage(context.Background(), "", "test-key", "")
		require.Error(t, err)
	})

	t.Run("empty api key", func(t *testing.T) {
		srv := newBigmodelServer(t, http.StatusOK, `{}`)
		_, err := bigmodelProvider{}.FetchUsage(context.Background(), srv.URL, "  ", "")
		require.Error(t, err)
	})
}

// 企业版(团队版):独立 provider bigmodel_enterprise,Key 字段填 JSON{token,org,project},
// 走 ?type=2 + JWT + 组织/项目头;响应结构与个人版一致,共用解析。
func TestBigmodelEnterpriseFetchUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/monitor/usage/quota/limit", r.URL.Path)
		assert.Equal(t, "2", r.URL.Query().Get("type"), "企业版应带 type=2")
		assert.Equal(t, "ent-jwt-token", r.Header.Get("Authorization"), "企业版 Authorization 是登录 JWT")
		assert.Equal(t, "org-abc", r.Header.Get("bigmodel-organization"))
		assert.Equal(t, "proj_xyz", r.Header.Get("bigmodel-project"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":200,"msg":"操作成功","success":true,"data":{"limits":[
			{"type":"TIME_LIMIT","unit":5,"percentage":2,"nextResetTime":1787546084997},
			{"type":"TOKENS_LIMIT","unit":3,"percentage":5,"nextResetTime":1787004354606},
			{"type":"TOKENS_LIMIT","unit":6,"percentage":26,"nextResetTime":1787286884997}
		],"level":"pro"}}`))
	}))
	t.Cleanup(srv.Close)

	cred := `{"token":"ent-jwt-token","org":"org-abc","project":"proj_xyz"}`
	usages, err := bigmodelEnterpriseProvider{}.FetchUsage(context.Background(), srv.URL, cred, "")
	require.NoError(t, err)
	require.Len(t, usages, 2, "TIME_LIMIT 应被忽略")

	byPeriod := map[string]PeriodUsage{}
	for _, u := range usages {
		byPeriod[u.Period] = u
	}
	assert.InDelta(t, 5, byPeriod[model.PlanPeriod5Hour].UsedPercent, 0.001)
	assert.Equal(t, int64(1787004354606)/1000, byPeriod[model.PlanPeriod5Hour].PeriodEndTime)
	assert.InDelta(t, 26, byPeriod[model.PlanPeriodWeekly].UsedPercent, 0.001)
	assert.Equal(t, int64(1787286884997)/1000, byPeriod[model.PlanPeriodWeekly].PeriodEndTime)
}

// 个人版收到 JSON 凭证应报错(防止企业版凭证误配到个人版)。
func TestBigmodelPersonalRejectsJSONCredential(t *testing.T) {
	srv := newBigmodelServer(t, http.StatusOK, `{"data":{"limits":[{"type":"TOKENS_LIMIT","unit":3,"percentage":1,"nextResetTime":1786982400000}]}}`)
	_, err := bigmodelProvider{}.FetchUsage(context.Background(), srv.URL, `{"token":"jwt","org":"o"}`, "")
	require.Error(t, err, "个人版凭证必须是 API key;JSON 凭证应引导用 bigmodel_enterprise")
}

// 企业版凭证缺 token 应报错。
func TestBigmodelEntCred_MissingToken(t *testing.T) {
	_, err := parseBigmodelEntCred(`{"org":"org-abc"}`)
	require.Error(t, err)
	_, err = parseBigmodelEntCred("plain-api-key")
	require.Error(t, err, "企业版凭证必须是 JSON")
	_, err = parseBigmodelEntCred("  ")
	require.Error(t, err)
}

// 业务错误:HTTP 200 但 success=false(如「当前用户不存在coding plan」)。个人/企业版共用解析。
func TestBigmodelFetchUsage_BusinessError(t *testing.T) {
	srv := newBigmodelServer(t, http.StatusOK, `{"code":500,"msg":"当前用户不存在coding plan","success":false}`)
	_, err := bigmodelProvider{}.FetchUsage(context.Background(), srv.URL, "test-key", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不存在coding plan")
}
