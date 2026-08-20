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

// newMiniMaxServer 起一个 mock,token_plan 路径直接应答,旧 coding_plan 路径返回 404
// (验证优先新接口、404 回退旧接口的行为由专门用例覆盖)。
func newMiniMaxServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		if r.URL.Path == "/v1/api/openplatform/coding_plan/remains" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("404 page not found"))
			return
		}
		assert.Equal(t, "/v1/token_plan/remains", r.URL.Path)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestMiniMaxFetchUsage_ParsesRemainingIntoUsed(t *testing.T) {
	// 关键:接口返回「剩余」百分比,展示用量 = 100 - remaining。
	body := `{"model_remains":[{
		"current_interval_remaining_percent": 40,
		"current_interval_usage_count": 1200,
		"current_weekly_remaining_percent": 15,
		"current_weekly_status": 1,
		"end_time": 1760000000000,
		"weekly_end_time": 1760600000000
	}]}`
	srv := newMiniMaxServer(t, http.StatusOK, body)

	usages, err := miniMaxProvider{}.FetchUsage(context.Background(), srv.URL, "test-key")
	require.NoError(t, err)
	require.Len(t, usages, 2)

	byPeriod := map[string]PeriodUsage{}
	for _, u := range usages {
		byPeriod[u.Period] = u
	}

	fiveH, ok := byPeriod[model.PlanPeriod5Hour]
	require.True(t, ok, "should include 5h period")
	assert.InDelta(t, 60, fiveH.UsedPercent, 0.001, "5h used = 100 - remaining(40)")
	assert.InDelta(t, 40, fiveH.RemainingPercent, 0.001)
	assert.Equal(t, int64(1760000000000)/1000, fiveH.PeriodEndTime, "ms timestamp converted to sec")

	weekly, ok := byPeriod[model.PlanPeriodWeekly]
	require.True(t, ok, "should include weekly period")
	assert.InDelta(t, 85, weekly.UsedPercent, 0.001, "weekly used = 100 - remaining(15)")
	assert.Equal(t, int64(1760600000000)/1000, weekly.PeriodEndTime)
}

func TestMiniMaxFetchUsage_WeeklyOmittedWhenStatusNotOne(t *testing.T) {
	body := `{"model_remains":[{
		"current_interval_remaining_percent": 80,
		"current_weekly_remaining_percent": 50,
		"current_weekly_status": 0,
		"end_time": 1760000000000,
		"weekly_end_time": 0
	}]}`
	srv := newMiniMaxServer(t, http.StatusOK, body)

	usages, err := miniMaxProvider{}.FetchUsage(context.Background(), srv.URL, "test-key")
	require.NoError(t, err)
	require.Len(t, usages, 1, "weekly should be omitted when current_weekly_status != 1")
	assert.Equal(t, model.PlanPeriod5Hour, usages[0].Period)
}

func TestMiniMaxFetchUsage_Errors(t *testing.T) {
	t.Run("non-200 status", func(t *testing.T) {
		srv := newMiniMaxServer(t, http.StatusUnauthorized, `{"error":"bad key"}`)
		_, err := miniMaxProvider{}.FetchUsage(context.Background(), srv.URL, "test-key")
		require.Error(t, err)
	})

	t.Run("invalid json", func(t *testing.T) {
		srv := newMiniMaxServer(t, http.StatusOK, `{not-json`)
		_, err := miniMaxProvider{}.FetchUsage(context.Background(), srv.URL, "test-key")
		require.Error(t, err)
	})

	t.Run("empty model_remains", func(t *testing.T) {
		srv := newMiniMaxServer(t, http.StatusOK, `{"model_remains":[]}`)
		_, err := miniMaxProvider{}.FetchUsage(context.Background(), srv.URL, "test-key")
		require.Error(t, err)
	})

	t.Run("empty api url", func(t *testing.T) {
		_, err := miniMaxProvider{}.FetchUsage(context.Background(), "", "test-key")
		require.Error(t, err)
	})

	t.Run("empty api key", func(t *testing.T) {
		srv := newMiniMaxServer(t, http.StatusOK, `{}`)
		_, err := miniMaxProvider{}.FetchUsage(context.Background(), srv.URL, "  ")
		require.Error(t, err)
	})
}

func TestClampPercent(t *testing.T) {
	assert.Equal(t, 0.0, clampPercent(-5))
	assert.Equal(t, 100.0, clampPercent(150))
	assert.Equal(t, 42.5, clampPercent(42.5))
}

func TestMsToSec(t *testing.T) {
	assert.Equal(t, int64(1760000000), msToSec(1760000000000)) // ms → sec
	assert.Equal(t, int64(1760000000), msToSec(1760000000))    // already sec
}

// 新接口 token_plan 404 时,应回退到旧接口 coding_plan 并成功解析。
func TestMiniMaxFetchUsage_FallsBackToLegacyOn404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/token_plan/remains" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("404 page not found"))
			return
		}
		assert.Equal(t, "/v1/api/openplatform/coding_plan/remains", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"model_remains":[{
			"current_interval_remaining_percent": 40,
			"current_weekly_status": 0,
			"end_time": 1760000000000
		}]}`))
	}))
	t.Cleanup(srv.Close)

	usages, err := miniMaxProvider{}.FetchUsage(context.Background(), srv.URL, "test-key")
	require.NoError(t, err)
	require.Len(t, usages, 1)
	assert.InDelta(t, 60, usages[0].UsedPercent, 0.001, "used = 100 - remaining(40)")
}

// 两个接口都 404 时应报错(不静默吞掉)。
func TestMiniMaxFetchUsage_Both404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("404 page not found"))
	}))
	t.Cleanup(srv.Close)

	_, err := miniMaxProvider{}.FetchUsage(context.Background(), srv.URL, "test-key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}
