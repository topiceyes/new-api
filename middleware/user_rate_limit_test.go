package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupUserRateLimitRouter(t *testing.T, setting *dto.UserSetting, userID int) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	// 超限文案走 i18n.T,未初始化时 bundle 为 nil 会 panic
	require.NoError(t, i18n.Init())
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if userID != 0 {
			c.Set("id", userID)
		}
		if setting != nil {
			common.SetContextKey(c, constant.ContextKeyUserSetting, *setting)
		}
		c.Next()
	})
	router.Use(UserRequestRateLimit())
	router.GET("/v1/models", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	return router
}

func hitUserRateLimit(router http.Handler) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestUserRequestRateLimitDisabledWithoutSetting(t *testing.T) {
	useRateLimitMiniRedis(t)

	// 无 setting 上下文:直接放行
	router := setupUserRateLimitRouter(t, nil, 42)
	for range 3 {
		assert.Equal(t, http.StatusNoContent, hitUserRateLimit(router).Code)
	}

	// RPM=0:不限流
	zero := dto.UserSetting{RateLimitRPM: 0}
	router = setupUserRateLimitRouter(t, &zero, 42)
	for range 3 {
		assert.Equal(t, http.StatusNoContent, hitUserRateLimit(router).Code)
	}
}

func TestUserRequestRateLimitRejectsOverRPM(t *testing.T) {
	useRateLimitMiniRedis(t)

	setting := dto.UserSetting{RateLimitRPM: 2}
	router := setupUserRateLimitRouter(t, &setting, 42)

	assert.Equal(t, http.StatusNoContent, hitUserRateLimit(router).Code)
	assert.Equal(t, http.StatusNoContent, hitUserRateLimit(router).Code)

	limited := hitUserRateLimit(router)
	require.Equal(t, http.StatusTooManyRequests, limited.Code)
	assert.NotEmpty(t, limited.Header().Get("Retry-After"))
	assert.Contains(t, limited.Body.String(), "2 req/min")

	// 同用户的另一个"令牌"(同一 user id)共享额度,仍被拒
	assert.Equal(t, http.StatusTooManyRequests, hitUserRateLimit(router).Code)
}

func TestUserRequestRateLimitIsolatedPerUser(t *testing.T) {
	useRateLimitMiniRedis(t)

	setting := dto.UserSetting{RateLimitRPM: 1}
	routerA := setupUserRateLimitRouter(t, &setting, 1001)
	routerB := setupUserRateLimitRouter(t, &setting, 1002)

	assert.Equal(t, http.StatusNoContent, hitUserRateLimit(routerA).Code)
	require.Equal(t, http.StatusTooManyRequests, hitUserRateLimit(routerA).Code)
	// 用户 B 的额度不受用户 A 影响
	assert.Equal(t, http.StatusNoContent, hitUserRateLimit(routerB).Code)
}

func TestUserRequestRateLimitInMemoryFallback(t *testing.T) {
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedisEnabled })

	setting := dto.UserSetting{RateLimitRPM: 1}
	router := setupUserRateLimitRouter(t, &setting, 77)

	assert.Equal(t, http.StatusNoContent, hitUserRateLimit(router).Code)
	limited := hitUserRateLimit(router)
	require.Equal(t, http.StatusTooManyRequests, limited.Code)
	assert.Equal(t, "60", limited.Header().Get("Retry-After"))
}
