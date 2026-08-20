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

// newOpencodeServer 起 mock,断言 Bearer 认证与 /zen/go/v1/usage 路径,按给定响应应答。
func newOpencodeServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/zen/go/v1/usage", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// 标准:usage 包裹 rolling/weekly/monthly,percent=已用,resetsAt=ISO8601。
// 响应体取自 CodexBar 测试 fixture 的真实形状。
func TestOpencodeFetchUsage_ParsesAllPeriods(t *testing.T) {
	body := `{"usage":{
		"rolling": {"percent": 12, "resetsAt": "2026-08-12T02:00:00.000Z"},
		"weekly":  {"percent": 8,  "resetsAt": "2026-08-18T00:00:00.000Z"},
		"monthly": {"percent": 35, "resetsAt": "2026-09-01T00:00:00.000Z"}
	}}`
	srv := newOpencodeServer(t, http.StatusOK, body)

	usages, err := opencodeProvider{}.FetchUsage(context.Background(), srv.URL, "test-key")
	require.NoError(t, err)
	require.Len(t, usages, 3)

	byPeriod := map[string]PeriodUsage{}
	for _, u := range usages {
		byPeriod[u.Period] = u
	}

	fiveH := byPeriod[model.PlanPeriod5Hour]
	assert.InDelta(t, 12, fiveH.UsedPercent, 0.001, "rolling.percent 即已用")
	assert.InDelta(t, 88, fiveH.RemainingPercent, 0.001)
	want5h, _ := time.Parse(time.RFC3339, "2026-08-12T02:00:00Z")
	assert.Equal(t, want5h.Unix(), fiveH.PeriodEndTime)

	weekly := byPeriod[model.PlanPeriodWeekly]
	assert.InDelta(t, 8, weekly.UsedPercent, 0.001)
	wantW, _ := time.Parse(time.RFC3339, "2026-08-18T00:00:00Z")
	assert.Equal(t, wantW.Unix(), weekly.PeriodEndTime)

	monthly := byPeriod[model.PlanPeriodMonthly]
	assert.InDelta(t, 35, monthly.UsedPercent, 0.001)
}

// data 包裹层兼容。
func TestOpencodeFetchUsage_DataWrapper(t *testing.T) {
	body := `{"data":{"rolling":{"percent":42,"resetsAt":"2026-08-12T02:00:00.000Z"}}}`
	srv := newOpencodeServer(t, http.StatusOK, body)

	usages, err := opencodeProvider{}.FetchUsage(context.Background(), srv.URL, "test-key")
	require.NoError(t, err)
	require.Len(t, usages, 1)
	assert.InDelta(t, 42, usages[0].UsedPercent, 0.001)
}

// seroval 风格键名 rollingUsage + usagePercent + resetInSec 兼容。
func TestOpencodeFetchUsage_SerovalKeys(t *testing.T) {
	body := `{"usage":{"rollingUsage":{"usagePercent":17,"resetInSec":5944}}}`
	srv := newOpencodeServer(t, http.StatusOK, body)

	before := time.Now().Unix()
	usages, err := opencodeProvider{}.FetchUsage(context.Background(), srv.URL, "test-key")
	require.NoError(t, err)
	require.Len(t, usages, 1)
	assert.InDelta(t, 17, usages[0].UsedPercent, 0.001)
	// resetInSec 是相对秒:截止时间 ≈ now + 5944。
	assert.InDelta(t, before+5944, usages[0].PeriodEndTime, 10)
}

// 平铺兜底(无包裹层)。
func TestOpencodeFetchUsage_Flattened(t *testing.T) {
	body := `{"rolling":{"percent":5,"resetsAt":"2026-08-12T02:00:00.000Z"}}`
	srv := newOpencodeServer(t, http.StatusOK, body)

	usages, err := opencodeProvider{}.FetchUsage(context.Background(), srv.URL, "test-key")
	require.NoError(t, err)
	require.Len(t, usages, 1)
	assert.InDelta(t, 5, usages[0].UsedPercent, 0.001)
}

// 只有 rolling 一个窗口也合法(weekly/monthly 可缺)。
func TestOpencodeFetchUsage_RollingOnly(t *testing.T) {
	body := `{"usage":{"rolling":{"percent":60,"resetsAt":"2026-08-12T02:00:00.000Z"}}}`
	srv := newOpencodeServer(t, http.StatusOK, body)

	usages, err := opencodeProvider{}.FetchUsage(context.Background(), srv.URL, "test-key")
	require.NoError(t, err)
	require.Len(t, usages, 1)
	assert.Equal(t, model.PlanPeriod5Hour, usages[0].Period)
}

func TestOpencodeFetchUsage_Errors(t *testing.T) {
	t.Run("401 invalid credentials", func(t *testing.T) {
		// 用独立 server,避免与成功用例共享 Authorization 断言。
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "Bearer bad-key", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		}))
		t.Cleanup(srv.Close)
		_, err := opencodeProvider{}.FetchUsage(context.Background(), srv.URL, "bad-key")
		require.Error(t, err)
	})

	t.Run("invalid json", func(t *testing.T) {
		srv := newOpencodeServer(t, http.StatusOK, `{not-json`)
		_, err := opencodeProvider{}.FetchUsage(context.Background(), srv.URL, "test-key")
		require.Error(t, err)
	})

	t.Run("no usable windows", func(t *testing.T) {
		srv := newOpencodeServer(t, http.StatusOK, `{"usage":{}}`)
		_, err := opencodeProvider{}.FetchUsage(context.Background(), srv.URL, "test-key")
		require.Error(t, err)
	})

	t.Run("window without percent", func(t *testing.T) {
		srv := newOpencodeServer(t, http.StatusOK, `{"usage":{"rolling":{"resetsAt":"2026-08-12T02:00:00.000Z"}}}`)
		_, err := opencodeProvider{}.FetchUsage(context.Background(), srv.URL, "test-key")
		require.Error(t, err, "窗口缺 percent/usagePercent 应无可用数据")
	})

	t.Run("empty api url", func(t *testing.T) {
		_, err := opencodeProvider{}.FetchUsage(context.Background(), "", "test-key")
		require.Error(t, err)
	})

	t.Run("empty api key", func(t *testing.T) {
		srv := newOpencodeServer(t, http.StatusOK, `{}`)
		_, err := opencodeProvider{}.FetchUsage(context.Background(), srv.URL, "  ")
		require.Error(t, err)
	})
}
