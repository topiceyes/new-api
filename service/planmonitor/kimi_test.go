package planmonitor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newKimiServer 起 mock,校验 Bearer 认证与 Kimi CLI UA;/usage 回退路径返回 404(回退由专门用例覆盖)。
func newKimiServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		assert.Equal(t, kimiUserAgent, r.Header.Get("User-Agent"), "UA 应伪装 Kimi CLI 防 access_terminated_error")
		if r.URL.Path == "/usage" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("404 page not found"))
			return
		}
		assert.Equal(t, "/usages", r.URL.Path)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// 标准:5h 窗口 duration=300+TIME_UNIT_MINUTE 在 limits[].detail,周窗口在 usage。
// 数值是字符串,resetTime 是带纳秒的 ISO 字符串。
func TestKimiFetchUsage_ParsesFiveHourAndWeekly(t *testing.T) {
	body := `{
		"usage": {"limit": "1500", "used": "300", "remaining": "1200", "resetTime": "2026-08-25T00:00:00Z"},
		"limits": [
			{"window": {"duration": 300, "timeUnit": "TIME_UNIT_MINUTE"},
			 "detail": {"limit": "1000", "used": "200", "remaining": "800", "resetTime": "2026-08-18T01:00:00Z"}}
		]
	}`
	srv := newKimiServer(t, http.StatusOK, body)

	usages, err := kimiProvider{}.FetchUsage(context.Background(), srv.URL, "test-key")
	require.NoError(t, err)
	require.Len(t, usages, 2)

	byPeriod := map[string]PeriodUsage{}
	for _, u := range usages {
		byPeriod[u.Period] = u
	}

	fiveH, ok := byPeriod[model.PlanPeriod5Hour]
	require.True(t, ok, "duration=300+MINUTE 应映射为 5h")
	assert.InDelta(t, 20, fiveH.UsedPercent, 0.001, "200/1000 = 20%")
	assert.InDelta(t, 80, fiveH.RemainingPercent, 0.001)
	want5h, _ := time.Parse(time.RFC3339, "2026-08-18T01:00:00Z")
	assert.Equal(t, want5h.Unix(), fiveH.PeriodEndTime)

	weekly, ok := byPeriod[model.PlanPeriodWeekly]
	require.True(t, ok, "usage 字段应映射为 weekly")
	assert.InDelta(t, 20, weekly.UsedPercent, 0.001, "300/1500 = 20%")
	wantW, _ := time.Parse(time.RFC3339, "2026-08-25T00:00:00Z")
	assert.Equal(t, wantW.Unix(), weekly.PeriodEndTime)
}

// 无 used 时用 remaining 反推:used = limit - remaining。
func TestKimiFetchUsage_RemainingFallback(t *testing.T) {
	body := `{
		"limits": [
			{"window": {"duration": 300, "timeUnit": "TIME_UNIT_MINUTE"},
			 "detail": {"limit": "1000", "remaining": "400", "resetTime": "2026-08-18T01:00:00Z"}}
		]
	}`
	srv := newKimiServer(t, http.StatusOK, body)

	usages, err := kimiProvider{}.FetchUsage(context.Background(), srv.URL, "test-key")
	require.NoError(t, err)
	require.Len(t, usages, 1)
	assert.InDelta(t, 60, usages[0].UsedPercent, 0.001, "used=1000-400=600 → 60%")
}

// 数值以 JSON number(非字符串)返回也能解析。
func TestKimiFetchUsage_NumericValues(t *testing.T) {
	body := `{
		"limits": [
			{"window": {"duration": 300, "timeUnit": "TIME_UNIT_MINUTE"},
			 "detail": {"limit": 500, "used": 250, "resetTime": "2026-08-18T01:00:00Z"}}
		]
	}`
	srv := newKimiServer(t, http.StatusOK, body)

	usages, err := kimiProvider{}.FetchUsage(context.Background(), srv.URL, "test-key")
	require.NoError(t, err)
	require.Len(t, usages, 1)
	assert.InDelta(t, 50, usages[0].UsedPercent, 0.001)
}

// 带纳秒的 ISO resetTime 能解析(标准 RFC3339 会拒绝过多纳秒位)。
func TestKimiFetchUsage_NanoResetTime(t *testing.T) {
	body := `{
		"limits": [
			{"window": {"duration": 300, "timeUnit": "TIME_UNIT_MINUTE"},
			 "detail": {"limit": "1000", "used": "100", "resetTime": "2026-08-18T01:00:00.716839300Z"}}
		]
	}`
	srv := newKimiServer(t, http.StatusOK, body)

	usages, err := kimiProvider{}.FetchUsage(context.Background(), srv.URL, "test-key")
	require.NoError(t, err)
	require.Len(t, usages, 1)
	want, _ := time.Parse(time.RFC3339Nano, "2026-08-18T01:00:00.716839300Z")
	assert.Equal(t, want.Unix(), usages[0].PeriodEndTime, "纳秒 ISO 时间应解析为秒时间戳")
}

// 周窗口识别:timeUnit 含 WEEK 或 DAY duration=7。
func TestKimiFetchUsage_WeeklyWindowInLimits(t *testing.T) {
	body := `{
		"limits": [
			{"window": {"duration": 300, "timeUnit": "TIME_UNIT_MINUTE"},
			 "detail": {"limit": "1000", "used": "100", "resetTime": "2026-08-18T01:00:00Z"}},
			{"window": {"duration": 7, "timeUnit": "TIME_UNIT_DAY"},
			 "detail": {"limit": "2000", "used": "600", "resetTime": "2026-08-25T00:00:00Z"}}
		]
	}`
	srv := newKimiServer(t, http.StatusOK, body)

	usages, err := kimiProvider{}.FetchUsage(context.Background(), srv.URL, "test-key")
	require.NoError(t, err)
	byPeriod := map[string]PeriodUsage{}
	for _, u := range usages {
		byPeriod[u.Period] = u
	}
	assert.Contains(t, byPeriod, model.PlanPeriod5Hour)
	assert.Contains(t, byPeriod, model.PlanPeriodWeekly)
	assert.InDelta(t, 30, byPeriod[model.PlanPeriodWeekly].UsedPercent, 0.001)
}

// /usages 404 时回退 /usage 并成功解析。
func TestKimiFetchUsage_FallsBackToUsageOn404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/usages" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("404 page not found"))
			return
		}
		assert.Equal(t, "/usage", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"usage": {"limit": "1000", "used": "100", "resetTime": "2026-08-25T00:00:00Z"}}`))
	}))
	t.Cleanup(srv.Close)

	usages, err := kimiProvider{}.FetchUsage(context.Background(), srv.URL, "test-key")
	require.NoError(t, err)
	require.Len(t, usages, 1)
	assert.Equal(t, model.PlanPeriodWeekly, usages[0].Period)
	assert.InDelta(t, 10, usages[0].UsedPercent, 0.001)
}

func TestKimiFetchUsage_Errors(t *testing.T) {
	t.Run("non-200 status", func(t *testing.T) {
		srv := newKimiServer(t, http.StatusUnauthorized, `{"error":"bad key"}`)
		_, err := kimiProvider{}.FetchUsage(context.Background(), srv.URL, "test-key")
		require.Error(t, err)
	})

	t.Run("invalid json", func(t *testing.T) {
		srv := newKimiServer(t, http.StatusOK, `{not-json`)
		_, err := kimiProvider{}.FetchUsage(context.Background(), srv.URL, "test-key")
		require.Error(t, err)
	})

	t.Run("no usable data", func(t *testing.T) {
		srv := newKimiServer(t, http.StatusOK, `{"usage": null, "limits": []}`)
		_, err := kimiProvider{}.FetchUsage(context.Background(), srv.URL, "test-key")
		require.Error(t, err)
	})

	t.Run("zero limit skipped", func(t *testing.T) {
		srv := newKimiServer(t, http.StatusOK, `{"limits":[{"window":{"duration":300,"timeUnit":"TIME_UNIT_MINUTE"},"detail":{"used":"10","limit":"0"}}]}`)
		_, err := kimiProvider{}.FetchUsage(context.Background(), srv.URL, "test-key")
		require.Error(t, err, "limit=0 无法换算百分比,应无可用数据")
	})

	t.Run("empty api url", func(t *testing.T) {
		_, err := kimiProvider{}.FetchUsage(context.Background(), "", "test-key")
		require.Error(t, err)
	})

	t.Run("empty api key", func(t *testing.T) {
		srv := newKimiServer(t, http.StatusOK, `{}`)
		_, err := kimiProvider{}.FetchUsage(context.Background(), srv.URL, "  ")
		require.Error(t, err)
	})
}
